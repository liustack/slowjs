// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build cengine

// Package cengine is the C engine of engine/ generated as Go assembly, with
// its Go host. internal/goasm/gen/cmd/engine writes the assembly and
// engine_decl.go into this folder (-out internal/cengine), next to the host
// files checked in here, and nothing generated is checked in. The host files
// build only with the tag cengine, so that the module builds without the
// generated files.
//
// The host is a first one, for running scripts and measuring: VMs with their
// memory, clocks, seed and interrupt (package gohost answers the callbacks),
// Eval of a script to a string, RunGC, ReleaseMemory and Close with the count
// of blocks the engine still had. Registered Go functions, Builtin, Intl and
// Temporal are not there yet.
package cengine

import (
	"errors"
	"fmt"
	"time"
	"unsafe"

	"github.com/liustack/slowjs/internal/gohost"
	"github.com/liustack/slowjs/internal/hostmem"
)

// The bottom of the shadow stack is the host's: past the header of gohost,
// which names the VM for the callbacks that find it by their shadow pointer,
// it holds what the host passes the engine by address (rule 5 of the plan:
// no address of Go memory goes to C). Every entry starts above it.
const (
	scratchLen   = 8    // size_t, the length JS_ToCStringLen2 writes
	scratchCtx   = 16   // JSContext *, the context JS_ExecutePendingJob writes
	scratchUsage = 24   // JSMemoryUsage
	scratchStr   = 512  // C strings: the file name, property names
	scratchSize  = 4096 // where the engine's own use of the stack starts

	_ = uint(scratchLen - gohost.StackHeader) // fails to compile when they overlap

	// maxFileName is the longest file name the scratch holds, with its 0
	// byte.
	maxFileName = scratchSize - scratchStr - 1
)

// The counter of the engine's polls is a word of the scratch, at the offset
// the generator put in engine_decl.go (pollCounterOffset), between the usage
// report and the strings: these fail to compile when it is not.
const (
	_ = uint(pollCounterOffset - (scratchUsage + unsafe.Sizeof(memoryUsage{})))
	_ = uint(scratchStr - (pollCounterOffset + 8))
)

// pollCounter is the offset of the poll counter for gohost, 0 when the engine
// was generated without polls.
func pollCounter() uintptr {
	if !polls {
		return 0
	}
	return pollCounterOffset
}

// JS tags (engine/quickjs.h).
const (
	tagFirst     = -9
	tagObject    = -1
	tagNull      = 2
	tagUndefined = 3
	tagString    = -7
	tagRope      = -6
	tagException = 6
)

// DefaultShapeCache is the number of shapes of plain objects a new VM keeps
// after their last object is gone, as a VM of package slowjs does
// (slowjs.DefaultShapeCache): an object that gets the same keys in the same
// order as one before it finds its shapes kept.
const DefaultShapeCache = 2048

// VM is one runtime of the engine with one context.
type VM struct {
	h    *gohost.VM
	rt   uintptr
	ctx  uintptr
	base uintptr // the scratch
	sp   uintptr // where each entry starts

	freed  bool    // Close freed the engine, or tried to
	leaked uintptr // what JS_FreeRuntime returned
}

// Options are the settings of a new VM.
type Options struct {
	// Seed is where Math.random starts.
	Seed uint64
	// StackSize is the size of the shadow stack, 0 for the region the
	// engine was generated for (-shadow-size-log2 of the generator). Any
	// other size fails New: the engine finds the base of its stack by
	// rounding its stack pointer down to that region.
	StackSize uintptr
}

// ErrBroken is what a VM returns after a panic or an abort left through
// engine frames.
var ErrBroken = errors.New("cengine: VM unusable after a panic in the engine, only Close works")

// ErrKilled is what Eval returns when the script did not stop by the grace
// after Interrupt (gohost.VM.SetKillGrace) and a poll killed it. The VM is
// broken then: only Close works. The other calls into the engine panic with
// it, as they do with ErrBroken.
var ErrKilled = gohost.ErrKilled

// ErrClosed is what a VM returns, or panics with where it has no error to
// return, once Close began: also when Close failed to give all the memory
// back, since the engine is freed by then.
var ErrClosed = errors.New("cengine: VM closed")

// usable is nil when the engine can be entered.
func (v *VM) usable() error {
	if v.freed {
		return ErrClosed
	}
	if v.h.Broken() {
		return ErrBroken
	}
	return nil
}

// New makes a runtime and a context.
func New(o Options) (_ *VM, err error) {
	if o.StackSize == 0 {
		o.StackSize = 1 << shadowSizeLog2
	}
	if o.StackSize != 1<<shadowSizeLog2 {
		// The engine takes the space left from its stack pointer and the
		// size it was generated for (engine_decl.go): in a larger region
		// the base it rounds to, with the poll counter and the scratch,
		// would be in the middle of its frames.
		return nil, fmt.Errorf("cengine: a shadow stack of %d bytes, not the %d the engine was generated for",
			o.StackSize, uintptr(1)<<shadowSizeLog2)
	}
	h, err := gohost.New(gohost.Options{StackSize: o.StackSize, PollCounter: pollCounter()})
	if err != nil {
		return nil, err
	}
	// Unless New hands the VM back, the host forgets it and unmaps its
	// memory, also when a panic or a Goexit leaves New: the caller has no VM
	// to close. The engine is not freed then, since it stopped halfway.
	// What the OS refuses to unmap stays among gohost.Leftovers, with the
	// error, for gohost.CloseLeftovers to try again.
	handed := false
	defer func() {
		if !handed {
			if cerr := h.Close(); err != nil {
				err = errors.Join(err, cerr)
			}
		}
	}()
	base, _ := h.Stack()
	v := &VM{h: h, base: base, sp: base + scratchSize}
	v.enter(func() { v.rt = JS_NewRuntime(v.sp, uintptr(h.ID()), uintptr(o.Seed)) })
	if v.rt == 0 {
		// The engine gave back what it had: the host unmaps the chunks.
		return nil, fmt.Errorf("cengine: no runtime: %w", h.LastError())
	}
	if h.AllowRemap() {
		v.enter(func() { JS_SetHostRemap(v.sp, v.rt, 1) })
	}
	// The flag the polls set after Interrupt, which the engine throws
	// "interrupted" on.
	var flag uintptr
	v.enter(func() { flag = JS_GetInterruptFlag(v.sp, v.rt) })
	h.SetInterruptFlag(flag)
	v.enter(func() { v.ctx = JS_NewContext(v.sp, v.rt) })
	if v.ctx == 0 {
		var leaked uintptr
		v.enter(func() { leaked = JS_FreeRuntime(v.sp, v.rt) })
		return nil, fmt.Errorf("cengine: no context (%d blocks left)", leaked)
	}
	var shapes uintptr
	v.enter(func() { shapes = JS_SetShapeCache(v.sp, v.ctx, DefaultShapeCache) })
	if int32(shapes) != 0 {
		var leaked uintptr
		v.enter(func() {
			JS_FreeContext(v.sp, v.ctx)
			leaked = JS_FreeRuntime(v.sp, v.rt)
		})
		return nil, fmt.Errorf("cengine: no memory for a shape cache of %d shapes (%d blocks left)", DefaultShapeCache, leaked)
	}
	handed = true
	return v, nil
}

// Host is the host side of the VM: its clock, interrupt, memory and counts.
func (v *VM) Host() *gohost.VM { return v.h }

// enter runs f, which calls into the engine. A panic or a Goexit that leaves
// through the engine's frames leaves the VM broken.
func (v *VM) enter(f func()) {
	v.h.Enter()
	ok := false
	defer func() {
		v.h.Leave()
		if !ok {
			v.h.MarkBroken()
		}
	}()
	f()
	ok = true
}

// JSError is an exception thrown by the script, or one the engine ended the
// call with.
type JSError struct {
	Message string // String(e)
	Stack   string // e.stack, when e is an object that has one
	// Uncatchable: the engine ended the call with an error no try catches,
	// "interrupted" (Interrupt) or "out of memory". Message is then
	// "InternalError: " and the message the engine made it with.
	Uncatchable bool
}

func (e *JSError) Error() string {
	if e.Stack == "" {
		return e.Message
	}
	return e.Message + "\n" + e.Stack
}

// Eval runs src as a global script, then the jobs it queued, and returns
// String() of its completion value. It returns ErrClosed after Close began
// and ErrBroken after a panic in the engine. file names the script in the
// stack of an exception: it is at most maxFileName bytes, and a longer name is
// an error, not cut, so that two long names that differ only at their end are
// not taken for one.
func (v *VM) Eval(src, file string) (out string, err error) {
	if err := v.usable(); err != nil {
		return "", err
	}
	if len(file) > maxFileName {
		return "", fmt.Errorf("cengine: a file name of %d bytes, longer than the %d bytes the host holds", len(file), maxFileName)
	}
	defer func() {
		if r := recover(); r != nil {
			if r != gohost.ErrKilled {
				panic(r)
			}
			out, err = "", ErrKilled
		}
	}()
	v.enter(func() { out, err = v.eval(src, file) })
	return out, err
}

func (v *VM) eval(src, file string) (string, error) {
	// JS_Eval reads the source up to a 0 byte past its end.
	n := uintptr(len(src))
	buf := js_malloc_rt(v.sp, v.rt, n+1)
	if buf == 0 {
		return "", errors.New("cengine: out of memory for the source")
	}
	b := bytesAt(buf, n+1)
	copy(b, src)
	b[n] = 0
	u, tag := JS_Eval(v.sp, v.ctx, buf, n, v.cstring(file), 0)
	js_free_rt(v.sp, v.rt, buf)
	if int32(tag) == tagException {
		return "", v.exception()
	}
	defer v.free(u, tag)
	for {
		r := int32(JS_ExecutePendingJob(v.sp, v.rt, v.base+scratchCtx))
		if r < 0 {
			return "", v.exception()
		}
		if r == 0 {
			break
		}
	}
	return v.string(u, tag)
}

// string is String(value), or the exception it threw as an error.
func (v *VM) string(u, tag uintptr) (string, error) {
	s, ok := v.text(u, tag)
	if !ok {
		return "", v.exception()
	}
	return s, nil
}

// text is String(value). When that throws, text reports false and leaves
// what it threw pending.
func (v *VM) text(u, tag uintptr) (string, bool) {
	p := JS_ToCStringLen2(v.sp, v.ctx, v.base+scratchLen, u, tag, 0)
	if p == 0 {
		return "", false
	}
	s := string(bytesAt(p, *(*uintptr)(addr(v.base + scratchLen))))
	JS_FreeCString(v.sp, v.ctx, p)
	return s, true
}

// unprintable is the message of an exception whose String() threw.
const unprintable = "an exception whose String() threw"

// exception takes the pending exception of the context. It converts the
// exception and its stack once each, and leaves out what does not convert:
// an exception may throw again on every conversion, so what a conversion
// here throws is dropped, not converted in turn.
func (v *VM) exception() error {
	// The flag of an uncatchable exception outlives it: cleared here, or
	// the next exception would be uncatchable too.
	uncatchable := JS_IsUncatchableException(v.sp, v.ctx) != 0
	u, tag := JS_GetException(v.sp, v.ctx)
	JS_SetUncatchableException(v.sp, v.ctx, 0)
	defer v.free(u, tag)
	if uncatchable {
		return v.engineError(u, tag)
	}
	e := &JSError{Message: unprintable}
	if msg, ok := v.text(u, tag); ok {
		e.Message = msg
	} else {
		v.dropException()
	}
	if int32(tag) == tagObject {
		su, stag := JS_GetPropertyStr(v.sp, v.ctx, u, tag, v.cstring("stack"))
		if int32(stag) == tagException {
			v.dropException()
		} else {
			if int32(stag) != tagUndefined {
				if s, ok := v.text(su, stag); ok {
					e.Stack = s
				} else {
					v.dropException()
				}
			}
			v.free(su, stag)
		}
	}
	return e
}

// engineError converts an uncatchable exception, which the engine made, and
// runs no JavaScript to do it: after an interrupt the engine's flag stays set
// until the next call from Go, and every call of a function would throw
// "interrupted" again. It reads message and stack only as own properties of
// the error, where the engine defined them, so that no getter of a prototype
// runs. The engine throws null when it runs out of memory making the error.
func (v *VM) engineError(u, tag uintptr) *JSError {
	e := &JSError{Message: "InternalError", Uncatchable: true}
	switch int32(tag) {
	case tagNull:
		e.Message = "InternalError: out of memory"
	case tagObject:
		if m := v.ownString(u, tag, "message"); m != "" {
			e.Message += ": " + m
		}
		e.Stack = v.ownString(u, tag, "stack")
	}
	return e
}

// ownString is the own property name of the object (u, tag) when that is a
// string, and "" otherwise. What the read throws is dropped.
func (v *VM) ownString(u, tag uintptr, name string) string {
	atom := JS_NewAtom(v.sp, v.ctx, v.cstring(name))
	if atom == 0 {
		v.dropException()
		return ""
	}
	defer JS_FreeAtom(v.sp, v.ctx, atom)
	switch int32(JS_GetOwnProperty(v.sp, v.ctx, 0, u, tag, atom)) {
	case 1:
	case 0:
		return ""
	default:
		v.dropException()
		return ""
	}
	pu, ptag := JS_GetPropertyStr(v.sp, v.ctx, u, tag, v.cstring(name))
	if int32(ptag) == tagException {
		v.dropException()
		return ""
	}
	defer v.free(pu, ptag)
	if int32(ptag) != tagString && int32(ptag) != tagRope {
		return ""
	}
	s, ok := v.text(pu, ptag)
	if !ok {
		v.dropException()
	}
	return s
}

// dropException frees the pending exception of the context, and the flag
// that says it is uncatchable.
func (v *VM) dropException() {
	v.free(JS_GetException(v.sp, v.ctx))
	JS_SetUncatchableException(v.sp, v.ctx, 0)
}

// free is JS_FreeValue, which is inline in quickjs.h: the reference count is
// the int32 before the block.
func (v *VM) free(u, tag uintptr) {
	// JS_VALUE_HAS_REF_COUNT: the tags from JS_TAG_FIRST to -1, compared
	// as unsigned.
	if uint32(tag) < 1<<32+tagFirst {
		return
	}
	rc := (*int32)(addr(u - 4))
	if *rc--; *rc <= 0 {
		__JS_FreeValue(v.sp, v.ctx, u, tag)
	}
}

// cstring writes s with a 0 byte into the scratch and returns its address.
// The engine copies what it keeps of it. s is at most maxFileName bytes.
func (v *VM) cstring(s string) uintptr {
	if len(s) > maxFileName {
		panic(fmt.Sprintf("cengine: a string of %d bytes does not fit the scratch of %d", len(s), maxFileName))
	}
	b := bytesAt(v.base+scratchStr, uintptr(len(s))+1)
	copy(b, s)
	b[len(s)] = 0
	return v.base + scratchStr
}

// RunGC runs the engine's full garbage collection. It has no error to
// return: after Close began it panics with ErrClosed, and after a panic in the
// engine with ErrBroken.
func (v *VM) RunGC() {
	if err := v.usable(); err != nil {
		panic(err)
	}
	v.enter(func() { JS_RunGC(v.sp, v.rt) })
}

// ReleaseMemory gives the free memory of the VM back to the OS, keeping keep
// bytes of it for the next allocations: whole free chunks are unmapped, the
// pages of free blocks dropped, and the shadow stack above its first part. It
// returns ErrClosed after Close began and ErrBroken after a panic in the
// engine.
func (v *VM) ReleaseMemory(keep uintptr) (hostmem.Released, error) {
	if err := v.usable(); err != nil {
		return hostmem.Released{}, err
	}
	var r hostmem.Released
	var err error
	v.enter(func() {
		JS_ReleaseStart(v.sp, v.rt, keep)
		r, err = v.h.Memory().Release(
			func() (uintptr, uintptr) { return JS_ReleaseNextChunk(v.sp, v.rt) },
			func() (uintptr, uintptr) { return JS_ReleaseNextRange(v.sp, v.rt) },
			0)
	})
	return r, err
}

// Usage is what the engine and the host count of the memory of a VM.
type Usage struct {
	MemorySize uintptr // what the limit and the GC count: the blocks in use, with their headers
	Footprint  uintptr // the chunks, as the engine counts them
	Chunks     uintptr
	Host       hostmem.Usage // the chunks and the stack, as the host counts them
	Counts     gohost.Counts
}

// memoryUsage is JSMemoryUsage of engine/quickjs.h.
type memoryUsage struct {
	MallocSize, MallocLimit, MemoryUsedSize                   int64
	MallocCount, MemoryUsedCount                              int64
	AtomCount, AtomSize, StrCount, StrSize                    int64
	ObjCount, ObjSize, PropCount, PropSize                    int64
	ShapeCount, ShapeSize                                     int64
	JSFuncCount, JSFuncSize, JSFuncCodeSize                   int64
	JSFuncPC2LineCount, JSFuncPC2LineSize                     int64
	CFuncCount, ArrayCount, FastArrayCount, FastArrayElements int64
	BinaryObjectCount, BinaryObjectSize                       int64
	Footprint, FootprintLimit, ChunkCount, ReleaseCandidate   int64
}

// Usage returns what the VM holds now. Like RunGC it panics with ErrClosed
// after Close began and with ErrBroken after a panic in the engine. The host
// side of a closed VM is still there to read: Host().Memory().Usage().
func (v *VM) Usage() Usage {
	if err := v.usable(); err != nil {
		panic(err)
	}
	var u Usage
	v.enter(func() {
		u.MemorySize = JS_GetMemorySize(v.sp, v.rt)
		JS_ComputeMemoryUsage(v.sp, v.rt, v.base+scratchUsage)
	})
	m := *(*memoryUsage)(addr(v.base + scratchUsage))
	u.Footprint, u.Chunks = uintptr(m.Footprint), uintptr(m.ChunkCount)
	u.Host = v.h.Memory().Usage()
	u.Counts = v.h.Counts()
	return u
}

// SetWallClock sets the clock Date reads: fixed at t, or moving on from t.
func (v *VM) SetWallClock(t time.Time, moving bool) { v.h.SetWallClock(t, moving) }

// Interrupt asks the running script to stop. Any goroutine may call it. The
// next poll of the engine sets its interrupt flag, and the engine throws the
// error "interrupted", which no try catches, where it next reads the flag:
// Eval returns it as a *JSError and the VM goes on. A script still running
// when the grace is over (gohost.VM.SetKillGrace) is killed at the poll
// after: Eval returns ErrKilled and the VM takes only Close.
func (v *VM) Interrupt() { v.h.Interrupt() }

// Close frees the context and the runtime and gives all the memory of the VM
// back to the OS. It returns the blocks the engine still had in use, which is
// 0 when nothing leaked. A broken VM is not freed by the engine, whose state
// is halfway: its memory is unmapped all the same. The engine is freed once:
// when the OS refuses some memory, Close returns the error and Close again
// only tries what is left.
func (v *VM) Close() (leaked uintptr, err error) {
	if !v.freed {
		v.freed = true
		if !v.h.Broken() {
			done := false
			defer func() {
				if !done {
					// A panic or a Goexit left the engine halfway: it is not
					// entered again, the host still gives the memory back,
					// and the panic goes on. What the OS refuses here is
					// listed by gohost.Leftovers with the error, and Close
					// again tries it.
					v.h.Close()
				}
			}()
			engineFreed(v)
			v.enter(func() {
				// The kept shapes hold the context: they go first.
				JS_SetShapeCache(v.sp, v.ctx, 0)
				JS_FreeContext(v.sp, v.ctx)
				v.leaked = JS_FreeRuntime(v.sp, v.rt)
			})
			done = true
		}
	}
	return v.leaked, v.h.Close()
}

// addr and bytesAt reach the engine's memory, outside the Go heap (see
// gohost).
func addr(a uintptr) unsafe.Pointer {
	return *(*unsafe.Pointer)(unsafe.Pointer(&a))
}

func bytesAt(a, n uintptr) []byte {
	return unsafe.Slice((*byte)(addr(a)), n)
}
