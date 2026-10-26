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
//	host_call(vm, this, argc, argv, magic)        Call
//	host_release(vm, handle)                      Release
//
// The memory of a VM is a hostmem.Memory: its chunks and its shadow stack,
// outside the Go heap. A VM is used by one goroutine at a time, as the
// engine's VM is. Interrupt may be called from any goroutine.
//
// What only a bug of the engine or the host does panics: a VM id or a shadow
// pointer the host does not know, a request it does not have, a chunk the VM
// does not hold. A panic that leaves through engine frames leaves the VM
// broken, and then it only takes Close.
package gohost // import "github.com/liustack/slowjs/internal/gohost"

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/liustack/slowjs/internal/hostmem"
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

// Options are the settings of a new VM.
type Options struct {
	// StackSize is the size of the shadow stack: a power of two of at least
	// 64 KiB, and no smaller than the region the engine was generated for.
	// 0 is hostmem.DefaultStackSize.
	StackSize uintptr
}

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
	interrupt atomic.Int32
	polls     atomic.Int64
	counts    Counts
	lastErr   error
	depth     int
	broken    bool
	remap     bool
	handles   map[uint32]any
	handle    uint32
}

// The VMs alive, by id and by the base of their shadow stack. sizes counts
// the VMs of each stack size, so that a shadow pointer finds its base.
var (
	mu     sync.RWMutex
	byID   = map[uint32]*VM{}
	byBase = map[uintptr]*VM{}
	sizes  = map[uintptr]int{}
	nextID uint32
)

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
	v := &VM{mem: mem, stack: base, stackSize: size, wall: realClock()}
	mu.Lock()
	for {
		nextID++
		if _, used := byID[nextID]; nextID != 0 && !used {
			break
		}
	}
	v.id = nextID
	byID[v.id] = v
	byBase[base] = v
	sizes[size]++
	mu.Unlock()
	return v, nil
}

// Close forgets the VM and unmaps its chunks and its shadow stack. The
// engine has freed its runtime by then, or never made one. When the OS
// refuses some memory, Close returns the error, and Close again tries what is
// left.
func (v *VM) Close() error {
	mu.Lock()
	if byID[v.id] == v {
		delete(byID, v.id)
		delete(byBase, v.stack)
		if sizes[v.stackSize]--; sizes[v.stackSize] == 0 {
			delete(sizes, v.stackSize)
		}
	}
	mu.Unlock()
	return v.mem.Close()
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

// Lookup returns the VM of the id, or nil.
func Lookup(id uint32) *VM {
	mu.RLock()
	v := byID[id]
	mu.RUnlock()
	return v
}

// ByStack returns the VM whose shadow stack holds sp, the pointer the engine
// passes to every callback. It panics when there is none.
func ByStack(sp uintptr) *VM {
	mu.RLock()
	defer mu.RUnlock()
	for size := range sizes {
		if v := byBase[sp&^(size-1)]; v != nil && v.stackSize == size {
			return v
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
// left completely, whatever inner entries saw. Enter panics on a broken VM.
func (v *VM) Enter() {
	if v.broken {
		panic("gohost: a VM broken by a panic in the engine takes only Close")
	}
	if v.depth == 0 {
		v.interrupt.Store(0)
	}
	v.depth++
}

// Leave ends the entry Enter began.
func (v *VM) Leave() {
	if v.depth == 0 {
		panic("gohost: Leave without Enter")
	}
	v.depth--
}

// Interrupt asks the engine to stop at its next poll. Any goroutine may call
// it.
func (v *VM) Interrupt() { v.interrupt.Store(1) }

// AllowRemap tells whether the engine may ask to remap a dedicated chunk,
// which only linux can. When it says yes, the host must tell the engine with
// JS_SetHostRemap, right after JS_NewRuntime.
func (v *VM) AllowRemap() bool {
	v.remap = hostmem.CanRemap
	return v.remap
}

// Keep stores x for the engine, which knows it by the handle returned until
// host_release.
func (v *VM) Keep(x any) uint32 {
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

// Poll is host_poll: the engine lets the Go scheduler and garbage collector
// stop it here, and learns whether the VM is asked to stop. It does not call
// into the engine.
func Poll(sp uintptr) int64 {
	v := ByStack(sp)
	v.polls.Add(1)
	return int64(v.interrupt.Load())
}

// Call is host_call. A magic of 0 or more names a function of the host:
// none is registered yet. Below 0 are the requests of the engine itself.
// argv points at argc values in the engine's memory.
func Call(sp uintptr, vm uint32, this Value, argc int32, argv uintptr, magic int32) Value {
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
