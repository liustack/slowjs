// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build !slowjs_cengine

package slowjs // import "github.com/liustack/slowjs"

import (
	"fmt"
	"strings"
	"testing"
	"unsafe"

	lib "github.com/liustack/slowjs/lib"
	"modernc.org/libc"
)

// failAt is the allocation that the failing allocator fails, counting from
// 1 since allocs was last set to 0, or 0 to fail none.
var failAt, allocs int

// failing reports whether the failing allocator fails the allocation it is
// about to make.
func failing() bool {
	allocs++
	return allocs == failAt
}

func failingMalloc(tls *libc.TLS, s uintptr, size lib.Tsize_t) uintptr {
	if failing() {
		return 0
	}
	return libc.Xmalloc(tls, libc.Tsize_t(size))
}

func failingFree(tls *libc.TLS, s, p uintptr) {
	libc.Xfree(tls, p)
}

func failingRealloc(tls *libc.TLS, s, p uintptr, size lib.Tsize_t) uintptr {
	if size == 0 {
		libc.Xfree(tls, p)
		return 0
	}
	if failing() {
		return 0
	}
	return libc.Xrealloc(tls, p, libc.Tsize_t(size))
}

func failingUsableSize(tls *libc.TLS, p uintptr) lib.Tsize_t {
	return lib.Tsize_t(libc.Xmalloc_usable_size(tls, p))
}

// compileFailing compiles js, what, in a new runtime whose allocation number fail
// of the compile fails, 0 for none, and runs the code compiled, with no
// allocation failing. It returns the result of the code as a string, or
// failed, and the number of allocations of the compile. A compile may only
// fail with the out of memory error.
func compileFailing(t *testing.T, what, js string, fail int) (result string, failed bool, n int) {
	t.Helper()
	tls := libc.NewTLS()
	defer tls.Close()
	// The engine takes C pointers, so the functions are in C memory, not
	// in a Go variable: the pointer arithmetic of the transpiled code on Go
	// memory fails checkptr, which -race turns on.
	mf := tls.Alloc(int(unsafe.Sizeof(lib.TJSMallocFunctions{})))
	defer tls.Free(int(unsafe.Sizeof(lib.TJSMallocFunctions{})))
	*(*lib.TJSMallocFunctions)(unsafe.Pointer(mf)) = lib.TJSMallocFunctions{
		Fjs_malloc:             fp(failingMalloc),
		Fjs_free:               fp(failingFree),
		Fjs_realloc:            fp(failingRealloc),
		Fjs_malloc_usable_size: fp(failingUsableSize),
	}
	rt := lib.XJS_NewRuntime2(tls, mf, 0)
	ctx := lib.XJS_NewContext(tls, rt)
	if rt == 0 || ctx == 0 {
		t.Fatal("cannot make a runtime")
	}
	defer lib.XJS_FreeRuntime(tls, rt)
	defer lib.XJS_FreeContext(tls, ctx)
	src, err := libc.CString(js)
	if err != nil {
		t.Fatal(err)
	}
	defer libc.Xfree(tls, src)

	func() {
		defer func() {
			failAt = 0
			if p := recover(); p != nil {
				t.Fatalf("%s, allocation %d failing: panic: %v", what, fail, p)
			}
		}()
		allocs, failAt = 0, fail
		v := lib.XJS_Eval(tls, ctx, src, libc.Tsize_t(len(js)), uintptr(unsafe.Pointer(&evalFN)), EvalGlobal|evalFlagCompileOnly)
		n = allocs
		if isException(v) {
			failed = true
			if lib.XJS_HasException(tls, ctx) == 0 || lib.XJS_IsUncatchableException(tls, ctx) == 0 {
				t.Fatalf("%s, allocation %d failing: the compile fails without the out of memory error", what, fail)
			}
			lib.XFreeValue(tls, ctx, lib.XJS_GetException(tls, ctx))
			return
		}
		if lib.XJS_HasException(tls, ctx) != 0 {
			t.Fatalf("%s, allocation %d failing: the compile works, but leaves an exception", what, fail)
		}
		failAt = 0
		v = lib.XJS_EvalFunction(tls, ctx, v)
		if isException(v) {
			lib.XFreeValue(tls, ctx, lib.XJS_GetException(tls, ctx))
			result = "exception"
			return
		}
		s := lib.XJS_ToCStringLen2(tls, ctx, 0, v, 0)
		result = libc.GoString(s)
		lib.XJS_FreeCString(tls, ctx, s)
		lib.XFreeValue(tls, ctx, v)
	}()
	return result, failed, n
}

// Compiling with one allocation failing, and the others working, fails
// with the out of memory error or gives code that works as it should. A
// failing allocation followed by working ones is what a large allocation
// failing under the memory limit, as small ones still fit, looks like.
func TestCompileOneFailure(t *testing.T) {
	scripts := append(append([]string{}, compileScripts...), compileFineScripts...)
	scripts = append(scripts,
		`function f() { let n = 0; `+strings.Repeat(`{ let a = 1; n += a } `, 400)+`return n } f()`,
		strings.Repeat(`{ let a = 1; `, 300)+strings.Repeat(`} `, 300)+`1`,
		`class A { #m() { return 1 } `+strings.Repeat(`x = 1; `, 200)+`y() { return this.#m() } }; new A().y()`,
		`switch (2) { `+strings.Repeat(`case 1: break; `, 300)+`default: 3 }`,
		// Private methods and no fields: the code that adds the brand is
		// made and patched at the end of the class, with no token between.
		`class A { #m() { return 1 } y() { return this.#m() } }; new A().y()`,
	)
	for i, js := range scripts {
		what := fmt.Sprintf("script %d", i)
		want, failed, n := compileFailing(t, what, js, 0)
		if failed {
			t.Fatalf("script %d does not compile", i)
		}
		failures := 0
		for fail := 1; fail <= n; fail++ {
			got, failed, _ := compileFailing(t, what, js, fail)
			if failed {
				failures++
			} else if got != want {
				t.Fatalf("script %d, allocation %d failing: the code compiled gives %q, want %q", i, fail, got, want)
			}
		}
		if failures == 0 {
			t.Fatalf("script %d: no compile of its %d allocations failed", i, n)
		}
	}
}

// Once growing a buffer failed, it takes no more bytes, not even those that
// still fit in its memory, so that what it holds is a run of whole writes.
func TestBufferAfterFailure(t *testing.T) {
	tls := libc.NewTLS()
	defer tls.Close()
	p := tls.Alloc(int(unsafe.Sizeof(lib.TDynBuf{})))
	defer tls.Free(int(unsafe.Sizeof(lib.TDynBuf{})))
	b := (*lib.TDynBuf)(unsafe.Pointer(p))
	lib.Xdbuf_init2(tls, p, 0, fp(func(tls *libc.TLS, opaque, ptr uintptr, size lib.Tsize_t) uintptr {
		if failing() {
			return 0
		}
		return libc.Xrealloc(tls, ptr, libc.Tsize_t(size))
	}))
	defer lib.Xdbuf_free(tls, p)
	d := tls.Alloc(4096)
	defer tls.Free(4096)

	// Writes of 64, 64 and 1 bytes leave room for more in the buffer's
	// memory, as it grows by half.
	allocs, failAt = 0, 0
	defer func() { failAt = 0 }()
	for _, n := range []lib.Tsize_t{64, 64, 1} {
		if lib.Xdbuf_put(tls, p, d, n) != 0 {
			t.Fatal("a write fails")
		}
	}
	if b.Fallocated_size <= b.Fsize {
		t.Fatalf("the buffer holds %d bytes in %d, want room for more", b.Fsize, b.Fallocated_size)
	}
	failAt = allocs + 1
	if lib.Xdbuf_put(tls, p, d, 4096) == 0 {
		t.Fatal("a write past the memory of the buffer did not fail")
	}
	failAt = allocs + 1
	if lib.Xdbuf_put(tls, p, d, 1) == 0 || b.Fsize != 129 || b.Ferror1 == 0 {
		t.Fatalf("a write that fits after a failed one: size %d, error %d, want it to fail", b.Fsize, b.Ferror1)
	}
}
