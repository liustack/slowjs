// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// Package gohost is the Go side of the callbacks the C engine asks of its
// host (engine/host.h and host_memory_grow in engine/heap.h), for the engine
// generated as Go assembly. It keeps the VMs by id and by shadow stack and
// answers each callback, with no engine linked in, so that it can be tested
// alone. The generated package has the functions the assembly calls by name
// and passes their arguments on to the ones here:
//
//	host_memory_grow(vm, size)                    MemoryGrow
//	host_now(vm, kind)                            Now
//	host_poll(sp)                                 Poll
//	host_call(vm, &this, argc, argv, magic)       Call
//	host_release(vm, handle)                      Release
//
// The memory of a VM is a hostmem.Memory: its chunks and its shadow stack,
// outside the Go heap. What the OS refuses to unmap at Close stays with the
// package (Leftovers) until a later try gives it back. A VM is used by one
// goroutine at a time, as the engine's VM is. Interrupt, Leftovers and
// CloseLeftovers may be called from any goroutine.
//
// What only a bug of the engine or the host does panics: a VM id or a shadow
// pointer the host does not know, a request it does not have, a chunk the VM
// does not hold. A panic that leaves through engine frames leaves the VM
// broken, and then it only takes Close.
//
// Finding the VM of a callback takes no lock and writes nothing shared, so
// that VMs on many goroutines do not slow each other down: a VM id is two
// atomic reads in a table (package idtable), and a shadow pointer finds the
// base of its stack by its alignment, where New wrote the VM's id
// (StackHeader).
package gohost // import "github.com/liustack/slowjs/internal/gohost"

import (
	"cmp"
	"errors"
	"fmt"
	"runtime/debug"
	"slices"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/liustack/slowjs/internal/hostmem"
	"github.com/liustack/slowjs/internal/idtable"
)

// Value is a JSValue as the engine passes it: the word of its union, then its
// tag.
type Value struct{ U, Tag uintptr }

// Undefined is JS_UNDEFINED.
var Undefined = Value{Tag: 3}

// Word is HOST_WORD(x): a pointer or a size passed as the u of a value with
// the tag JS_TAG_INT, which means nothing.
func Word(x uintptr) Value { return Value{U: x} }

// The kinds of host_now.
const (
	NowWall      = 0 // nanoseconds since 1970, on the clock of the VM
	NowMonotonic = 1 // nanoseconds from a fixed start, never going back
)

// The requests of the engine itself in host_call: magic below 0.
const (
	CallAbort         = -1 // the engine cannot go on, and does not return
	CallMemoryRelease = -2 // (address, size): unmap a dedicated chunk
	CallMemoryRemap   = -3 // (address, old size, new size): resize a dedicated chunk, linux only
)

// ErrAbort is what Call panics with when the engine aborts.
var ErrAbort = errors.New("gohost: the engine aborted")

// ErrTrap is what Trap panics with.
var ErrTrap = errors.New("gohost: the engine reached a trap")

// Options are the settings of a new VM.
type Options struct {
	// StackSize is the size of the shadow stack: a power of two of at least
	// 64 KiB, the size of the region the engine was generated for. 0 is
	// hostmem.DefaultStackSize.
	StackSize uintptr
	// PollCounter is where the counter of the engine's polls is, from the
	// base of the shadow stack: a word the host keeps for it, which host_poll
	// sets to the interval of the VM, above the StackHeader (0 is no
	// counter: the engine counts nothing).
	PollCounter uintptr
}

// StackHeader is the bytes at the base of each shadow stack that the host
// keeps: the id of the VM as a uint32, then its complement, which tells a
// header from other data. The engine's entries start above them, and so does
// what the code that calls the engine keeps there.
const StackHeader = 8

// DefaultPollInterval is the number of ticks between two polls of a new VM:
// the engine ticks on every cycle it runs, and polls when the count runs out.
const DefaultPollInterval = 10000

// DefaultKillGrace is how long a new VM gives a script to stop after
// Interrupt before a poll kills it.
const DefaultKillGrace = 50 * time.Millisecond

// ErrKilled is what Poll panics with when a script has not stopped by the
// grace after Interrupt: the panic leaves through the engine's frames, and
// the VM is broken.
var ErrKilled = errors.New("gohost: the script did not stop after Interrupt and was killed")

// Counts are what the callbacks of a VM did, for tests and measuring.
type Counts struct {
	Grows, GrowFailures   int64   // chunks mapped, and refused by the OS
	GrowBytes             uintptr // bytes mapped by Grow
	Frees, FreeFailures   int64   // dedicated chunks unmapped, and refused
	FreeBytes             uintptr
	Remaps, RemapFailures int64   // dedicated chunks resized, and refused
	RemapBytes            uintptr // bytes the resizes added
	Polls                 int64
}

// VM is the host side of one VM of the engine.
type VM struct {
	id        uint32
	mem       *hostmem.Memory
	stack     uintptr
	stackSize uintptr
	wall      clock
	// interrupt is 0, or the time of the first Interrupt since the
	// outermost entry began, on the monotonic clock of Now, plus 1.
	interrupt atomic.Int64
	polls     atomic.Int64
	counter   uintptr // the poll counter, or 0
	flag      uintptr // the engine's interrupt flag (SetInterruptFlag), or 0
	interval  int64
	grace     time.Duration
	killed    bool
	counts    Counts
	lastErr   error
	depth     int
	faultWas  bool // SetPanicOnFault of the goroutine before the outermost Enter
	broken    bool
	remap     bool
	closed    bool
	handles   map[uint32]any
	handle    uint32
	unmapping sync.Mutex // held while Close or CloseLeftovers unmaps mem
}

// vms are the VMs alive, by id. sizes counts the VMs of each stack size, and
// stackSizes lists them, smallest first, for ByStack to read without the
// lock. leftovers are the closed VMs that still hold memory the OS refused to
// unmap, with what it said the last time.
var (
	vms        idtable.Table[VM]
	mu         sync.Mutex
	sizes      = map[uintptr]int{}
	stackSizes atomic.Pointer[[]uintptr]
	leftovers  = map[*VM]error{}
)

// countSize adds n VMs of a stack of size bytes, under mu.
func countSize(size uintptr, n int) {
	was := sizes[size]
	sizes[size] += n
	if sizes[size] == 0 {
		delete(sizes, size)
	}
	if (was == 0) == (sizes[size] == 0) {
		return // the same sizes
	}
	list := make([]uintptr, 0, len(sizes))
	for s := range sizes {
		list = append(list, s)
	}
	slices.Sort(list)
	stackSizes.Store(&list)
}

// New maps the shadow stack of a new VM and gives it an id, which is never 0
// and not the id of another VM alive. The engine asks for its first chunk
// from inside JS_NewRuntime, so the VM is found by its id from now on.
func New(o Options) (*VM, error) {
	size := o.StackSize
	if size == 0 {
		size = hostmem.DefaultStackSize
	}
	mem, err := hostmem.New(size)
	if err != nil {
		return nil, err
	}
	base, _ := mem.Stack()
	v := &VM{mem: mem, stack: base, stackSize: size, wall: realClock(), interval: DefaultPollInterval, grace: DefaultKillGrace}
	if o.PollCounter != 0 {
		if o.PollCounter%8 != 0 || o.PollCounter > size-8 {
			mem.Close()
			panic(fmt.Sprintf("gohost: a poll counter at %d is not a word of the %d-byte stack", o.PollCounter, size))
		}
		v.counter = base + o.PollCounter
		*(*int64)(addr(v.counter)) = v.interval
	}
	id, err := vms.Add(v)
	if err != nil {
		mem.Close()
		return nil, fmt.Errorf("gohost: %w", err)
	}
	v.id = id
	*(*uint32)(addr(base)) = id
	*(*uint32)(addr(base + 4)) = ^id
	mu.Lock()
	countSize(size, 1)
	mu.Unlock()
	return v, nil
}

// Close forgets the VM, drops what Keep holds and unmaps its chunks and its
// shadow stack. The engine has freed its runtime by then, never made one, or
// was left halfway by a panic and will not run again: either way it releases
// no more handles. When the OS refuses some memory, Close returns the error
// and the VM stays among the Leftovers of the package until Close again or
// CloseLeftovers unmaps the rest, so that the memory is not lost when the VM
// itself is, as after a panic in the constructor of the engine's VM.
func (v *VM) Close() error {
	v.closed = true
	v.handles = nil
	if vms.Get(v.id) == v {
		// What the OS may refuse to unmap no longer names the VM.
		*(*uint64)(addr(v.stack)) = 0
		vms.Remove(v.id)
		mu.Lock()
		countSize(v.stackSize, -1)
		mu.Unlock()
	}
	return v.unmap()
}

// unmap unmaps what the closed VM holds, and keeps it among the leftovers
// while the OS refuses some of it.
func (v *VM) unmap() error {
	v.unmapping.Lock()
	defer v.unmapping.Unlock()
	err := v.mem.Close()
	mu.Lock()
	if err != nil {
		leftovers[v] = err
	} else {
		delete(leftovers, v)
	}
	mu.Unlock()
	return err
}

// Leftover is a closed VM that still holds memory, because the OS refused to
// unmap it.
type Leftover struct {
	ID    uint32        // the id the VM had
	Usage hostmem.Usage // what it still holds
	Err   error         // what the OS said the last time
}

// Leftovers lists the closed VMs that still hold memory, by id.
func Leftovers() []Leftover {
	var out []Leftover
	for _, v := range leftoverVMs() {
		v.unmapping.Lock()
		u := v.mem.Usage()
		mu.Lock()
		err, ok := leftovers[v]
		mu.Unlock()
		v.unmapping.Unlock()
		if ok {
			out = append(out, Leftover{ID: v.id, Usage: u, Err: err})
		}
	}
	return out
}

// CloseLeftovers tries again to unmap what the Leftovers hold, and returns
// what the OS refused this time.
func CloseLeftovers() error {
	var errs []error
	for _, v := range leftoverVMs() {
		errs = append(errs, v.unmap())
	}
	return errors.Join(errs...)
}

func leftoverVMs() []*VM {
	mu.Lock()
	vs := make([]*VM, 0, len(leftovers))
	for v := range leftovers {
		vs = append(vs, v)
	}
	mu.Unlock()
	slices.SortFunc(vs, func(a, b *VM) int { return cmp.Compare(a.id, b.id) })
	return vs
}

// ID is the id the engine passes to the callbacks of this VM.
func (v *VM) ID() uint32 { return v.id }

// Memory is the memory of the VM.
func (v *VM) Memory() *hostmem.Memory { return v.mem }

// Stack returns the base and the size of the shadow stack, which the
// engine's entries take a pointer into.
func (v *VM) Stack() (base, size uintptr) { return v.stack, v.stackSize }

// Counts returns what the callbacks of the VM did so far.
func (v *VM) Counts() Counts {
	c := v.counts
	c.Polls = v.polls.Load()
	return c
}

// LastError is the last error of the OS a callback met: the engine sees only
// a failure.
func (v *VM) LastError() error { return v.lastErr }

// Lookup returns the VM of the id, or nil. It takes no lock.
func Lookup(id uint32) *VM { return vms.Get(id) }

// ByStack returns the VM whose shadow stack holds sp, the pointer the engine
// passes to every callback. It takes no lock: for each size of stack, from
// the smallest, it reads the header at sp rounded down to the size, and takes
// the VM the header names when that VM's stack starts there. A smaller size
// rounds down to an address inside the same stack, so for a pointer the
// engine passed it reads only memory of that stack. It panics when no VM
// matches. A pointer the engine did not pass may read memory mapped to
// nothing, which faults, a panic under the SetPanicOnFault of Enter.
func ByStack(sp uintptr) *VM {
	if list := stackSizes.Load(); list != nil {
		for _, size := range *list {
			base := sp &^ (size - 1)
			if base == 0 {
				continue
			}
			id := *(*uint32)(addr(base))
			if *(*uint32)(addr(base + 4)) != ^id {
				continue
			}
			if v := vms.Get(id); v != nil && v.stack == base && v.stackSize == size {
				return v
			}
		}
	}
	panic(fmt.Sprintf("gohost: shadow pointer %#x outside every VM", sp))
}

func mustLookup(id uint32, what string) *VM {
	v := Lookup(id)
	if v == nil {
		panic(fmt.Sprintf("gohost: %s for VM %d, which the host does not have", what, id))
	}
	return v
}

// Broken reports whether a panic or an abort left through engine frames.
func (v *VM) Broken() bool { return v.broken }

// MarkBroken records that a panic or a Goexit left through engine frames: the
// engine's state is halfway, and the VM takes only Close from now on.
func (v *VM) MarkBroken() { v.broken = true }

// Enter is called before each call into the engine and Leave after it. The
// outermost entry clears the interrupt: a request holds until the engine has
// left completely, whatever inner entries saw. It also turns on
// debug.SetPanicOnFault for the goroutine until the matching Leave, which puts
// back what the goroutine had: a C access to an address that is mapped to
// nothing (a wild pointer, the guard page of the shadow stack) is then a
// panic that recovers, where the runtime would print "unexpected fault
// address" and end the process. Only an address below 0x1000 is a panic
// without it. Enter panics on a broken VM.
//
// The setting belongs to the goroutine, so Enter and Leave must run on the
// goroutine that calls the engine, which a call into the engine does by
// being synchronous. A fault that does not come as SIGSEGV or SIGBUS (an
// illegal instruction, a breakpoint) is not helped by it: the generator
// leaves no such instruction in the engine (see internal/goasm/gen).
func (v *VM) Enter() {
	if v.broken {
		panic("gohost: a VM broken by a panic in the engine takes only Close")
	}
	if v.depth == 0 {
		v.interrupt.Store(0)
		if v.flag != 0 {
			*(*int32)(addr(v.flag)) = 0
		}
		v.faultWas = debug.SetPanicOnFault(true)
	}
	v.depth++
}

// Leave ends the entry Enter began.
func (v *VM) Leave() {
	if v.depth == 0 {
		panic("gohost: Leave without Enter")
	}
	v.depth--
	if v.depth == 0 {
		debug.SetPanicOnFault(v.faultWas)
	}
}

// SetInterruptFlag gives the address of the engine's interrupt flag, an
// int32 in its memory that the engine reads where it can throw (see
// JS_GetInterruptFlag in engine/quickjs.h): Poll sets it to 1 once Interrupt
// asked to stop, and the outermost Enter clears it. The engine keeps the
// flag where it is for as long as it lives.
func (v *VM) SetInterruptFlag(a uintptr) {
	if a == 0 || a%4 != 0 {
		panic(fmt.Sprintf("gohost: an interrupt flag at %#x", a))
	}
	v.flag = a
}

// Interrupt asks the engine to stop at its next poll. Any goroutine may call
// it.
func (v *VM) Interrupt() {
	v.interrupt.CompareAndSwap(0, int64(time.Since(start))+1)
}

// Interrupted reports whether Interrupt asked the engine to stop since the
// outermost entry began.
func (v *VM) Interrupted() bool { return v.interrupt.Load() != 0 }

// SetPollInterval sets the number of ticks between two polls, at least 1:
// 1 makes every cycle poll, which the tests use to show that a poll keeps
// what the generated code holds. It takes effect at once. Like the engine,
// call it from the goroutine that uses the VM.
func (v *VM) SetPollInterval(n int64) {
	if n < 1 {
		panic(fmt.Sprintf("gohost: a poll interval of %d", n))
	}
	v.interval = n
	if v.counter != 0 {
		*(*int64)(addr(v.counter)) = n
	}
}

// SetKillGrace sets how long a script may run on after Interrupt before the
// next poll kills it: the poll panics with ErrKilled through the engine's
// frames, and the VM takes only Close from then on. A negative grace never
// kills.
func (v *VM) SetKillGrace(d time.Duration) { v.grace = d }

// KillGrace is the grace SetKillGrace set.
func (v *VM) KillGrace() time.Duration { return v.grace }

// Killed reports whether a poll killed the script.
func (v *VM) Killed() bool { return v.killed }

// AllowRemap tells whether the engine may ask to remap a dedicated chunk,
// which only linux can. When it says yes, the host must tell the engine with
// JS_SetHostRemap, right after JS_NewRuntime.
func (v *VM) AllowRemap() bool {
	v.remap = hostmem.CanRemap
	return v.remap
}

// Keep stores x for the engine, which knows it by the handle returned until
// host_release or Close.
func (v *VM) Keep(x any) uint32 {
	if v.closed {
		panic("gohost: Keep on a closed VM")
	}
	if v.handles == nil {
		v.handles = map[uint32]any{}
	}
	for {
		v.handle++
		if _, used := v.handles[v.handle]; v.handle != 0 && !used {
			break
		}
	}
	v.handles[v.handle] = x
	return v.handle
}

// Handle returns what Keep stored under h, or nil.
func (v *VM) Handle(h uint32) any { return v.handles[h] }

// MemoryGrow is host_memory_grow: a new chunk of size bytes, a multiple of
// 64 KiB, or 0 when the OS refuses, and the engine runs out of memory.
func MemoryGrow(vm uint32, size uintptr) uintptr {
	v := mustLookup(vm, "a chunk")
	p, err := v.mem.Grow(size)
	if err != nil {
		v.lastErr = err
		v.counts.GrowFailures++
		return 0
	}
	v.counts.Grows++
	v.counts.GrowBytes += size
	return p
}

// Now is host_now: the wall clock of the VM, or the monotonic clock, which
// needs no VM.
func Now(vm uint32, kind int32) int64 {
	switch kind {
	case NowMonotonic:
		return int64(time.Since(start))
	case NowWall:
		return mustLookup(vm, "the wall clock").wall.now()
	}
	panic(fmt.Sprintf("gohost: host_now of kind %d", kind))
}

// Poll is host_poll, which the engine calls when its count of cycles runs
// out. Its stack check is where the Go scheduler and garbage collector stop
// the goroutine in the engine: nothing in the generated code is stopped
// otherwise. It sets the counter again. Once Interrupt asked to stop, it sets
// the engine's interrupt flag, which the engine throws "interrupted" on where
// it can, and kills a script still running longer than the grace after the
// Interrupt. It does not call into the engine.
func Poll(sp uintptr) {
	v := ByStack(sp)
	v.polls.Add(1)
	if v.counter != 0 {
		*(*int64)(addr(v.counter)) = v.interval
	}
	t := v.interrupt.Load()
	if t == 0 {
		return
	}
	if v.flag != 0 {
		*(*int32)(addr(v.flag)) = 1
	}
	if v.grace >= 0 && time.Since(start)-time.Duration(t-1) >= v.grace {
		v.killed = true
		v.MarkBroken()
		panic(ErrKilled)
	}
}

// Call is host_call. A magic of 0 or more names a function of the host:
// none is registered yet. Below 0 are the requests of the engine itself.
// this is the address of the this value and argv points at argc values, both
// in the engine's memory. A request of the engine has no this value and the
// engine passes 0: another word there means that the host and the generated
// code do not agree on the arguments, so it panics.
func Call(sp uintptr, vm uint32, this uintptr, argc int32, argv uintptr, magic int32) Value {
	if magic < 0 && this != 0 {
		panic(fmt.Sprintf("gohost: host_call request %d with a this value at %#x: the arguments of the callback do not match the engine", magic, this))
	}
	switch magic {
	case CallAbort:
		// abort does not know its VM and passes 0: the shadow pointer tells.
		ByStack(sp).MarkBroken()
		panic(ErrAbort)
	case CallMemoryRelease:
		v := mustLookup(vm, "a chunk to release")
		mustArgs(argc, 2, "release")
		base, size := word(argv, 0), word(argv, 1)
		// The engine has taken the chunk out of its heap already. When the
		// OS refuses, it stays mapped and counted, and Close tries again.
		if err := v.mem.Free(base, size); err != nil {
			v.lastErr = err
			v.counts.FreeFailures++
			return Undefined
		}
		v.counts.Frees++
		v.counts.FreeBytes += size
		return Undefined
	case CallMemoryRemap:
		v := mustLookup(vm, "a chunk to remap")
		mustArgs(argc, 3, "remap")
		if !v.remap {
			panic("gohost: the engine asks to remap a chunk, and the host never said it can")
		}
		base, size, newSize := word(argv, 0), word(argv, 1), word(argv, 2)
		p, err := v.mem.Remap(base, size, newSize)
		if err != nil {
			v.lastErr = err
			v.counts.RemapFailures++
			return Word(0)
		}
		v.counts.Remaps++
		v.counts.RemapBytes += newSize - size
		return Word(p)
	}
	if magic >= 0 {
		panic(fmt.Sprintf("gohost: host_call of function %d: the host registers no function yet", magic))
	}
	panic(fmt.Sprintf("gohost: host_call with the unknown request %d", magic))
}

// Trap is host_trap, which the generator puts where the C, or a check the
// generator made, would trap. A trap instruction ends the process in Go
// (SIGILL and SIGTRAP cannot be recovered), so there is none in the engine:
// this panics with ErrTrap instead, and the VM is broken.
func Trap(sp uintptr) {
	ByStack(sp).MarkBroken()
	panic(ErrTrap)
}

// Release is host_release: the engine dropped the object that held the
// handle.
func Release(vm uint32, h uint32) {
	v := mustLookup(vm, "a handle to release")
	if _, ok := v.handles[h]; !ok {
		panic(fmt.Sprintf("gohost: VM %d releases handle %d, which it does not hold", vm, h))
	}
	delete(v.handles, h)
}

func mustArgs(argc, want int32, what string) {
	if argc != want {
		panic(fmt.Sprintf("gohost: %s with %d arguments, not %d", what, argc, want))
	}
}

// word reads the u of argument i, a HOST_WORD.
func word(argv uintptr, i int) uintptr {
	return *(*uintptr)(addr(argv + uintptr(16*i)))
}

// addr turns an address in the engine's memory, outside the Go heap, into a
// pointer. As in hostmem, the garbage collector ignores pointers outside the
// memory it manages, and the conversion reads the address through memory so
// that go vet does not take it for a Go pointer kept in a uintptr.
func addr(a uintptr) unsafe.Pointer {
	return *(*unsafe.Pointer)(unsafe.Pointer(&a))
}

// start is where the monotonic clock counts from.
var start = time.Now()

// clock is the wall clock of a VM: base nanoseconds since 1970 at the
// moment at, and moving from there with the monotonic clock, or fixed.
type clock struct {
	base   int64
	at     time.Time
	moving bool
}

func realClock() clock {
	now := time.Now()
	return clock{base: now.UnixNano(), at: now, moving: true}
}

func (c clock) now() int64 {
	if !c.moving {
		return c.base
	}
	return c.base + int64(time.Since(c.at))
}

// SetWallClock sets the wall clock of the VM to t: fixed there, as for a
// render that must give the same page every time, or moving on from there.
// A new VM has the real clock, moving.
func (v *VM) SetWallClock(t time.Time, moving bool) {
	v.wall = clock{base: t.UnixNano(), at: time.Now(), moving: moving}
}
