// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT AND BSD-3-Clause
// The fork's code here is under the MIT license in LICENSE-MIT at the root of
// this repository. Parts follow QuickJS (Copyright (c) 2017-2021 Fabrice
// Bellard and Charlie Gordon, MIT, see LICENSE-LIBQUICKJS) as translated by
// modernc.org/libquickjs (The libquickjs-go Authors, BSD-3-Clause, see
// lib/LICENSE).

package libquickjs // import "github.com/liustack/slowjs/lib"

import (
	"unsafe"

	"modernc.org/libc"
)

// ReadObjSharedDebug is the flag JS_READ_OBJ_SHARED_DEBUG of the C patch for
// JS_ReadObject: the source text and the line table of each function it
// reads point into the buffer it reads, which must stay unchanged until the
// runtime is freed, instead of being copied into the runtime. The runtime
// neither counts them in its memory nor frees them.
//
// internal/fixlib wires the functions below into the engine (see
// fixSharedDebug). The C patch keeps the flag in BCReaderState. Here it is in
// the runtime, set by every JS_ReadObject, as only JS_ReadObject reads
// functions and it runs no javascript.
const ReadObjSharedDebug = 1 << 4

// sharedDebugBit is the bit shared_debug that the C patch adds to
// JSFunctionBytecode after is_direct_or_indirect_eval: the fifth bit of the
// second byte of flags, F__ccgo18, which upstream leaves unused. It says that
// the source text and line table of the function point into a shared buffer.
const sharedDebugBit = 0x20

// readSharedDebug is called by JS_ReadObject with its flags.
func readSharedDebug(ctx uintptr, flags int32) {
	forkOf((*TJSContext)(unsafe.Pointer(ctx)).Frt).sharedDebug = flags&ReadObjSharedDebug != 0
}

// debugAlloc stands in JS_ReadFunctionTag for the js_mallocz that allocates
// the size bytes of the line table or source text of b, which debugGetBuf
// then fills from the reader s. Reading shared, it returns where the bytes
// are in the buffer instead and marks b, or throws the error of bc_get_buf
// and returns 0 when the buffer ends first.
func debugAlloc(tls *libc.TLS, s, b uintptr, size uint64) uintptr {
	r := (*TBCReaderState)(unsafe.Pointer(s))
	if !forkOf((*TJSContext)(unsafe.Pointer(r.Fctx)).Frt).sharedDebug {
		return Xjs_mallocz(tls, r.Fctx, size)
	}
	if size > uint64(r.Fbuf_end-r.Fptr) {
		_bc_read_error_end(tls, s)
		return 0
	}
	(*TJSFunctionBytecode)(unsafe.Pointer(b)).F__ccgo18 |= sharedDebugBit
	return r.Fptr
}

// debugGetBuf stands for the bc_get_buf that fills what debugAlloc returned.
// Reading shared, the bytes are there already: it only moves the reader past
// them.
func debugGetBuf(tls *libc.TLS, s, buf uintptr, n uint32) int32 {
	r := (*TBCReaderState)(unsafe.Pointer(s))
	if !forkOf((*TJSContext)(unsafe.Pointer(r.Fctx)).Frt).sharedDebug {
		return _bc_get_buf(tls, s, buf, n)
	}
	if buf != r.Fptr {
		panic("slowjs: a shared line table or source text is not where the reader is")
	}
	r.Fptr += uintptr(n)
	return 0
}

// debugFree stands for the js_free_rt with which free_function_bytecode
// frees the line table or source text p of b: it leaves a shared one to the
// buffer it points into.
func debugFree(tls *libc.TLS, rt, b, p uintptr) {
	if !debugShared(b) {
		Xjs_free_rt(tls, rt, p)
	}
}

// debugShared reports whether the line table and source text of b point into
// a shared buffer, outside the memory of the runtime.
func debugShared(b uintptr) bool {
	return (*TJSFunctionBytecode)(unsafe.Pointer(b)).F__ccgo18&sharedDebugBit != 0
}
