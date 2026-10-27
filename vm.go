// Copyright 2024 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package slowjs // import "github.com/liustack/slowjs"

import (
	"fmt"
	"math"
	"time"
)

// The eval timeout runs on the monotonic clock, which only goes forward. The
// wall clock can be set back or forward while a script runs (NTP, a person, a
// machine resumed from sleep): set back, a deadline on it fired that much
// later or never, and set forward, at once.
var (
	monoStart = time.Now()
	timeNow   = time.Now // tests replace it
)

// monoNow returns the time since monoStart in nanoseconds, from the
// monotonic readings of time.Now.
func monoNow() int64 {
	return int64(timeNow().Sub(monoStart))
}

// monoDeadline returns the time d from now on the clock of monoNow. A d
// further away than that clock reaches gives its end, which comes after
// 292 years: without the cap the deadline wrapped around to the past.
func monoDeadline(d time.Duration) int64 {
	now := monoNow()
	if int64(d) > math.MaxInt64-now {
		return math.MaxInt64
	}
	return now + int64(d)
}

// VM represents a Javascript context (or Realm). Each VM has its
// own global objects and system objects.
//
// Note: VM is not safe for concurrent use by multiple goroutines, except
// for [VM.Interrupt].
type VM struct {
	// intr is set when the VM is made and never written again, Close
	// included, so that Interrupt can read it from any goroutine.
	intr *interrupter
	vmState
}

// vmState is the state of a VM that Close clears.
type vmState struct {
	// engineState is the state of the VM in the engine (engine_lib.go).
	engineState
	// id is how a Builtin finds the VM, the context's opaque (see vms).
	id      uint32
	goFuncs map[string]int32
	timeout time.Duration
	// depth counts the calls into javascript in progress: a host function
	// that evaluates more javascript is one level down.
	depth int
	// host counts the calls of Go functions from javascript in progress.
	host int
	// panicked holds what a Go function called from javascript panicked
	// with, until the panic goes on in the Go code that called javascript
	// (see hostPanicked).
	panicked any
	// onClose holds the functions OnClose was given, for Close to run.
	onClose []func()
	// broken is set once a Go panic went up through the engine, after
	// which a call into javascript panics with brokenPanic.
	broken bool
	// shared holds the SharedBytecode the VM loaded, whose bytes its
	// functions point into, until Close has freed the runtime.
	shared []*SharedBytecode
}

// NewVM returns a newly created VM.
func NewVM() (*VM, error) {
	vm, err := newEngineVM()
	if err != nil {
		return nil, err
	}

	if err := vm.SetShapeCache(DefaultShapeCache); err != nil {
		vm.Close()
		return nil, err
	}
	return vm, nil
}

// DefaultShapeCache is the number of shapes a new VM keeps (see
// [VM.SetShapeCache]).
const DefaultShapeCache = 2048

// SetShapeCache sets how many shapes of plain objects m keeps after their
// last object is gone, 0 for none. The shape of an object is its keys in
// order: an object that gets the same keys in the same order as an object
// before it, as the objects a function returns often do, finds the shapes
// it goes through kept, instead of copying a shape for each key. m keeps
// the last n shapes it would have freed, of objects whose prototype is
// Object.prototype and that have at most 32 keys, none of them a symbol
// that is not registered. A new VM keeps DefaultShapeCache shapes.
//
// Kept shapes take memory that counts against the limit of SetMemoryLimit,
// and each holds the atoms of its keys. A kept shape of a few keys takes
// about 100 bytes. A shape of 32 keys of about ten characters, that no
// other object has, takes about 3 KiB with its atoms, so 2048 of them take
// about 6 MiB, and the table of atoms m grew for them keeps its size after
// they are let go. So a script
// that runs close to its memory limit can run out of memory with the cache
// on where it would not without it: give such a VM a smaller cache, or 0.
// Kept shapes are not let go when an allocation fails.
func (m *VM) SetShapeCache(n int) error {
	if n < 0 || n > math.MaxInt32/8 {
		return fmt.Errorf("slowjs: a shape cache of %d shapes", n)
	}
	if m.jsSetShapeCache(int32(n)) != 0 {
		return fmt.Errorf("slowjs: out of memory for a shape cache of %d shapes", n)
	}
	return nil
}

// DefaultRegExpCache and DefaultRegExpCacheBytes are how many compiled
// regexps a new VM keeps at most, and in how many bytes (see
// [VM.SetRegExpCache]).
const (
	DefaultRegExpCache      = 256
	DefaultRegExpCacheBytes = 1 << 20
)

// SetRegExpCache sets how many compiled regexps m keeps at most, n, and in
// how many bytes, 0 for none, and lets go of the ones it kept. Compiling a
// regexp whose pattern and flags are strings (a literal, new RegExp, RegExp,
// RegExp.prototype.compile, and the copy split makes of its regexp) takes
// the bytecode m kept for the same pattern and flags instead of compiling
// again. Code that makes a new RegExp of the same pattern on every call, as
// a syntax highlighter made anew for each render does, then compiles each
// pattern once. When a new one does not fit, m lets go of the one used
// least recently. A new VM keeps DefaultRegExpCache regexps in
// DefaultRegExpCacheBytes bytes, and n can be at most math.MaxInt32/8.
//
// What m keeps counts against the limit of SetMemoryLimit, the patterns and
// flags as well as the bytecode, and is not let go when an allocation
// fails. So a script that runs close to its memory limit can run out of
// memory with the cache on where it would not without it.
func (m *VM) SetRegExpCache(n, bytes int) error {
	if n < 0 || bytes < 0 || n > math.MaxInt32/8 || uint64(bytes) > uint64(^uintptr(0)) {
		return fmt.Errorf("slowjs: a regexp cache of %d regexps in %d bytes", n, bytes)
	}
	m.jsSetRegExpCache(int32(n), uint64(bytes))
	return nil
}

// SetEvalTimeout sets the timeout for the various eval functions. Passing
// zero 'd' disables timeouts. The timeout starts with the outermost call into
// javascript: a host function that evaluates more javascript on m runs inside
// the time left to the call that got it there. The timeout runs on the
// monotonic clock, so setting the system clock does not move it. A timeout
// too long for that clock (about 292 years) never fires.
func (m *VM) SetEvalTimeout(d time.Duration) error {
	m.timeout = d
	return nil
}

// SetMemoryLimit limits m's maxmimum memory usage, in bytes. Small values of
// 'limit' are not honored because the VM needs to allocate memory also for the
// exception object itself.  The particular value of "small" is unspecified and
// subject to change without notice.
func (m *VM) SetMemoryLimit(limit uintptr) {
	m.jsSetMemoryLimit(max(limit, 128*1024))
}

// SetGCThreshold sets the memory threshold at which a GC will be performed, in bytes.
func (m *VM) SetGCThreshold(threshold uintptr) {
	m.jsSetGCThreshold(threshold)
}

// GCThreshold returns the memory threshold at which a GC will be performed, in
// bytes. The engine checks it when it makes an object: when [VM.MemorySize]
// would pass it, the engine runs a GC and then sets the threshold to 1.5 times
// what the GC left. So a threshold other than the one [VM.SetGCThreshold] set
// last means the engine ran a GC since. [VM.RunGC] does not change it.
//
// As with SetGCThreshold, no other goroutine may use m meanwhile (see [VM]),
// and a call after Close panics.
func (m *VM) GCThreshold() uintptr {
	return m.jsGetGCThreshold()
}

// RunGC performs a GC now. It frees what javascript can no longer reach,
// objects in reference cycles included, and gives the small blocks the runtime
// keeps for reuse back to their arenas. Unlike the GC the engine runs when
// [VM.MemorySize] passes [VM.GCThreshold], it leaves the threshold as it is.
// It runs no javascript: the callbacks of a FinalizationRegistry whose targets
// it frees are queued and run at the next [VM.ExecutePendingJobs], so what they
// hold goes at a later GC.
//
// As with SetGCThreshold, no other goroutine may use m meanwhile (see [VM]),
// and a call after Close panics.
func (m *VM) RunGC() {
	m.jsRunGC()
}

// RunYoungGC performs a young GC now: it frees the reference cycles among
// the objects made since the last GC, young or full, and makes the objects
// it keeps old. It visits only those objects, so it takes about as long as
// the young objects are many, where [VM.RunGC] visits every object twice. It
// is meant to run after each unit of work, such as a page render, whose
// objects are mostly garbage once it ends.
//
// It frees no object RunGC would keep. It is conservative where RunGC is
// not: a cycle that runs through an old object, and a value of a WeakMap
// entry, with what it holds, while its map is reachable or old, stay until the
// next RunGC or the GC the engine runs by itself. So memory can grow over
// many young GCs: run RunGC from time to time, such as when [VM.MemorySize]
// after a young GC has grown well past what it was after the last RunGC.
//
// Like RunGC, it removes the weak references to what it frees, queues the
// callbacks of a FinalizationRegistry for the next [VM.ExecutePendingJobs],
// gives the small blocks the runtime keeps for reuse back to their arenas,
// runs no javascript and leaves [VM.GCThreshold] as it is. No other
// goroutine may use m meanwhile (see [VM]), and a call after Close panics.
func (m *VM) RunYoungGC() {
	m.jsRunYoungGC()
}

// MemorySize returns the memory m takes, in bytes, as [VM.SetMemoryLimit] and
// [VM.SetGCThreshold] count it. After [VM.RunGC] it is what the objects that
// javascript can still reach take, with the arenas they are in, plus the
// runtime's own structures and the object shapes it keeps for reuse.
//
// As with SetGCThreshold, no other goroutine may use m meanwhile (see [VM]),
// and a call after Close panics.
func (m *VM) MemorySize() uintptr {
	return m.jsGetMemorySize()
}

// SetMaxStackSize limits the JS call depth, in libc.TLS stack slots (~one per JS
// call frame in the CGo-free port). Deep recursion then throws a catchable
// RangeError instead of overflowing the Go goroutine stack. 0 restores the
// MaxStackSlots default.
func (m *VM) SetMaxStackSize(threshold uintptr) {
	m.jsSetMaxStackSize(threshold)
}

// Strip says what a VM leaves out of the functions it compiles (see
// [VM.SetStrip]).
type Strip int

const (
	// StripSource leaves out the source text: the toString of a function
	// shows [native code] instead, as it does for the engine's built-ins.
	StripSource Strip = 1
	// StripDebug leaves out the file name, line and column numbers, so a
	// stack frame of the function shows only its name, and the source too.
	StripDebug Strip = 2
)

// SetStrip sets what m leaves out of the functions it compiles from then on:
// StripSource, StripDebug, both or 0, the default, for nothing. That is every
// function m compiles: from source by Eval and the like, into bytecode by
// Compile, and in the page's own code by eval and new Function. Functions
// compiled before keep what they have. The bytecode of Compile keeps what is
// left out left out in any VM that loads it, which suits a script that plays
// a built-in, such as a polyfill whose functions should not show their
// source: compile it on a VM of its own, so that the page's functions keep
// their source.
func (m *VM) SetStrip(s Strip) {
	m.jsSetStripInfo(int32(s))
}

// Close releases the resources held by m.
func (m *VM) Close() error {
	goFuncsMu.Lock()
	for _, magic := range m.goFuncs {
		if goFuncs[magic] != nil {
			delete(goFuncs, magic)
			freeMagic = append(freeMagic, magic)
		}
	}
	goFuncsMu.Unlock()

	id := m.id
	onClose := m.onClose
	shared := m.shared
	err := m.closeEngine()
	forgetVM(id)
	for _, b := range shared {
		b.drop()
	}
	for _, f := range onClose {
		f()
	}
	return err
}

// OnClose arranges for f to run when Close runs, after the VM is released,
// so f must not use it. Close runs the functions once each, in the order
// OnClose was given them. Nothing else runs them: a VM that is never closed
// never runs them, and neither does the garbage collector.
//
// A package that keeps something per VM in a table of its own, such as a
// closure that may reference the VM, removes it here, so that the table
// does not keep the VM, or what it closes over, reachable after Close.
func (m *VM) OnClose(f func()) {
	m.onClose = append(m.onClose, f)
}
