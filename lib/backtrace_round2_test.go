// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package libquickjs

import (
	"fmt"
	"strings"
	"testing"
	"unsafe"

	"modernc.org/libc"
)

// Printing an error whose stack cannot be formatted for lack of memory
// must not leave an exception behind: a native read reports no stack and
// restores the exception state from before the read. This is round-2
// problem 3.
func TestBacktracePrintReadOOMLeavesNoException(t *testing.T) {
	for _, room := range []int64{0, 512, 8192} {
		t.Run(fmt.Sprint(room), func(t *testing.T) {
			v := newBtVM(t)
			ref := v.pendingErr()
			want := v.stack(ref)
			XFreeValue(v.tls, v.ctx, ref)

			e := v.pendingErr()
			defer XFreeValue(v.tls, v.ctx, e)
			v.limit(room)
			p := _get_prop_string(v.tls, v.ctx, e, EJS_ATOM_stack)
			hadException := XJS_HasException(v.tls, v.ctx)
			v.limit(-1)
			if p != 0 {
				defer XJS_FreeCString(v.tls, v.ctx, p)
			}
			// The read either gives the whole text or reports no stack,
			// but it never reports no stack while leaving an exception.
			if hadException != 0 {
				t.Fatalf("get_prop_string left an exception, returned %#x", p)
			}
			if p != 0 && libc.GoString(p) != want {
				t.Fatalf("get_prop_string gave %q, want %q", libc.GoString(p), want)
			}
			// Later, with memory back, the stack reads in full.
			if got := v.stack(e); got != want {
				t.Fatalf("read after the print gave %q, want %q", got, want)
			}
		})
	}
}

// A raw dump (JSPrintValueOptions.raw_dump) must not format a pending
// stack: it does no allocation and leaves the error pending, so it works
// with no memory to spare and a later read still gives the whole stack.
// This is round-2 problem 4.
func TestBacktraceRawDumpDoesNotFormat(t *testing.T) {
	v := newBtVM(t)
	ref := v.pendingErr()
	want := v.stack(ref)
	XFreeValue(v.tls, v.ctx, ref)

	e := v.pendingErr()
	defer XFreeValue(v.tls, v.ctx, e)

	opt := v.tls.Alloc(int(unsafe.Sizeof(TJSPrintValueOptions{})))
	defer v.tls.Free(int(unsafe.Sizeof(TJSPrintValueOptions{})))
	XJS_PrintValueSetDefaultOptions(v.tls, opt)
	libc.SetBitFieldPtr32Uint32(opt, 1, 8, 0xff00) // raw_dump = TRUE
	var sink strings.Builder
	write := func(tls *libc.TLS, opaque uintptr, buf uintptr, n Tsize_t) {
		sink.Write(unsafe.Slice((*byte)(unsafe.Pointer(buf)), int(n)))
	}
	v.limit(0)
	XJS_PrintValue(v.tls, v.ctx, __ccgo_fp(write), 0, e, opt)
	had := XJS_HasException(v.tls, v.ctx)
	v.limit(-1)
	if had != 0 {
		t.Fatalf("raw dump threw: %s", v.exception())
	}
	if got := v.stack(e); got != want {
		t.Fatalf("read after a raw dump gave %q, want %q", got, want)
	}
}

// Throwing an error whose stack has to be built when there is no memory
// must not crash: building the stack used to free the error, which is the
// current exception, and then read it again. The VM keeps working and the
// reference counts stay sound (checked under -race and memgrind). This is
// the regression for the pre-existing crash the lazy stacks avoid.
func TestBacktraceThrowStacklessOOM(t *testing.T) {
	for _, room := range []int64{0, 256, 1024, 4096} {
		t.Run(fmt.Sprint(room), func(t *testing.T) {
			v := newBtVM(t)
			XFreeValue(v.tls, v.ctx, v.eval(`
var limit
function make() {
	function stackless() { var e = new Error("x"); delete e.stack; limit(); return e }
	function thrower() { throw stackless() }
	Object.defineProperty(thrower, "name", { value: "名前".repeat(10000) })
	return thrower
}
`))
			// limit() leaves the runtime only room more bytes.
			tighten := func(tls *libc.TLS, ctx uintptr, this TJSValue, argc int32, argv uintptr) TJSValue {
				v.limit(room)
				return jsUndefined()
			}
			name, _ := libc.CString("limit")
			defer libc.Xfree(v.tls, name)
			g := XJS_GetGlobalObject(v.tls, v.ctx)
			XJS_SetPropertyStr(v.tls, v.ctx, g, name, XJS_NewCFunction2(v.tls, v.ctx, __ccgo_fp(tighten), name, 0, EJS_CFUNC_generic, 0))
			XFreeValue(v.tls, v.ctx, g)

			thrower := v.call("make")
			defer XFreeValue(v.tls, v.ctx, thrower)
			r := XJS_Call(v.tls, v.ctx, thrower, jsUndefined(), 0, 0)
			v.limit(-1)
			if tag(r) != EJS_TAG_EXCEPTION {
				XFreeValue(v.tls, v.ctx, r)
				t.Fatalf("call returned tag %d, want an exception", tag(r))
			}
			XFreeValue(v.tls, v.ctx, XJS_GetException(v.tls, v.ctx))
			// The VM still runs.
			if got := v.text(v.eval("6 * 7")); got != "42" {
				t.Fatalf("after the throw the VM gave %q", got)
			}
		})
	}
}
