// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package slowjs // import "github.com/liustack/slowjs"

// Memory on the engine of lib: what a VM holds, and the shared bytecode
// outside every VM.

import (
	"unsafe"

	lib "github.com/liustack/slowjs/lib"
	"modernc.org/libc"
)

// allocShared returns n bytes of C memory for a SharedBytecode, at least one,
// or 0 when there is no memory.
func allocShared(n int) uintptr {
	tls := libc.NewTLS()
	defer tls.Close()
	return libc.Xmalloc(tls, libc.Tsize_t(max(n, 1)))
}

// freeShared frees the memory of allocShared.
func freeShared(p uintptr) {
	tls := libc.NewTLS()
	libc.Xfree(tls, p)
	tls.Close()
}

// memoryUsage returns what JS_ComputeMemoryUsage counts in the runtime of m.
func (m *VM) memoryUsage() MemoryUsage {
	tls := m.runtime.tls
	n := int(unsafe.Sizeof(lib.TJSMemoryUsage{}))
	// Freed before it returns, not in a defer (see tlsMark).
	p := tls.Alloc(n)
	lib.XJS_ComputeMemoryUsage(tls, m.runtime.cRuntime, p)
	s := *(*lib.TJSMemoryUsage)(unsafe.Pointer(p))
	tls.Free(n)
	return MemoryUsage{
		MallocSize:         s.Fmalloc_size,
		MallocLimit:        s.Fmalloc_limit,
		MallocCount:        s.Fmalloc_count,
		MemoryUsedSize:     s.Fmemory_used_size,
		MemoryUsedCount:    s.Fmemory_used_count,
		AtomCount:          s.Fatom_count,
		AtomSize:           s.Fatom_size,
		StrCount:           s.Fstr_count,
		StrSize:            s.Fstr_size,
		ObjCount:           s.Fobj_count,
		ObjSize:            s.Fobj_size,
		PropCount:          s.Fprop_count,
		PropSize:           s.Fprop_size,
		ShapeCount:         s.Fshape_count,
		ShapeSize:          s.Fshape_size,
		JSFuncCount:        s.Fjs_func_count,
		JSFuncSize:         s.Fjs_func_size,
		JSFuncCodeSize:     s.Fjs_func_code_size,
		JSFuncPC2LineCount: s.Fjs_func_pc2line_count,
		JSFuncPC2LineSize:  s.Fjs_func_pc2line_size,
		CFuncCount:         s.Fc_func_count,
		ArrayCount:         s.Farray_count,
		FastArrayCount:     s.Ffast_array_count,
		FastArrayElements:  s.Ffast_array_elements,
		BinaryObjectCount:  s.Fbinary_object_count,
		BinaryObjectSize:   s.Fbinary_object_size,
	}
}

// releaseMemory gives nothing back: the memory of lib is libc's.
func (m *VM) releaseMemory(keep uintptr) uintptr {
	return 0
}
