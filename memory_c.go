// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build slowjs_cengine && linux && amd64

package slowjs // import "github.com/liustack/slowjs"

// Memory on the C engine: what a VM holds, what it gives back, and the shared
// bytecode outside every VM.

import (
	"sync"
	"syscall"
	"unsafe"

	"github.com/liustack/slowjs/internal/cengine"
)

// sharedMem holds the mapping of each SharedBytecode, by its address, until
// freeShared: the engine reads a copy of it (jsReadShared). It is mapped
// outside the Go heap, so that its address may go to and come back from a
// uintptr, as that of lib's C memory does.
var sharedMem sync.Map

// allocShared returns n bytes for a SharedBytecode, at least one, or 0 when
// the OS gives none.
func allocShared(n int) uintptr {
	b, err := syscall.Mmap(-1, 0, max(n, 1), syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_ANON|syscall.MAP_PRIVATE)
	if err != nil {
		return 0
	}
	p := uintptr(unsafe.Pointer(&b[0]))
	sharedMem.Store(p, b)
	return p
}

// freeShared unmaps the memory of allocShared.
func freeShared(p uintptr) {
	b, ok := sharedMem.LoadAndDelete(p)
	if !ok {
		panic("slowjs: freeShared of memory allocShared did not give")
	}
	if err := syscall.Munmap(b.([]byte)); err != nil {
		panic("slowjs: unmapping shared bytecode: " + err.Error())
	}
}

// jsMemoryUsage is JSMemoryUsage of engine/quickjs.h.
type jsMemoryUsage struct {
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

// memoryUsage returns what JS_ComputeMemoryUsage counts in the runtime of m.
func (m *VM) memoryUsage() (u MemoryUsage) {
	if m.entered == 0 {
		m.outer(func() { u = m.memoryUsage() })
		return u
	}
	p := m.sp
	cengine.JS_ComputeMemoryUsage(p+(unsafe.Sizeof(jsMemoryUsage{})+15)&^15, m.rt, p)
	s := *(*jsMemoryUsage)(addr(p))
	return MemoryUsage{
		MallocSize:         s.MallocSize,
		MallocLimit:        s.MallocLimit,
		MallocCount:        s.MallocCount,
		MemoryUsedSize:     s.MemoryUsedSize,
		MemoryUsedCount:    s.MemoryUsedCount,
		AtomCount:          s.AtomCount,
		AtomSize:           s.AtomSize,
		StrCount:           s.StrCount,
		StrSize:            s.StrSize,
		ObjCount:           s.ObjCount,
		ObjSize:            s.ObjSize,
		PropCount:          s.PropCount,
		PropSize:           s.PropSize,
		ShapeCount:         s.ShapeCount,
		ShapeSize:          s.ShapeSize,
		JSFuncCount:        s.JSFuncCount,
		JSFuncSize:         s.JSFuncSize,
		JSFuncCodeSize:     s.JSFuncCodeSize,
		JSFuncPC2LineCount: s.JSFuncPC2LineCount,
		JSFuncPC2LineSize:  s.JSFuncPC2LineSize,
		CFuncCount:         s.CFuncCount,
		ArrayCount:         s.ArrayCount,
		FastArrayCount:     s.FastArrayCount,
		FastArrayElements:  s.FastArrayElements,
		BinaryObjectCount:  s.BinaryObjectCount,
		BinaryObjectSize:   s.BinaryObjectSize,
		Footprint:          s.Footprint,
		FootprintLimit:     s.FootprintLimit,
		Chunks:             s.ChunkCount,
		ReleaseCandidate:   s.ReleaseCandidate,
	}
}

// releaseMemory gives the free memory of m back to the OS, keeping keep
// bytes of it, and returns how many bytes it gave back: whole free chunks,
// the pages of free blocks and the shadow stack above its first part. What
// the OS refuses stays, and Close tries it again.
func (m *VM) releaseMemory(keep uintptr) (n uintptr) {
	if m.entered == 0 {
		m.outer(func() { n = m.releaseMemory(keep) })
		return n
	}
	cengine.JS_ReleaseStart(m.sp, m.rt, keep)
	r, _ := m.h.Memory().Release(
		func() (uintptr, uintptr) { return cengine.JS_ReleaseNextChunk(m.sp, m.rt) },
		func() (uintptr, uintptr) { return cengine.JS_ReleaseNextRange(m.sp, m.rt) },
		0)
	return r.Chunks + r.Pages + r.Stack
}
