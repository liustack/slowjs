// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !(386 || arm)

package libquickjs // import "github.com/liustack/quickjs/lib"

import (
	"unsafe"

	"modernc.org/libc"
)

// eqFree frees sp[-2] when freeA says so and sp[-1] when freeB does, after
// the interpreter compared them and dropped the last reference to them
// (internal/fixlib writes the comparisons out, see fixEq). It is
// __JS_FreeValue, after the decrement JS_FreeValue does, for the two
// operands. Reading them from the stack, rather than taking them as
// arguments, keeps them out of the frame of the interpreter.
func eqFree(tls *libc.TLS, ctx, sp uintptr, freeA, freeB bool) {
	if freeA {
		x__JS_FreeValue(tls, ctx, *(*TJSValue)(unsafe.Pointer(sp - 2*unsafe.Sizeof(TJSValue{}))))
	}
	if freeB {
		x__JS_FreeValue(tls, ctx, *(*TJSValue)(unsafe.Pointer(sp - unsafe.Sizeof(TJSValue{}))))
	}
}
