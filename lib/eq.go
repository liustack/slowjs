// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

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
