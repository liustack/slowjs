// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build slowjs_cengine && linux && amd64

package slowjs // import "github.com/liustack/slowjs"

// Helpers of the tests that read the C engine, with the names and the
// results of the ones of lib (enginehelp_lib_test.go).

import (
	"testing"

	"github.com/liustack/slowjs/internal/cengine"
)

// engineMemoryUsage holds the fields of JSMemoryUsage the tests read, under
// the names of lib.TJSMemoryUsage.
type engineMemoryUsage struct {
	Fmalloc_size, Fatom_count, Fshape_count, Fobj_count      int64
	Fjs_func_size, Fjs_func_code_size, Fjs_func_pc2line_size int64
}

// deepRecursion is how deep a recursion that ends goes in a test of the
// default limits: below the default call depth of the C engine, 5000 calls,
// where the engine of lib goes to 10000.
const deepRecursion = 4900

// vmMemory runs the GC and returns the memory usage of the runtime of m.
func vmMemory(m *VM) engineMemoryUsage {
	m.RunGC()
	u := m.MemoryUsage()
	return engineMemoryUsage{Fmalloc_size: u.MallocSize, Fatom_count: u.AtomCount, Fshape_count: u.ShapeCount,
		Fobj_count: u.ObjCount, Fjs_func_size: u.JSFuncSize, Fjs_func_code_size: u.JSFuncCodeSize,
		Fjs_func_pc2line_size: u.JSFuncPC2LineSize}
}

// wantNoPendingException fails the test if an exception is still pending in
// m's context.
func wantNoPendingException(t *testing.T, m *VM) {
	t.Helper()
	var pending bool
	m.outer(func() { pending = int32(cengine.JS_HasException(m.sp, m.ctx)) != 0 })
	if pending {
		t.Fatal("an exception is still pending in the context")
	}
}

// engineFramesLeft reports whether the engine still has a stack frame of
// javascript, or the shadow stack anything on it: none is left outside a
// call.
func engineFramesLeft(m *VM) bool {
	var depth uintptr
	m.outer(func() { depth = cengine.JS_GetCallDepth(m.sp, m.rt) })
	return int32(depth) != 0 || m.sp != m.start
}

// exhaustMemory leaves m no memory: it lowers the limit to what m uses and
// takes every free block the engine's arenas have left, which freeBlocks
// gives back. It runs inside a call into the engine (a Go function called
// from javascript).
func exhaustMemory(m *VM) (blocks []uintptr) {
	m.jsSetMemoryLimit(uintptr(m.memoryUsage().MallocSize))
	for size := uintptr(4096); size >= 8; size -= 8 {
		for {
			p := cengine.Malloc(m.sp, m.rt, size)
			if p == 0 {
				break
			}
			blocks = append(blocks, p)
		}
	}
	return blocks
}

// freeBlocks frees the blocks of exhaustMemory.
func freeBlocks(m *VM, blocks []uintptr) {
	for _, p := range blocks {
		m.outer(func() { cengine.FreeRT(m.sp, m.rt, p) })
	}
}

// heldFuncs returns how many Go functions of RegisterFunc and Then the host
// of m holds for it, until the engine releases them or m closes.
func heldFuncs(m *VM) int {
	return m.h.Handles()
}
