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

// The bottom of the shadow stack is the host's: it holds what the host
// passes the engine by address (rule 5 of the plan: no address of Go memory
// goes to C). Every entry starts above it.
const (
	scratchLen   = 0    // size_t, the length JS_ToCStringLen2 writes
	scratchCtx   = 8    // JSContext *, the context JS_ExecutePendingJob writes
	scratchUsage = 16   // JSMemoryUsage
	scratchStr   = 512  // C strings: the file name, property names
	scratchSize  = 4096 // where the engine's own use of the stack starts
)

// JS tags (engine/quickjs.h).
const (
	tagFirst     = -9
	tagObject    = -1
	tagException = 6
)

// VM is one runtime of the engine with one context.
type VM struct {
	h    *gohost.VM
	rt   uintptr
	ctx  uintptr
	base uintptr // the scratch
	sp   uintptr // where each entry starts
}

// Options are the settings of a new VM.
type Options struct {
	// Seed is where Math.random starts.
	Seed uint64
	// StackSize is the size of the shadow stack, 0 for the default of 32
	// MiB. It must be no smaller than the region the engine was generated
	// for (-shadow-size-log2 of the pass).
	StackSize uintptr
}

// ErrBroken is what a VM returns after a panic or an abort left through
// engine frames.
var ErrBroken = errors.New("cengine: VM unusable after a panic in the engine, only Close works")

// New makes a runtime and a context.
func New(o Options) (*VM, error) {
	h, err := gohost.New(gohost.Options{StackSize: o.StackSize})
	if err != nil {
		return nil, err
	}
	base, _ := h.Stack()
	v := &VM{h: h, base: base, sp: base + scratchSize}
	v.enter(func() { v.rt = JS_NewRuntime(v.sp, uintptr(h.ID()), uintptr(o.Seed)) })
	if v.rt == 0 {
		// The engine gave back what it had: the host unmaps the chunks.
		err := fmt.Errorf("cengine: no runtime: %w", h.LastError())
		return nil, errors.Join(err, h.Close())
	}
	if h.AllowRemap() {
		v.enter(func() { JS_SetHostRemap(v.sp, v.rt, 1) })
	}
	v.enter(func() { v.ctx = JS_NewContext(v.sp, v.rt) })
	if v.ctx == 0 {
		var leaked uintptr
		v.enter(func() { leaked = JS_FreeRuntime(v.sp, v.rt) })
		return nil, errors.Join(fmt.Errorf("cengine: no context (%d blocks left)", leaked), h.Close())
	}
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

// JSError is an exception thrown by the script.
type JSError struct {
	Message string // String(e)
	Stack   string // e.stack, when e is an object that has one
}

func (e *JSError) Error() string {
	if e.Stack == "" {
		return e.Message
	}
	return e.Message + "\n" + e.Stack
}

// Eval runs src as a global script, then the jobs it queued, and returns
// String() of its completion value.
func (v *VM) Eval(src, file string) (string, error) {
	if v.h.Broken() {
		return "", ErrBroken
	}
	var out string
	var err error
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
	p := JS_ToCStringLen2(v.sp, v.ctx, v.base+scratchLen, u, tag, 0)
	if p == 0 {
		return "", v.exception()
	}
	s := string(bytesAt(p, *(*uintptr)(addr(v.base + scratchLen))))
	JS_FreeCString(v.sp, v.ctx, p)
	return s, nil
}

// exception takes the pending exception of the context.
func (v *VM) exception() error {
	u, tag := JS_GetException(v.sp, v.ctx)
	defer v.free(u, tag)
	e := &JSError{}
	msg, err := v.string(u, tag)
	if err != nil {
		msg = "an exception whose String() threw: " + err.Error()
	}
	e.Message = msg
	if int32(tag) == tagObject {
		su, stag := JS_GetPropertyStr(v.sp, v.ctx, u, tag, v.cstring("stack"))
		if int32(stag) == tagException {
			v.free(JS_GetException(v.sp, v.ctx))
		} else {
			if int32(stag) != 3 { // undefined
				e.Stack, _ = v.string(su, stag)
			}
			v.free(su, stag)
		}
	}
	return e
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
// The engine copies what it keeps of it.
func (v *VM) cstring(s string) uintptr {
	if len(s) > scratchSize-scratchStr-1 {
		s = s[:scratchSize-scratchStr-1]
	}
	b := bytesAt(v.base+scratchStr, uintptr(len(s))+1)
	copy(b, s)
	b[len(s)] = 0
	return v.base + scratchStr
}

// RunGC runs the engine's full garbage collection.
func (v *VM) RunGC() {
	if v.h.Broken() {
		panic(ErrBroken)
	}
	v.enter(func() { JS_RunGC(v.sp, v.rt) })
}

// ReleaseMemory gives the free memory of the VM back to the OS, keeping keep
// bytes of it for the next allocations: whole free chunks are unmapped, the
// pages of free blocks dropped, and the shadow stack above its first part.
func (v *VM) ReleaseMemory(keep uintptr) (hostmem.Released, error) {
	if v.h.Broken() {
		return hostmem.Released{}, ErrBroken
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

// Usage returns what the VM holds now.
func (v *VM) Usage() Usage {
	if v.h.Broken() {
		panic(ErrBroken)
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

// Interrupt asks the running script to stop. It has no effect until the
// generator inserts polls.
func (v *VM) Interrupt() { v.h.Interrupt() }

// Close frees the context and the runtime and gives all the memory of the VM
// back to the OS. It returns the blocks the engine still had in use, which is
// 0 when nothing leaked. A broken VM is not freed by the engine, whose state
// is halfway: its memory is unmapped all the same.
func (v *VM) Close() (leaked uintptr, err error) {
	if !v.h.Broken() {
		v.enter(func() {
			JS_FreeContext(v.sp, v.ctx)
			leaked = JS_FreeRuntime(v.sp, v.rt)
		})
	}
	return leaked, v.h.Close()
}

// addr and bytesAt reach the engine's memory, outside the Go heap (see
// gohost).
func addr(a uintptr) unsafe.Pointer {
	return *(*unsafe.Pointer)(unsafe.Pointer(&a))
}

func bytesAt(a, n uintptr) []byte {
	return unsafe.Slice((*byte)(addr(a)), n)
}
