//go:build goasm && linux && amd64

// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
package libquickjs

import (
	"modernc.org/libc"
	"unsafe"
)

const goasmEnabled = true
const goasmFrame = 2048

// goasmLocals is where the C locals of the interpreter start in its TLS
// slot: after goasmArgs and 48 bytes for the arguments of varargs calls.
// The small alloca follows the 528 bytes of locals.
const goasmLocals = 192

//go:noescape
func goasmRun(tls *libc.TLS, env, locals uintptr)

// goasmArgs is goasm_args of the generated C. TLS is a uintptr: the block
// lives in TLS memory, so storing a Go pointer there needs no write barrier,
// and the caller's tls keeps the TLS alive for the whole call.
type goasmArgs struct {
	TLS                 uintptr
	Ctx                 uintptr
	Fn, This, NewTarget TJSValue
	Argc                int32
	Argv                uintptr
	Flags               int32
	Result              TJSValue
	Allocated           uintptr
	// Where tlsTake and tlsDrop read and write libc's TLS, so that the
	// interpreter calls itself without Go when a slot is ready.
	TLSSP, TLSStack, TLSSignals uintptr
}

func goasmCall(tls *libc.TLS, ctx uintptr, fn, this, newTarget TJSValue, argc int32, argv uintptr, flags int32) TJSValue {
	// tlsTake and tlsDrop (fastframes_linux.go) are TLS.Alloc and TLS.Free
	// without a call, with the same slots.
	bp := tlsTake(tls, goasmFrame)
	if bp == 0 {
		bp = tls.Alloc(goasmFrame)
	}
	env := (*goasmArgs)(unsafe.Pointer(bp))
	// Field by field: a composite literal is built on the Go stack and
	// copied, through wbMove while the GC runs. Result is always set.
	env.TLS = uintptr(unsafe.Pointer(tls))
	env.Ctx = ctx
	env.Fn = fn
	env.This = this
	env.NewTarget = newTarget
	env.Argc = argc
	env.Argv = argv
	env.Flags = flags
	env.Allocated = 0
	t := uintptr(unsafe.Pointer(tls))
	env.TLSSP = t + tlsSP
	env.TLSStack = t + tlsStack
	env.TLSSignals = t + tlsSignalsOn
	// One TLS slot matches the ccgo entry. Keep locals and the small alloca
	// within the same 2048-byte allocation, including under memgrind.
	goasmRun(tls, bp, bp+goasmLocals)
	result := env.Result
	if env.Allocated != 0 {
		libc.Xfree(tls, env.Allocated)
	}
	if tlsDrop(tls, goasmFrame) {
		tlsSignals(tls)
	}
	return result
}
func goasmAllocate(env, size uintptr) uintptr {
	if size <= 768 {
		return env + goasmLocals + 528
	}
	tls := *(**libc.TLS)(unsafe.Pointer(env))
	p := libc.Xmalloc(tls, uint64(size))
	(*goasmArgs)(unsafe.Pointer(env)).Allocated = p
	return p
}

// _goasm_leave ends a call the interpreter made to itself, after its frame
// returned, when the call allocated a large alloca or libc checks signals
// on a free: what goasmCall does after goasmRun.
func _goasm_leave(tls *libc.TLS, env uintptr) {
	e := (*goasmArgs)(unsafe.Pointer(env))
	if e.Allocated != 0 {
		libc.Xfree(tls, e.Allocated)
	}
	if tlsDrop(tls, goasmFrame) {
		tlsSignals(tls)
	}
}
