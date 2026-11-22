// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

package faulttest

import (
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

// TestFaultStateFits: on windows/arm64 the state of the CPU that the system
// writes below the stack pointer at a fault, with the rest of the dispatch,
// fits in the 4 KiB Go keeps below each stack, which is why the generated
// engine has no room check there (docs/changes/c-engine.md, Room for a fault
// on windows). Measured on the windows-11-arm runner of GitHub (Cobalt 100,
// build 26200): the CONTEXT with its extended state (SVE, 128-bit vectors)
// takes 1,137 bytes, and the write reaches 2,488 bytes below the stack
// pointer, so the record, the frames of KiUserExceptionDispatcher and those of
// Go's handler before it moves to g0 take 1,351. The test fails on a machine
// whose state is larger by more than the 1,608 bytes left, such as SVE with
// longer vectors or SME, where the room check of windows/amd64 is needed on
// arm64 too.
func TestFaultStateFits(t *testing.T) {
	const (
		stackSystem   = 4096     // runtime/stack.go, on windows
		contextARM64  = 0x400000 // CONTEXT_ARM64
		contextAll    = contextARM64 | 0xf
		contextXState = contextARM64 | 0x20
		rest          = 2488 - 1137
	)
	k32 := windows.NewLazySystemDLL("kernel32.dll")
	features := k32.NewProc("GetEnabledXStateFeatures")
	initialize := k32.NewProc("InitializeContext2")
	if err := features.Find(); err != nil {
		t.Fatal(err)
	}
	if err := initialize.Find(); err != nil {
		t.Fatal(err)
	}
	mask, _, _ := features.Call()
	var ctx uintptr
	var size uint32
	// With no buffer it fails and gives the size it needs.
	initialize.Call(0, contextAll|contextXState, uintptr(unsafe.Pointer(&ctx)), uintptr(unsafe.Pointer(&size)), mask)
	if size < 0x390 {
		t.Fatalf("InitializeContext2 gives %d bytes, less than a CONTEXT", size)
	}
	t.Logf("extended state %#x, CONTEXT with it %d bytes, the write about %d of %d", mask, size, int(size)+rest, stackSystem)
	if int(size)+rest > stackSystem {
		t.Fatalf("the state of the CPU at a fault takes %d bytes: with the rest of the dispatch, %d bytes go below the stack pointer, more than the %d Go keeps", size, int(size)+rest, stackSystem)
	}
}
