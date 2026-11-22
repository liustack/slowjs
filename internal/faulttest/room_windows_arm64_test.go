// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

package faulttest

import (
	"os"
	"os/exec"
	"runtime/debug"
	"strings"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/liustack/slowjs/internal/faultstate"
)

const (
	// span is how far below the stack pointer the fill reaches.
	span = 8 << 10
	// room is the stack the probe makes the goroutine hold below its frame
	// before each fault, well past span and the frames of the panic.
	room = 64 << 10
	// probeEnv marks the process that runs the probe.
	probeEnv = "SLOWJS_FAULTTEST_PROBE"
)

var sink byte

// grow makes the stack of the goroutine hold room bytes below the frame of
// its caller.
//
//go:noinline
func grow(n int) byte {
	var buf [room]byte
	buf[n%len(buf)] = byte(n)
	return buf[(n*7)%len(buf)]
}

// faultBelow fills below its stack pointer, which it writes to *top, faults
// on a load from wild and recovers, and returns the address of the fault,
// or what else the panic was.
//
//go:noinline
func faultBelow(wild uintptr, top *uintptr) (at uintptr, other any) {
	defer func() {
		r := recover()
		if e, ok := r.(interface{ Addr() uintptr }); ok {
			at = e.Addr()
		} else {
			other = r
		}
	}()
	fillAndFault(wild, top, span)
	return 0, nil
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
//
// The probe writes and reads below the stack pointer, so it runs in a
// process of its own in which the stack of a goroutine never shrinks
// (GODEBUG=gcshrinkstackoff=1) and no collection runs. Before each fault it
// grows the stack to room bytes below its frame, and it fails, reading
// nothing, when the stack moved between the fill and the read.
func TestFaultStateFits(t *testing.T) {
	if os.Getenv(probeEnv) != "" {
		probe(t)
		return
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe, "-test.run=^TestFaultStateFits$", "-test.count=1", "-test.v")
	cmd.Env = append(os.Environ(), probeEnv+"=1", "GODEBUG=gcshrinkstackoff=1,asyncpreemptoff=1", "GOGC=off", "GOMEMLIMIT=off")
	out, err := cmd.CombinedOutput()
	t.Logf("the probe said\n%s", out)
	if err != nil {
		t.Fatalf("the probe: %v", err)
	}
	if !strings.Contains(string(out), "--- PASS: TestFaultStateFits") {
		t.Fatal("the probe did not pass")
	}
}

func probe(t *testing.T) {
	mask, size, err := faultstate.Query()
	if err != nil {
		t.Fatal(err)
	}
	if err := faultstate.Decide(mask, size, nil); err != nil {
		t.Fatal(err)
	}
	// The fault is on a page the probe reserves with no access, so that it is
	// the load of fillAndFault that faults, at that address.
	page, err := windows.VirtualAlloc(0, 1<<16, windows.MEM_RESERVE, windows.PAGE_NOACCESS)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.VirtualFree(page, 0, windows.MEM_RELEASE)
	defer debug.SetGCPercent(debug.SetGCPercent(-1))
	defer debug.SetPanicOnFault(debug.SetPanicOnFault(true))
	deepest := uintptr(0)
	for range 4 {
		sink = grow(1)
		var here byte
		before := uintptr(unsafe.Pointer(&here))
		var top uintptr
		at, other := faultBelow(page, &top)
		if uintptr(unsafe.Pointer(&here)) != before {
			t.Fatal("the stack moved between the fill and the read")
		}
		w := written(top, span)
		if other != nil || at != page {
			t.Fatalf("the fault was at %#x (%v), want the load from %#x", at, other, page)
		}
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
