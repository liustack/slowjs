// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package slowjs // import "github.com/liustack/slowjs"

// Making a VM, its settings, its memory and closing it.

import (
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/liustack/slowjs/internal/hostmem"
)

// VM is a javascript realm, with its global object, and the runtime that runs
// it. Each VM has its own memory and its own globals.
//
// A VM is not safe for use by several goroutines at once: one goroutine uses
// it at a time. [VM.Interrupt] is the exception, and may be called from any
// goroutine at any time.
type VM struct {
	// What stays after Close. intr is set when the VM is made and never
	// changes, so that Interrupt reads it from any goroutine. keep is the
	// block of memory the Go functions of the VM borrow.
	intr    *interrupter
	closed  bool
	closing bool
	keep    hostmem.Keep

	// What Close clears in one go.
	vmState
}

// vmState is the state of a VM that Close clears (closeEngine). A field that
// is to go with the VM when it closes belongs here.
type vmState struct {
	engineState

	// id is the VM's number in vms, by which a Builtin finds it.
	id uint32
	// timeout is the eval timeout, which the outermost enter arms.
	timeout time.Duration
	// memoryLimit is the limit of SetMemoryLimit, 0 for none.
	memoryLimit uintptr
	// depth counts the calls into javascript in progress, and host the calls
	// of Go functions from javascript in progress.
	depth int
	host  int
	// elems counts the elements and properties the conversion of results
	// made (elemStop).
	elems uint
	// snap is the snapshot of results kept for the next call (result.go).
	snap *snapshot
	// panicked is the panic of a Go function that javascript called, for the
	// Go code that called javascript to go on with (repanic).
	panicked any
	// shared are the SharedBytecodes the VM loaded, which it holds until
	// Close.
	shared []*SharedBytecode
	// readingRejections is set while UnhandledRejections reads.
	readingRejections bool

	// registered are the names RegisterFunc and RegisterHostFunc took, as
	// javascript sees them, with the handle of each function.
	registered map[string]int32
	// onClose are the functions of OnClose, in order.
	onClose []func()
}

// NewVM returns a new VM.
func NewVM() (*VM, error) {
	m, err := newEngineVM()
	if err != nil {
		return nil, err
	}
	if err := m.SetShapeCache(DefaultShapeCache); err != nil {
		m.Close()
		return nil, err
	}
	m.keep.Max = keepMax
	m.keep.Allow = m.mayKeep
	m.keep.Hold = m.h.Hold
	m.h.SetPressure(m.goPressure)
	return m, nil
}

// SetShapeCache sets how many shapes m keeps once nothing uses them, n from 0,
// which keeps none, to math.MaxInt32/8. A shape is the list of an object's
// keys in their order. m keeps the shapes of plain objects: whose prototype is
// Object.prototype, with at most 32 keys and no symbol key that is not
// registered. The next object made with the same keys then finds its shape
// kept. A new VM keeps [DefaultShapeCache].
//
// The kept shapes count toward the memory limit (see [VM.SetMemoryLimit]),
// and each holds the atoms of its keys: about 100 bytes for a few keys, and
// about 3 KiB for 32 keys of ten characters each that no other object has, so
// 2048 such shapes take about 6 MiB. The table of atoms does not shrink once
// it grew. An allocation that would pass the memory limit lets go of all the
// kept shapes first.
func (m *VM) SetShapeCache(n int) error {
	if err := m.usable(); err != nil {
		return err
	}
	if n < 0 || n > math.MaxInt32/8 {
		return fmt.Errorf("slowjs: a shape cache of %d shapes", n)
	}
	if m.jsSetShapeCache(int32(n)) != 0 {
		return fmt.Errorf("slowjs: out of memory for a shape cache of %d shapes", n)
	}
	return nil
}

// SetRegExpCache sets how many compiled regexps m keeps at most, n, and in how
// many bytes, counting their patterns and flags. It lets go of the regexps
// kept so far. 0 for either keeps none. A new VM keeps [DefaultRegExpCache]
// in [DefaultRegExpCacheBytes].
//
// A regexp whose pattern and flags are strings, from a literal, new RegExp,
// RegExp, compile or the copy split makes, takes the bytecode kept for the
// same pattern and flags when it is compiled. The one used least recently goes
// when there is no room. What is kept counts toward the memory limit, and m
// does not let it go when an allocation fails, so a script that runs close to
// the limit can run out of memory with the cache on where it would not
// without it.
func (m *VM) SetRegExpCache(n, bytes int) error {
	if err := m.usable(); err != nil {
		return err
	}
	if n < 0 || bytes < 0 || n > math.MaxInt32/8 || uint64(bytes) > uint64(^uintptr(0)) {
		return fmt.Errorf("slowjs: a regexp cache of %d regexps in %d bytes", n, bytes)
	}
	m.jsSetRegExpCache(int32(n), uint64(bytes))
	return nil
}

// SetEvalTimeout sets how long a call into javascript may run, d, from the
// next call on: each outermost call has d from when it starts, on the
// monotonic clock, so setting the system clock changes nothing. d of 0 or
// less is no timeout, and so is one longer than the clock can count, about
// 292 years. A call that runs out of time ends with an *Error whose
// Interrupted is set.
//
// The timeout holds for every method that runs javascript: the Eval methods,
// Compile, the Call methods, ExecutePendingJobs, Value.Then, the property
// methods GetProperty, GetPropertyValue, SetProperty and SetPropertyValue and
// the methods of Value that call them, Value.MarshalJSON, Value.Any and
// [ErrorFromValue]. A method that runs no javascript, such as NewAtom or
// NewObjectValue, has none. A call a Go function makes from javascript, on
// the same VM, runs in the time left to the outer call.
func (m *VM) SetEvalTimeout(d time.Duration) error {
	if err := m.usable(); err != nil {
		return err
	}
	m.timeout = d
	return nil
}

// SetMemoryLimit sets the most memory m may take, in bytes, as
// [VM.MemorySize] counts it. A limit below 128 KiB is taken as 128 KiB, 0
// too: it is not "no limit". ^uintptr(0) is no limit. A call that would pass
// the limit ends with an *Error whose OutOfMemory is set.
//
// A limit that is too small is not kept to, as m needs memory to make the
// error itself. How small that is is not promised.
func (m *VM) SetMemoryLimit(limit uintptr) {
	limit = max(limit, 128<<10)
	if limit != ^uintptr(0) && m.jsGetMemorySize()+m.keep.Size() > limit {
		// The block m keeps does not fit under the new limit. Giving it back
		// panics when the OS refuses it, with the limit as it was.
		m.keep.Release()
	}
	m.jsSetMemoryLimit(limit)
	if limit == ^uintptr(0) {
		limit = 0
	}
	m.memoryLimit = limit
}

// SetGCThreshold sets how much memory m may take before the engine runs a
// collection by itself (see [VM.GCThreshold]). ^uintptr(0) keeps the engine
// from running one.
func (m *VM) SetGCThreshold(threshold uintptr) {
	m.jsSetGCThreshold(threshold)
}

// GCThreshold returns the threshold of the next collection the engine runs by
// itself. It counts the memory a collection can free: MemorySize without the
// stack of the register loop. After each full collection, of the engine or of
// [VM.RunGC], the threshold is 1.5 times what that memory is then, unless it
// is ^uintptr(0): so a threshold other than the one SetGCThreshold set says a
// full collection ran since.
//
// As every method of m, it is not for use by other goroutines at the same
// time. After Close it panics. After a kill it still answers.
func (m *VM) GCThreshold() uintptr {
	return m.jsGetGCThreshold()
}

// RunGC frees what javascript can no longer reach, cycles too, and gives the
// small blocks the runtime keeps for reuse back to its arenas. The GC
// threshold is then 1.5 times what is left, unless it is ^uintptr(0). It runs
// no javascript: the callbacks of a FinalizationRegistry are queued for the
// next [VM.ExecutePendingJobs].
func (m *VM) RunGC() {
	m.jsRunGC()
}

// RunYoungGC frees the cycles among the objects made since the last
// collection, young or full, and makes those it keeps old. It takes time in
// proportion to the young objects, which suits a run after each render.
//
// It frees less than [VM.RunGC]: a cycle through an old object, and the value
// of a WeakMap entry whose map is reachable or old, wait for the next RunGC,
// so run RunGC now and then too. It treats weak references and
// FinalizationRegistry as RunGC does, and leaves the GC threshold as it is.
func (m *VM) RunYoungGC() {
	m.jsRunYoungGC()
}

// MemorySize returns the memory m takes, in bytes, as [VM.SetMemoryLimit]
// counts it. [VM.MemoryFree] says how much of it no block in use holds.
func (m *VM) MemorySize() uintptr {
	return m.jsGetMemorySize()
}

// MemoryFree returns the bytes of MemorySize that no block in use holds: holes
// in the arenas, the free end of an arena, and the free blocks the caches
// keep. After [VM.RunGC], MemorySize minus MemoryFree is what the objects
// javascript reaches and the structures of the runtime take, a figure that
// stays put while the same objects are kept: it is the one to judge when to
// replace a VM. It walks the arenas, some tens of microseconds for thousands
// of them, and may be read at any time, from a Go function javascript called
// and after a kill too.
func (m *VM) MemoryFree() uintptr {
	return m.jsGetMemoryFree()
}

// SetMaxStackSize sets how deep javascript calls may nest, 0 for the default
// of 5000, at most 10000. A deeper recursion throws a RangeError the script
// can catch.
func (m *VM) SetMaxStackSize(threshold uintptr) {
	m.jsSetMaxStackSize(threshold)
}

// SetStrip sets what the functions m compiles from now on leave out: those of
// the Eval methods and Compile, and those of a script's own eval and new
// Function. With [StripSource] the toString of a function shows [native
// code]. [StripDebug] leaves out file names and line and column numbers, so
// that a stack frame shows only the name of its function, and the source
// too. Functions compiled before stay as they are.
//
// The bytecode of [VM.Compile] keeps what it left out in every VM that loads
// it. Compiled on a VM of its own with both, a polyfill looks like a built-in
// function.
func (m *VM) SetStrip(s Strip) {
	m.jsSetStripInfo(int32(s))
}

// errCloseInCall is the error of Close during a call into the VM.
var errCloseInCall = errors.New("slowjs: Close while the VM runs a call")

// Close frees m and all it holds, then runs the functions of [VM.OnClose].
//
// Close of a closed VM does nothing and returns nil, so a deferred Close
// after an explicit one is fine. When the OS refuses to take some memory
// back, Close returns an error, and a Close after it tries what is left
// again: the error only says that some memory was not given back when Close
// returned.
//
// A step of Close that panics does not keep the others from running: the
// other functions of OnClose run too. Close then panics with the first panic.
// Close from a function of OnClose returns nil. Close during a call into m,
// such as from a Go function that javascript called, returns an error and
// changes nothing: a Close after the call closes m.
func (m *VM) Close() (err error) {
	if m.closing {
		return nil
	}
	if m.entered != 0 {
		return errCloseInCall
	}
	var first any
	step := func(f func()) {
		defer func() {
			if p := recover(); p != nil && first == nil {
				first = p
			}
		}()
		f()
	}
	if m.closed {
		step(m.keep.Release)
		step(func() { err = m.closeEngineAgain() })
		if first != nil {
			panic(first)
		}
		return err
	}
	m.closed = true
	m.closing = true
	defer func() { m.closing = false }()
	m.forgetFuncs()

	// closeEngine clears vmState: what the steps after it need is taken now.
	id, onClose, shared := m.id, m.onClose, m.shared
	// The block goes first: a block given back after it, by a function of
	// OnClose, goes straight to the OS.
	step(m.keep.Close)
	step(func() { err = m.closeEngine() })
	step(func() { forgetVM(id) })
	for _, b := range shared {
		step(b.drop)
	}
	for _, f := range onClose {
		step(f)
	}
	if first != nil {
		panic(first)
	}
	return err
}

// OnClose makes Close run f once m is freed, so f must not use m. Each
// function runs once, in the order given. Nothing else runs them: not the
// GC, and not for a VM that is never closed. A package that keeps something
// for each VM in a table of its own drops it here, so that the table does not
// keep the VM alive.
func (m *VM) OnClose(f func()) {
	m.onClose = append(m.onClose, f)
}
