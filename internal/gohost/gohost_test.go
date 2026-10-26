// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package gohost

import (
	"errors"
	"sync"
	"testing"
	"time"
	"unsafe"

	"github.com/liustack/slowjs/internal/hostmem"
)

// small is the stack of the VMs of most tests: the code under test does not
// depend on its size, and 300 VMs of 32 MiB would only take address space.
const small = 1 << 20

func newVM(t *testing.T, size uintptr) *VM {
	t.Helper()
	v, err := New(Options{StackSize: size})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if Lookup(v.ID()) != nil {
			if err := v.Close(); err != nil {
				t.Error(err)
			}
		}
	})
	return v
}

func mustPanic(t *testing.T, what string, f func()) any {
	t.Helper()
	var got any
	func() {
		defer func() { got = recover() }()
		f()
	}()
	if got == nil {
		t.Fatalf("%s did not panic", what)
	}
	return got
}

// argv writes the words as the u of JSValues with the tag JS_TAG_INT, as
// HOST_WORD does, into memory outside the Go heap, and returns its address.
func argv(t *testing.T, v *VM, words ...uintptr) uintptr {
	t.Helper()
	p := MemoryGrow(v.ID(), hostmem.Unit)
	if p == 0 {
		t.Fatal("no chunk for the arguments")
	}
	for i, w := range words {
		*(*uintptr)(addr(p + uintptr(16*i))) = w
		*(*uintptr)(addr(p + uintptr(16*i) + 8)) = 0
	}
	return p
}

func TestRegistry(t *testing.T) {
	a := newVM(t, small)
	b := newVM(t, 2*small)
	if a.ID() == 0 || b.ID() == 0 || a.ID() == b.ID() {
		t.Fatalf("ids %d and %d: not 0 and not the same", a.ID(), b.ID())
	}
	if Lookup(a.ID()) != a || Lookup(b.ID()) != b || Lookup(0) != nil {
		t.Fatal("Lookup does not find each VM by its id, or finds one for 0")
	}
	for _, v := range []*VM{a, b} {
		base, size := v.Stack()
		if base == 0 || base%size != 0 {
			t.Fatalf("stack at %#x of %d bytes is not aligned to its size", base, size)
		}
		for _, sp := range []uintptr{base, base + 1, base + size/2, base + size - 1} {
			if got := ByStack(sp); got != v {
				t.Fatalf("ByStack(%#x) = %v, want the VM %d", sp, got, v.ID())
			}
		}
	}
	base, size := a.Stack()
	mustPanic(t, "ByStack outside every stack", func() { ByStack(base + size) })
	mustPanic(t, "ByStack(0)", func() { ByStack(0) })

	id := a.ID()
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	if Lookup(id) != nil {
		t.Fatal("a closed VM is still found by its id")
	}
	mustPanic(t, "ByStack of a closed VM", func() { ByStack(base) })
	if u := a.Memory().Usage(); u.Chunks != 0 || u.Footprint != 0 || u.Stack != 0 {
		t.Fatalf("after Close the VM holds %+v", u)
	}
}

func TestDefaultStack(t *testing.T) {
	v := newVM(t, 0)
	if _, size := v.Stack(); size != hostmem.DefaultStackSize {
		t.Fatalf("stack of %d bytes, want %d", size, hostmem.DefaultStackSize)
	}
}

func TestMemoryGrow(t *testing.T) {
	v := newVM(t, small)
	p := MemoryGrow(v.ID(), 4*hostmem.Unit)
	if p == 0 {
		t.Fatal(v.LastError())
	}
	b := unsafe.Slice((*byte)(addr(p)), 4*hostmem.Unit)
	b[0], b[len(b)-1] = 1, 2
	u := v.Memory().Usage()
	if u.Chunks != 1 || u.Footprint != 4*hostmem.Unit {
		t.Fatalf("usage %+v after one chunk of 256 KiB", u)
	}
	if c := v.Counts(); c.Grows != 1 || c.GrowBytes != 4*hostmem.Unit {
		t.Fatalf("counts %+v", c)
	}
	// The OS refuses: 0, the error kept, nothing counted.
	if p := MemoryGrow(v.ID(), 1<<50); p != 0 {
		t.Fatalf("a chunk of 1 PiB at %#x", p)
	}
	if v.LastError() == nil {
		t.Fatal("no error kept for a refused chunk")
	}
	if c := v.Counts(); c.Grows != 1 || c.GrowFailures != 1 {
		t.Fatalf("counts %+v after a refused chunk", c)
	}
	mustPanic(t, "a chunk for no VM", func() { MemoryGrow(0, hostmem.Unit) })
	mustPanic(t, "a chunk not a multiple of 64 KiB", func() { MemoryGrow(v.ID(), 4096) })
}

func TestCallRelease(t *testing.T) {
	v := newVM(t, small)
	sp, _ := v.Stack()
	p := MemoryGrow(v.ID(), 8*hostmem.Unit)
	args := argv(t, v, p, 8*hostmem.Unit)
	before := v.Memory().Usage()
	r := Call(sp, v.ID(), Undefined, 2, args, CallMemoryRelease)
	if r != Undefined {
		t.Fatalf("release returned %+v", r)
	}
	u := v.Memory().Usage()
	if u.Chunks != before.Chunks-1 || u.Footprint != before.Footprint-8*hostmem.Unit {
		t.Fatalf("usage %+v after releasing a chunk of %+v", u, before)
	}
	if c := v.Counts(); c.Frees != 1 || c.FreeBytes != 8*hostmem.Unit {
		t.Fatalf("counts %+v", c)
	}
	// A chunk the VM does not hold, or not whole, is a bug of the engine.
	args = argv(t, v, p, 8*hostmem.Unit)
	mustPanic(t, "releasing a chunk released before", func() { Call(sp, v.ID(), Undefined, 2, args, CallMemoryRelease) })
	q := MemoryGrow(v.ID(), 2*hostmem.Unit)
	args = argv(t, v, q, hostmem.Unit)
	mustPanic(t, "releasing half a chunk", func() { Call(sp, v.ID(), Undefined, 2, args, CallMemoryRelease) })
	mustPanic(t, "release with one argument", func() { Call(sp, v.ID(), Undefined, 1, args, CallMemoryRelease) })
}

func TestCallRemap(t *testing.T) {
	v := newVM(t, small)
	sp, _ := v.Stack()
	p := MemoryGrow(v.ID(), hostmem.Unit)
	b := unsafe.Slice((*byte)(addr(p)), hostmem.Unit)
	for i := range b {
		b[i] = byte(i * 7)
	}
	args := argv(t, v, p, hostmem.Unit, 16*hostmem.Unit)
	if !hostmem.CanRemap {
		if v.AllowRemap() {
			t.Fatal("AllowRemap says yes where the OS cannot remap")
		}
		mustPanic(t, "a remap the engine was never told it can ask for", func() {
			Call(sp, v.ID(), Undefined, 3, args, CallMemoryRemap)
		})
		return
	}
	mustPanic(t, "a remap before AllowRemap", func() { Call(sp, v.ID(), Undefined, 3, args, CallMemoryRemap) })
	if !v.AllowRemap() {
		t.Fatal("AllowRemap says no on linux")
	}
	r := Call(sp, v.ID(), Undefined, 3, args, CallMemoryRemap)
	if r.Tag != 0 || r.U == 0 {
		t.Fatalf("remap returned %+v", r)
	}
	nb := unsafe.Slice((*byte)(addr(r.U)), 16*hostmem.Unit)
	for i := range hostmem.Unit {
		if nb[i] != byte(i*7) {
			t.Fatalf("byte %d of the remapped chunk changed", i)
		}
	}
	nb[len(nb)-1] = 1
	if c := v.Counts(); c.Remaps != 1 || c.RemapBytes != 15*hostmem.Unit {
		t.Fatalf("counts %+v", c)
	}
	// The OS refuses: the word 0, and the chunk as it was.
	args = argv(t, v, r.U, 16*hostmem.Unit, 1<<50)
	if r2 := Call(sp, v.ID(), Undefined, 3, args, CallMemoryRemap); r2 != Word(0) {
		t.Fatalf("a remap to 1 PiB returned %+v", r2)
	}
	if c := v.Counts(); c.RemapFailures != 1 || v.LastError() == nil {
		t.Fatalf("counts %+v, error %v after a refused remap", c, v.LastError())
	}
	if nb[0] != 0 || nb[len(nb)-1] != 1 {
		t.Fatal("the chunk changed when the remap was refused")
	}
}

func TestCallAbort(t *testing.T) {
	v := newVM(t, small)
	base, _ := v.Stack()
	// abort does not know its VM and passes 0: the host finds it by the
	// shadow pointer.
	got := mustPanic(t, "abort", func() { Call(base+4096, 0, Undefined, 0, 0, CallAbort) })
	if err, ok := got.(error); !ok || !errors.Is(err, ErrAbort) {
		t.Fatalf("abort panicked with %v, want ErrAbort", got)
	}
	if !v.Broken() {
		t.Fatal("the VM is not broken after abort")
	}
}

func TestCallUnknown(t *testing.T) {
	v := newVM(t, small)
	sp, _ := v.Stack()
	mustPanic(t, "a registered function", func() { Call(sp, v.ID(), Undefined, 0, 0, 0) })
	mustPanic(t, "an unknown request", func() { Call(sp, v.ID(), Undefined, 0, 0, -99) })
	mustPanic(t, "a call for no VM", func() { Call(sp, 0, Undefined, 0, 0, CallMemoryRelease) })
}

func TestNow(t *testing.T) {
	v := newVM(t, small)
	// By default the wall clock is the real one, moving.
	got := time.Unix(0, Now(v.ID(), NowWall))
	if d := time.Since(got); d < -time.Second || d > time.Second {
		t.Fatalf("the default wall clock reads %v", got)
	}
	fixed := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	v.SetWallClock(fixed, false)
	a := Now(v.ID(), NowWall)
	time.Sleep(2 * time.Millisecond)
	if b := Now(v.ID(), NowWall); a != fixed.UnixNano() || b != a {
		t.Fatalf("a fixed clock read %d then %d, want %d", a, b, fixed.UnixNano())
	}
	v.SetWallClock(fixed, true)
	a = Now(v.ID(), NowWall)
	time.Sleep(2 * time.Millisecond)
	b := Now(v.ID(), NowWall)
	if a < fixed.UnixNano() || b-a < int64(time.Millisecond) || b-fixed.UnixNano() > int64(time.Minute) {
		t.Fatalf("a moving clock from %d read %d then %d", fixed.UnixNano(), a, b)
	}
	// Each VM has its own clock.
	w := newVM(t, small)
	w.SetWallClock(fixed.Add(time.Hour), false)
	if Now(w.ID(), NowWall) != fixed.Add(time.Hour).UnixNano() || Now(v.ID(), NowWall) < fixed.UnixNano() {
		t.Fatal("the clocks of two VMs are not their own")
	}
	// The monotonic clock needs no VM, never goes back, and moves on with
	// time, in nanoseconds.
	m := Now(0, NowMonotonic)
	for range 1000 {
		n := Now(v.ID(), NowMonotonic)
		if n < m {
			t.Fatalf("the monotonic clock went back from %d to %d", m, n)
		}
		m = n
	}
	time.Sleep(5 * time.Millisecond)
	if n := Now(v.ID(), NowMonotonic); n-m < int64(5*time.Millisecond) || n-m > int64(time.Minute) {
		t.Fatalf("the monotonic clock read %d then %d after a sleep of 5 ms", m, n)
	}
	mustPanic(t, "the wall clock of no VM", func() { Now(0, NowWall) })
	mustPanic(t, "an unknown kind", func() { Now(v.ID(), 2) })
}

func TestPoll(t *testing.T) {
	v := newVM(t, small)
	base, _ := v.Stack()
	v.Enter()
	if r := Poll(base + 100); r != 0 {
		t.Fatalf("poll returned %d without an interrupt", r)
	}
	done := make(chan bool)
	go func() { v.Interrupt(); close(done) }()
	<-done
	if r := Poll(base + 200); r != 1 {
		t.Fatalf("poll returned %d after Interrupt", r)
	}
	// The request holds for inner entries, and only the outermost entry
	// clears it.
	v.Enter()
	if Poll(base+300) != 1 {
		t.Fatal("an inner entry cleared the interrupt")
	}
	v.Leave()
	v.Leave()
	if v.Counts().Polls != 3 {
		t.Fatalf("%d polls counted, want 3", v.Counts().Polls)
	}
	v.Enter()
	if Poll(base) != 0 {
		t.Fatal("the outermost entry kept the interrupt")
	}
	v.Leave()
	mustPanic(t, "Leave without Enter", v.Leave)
}

func TestRelease(t *testing.T) {
	v := newVM(t, small)
	type item struct{ n int }
	a, b := v.Keep(&item{1}), v.Keep(&item{2})
	if a == b || a == 0 || b == 0 {
		t.Fatalf("handles %d and %d", a, b)
	}
	if v.Handle(a).(*item).n != 1 || v.Handle(b).(*item).n != 2 {
		t.Fatal("handles give back the wrong items")
	}
	Release(v.ID(), a)
	if v.Handle(a) != nil || v.Handle(b) == nil {
		t.Fatal("release dropped the wrong item")
	}
	mustPanic(t, "releasing a handle twice", func() { Release(v.ID(), a) })
	mustPanic(t, "releasing for no VM", func() { Release(0, b) })
}

func TestMarkBroken(t *testing.T) {
	v := newVM(t, small)
	if v.Broken() {
		t.Fatal("a new VM is broken")
	}
	v.MarkBroken()
	if !v.Broken() {
		t.Fatal("MarkBroken did not stick")
	}
	mustPanic(t, "Enter on a broken VM", v.Enter)
}

// TestConcurrentVMs makes, uses and closes VMs on many goroutines, for -race.
func TestConcurrentVMs(t *testing.T) {
	var wg sync.WaitGroup
	for g := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 20 {
				v, err := New(Options{StackSize: small})
				if err != nil {
					t.Error(err)
					return
				}
				base, _ := v.Stack()
				if ByStack(base+uintptr(g)) != v || Lookup(v.ID()) != v {
					t.Error("another goroutine's VM found")
				}
				p := MemoryGrow(v.ID(), hostmem.Unit)
				*(*byte)(addr(p)) = byte(g)
				v.SetWallClock(time.Unix(int64(g), 0), false)
				if Now(v.ID(), NowWall) != int64(g)*1e9 {
					t.Error("another goroutine's clock read")
				}
				if err := v.Close(); err != nil {
					t.Error(err)
				}
			}
		}()
	}
	wg.Wait()
}

// TestCloseDropsHandles: Close lets go of what Keep holds, whether the
// engine released it or not, and a VM kept after Close holds nothing.
func TestCloseDropsHandles(t *testing.T) {
	v := newVM(t, small)
	big := v.Keep(make([]byte, 64<<20))
	small := v.Keep("small")
	if err := v.Close(); err != nil {
		t.Fatal(err)
	}
	if v.Handle(big) != nil || v.Handle(small) != nil {
		t.Fatal("handles still held after Close")
	}
	mustPanic(t, "Keep after Close", func() { v.Keep(1) })
	if err := v.Close(); err != nil {
		t.Fatal(err)
	}
}
