// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build slowjs_cengine && testhooks

package cengine

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"unsafe"

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
// points at nothing, as a bug in the C would. The low address is past the
// page Go takes for a nil pointer (a fault there names no address) and below
// what any target maps: Go links linux/arm64 text at 0x10000, and Linux keeps
// the first 32 KiB (arm64) or 64 KiB (amd64) unmapped.
func TestFaultWildPointer(t *testing.T) {
	wilds := []uintptr{0xdead0000, 0x4000, 1 << 46}
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

// TestFaultNoSplit: a fault at the bottom of a chain of functions without a
// stack check (the generator's NOSPLIT list, noSplitList) is a panic that
// recovers, wherever the bottom of the goroutine's stack is. The runtime
// pushes a call of sigpanic where the fault is, and sigpanic's stack check
// may call morestack: the generator leaves room for both below every chain.
// js_strdup measures the string with strlen, and both are on the list, so a
// string that points at nothing faults in strlen with js_strdup above it. One call shows the frames through
// runtime.Callers, then fresh goroutines make the same call deeper in their
// stacks, by steps of 8 bytes over 12 KiB, each on a VM of its own.
func TestFaultNoSplit(t *testing.T) {
	const wild = 0xdead0000
	on := map[string]bool{}
	for _, n := range strings.Fields(noSplitList) {
		on[n] = true
	}
	for _, n := range []string{"js_strdup", "strlen"} {
		if !on[n] {
			t.Fatalf("%s is not on the NOSPLIT list: the test needs another chain", n)
		}
	}
	read := func(v *VM) func() { return func() { js_strdup(v.sp, v.ctx, wild) } }

	v := newVM(t, Options{Seed: 1})
	var frames []string
	got := func() (got any) {
		defer func() {
			got = recover()
			pc := make([]uintptr, 64)
			fs := runtime.CallersFrames(pc[:runtime.Callers(1, pc)])
			for more := true; more; {
				var f runtime.Frame
				f, more = fs.Next()
				frames = append(frames, f.Function)
			}
		}()
		v.enter(read(v))
		return nil
	}()
	wantFault(t, got, "a read through the string", wild, wild+1)
	at := slices.Index(frames, "runtime.sigpanic")
	if at < 0 || at+2 >= len(frames) {
		t.Fatalf("no frames of the engine below sigpanic: %v", frames)
	}
	const pkg = "github.com/liustack/slowjs/internal/cengine."
	if f, g := frames[at+1], frames[at+2]; f != pkg+"strlen" || g != pkg+"js_strdup" {
		t.Fatalf("the fault is in %s called from %s, want strlen from js_strdup: %v", f, g, frames)
	}
	brokenAfter(t, v, "a fault in a NOSPLIT chain")

	depths := map[uintptr]bool{}
	for n := 0; n < 192; n++ {
		for k := 0; k < 8; k++ {
			v := newVM(t, Options{Seed: 1})
			type result struct {
				got   any
				depth uintptr
			}
			res := make(chan result, 1)
			go func() {
				var top byte
				var r result
				noSplitDive(n, k, &top, &r.depth, func() { r.got = fails(v, read(v)) })
				res <- r
			}()
			r := <-res
			wantFault(t, r.got, fmt.Sprintf("a read %d bytes down the stack", r.depth), wild, wild+1)
			if _, err := v.Close(); err != nil {
				t.Fatal(err)
			}
			depths[r.depth] = true
		}
	}
	lo := slices.Min(slices.Collect(maps.Keys(depths)))
	hi := lo
	for depths[hi+8] {
		hi += 8
	}
	if hi-lo < 8<<10 {
		t.Fatalf("the calls start at every 8 bytes from %d to %d down the stack only", lo, hi)
	}
	t.Logf("%d faults, from %d to %d bytes down the stack", len(depths), lo, hi)
}

// noSplitDive recurses n times and then calls f through the pad k picks,
// which takes k+1 words, so that f starts deeper in the goroutine's stack by
// steps of 8 bytes. depth is how far below top it starts. top is an argument
// all the way, so that a stack that grows moves it with the frames.
//
//go:noinline
func noSplitDive(n, k int, top *byte, depth *uintptr, f func()) {
	if n > 0 {
		noSplitDive(n-1, k, top, depth, f)
		return
	}
	switch k {
	case 0:
		noSplitPad[[1]uintptr](top, depth, f)
	case 1:
		noSplitPad[[2]uintptr](top, depth, f)
	case 2:
		noSplitPad[[3]uintptr](top, depth, f)
	case 3:
		noSplitPad[[4]uintptr](top, depth, f)
	case 4:
		noSplitPad[[5]uintptr](top, depth, f)
	case 5:
		noSplitPad[[6]uintptr](top, depth, f)
	case 6:
		noSplitPad[[7]uintptr](top, depth, f)
	default:
		noSplitPad[[8]uintptr](top, depth, f)
	}
}

//go:noinline
func noSplitPad[T any](top *byte, depth *uintptr, f func()) {
	var a T
	*depth = uintptr(unsafe.Pointer(top)) - uintptr(unsafe.Pointer(&a))
	f()
}
