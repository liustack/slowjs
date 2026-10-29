// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build (linux && amd64) || (darwin && arm64)

package slowjs // import "github.com/liustack/slowjs"

// Memory on the engine: what a VM holds, what it gives back, and the shared
// bytecode outside every VM.

import (
	"unsafe"

	"github.com/liustack/slowjs/internal/cengine"
	"github.com/liustack/slowjs/internal/hostmem"
)

// allocShared returns a copy of the bytes of a SharedBytecode in a read-only
// mapping outside every VM, which the engine of each VM that loads it reads
// in place (jsReadShared).
func allocShared(b []byte) (uintptr, error) {
	return hostmem.MapReadOnly(b)
}

// freeShared unmaps the n bytes of allocShared at p, which no VM reads any
// more.
func freeShared(p uintptr, n int) {
	if err := hostmem.UnmapReadOnly(p, n); err != nil {
		panic("slowjs: " + err.Error())
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
