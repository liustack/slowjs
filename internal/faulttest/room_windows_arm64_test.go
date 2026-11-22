// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

package faulttest

import (
	"runtime/debug"
	"testing"
	"unsafe"

	"github.com/liustack/slowjs/internal/faultstate"
)

const (
	// wild is an address mapped to nothing.
	wild = 0xdead0000
	// span is how far below the stack pointer the fill reaches.
	span = 8 << 10
)

var sink byte

// grow makes the stack of the goroutine hold its frame, larger than span,
// below the frame of its caller. With the collector off the stack does not
// shrink again.
//
//go:noinline
func grow(n int) byte {
	var buf [2 * span]byte
	buf[n%len(buf)] = byte(n)
	return buf[(n*7)%len(buf)]
}

// faultBelow fills below its stack pointer, which it writes to *top, faults
// and recovers.
//
//go:noinline
func faultBelow(top *uintptr) (faulted bool) {
	defer func() { _, faulted = recover().(interface{ Addr() uintptr }) }()
	fillAndFault(wild, top, span)
	return false
}

// written returns how far below top the lowest word of the fill that changed
// is, or 0 when none did.
//
//go:noinline
func written(top uintptr) uintptr {
	for a := top - span; a < top-16; a += 8 {
		if *(*uint64)(*(*unsafe.Pointer)(unsafe.Pointer(&a))) != pattern^uint64(a) {
			return top - a
		}
	}
	return 0
}

// TestFaultStateFits: on windows/arm64 the state of the CPU that the system
// writes below the stack pointer at a fault, with the rest of the dispatch,
// fits in the 4 KiB Go keeps below each stack, which is why the generated
// engine has no room check there (docs/changes/c-engine.md, Room for a fault
// on windows). internal/faultstate keeps the engine from running where the
// CONTEXT with the extended state the system turns on, with
// faultstate.Dispatch bytes for the rest, does not fit. This measures the
// rest: it fills the stack below the stack pointer, faults, and finds how far
// down the fill changed, which takes in the CONTEXT, the exception record,
// the frames of KiUserExceptionDispatcher, those of Go's handler before it
// moves to g0, and the panic of the fault. It fails when what is beside the
// CONTEXT takes more than faultstate.Dispatch, as a deeper dispatch of
// another windows or Go might. Measured on the windows-11-arm runner of
// GitHub (Cobalt 100, build 26200): 2,488 bytes, with a CONTEXT of 1,137.
func TestFaultStateFits(t *testing.T) {
	mask, size, err := faultstate.Query()
	if err != nil {
		t.Fatal(err)
	}
	if err := faultstate.Decide(mask, size, nil); err != nil {
		t.Fatal(err)
	}
	defer debug.SetGCPercent(debug.SetGCPercent(-1))
	defer debug.SetPanicOnFault(debug.SetPanicOnFault(true))
	sink = grow(1)
	deepest := uintptr(0)
	for range 4 {
		var top uintptr
		if !faultBelow(&top) {
			t.Fatalf("the load from %#x did not fault", wild)
		}
		w := written(top)
		if w == 0 {
			t.Fatal("the fault wrote nothing below the stack pointer")
		}
		deepest = max(deepest, w)
	}
	rest := int(deepest) - int(size)
	t.Logf("extended state %#x, CONTEXT with it %d bytes, the write %d bytes below the stack pointer, %d of them beside the CONTEXT, of %d", mask, size, deepest, rest, faultstate.StackSystem)
	if rest > faultstate.Dispatch {
		t.Fatalf("the dispatch of a fault takes %d bytes beside the CONTEXT, more than the %d internal/faultstate keeps for it", rest, faultstate.Dispatch)
	}
	if deepest > faultstate.StackSystem {
		t.Fatalf("a fault writes %d bytes below the stack pointer, more than the %d Go keeps", deepest, faultstate.StackSystem)
	}
}
