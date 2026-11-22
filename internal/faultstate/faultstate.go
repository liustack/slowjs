// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

// Package faultstate tells whether the state of the CPU that windows/arm64
// writes below the stack pointer at a fault fits in the room Go keeps for it,
// so that a fault in the engine cannot write past a goroutine stack into the
// heap.
//
// At a fault windows writes a CONTEXT with the extended state of the CPU, the
// exception record and the frames of its dispatch below the stack pointer of
// the thread, which in the engine is the goroutine's stack, and Go keeps 4 KiB
// there (golang/go#81238). windows/amd64 makes more room in the generated
// code itself. windows/arm64 runs the arm64 code of linux and darwin as it
// is, which is safe only while the state fits: Check asks windows how large
// the CONTEXT is with the extended state it turns on, and Decide refuses what
// was not shown to fit (docs/changes/c-engine.md, Room for a fault on
// windows).
package faultstate

import "fmt"

const (
	// StackSystem is the room Go keeps below each goroutine stack for the
	// system on windows (runtime/stack.go).
	StackSystem = 4096

	// Dispatch is what the exception record, the frames of
	// KiUserExceptionDispatcher and those of Go's handler may take below the
	// stack pointer besides the CONTEXT: 1,351 bytes were measured on the
	// windows-11-arm runner of GitHub, and half again is kept for a deeper
	// dispatch of another windows or Go. TestFaultStateFits
	// (internal/faulttest) measures it again on windows/arm64.
	Dispatch = 2048

	// SVE is the bit of the extended state of SVE (XSTATE_MASK_ARM64_SVE of
	// winnt.h), the only one the engine runs with. The others windows knows,
	// the ZA matrix, TPIDR2 and ZT of SME, are 4 KiB of ZA alone with vectors
	// of 512 bits.
	SVE = 1 << 2

	// contextSize is the size of the CONTEXT of arm64 without extended state
	// (ARM64_NT_CONTEXT of winnt.h).
	contextSize = 0x390
)

// Decide returns nil when the engine may run on a windows/arm64 machine that
// turns on the extended state mask and whose CONTEXT, with that state, takes
// size bytes, as InitializeContext2 gives it. err is the error of asking
// windows for them. It refuses when windows did not answer, when the mask
// has a bit other than SVE, and when the CONTEXT with Dispatch passes
// StackSystem.
func Decide(mask uint64, size uint32, err error) error {
	const doc = "the engine does not run there (docs/changes/c-engine.md, Room for a fault on windows)"
	if err != nil {
		return fmt.Errorf("slowjs: windows/arm64 did not say how large the state of the CPU at a fault is, and %s: %w", doc, err)
	}
	if other := mask &^ SVE; other != 0 {
		return fmt.Errorf("slowjs: windows/arm64 turns on the extended CPU state %#x, and %#x of it is not SVE (SME, or a state this engine does not know), whose size at a fault was never checked, so %s", mask, other, doc)
	}
	if size < contextSize {
		return fmt.Errorf("slowjs: windows/arm64 gives %d bytes for the CONTEXT with the extended CPU state %#x, less than the %d of a CONTEXT without it, so %s", size, mask, contextSize, doc)
	}
	if int(size)+Dispatch > StackSystem {
		return fmt.Errorf("slowjs: windows/arm64 saves %d bytes of CPU state at a fault (extended state %#x), and with the %d of the rest of the dispatch that is more than the %d bytes Go keeps below a goroutine stack, so %s", size, mask, Dispatch, StackSystem, doc)
	}
	return nil
}
