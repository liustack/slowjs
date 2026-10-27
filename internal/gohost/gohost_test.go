// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package gohost

import (
	"errors"
	"math"
	"runtime"
	"runtime/debug"
	"strings"
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
		if Lookup(v.ID()) == v {
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
	// ByStack reads the header of the stack a pointer would be in: outside
	// every stack that may be memory mapped to nothing, as the guard page
	// right above a stack is, a fault that SetPanicOnFault makes a panic, as
	// it is in the engine.
	defer debug.SetPanicOnFault(debug.SetPanicOnFault(true))
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

// TestStackHeader: New writes the id of the VM and its complement at the
// base of the stack, and ByStack finds a VM only where the header names a
// VM whose stack starts there: a header that names another VM, or whose
// check word is wrong, is a panic, not the wrong VM.
func TestStackHeader(t *testing.T) {
	a := newVM(t, small)
	b := newVM(t, small)
	base, _ := a.Stack()
	header := (*[2]uint32)(addr(base))
	if header[0] != a.ID() || header[1] != ^a.ID() {
		t.Fatalf("the header is %#x %#x, want the id %d and its complement", header[0], header[1], a.ID())
	}
	sp := base + 4096
	if ByStack(sp) != a {
		t.Fatal("ByStack does not find the VM of its header")
	}
	header[0], header[1] = b.ID(), ^b.ID()
	mustPanic(t, "a header that names another VM", func() { ByStack(sp) })
	header[0], header[1] = a.ID(), ^a.ID()+1
	mustPanic(t, "a header with a wrong check word", func() { ByStack(sp) })
	header[1] = ^a.ID()
	if ByStack(sp) != a {
		t.Fatal("ByStack does not find the VM once its header is back")
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
	r := Call(sp, v.ID(), 0, 2, args, CallMemoryRelease)
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
	mustPanic(t, "releasing a chunk released before", func() { Call(sp, v.ID(), 0, 2, args, CallMemoryRelease) })
	q := MemoryGrow(v.ID(), 2*hostmem.Unit)
	args = argv(t, v, q, hostmem.Unit)
	mustPanic(t, "releasing half a chunk", func() { Call(sp, v.ID(), 0, 2, args, CallMemoryRelease) })
	mustPanic(t, "release with one argument", func() { Call(sp, v.ID(), 0, 1, args, CallMemoryRelease) })
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
			Call(sp, v.ID(), 0, 3, args, CallMemoryRemap)
		})
		return
	}
	mustPanic(t, "a remap before AllowRemap", func() { Call(sp, v.ID(), 0, 3, args, CallMemoryRemap) })
	if !v.AllowRemap() {
		t.Fatal("AllowRemap says no on linux")
	}
	r := Call(sp, v.ID(), 0, 3, args, CallMemoryRemap)
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
	if r2 := Call(sp, v.ID(), 0, 3, args, CallMemoryRemap); r2 != Word(0) {
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
	got := mustPanic(t, "abort", func() { Call(base+4096, 0, 0, 0, 0, CallAbort) })
	if err, ok := got.(error); !ok || !errors.Is(err, ErrAbort) {
		t.Fatalf("abort panicked with %v, want ErrAbort", got)
	}
	if !v.Broken() {
		t.Fatal("the VM is not broken after abort")
	}
}

func TestTrap(t *testing.T) {
	v := newVM(t, small)
	base, _ := v.Stack()
	got := mustPanic(t, "host_trap", func() { Trap(base + 4096) })
	if err, ok := got.(error); !ok || !errors.Is(err, ErrTrap) {
		t.Fatalf("host_trap panicked with %v, want ErrTrap", got)
	}
	if !v.Broken() {
		t.Fatal("the VM is not broken after a trap")
	}
}

func TestCallUnknown(t *testing.T) {
	v := newVM(t, small)
	sp, _ := v.Stack()
	mustPanic(t, "a registered function", func() { Call(sp, v.ID(), 0, 0, 0, 0) })
	mustPanic(t, "an unknown request", func() { Call(sp, v.ID(), 0, 0, 0, -99) })
	mustPanic(t, "a call for no VM", func() { Call(sp, 0, 0, 0, 0, CallMemoryRelease) })
}

// A request of the engine has no this value and the engine passes 0. Any other
// word there is the arguments of the callback shifted: the host panics before
// it reads any of them.
func TestCallEngineRequestHasNoThis(t *testing.T) {
	v := newVM(t, small)
	sp, _ := v.Stack()
	args := sp + 64
	for _, magic := range []int32{CallAbort, CallMemoryRelease, CallMemoryRemap, -99} {
		got := mustPanic(t, "a request with a this value", func() { Call(sp, v.ID(), args, 2, args, magic) })
		if s, ok := got.(string); !ok || !strings.Contains(s, "this value") {
			t.Errorf("request %d panicked with %v, want the this value named", magic, got)
		}
	}
	if v.Broken() {
		t.Fatal("a refused request marked the VM broken: only abort does that")
	}
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

// TestPoll: a poll counts, and an interrupt holds for inner entries until the
// outermost entry begins again.
func TestPoll(t *testing.T) {
	v := newVM(t, small)
	base, _ := v.Stack()
	v.Enter()
	v.SetKillGrace(-1)
	Poll(base + 100)
	if v.Interrupted() {
		t.Fatal("interrupted without an Interrupt")
	}
	done := make(chan bool)
	go func() { v.Interrupt(); close(done) }()
	<-done
	Poll(base + 200)
	if !v.Interrupted() {
		t.Fatal("no interrupt after Interrupt")
	}
	// The request holds for inner entries, and only the outermost entry
	// clears it.
	v.Enter()
	Poll(base + 300)
	if !v.Interrupted() {
		t.Fatal("an inner entry cleared the interrupt")
	}
	v.Leave()
	v.Leave()
	if v.Counts().Polls != 3 {
		t.Fatalf("%d polls counted, want 3", v.Counts().Polls)
	}
	v.Enter()
	if v.Interrupted() {
		t.Fatal("the outermost entry kept the interrupt")
	}
	Poll(base)
	v.Leave()
	mustPanic(t, "Leave without Enter", v.Leave)
}

// faultMode is the setting of SetPanicOnFault for the calling goroutine.
func faultMode() bool {
	old := debug.SetPanicOnFault(false)
	debug.SetPanicOnFault(old)
	return old
}

// TestEnterPanicOnFault: the outermost entry turns SetPanicOnFault on, and
// the Leave that matches it puts back what the goroutine had. Inner entries
// leave it alone.
func TestEnterPanicOnFault(t *testing.T) {
	v := newVM(t, small)
	for _, was := range []bool{false, true} {
		defer debug.SetPanicOnFault(debug.SetPanicOnFault(was))
		v.Enter()
		if !faultMode() {
			t.Fatalf("was %v: not on inside the engine", was)
		}
		v.Enter()
		v.Leave()
		if !faultMode() {
			t.Fatalf("was %v: an inner Leave turned it off", was)
		}
		v.Leave()
		if faultMode() != was {
			t.Fatalf("was %v: Leave left it %v", was, faultMode())
		}
	}
}

// TestFaultInEngineIsAPanic: an access to an address that is mapped to
// nothing, such as the guard page of the shadow stack, is a panic that
// recovers while the engine runs. Without SetPanicOnFault the runtime prints
// "unexpected fault address" and ends the process.
func TestFaultInEngineIsAPanic(t *testing.T) {
	v := newVM(t, small)
	base, size := v.Stack()
	guard := (*byte)(addr(base + size))
	v.Enter()
	got := mustPanic(t, "a write to the guard page", func() { *guard = 1 })
	v.Leave()
	err, ok := got.(runtime.Error)
	if !ok {
		t.Fatalf("the fault panicked with %T %v, want a runtime.Error", got, got)
	}
	if a, ok := err.(interface{ Addr() uintptr }); !ok || a.Addr() != base+size {
		t.Fatalf("the fault panic %v does not name the address %#x", err, base+size)
	}
}

// TestPollCounter: host_poll sets the counter the generated code counts down
// to the interval of the VM, at its offset from the base of the stack. New
// sets it first, and so does a new interval. A VM made without a counter
// writes nothing there.
func TestPollCounter(t *testing.T) {
	const off = 384
	v, err := New(Options{StackSize: small, PollCounter: off})
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()
	base, _ := v.Stack()
	counter := (*int64)(addr(base + off))
	if *counter != DefaultPollInterval {
		t.Fatalf("a new VM's counter is %d, want %d", *counter, DefaultPollInterval)
	}
	*counter = -5
	v.Enter()
	Poll(base + 4096)
	if *counter != DefaultPollInterval {
		t.Fatalf("after a poll the counter is %d, want %d", *counter, DefaultPollInterval)
	}
	v.SetPollInterval(1)
	if *counter != 1 {
		t.Fatalf("after SetPollInterval(1) the counter is %d", *counter)
	}
	*counter = 0
	Poll(base + 4096)
	if *counter != 1 {
		t.Fatalf("after a poll the counter is %d, want 1", *counter)
	}
	v.Leave()
	mustPanic(t, "an interval of 0", func() { v.SetPollInterval(0) })
	mustPanic(t, "a counter outside the stack", func() { New(Options{StackSize: small, PollCounter: small}) })
	mustPanic(t, "a counter not on a word", func() { New(Options{StackSize: small, PollCounter: 4}) })

	w := newVM(t, small)
	wb, _ := w.Stack()
	*(*int64)(addr(wb + off)) = 7
	w.Enter()
	Poll(wb + 4096)
	w.SetPollInterval(3)
	w.Leave()
	if got := *(*int64)(addr(wb + off)); got != 7 {
		t.Fatalf("a VM without a counter wrote %d at the offset", got)
	}
}

// TestPollKill: when the script has not stopped by the grace after Interrupt,
// the next poll panics with ErrKilled and the VM is broken. A negative grace
// never kills, and a poll before the grace is over goes on. An entry that
// runs no script is never killed.
func TestPollKill(t *testing.T) {
	v := newVM(t, small)
	base, _ := v.Stack()
	if v.KillGrace() != DefaultKillGrace {
		t.Fatalf("the grace of a new VM is %v, want %v", v.KillGrace(), DefaultKillGrace)
	}
	v.SetKillGrace(30 * time.Millisecond)
	v.EnterScript(0)
	v.Interrupt()
	Poll(base + 64)
	if v.Broken() {
		t.Fatal("killed before the grace was over")
	}
	time.Sleep(40 * time.Millisecond)
	if got := mustPanic(t, "a poll after the grace", func() { Poll(base + 64) }); got != ErrKilled {
		t.Fatalf("the poll panicked with %v, want ErrKilled", got)
	}
	if !v.Broken() || !v.Killed() {
		t.Fatalf("after the kill: broken %v, killed %v", v.Broken(), v.Killed())
	}
	v.Leave()

	w := newVM(t, small)
	wb, _ := w.Stack()
	w.SetKillGrace(-1)
	w.EnterScript(0)
	w.Interrupt()
	time.Sleep(5 * time.Millisecond)
	Poll(wb + 64)
	if w.Broken() || w.Killed() {
		t.Fatal("a negative grace killed")
	}
	w.Leave()

	// A grace of 0 kills at the first poll after Interrupt, and only then.
	x := newVM(t, small)
	xb, _ := x.Stack()
	x.SetKillGrace(0)
	x.EnterScript(0)
	Poll(xb + 64)
	x.Interrupt()
	mustPanic(t, "a poll with a grace of 0", func() { Poll(xb + 64) })
	x.Leave()

	// An outermost entry that runs no script, as a collection is, only
	// sets the flag, and so does an inner script entry in it.
	g := newVM(t, small)
	gb, _ := g.Stack()
	testFlag = 0
	g.SetInterruptFlag(uintptr(unsafe.Pointer(&testFlag)))
	g.SetKillGrace(0)
	g.Enter()
	g.Interrupt()
	time.Sleep(time.Millisecond)
	Poll(gb + 64)
	g.EnterScript(0)
	Poll(gb + 64)
	g.Leave()
	if g.Broken() || g.Killed() || testFlag != 1 {
		t.Fatalf("an entry that runs no script: broken %v, killed %v, flag %d", g.Broken(), g.Killed(), testFlag)
	}
	g.Leave()
	// The next outermost entry decides again.
	g.EnterScript(0)
	g.Interrupt()
	mustPanic(t, "a script entry after one that runs none", func() { Poll(gb + 64) })
	g.Leave()
}

// TestPollDeadline: the outermost EnterScript with a timeout sets a deadline,
// and the first poll past it interrupts as Interrupt at the deadline would
// have: the flag is set, and the grace counts from the deadline. Inner
// entries keep the deadline of the outermost one. Enter, a timeout of 0, or
// one too far to count set none.
func TestPollDeadline(t *testing.T) {
	v := newVM(t, small)
	base, _ := v.Stack()
	testFlag = 0
	v.SetInterruptFlag(uintptr(unsafe.Pointer(&testFlag)))
	v.SetKillGrace(-1)
	v.EnterScript(50 * time.Millisecond)
	Poll(base + 64)
	if v.Interrupted() || testFlag != 0 {
		t.Fatal("interrupted before the deadline")
	}
	v.EnterScript(time.Hour)
	v.Enter()
	time.Sleep(60 * time.Millisecond)
	Poll(base + 64)
	if !v.Interrupted() || testFlag != 1 {
		t.Fatalf("past the deadline: interrupted %v, flag %d", v.Interrupted(), testFlag)
	}
	v.Leave()
	v.Leave()
	v.Leave()

	for _, c := range []struct {
		name  string
		enter func()
	}{
		{"Enter", v.Enter},
		{"no timeout", func() { v.EnterScript(0) }},
		{"a negative timeout", func() { v.EnterScript(-time.Second) }},
		{"a timeout too far to count", func() { v.EnterScript(math.MaxInt64) }},
	} {
		c.enter()
		time.Sleep(2 * time.Millisecond)
		Poll(base + 64)
		if v.Interrupted() || testFlag != 0 {
			t.Fatalf("%s: interrupted", c.name)
		}
		v.Leave()
	}

	// The grace counts from the deadline: soon past it the script goes on,
	// past the grace the poll kills.
	w := newVM(t, small)
	wb, _ := w.Stack()
	w.SetKillGrace(200 * time.Millisecond)
	w.EnterScript(5 * time.Millisecond)
	time.Sleep(15 * time.Millisecond)
	Poll(wb + 64)
	if !w.Interrupted() || w.Broken() {
		t.Fatalf("10 ms past the deadline: interrupted %v, broken %v", w.Interrupted(), w.Broken())
	}
	time.Sleep(200 * time.Millisecond)
	if got := mustPanic(t, "a poll past the deadline and the grace", func() { Poll(wb + 64) }); got != ErrKilled {
		t.Fatalf("the poll panicked with %v, want ErrKilled", got)
	}
	w.Leave()
}

// testFlag stands for the engine's interrupt flag: a global, which does not
// move as a variable of a goroutine's stack could.
var testFlag int32

// TestInterruptFlag: a poll after Interrupt sets the engine's interrupt
// flag, a poll before it does not, and the outermost Enter clears it. A flag
// at 0 or off a word is refused.
func TestInterruptFlag(t *testing.T) {
	v := newVM(t, small)
	base, _ := v.Stack()
	testFlag = 0
	v.SetInterruptFlag(uintptr(unsafe.Pointer(&testFlag)))
	v.SetKillGrace(-1)
	v.Enter()
	Poll(base + 64)
	if testFlag != 0 {
		t.Fatalf("a poll with no Interrupt set the flag to %d", testFlag)
	}
	v.Interrupt()
	Poll(base + 64)
	if testFlag != 1 {
		t.Fatalf("a poll after Interrupt left the flag at %d", testFlag)
	}
	v.Enter()
	if testFlag != 1 {
		t.Fatal("an inner Enter cleared the flag")
	}
	v.Leave()
	v.Leave()
	v.Enter()
	if testFlag != 0 || v.Interrupted() {
		t.Fatalf("the outermost Enter left the flag at %d, interrupted %v", testFlag, v.Interrupted())
	}
	v.Leave()
	mustPanic(t, "a flag at 0", func() { v.SetInterruptFlag(0) })
	mustPanic(t, "a flag off a word", func() { v.SetInterruptFlag(uintptr(unsafe.Pointer(&testFlag)) + 1) })
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

// BenchmarkEnterLeave is the cost of an outermost entry and its Leave,
// SetPanicOnFault included.
func BenchmarkEnterLeave(b *testing.B) {
	v, err := New(Options{StackSize: small})
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { v.Close() })
	for b.Loop() {
		v.Enter()
		v.Leave()
	}
}
