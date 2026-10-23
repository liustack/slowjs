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

//go:noescape
func goasmRun(tls *libc.TLS, env, locals uintptr)

type goasmArgs struct {
	TLS                 *libc.TLS
	Ctx                 uintptr
	Fn, This, NewTarget TJSValue
	Argc                int32
	Argv                uintptr
	Flags               int32
	Result              TJSValue
	Allocated           uintptr
}

func goasmCall(tls *libc.TLS, ctx uintptr, fn, this, newTarget TJSValue, argc int32, argv uintptr, flags int32) TJSValue {
	bp := tls.Alloc(goasmFrame)
	env := (*goasmArgs)(unsafe.Pointer(bp))
	*env = goasmArgs{TLS: tls, Ctx: ctx, Fn: fn, This: this, NewTarget: newTarget, Argc: argc, Argv: argv, Flags: flags}
	// One TLS slot matches the ccgo entry. Keep locals and the small alloca
	// within the same 2048-byte allocation, including under memgrind.
	goasmRun(tls, bp, bp+160)
	result := env.Result
	if env.Allocated != 0 {
		libc.Xfree(tls, env.Allocated)
	}
	tls.Free(goasmFrame)
	return result
}
func goasmAllocate(env, size uintptr) uintptr {
	if size <= 768 {
		return env + 160 + 528
	}
	tls := *(**libc.TLS)(unsafe.Pointer(env))
	p := libc.Xmalloc(tls, uint64(size))
	(*goasmArgs)(unsafe.Pointer(env)).Allocated = p
	return p
}
