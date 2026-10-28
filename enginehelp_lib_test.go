// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build !slowjs_cengine

package slowjs // import "github.com/liustack/slowjs"

// Helpers of the tests that read the engine of lib, with the same names on
// the C engine (enginehelp_c_test.go).

import (
	"testing"
	"unsafe"

	lib "github.com/liustack/slowjs/lib"
)

// cptr turns an address in C memory, which libc allocates outside the Go heap
// and never moves, into a pointer. go vet flags the direct conversion of a
// uintptr, which is right for an address in Go memory and wrong for these.
func cptr(a uintptr) unsafe.Pointer { return *(*unsafe.Pointer)(unsafe.Pointer(&a)) }

// deepRecursion is how deep a recursion that ends goes in a test of the
// default limits.
const deepRecursion = 5000

// vmMemory runs the GC and returns the memory usage of the runtime of m.
func vmMemory(m *VM) lib.TJSMemoryUsage {
	tls, rt := m.runtime.tls, m.runtime.cRuntime
	lib.XJS_RunGC(tls, rt)
	// The engine writes the result, so it goes in C memory.
	n := int(unsafe.Sizeof(lib.TJSMemoryUsage{}))
	p := tls.Alloc(n)
	defer tls.Free(n)
	lib.XJS_ComputeMemoryUsage(tls, rt, p)
	return *(*lib.TJSMemoryUsage)(cptr(p))
}

// wantNoPendingException fails the test if an exception is still pending in
// m's context. A failed call must take its exception out of the context: one
// left there is what the next caller of JS_GetException finds, and it keeps
// the thrown value alive.
func wantNoPendingException(t *testing.T, m *VM) {
	t.Helper()
	if lib.XJS_HasException(m.runtime.tls, m.cContext) != 0 {
		t.Fatal("an exception is still pending in the context")
	}
}

// engineFramesLeft reports whether the engine still has a stack frame of
// javascript: none is left outside a call.
func engineFramesLeft(m *VM) bool {
	return (*lib.TJSRuntime)(cptr(m.runtime.cRuntime)).Fcurrent_stack_frame != 0
}

// exhaustMemory leaves m no memory: it lowers the limit to what m uses and
// takes every free block the engine's arenas have left, which freeBlocks
// gives back.
func exhaustMemory(m *VM) (blocks []uintptr) {
	tls, rt := m.runtime.tls, m.runtime.cRuntime
	lib.XJS_SetMemoryLimit(tls, rt, (*lib.TJSRuntime)(cptr(rt)).Fmalloc_ctx.Fmalloc_state.Fmalloc_size)
	for size := lib.Tsize_t(4096); size >= 8; size -= 8 {
		for {
			p := lib.Xjs_malloc_rt(tls, rt, size)
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
		lib.Xjs_free_rt(m.runtime.tls, m.runtime.cRuntime, p)
	}
}

// heldFuncs returns how many Go functions of RegisterFunc and Then the engine
// of lib holds, for every VM of the process: it keeps them in one table.
func heldFuncs(*VM) int {
	goFuncsMu.Lock()
	defer goFuncsMu.Unlock()
	return len(goFuncs)
}

// jsonTooDeep is the error of JSON.parse and JSON.stringify of data nested too
// deep: the engine of lib runs out of stack, as it does for a recursion.
const jsonTooDeep = stackOverflow
