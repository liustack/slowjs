// Copyright 2026 The libquickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package libquickjs // import "github.com/liustack/quickjs/lib"

import (
	"strings"
	"testing"
	"unsafe"

	"modernc.org/libc"
)

// JS_ToCStringLen2 takes a reference to its argument. When the memory limit
// makes the UTF-8 copy fail, that reference must still be released, otherwise
// the string outlives JS_FreeRuntime. ASCII strings never allocate a copy, so
// only strings with non-ASCII characters reach this path: 8-bit strings with
// characters >= 0x80 and wide (16-bit) strings take different branches.
func TestToCStringOOMReleasesArgument(t *testing.T) {
	for _, tc := range []struct{ name, s string }{
		{"latin1", strings.Repeat("é", 256)},
		{"wide", strings.Repeat("中", 256)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tls := libc.NewTLS()
			defer tls.Close()

			rt := XJS_NewRuntime(tls)
			defer XJS_FreeRuntime(tls, rt)

			ctx := XJS_NewContext(tls, rt)
			defer XJS_FreeContext(tls, ctx)

			usage := func() TJSMemoryUsage {
				// The engine writes the result, so it goes in C memory.
				n := int(unsafe.Sizeof(TJSMemoryUsage{}))
				p := tls.Alloc(n)
				defer tls.Free(n)
				XJS_ComputeMemoryUsage(tls, rt, p)
				return *(*TJSMemoryUsage)(unsafe.Pointer(p))
			}

			cs, err := libc.CString(tc.s)
			if err != nil {
				t.Fatal(err)
			}
			defer libc.Xfree(tls, cs)

			before := usage().Fmalloc_count
			str := XJS_NewStringLen(tls, ctx, cs, Size_t(len(tc.s)))
			if tag(str) != EJS_TAG_STRING {
				t.Fatalf("JS_NewStringLen: tag %d", tag(str))
			}

			// No room for another allocation: the UTF-8 copy must fail.
			XJS_SetMemoryLimit(tls, rt, Size_t(usage().Fmalloc_size))
			if p := XJS_ToCStringLen2(tls, ctx, 0, str, 0); p != 0 {
				XJS_FreeCString(tls, ctx, p)
				t.Fatal("JS_ToCStringLen2 succeeded, want an out of memory failure")
			}
			XJS_SetMemoryLimit(tls, rt, Size_t(^uintptr(0)))
			XFreeValue(tls, ctx, XJS_GetException(tls, ctx))

			XFreeValue(tls, ctx, str)
			if after := usage().Fmalloc_count; after != before {
				t.Fatalf("string not freed after a failed JS_ToCStringLen2: %d allocations before, %d after", before, after)
			}
		})
	}
}
