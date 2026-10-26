// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build cengine && testhooks

package cengine

import (
	"errors"
	"os"
	"runtime"
	"runtime/debug"
	"sync"
	"testing"

	"github.com/liustack/slowjs/internal/gohost"
)

// These tests make the engine fail on purpose, by calling its entries with
// arguments no caller gives: an address that is mapped to nothing, the end of
// the shadow stack, an abort. Each must come out as a panic that recovers,
// leave the VM broken so that only Close works, and leave the process and
// every other VM as they were. Without debug.SetPanicOnFault (gohost.Enter
// turns it on) the runtime would print "unexpected fault address" for the
// first two and end the process.

// fails calls f as a call into the engine of v, and returns what it panicked
// with, or nil.
func fails(v *VM, f func()) (got any) {
	defer func() { got = recover() }()
	v.enter(f)
	return nil
}

// brokenAfter checks what a VM must be after the engine failed in it: broken,
// refusing every entry with ErrBroken, and closing without a leak of host
// memory. The goroutine must have its setting of SetPanicOnFault back.
func brokenAfter(t *testing.T, v *VM, what string) {
	t.Helper()
	if !v.Host().Broken() {
		t.Errorf("%s: the VM is not broken", what)
	}
	if _, err := v.Eval("1", "after.js"); !errors.Is(err, ErrBroken) {
		t.Errorf("%s: Eval after the failure gave %v, want ErrBroken", what, err)
	}
	if _, err := v.ReleaseMemory(0); !errors.Is(err, ErrBroken) {
		t.Errorf("%s: ReleaseMemory after the failure gave %v, want ErrBroken", what, err)
	}
	if got := catch(v.RunGC); !errors.Is(asError(got), ErrBroken) {
		t.Errorf("%s: RunGC after the failure panicked with %v, want ErrBroken", what, got)
	}
	if _, err := v.Close(); err != nil {
		t.Errorf("%s: Close: %v", what, err)
	}
	if u := v.Host().Memory().Usage(); u.Chunks != 0 || u.Footprint != 0 || u.Stack != 0 {
		t.Errorf("%s: after Close the host holds %+v", what, u)
	}
	if debug.SetPanicOnFault(false) {
		t.Errorf("%s: the goroutine still has SetPanicOnFault on", what)
	}
	// A process that survived must also run a new VM.
	w := newVM(t, Options{Seed: 1})
	if got, err := w.Eval(`[1,2,3].map(x => x * 2).join()`, "next.js"); err != nil || got != "2,4,6" {
		t.Errorf("%s: a new VM gave %q, %v", what, got, err)
	}
	closeVM(t, w)
}

func catch(f func()) (got any) {
	defer func() { got = recover() }()
	f()
	return nil
}

func asError(x any) error {
	err, _ := x.(error)
	return err
}

// wantFault checks that got is the panic of a fault: a runtime.Error that
// names an address, which is in [lo, hi) when hi is not 0.
func wantFault(t *testing.T, got any, what string, lo, hi uintptr) {
	t.Helper()
	if got == nil {
		t.Fatalf("%s: the engine did not fail", what)
	}
	if _, ok := got.(runtime.Error); !ok {
		t.Fatalf("%s: panicked with %T %v, want a runtime.Error", what, got, got)
	}
	a, ok := got.(interface{ Addr() uintptr })
	if !ok {
		t.Fatalf("%s: the panic %v names no address", what, got)
	}
	if hi != 0 && (a.Addr() < lo || a.Addr() >= hi) {
		t.Fatalf("%s: the fault is at %#x, want %#x to %#x", what, a.Addr(), lo, hi)
	}
}

// TestFaultWildPointer: the engine reads and writes through a pointer that
// points at nothing, as a bug in the C would.
func TestFaultWildPointer(t *testing.T) {
	wilds := []uintptr{0xdead0000, 0x10000, 1 << 46}
	if runtime.GOARCH == "amd64" {
		wilds = append(wilds, 0xdeadbeefdeadbeef) // not canonical: a general protection fault, no address
	}
	for _, wild := range wilds {
		reads := func(v *VM) func() {
			return func() { JS_GetPropertyStr(v.sp, wild, 0, 0, v.cstring("x")) }
		}
		writes := func(v *VM) func() {
			// The length of the string of the integer 42 goes to *wild.
			return func() { JS_ToCStringLen2(v.sp, v.ctx, wild, 42, 0, 0) }
		}
		for _, c := range []struct {
			name string
			do   func(v *VM) func()
		}{{"read", reads}, {"write", writes}} {
			t.Run(c.name+"/"+hex(wild), func(t *testing.T) {
				v := newVM(t, Options{Seed: 1})
				got := fails(v, c.do(v))
				lo, hi := wild, wild+1<<20
				if wild == 0xdeadbeefdeadbeef {
					lo, hi = 0, 0
				}
				wantFault(t, got, c.name, lo, hi)
				brokenAfter(t, v, c.name)
			})
		}
	}
}

func hex(x uintptr) string {
	const digits = "0123456789abcdef"
	if x == 0 {
		return "0x0"
	}
	var b []byte
	for ; x > 0; x >>= 4 {
		b = append([]byte{digits[x&15]}, b...)
	}
	return "0x" + string(b)
}

// TestFaultShadowStackEnd: the engine runs out of the shadow stack and writes
// into its guard page. The C checks the room left before it recurses, so this
// takes an entry that is given a stack with nothing left.
func TestFaultShadowStackEnd(t *testing.T) {
	v := newVM(t, Options{Seed: 1})
	base, size := v.Host().Stack()
	end := base + size
	got := fails(v, func() { JS_NewContext(end-8, v.rt) })
	wantFault(t, got, "an entry with no stack left", end, end+uintptr(os.Getpagesize()))
	brokenAfter(t, v, "the end of the shadow stack")
}

// TestFaultAbort: the engine calls abort, as it does where the C cannot go
// on. The host panics with ErrAbort and the VM is broken.
func TestFaultAbort(t *testing.T) {
	v := newVM(t, Options{Seed: 1})
	got := fails(v, func() { abort(v.sp) })
	if !errors.Is(asError(got), gohost.ErrAbort) {
		t.Fatalf("abort panicked with %T %v, want gohost.ErrAbort", got, got)
	}
	brokenAfter(t, v, "abort")
}

// TestFaultTrap: the engine reaches a trap. abort ends in __builtin_trap, for
// a host whose abort returns, and the pass puts a call to host_trap in its
// place. A ud2 or a brk would be SIGILL or SIGTRAP, which the runtime cannot
// recover and ends the process with. The host's own abort never returns, so
// the test answers the host_call of the abort itself.
func TestFaultTrap(t *testing.T) {
	callOverrideHook = func(magic int32) (gohost.Value, bool) {
		return gohost.Undefined, magic == gohost.CallAbort
	}
	defer func() { callOverrideHook = nil }()
	v := newVM(t, Options{Seed: 1})
	got := fails(v, func() { abort(v.sp) })
	if !errors.Is(asError(got), gohost.ErrTrap) {
		t.Fatalf("a trap in the engine panicked with %T %v, want gohost.ErrTrap", got, got)
	}
	callOverrideHook = nil
	brokenAfter(t, v, "a trap")
}

// TestFaultsAmongVMs: a VM that fails does not touch the VMs that run beside
// it on other goroutines, nor the ones made after it.
func TestFaultsAmongVMs(t *testing.T) {
	const workers = 4
	stop := make(chan struct{})
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; ; i++ {
				select {
				case <-stop:
					return
				default:
				}
				v, err := New(Options{Seed: uint64(i)})
				if err != nil {
					errs <- err
					return
				}
				got, err := v.Eval(`let s = 0; for (let i = 0; i < 20000; i++) s += i % 7; JSON.stringify([s, "x".repeat(3)])`, "work.js")
				if err != nil || got != `[59997,"xxx"]` {
					errs <- errors.Join(err, errors.New("worker got "+got))
					return
				}
				if leaked, err := v.Close(); err != nil || leaked != 0 {
					errs <- errors.Join(err, errors.New("worker closed with a leak"))
					return
				}
			}
		}()
	}
	for i := 0; i < 40; i++ {
		v := newVM(t, Options{Seed: 1})
		var got any
		switch i % 3 {
		case 0:
			got = fails(v, func() { JS_GetPropertyStr(v.sp, 0xdead0000, 0, 0, v.cstring("x")) })
			wantFault(t, got, "a read", 0xdead0000, 0xdead0000+1<<20)
		case 1:
			got = fails(v, func() { abort(v.sp) })
			if !errors.Is(asError(got), gohost.ErrAbort) {
				t.Fatalf("abort panicked with %v", got)
			}
		default:
			base, size := v.Host().Stack()
			got = fails(v, func() { JS_NewContext(base+size-8, v.rt) })
			wantFault(t, got, "the end of the stack", base+size, 0)
		}
		if !v.Host().Broken() {
			t.Fatal("the VM is not broken")
		}
		if _, err := v.Close(); err != nil {
			t.Fatal(err)
		}
	}
	close(stop)
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}
