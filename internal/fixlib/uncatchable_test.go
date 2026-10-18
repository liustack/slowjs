// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Every generated file keeps an uncatchable exception pending, and
// fixUncatchable leaves a fixed file as it is.
func TestUncatchableEveryFile(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("..", "..", "lib", "ccgo_*.go"))
	if err != nil || len(files) == 0 {
		t.Fatal(files, err)
	}
	for _, fn := range files {
		b, err := os.ReadFile(fn)
		if err != nil {
			t.Fatal(err)
		}
		out, err := fixUncatchable(b)
		if err != nil {
			t.Fatalf("%s: %v", fn, err)
		}
		if !bytes.Equal(out, b) {
			t.Fatalf("%s: fixUncatchable changed a fixed file", fn)
		}
		// JS_Throw, JS_CallInternal and JS_IteratorClose.
		if n := bytes.Count(out, []byte("_js_uncatchable_pending(tls, ")); n != 3 {
			t.Errorf("%s: %d calls of _js_uncatchable_pending, want 3", fn, n)
		}
		calls := 0
		for _, n := range exceptionHandlers {
			calls += n
		}
		if n := bytes.Count(out, []byte("_js_get_exception(tls, ")); n != calls {
			t.Errorf("%s: %d calls of _js_get_exception, want %d", fn, n, calls)
		}
	}
}

// A new caller of JS_GetException stops fixlib.
func TestUncatchableRejectsNewHandler(t *testing.T) {
	b := engineFile(t)
	changed := bytes.Replace(b, []byte("\nfunc _js_regexp_finalizer("), []byte("\nfunc _js_new_handler(tls *libc.TLS, ctx uintptr) {\n\tXFreeValue(tls, ctx, XJS_GetException(tls, ctx))\n}\n\nfunc _js_regexp_finalizer("), 1)
	_, err := fixUncatchable(changed)
	if err == nil || !strings.Contains(err.Error(), "_js_new_handler (1)") {
		t.Fatalf("got %v, want the new caller named", err)
	}
}

// A new function that takes the exception out of the runtime itself, or a
// JS_Throw that replaces the exception before the place of the guard, stops
// fixlib.
func TestUncatchableRejectsNewTake(t *testing.T) {
	b := engineFile(t)
	take := "\nfunc _js_new_take(tls *libc.TLS, ctx uintptr) {\n\t(*TJSRuntime)(unsafe.Pointer((*TJSContext)(unsafe.Pointer(ctx)).Frt)).Fcurrent_exception = TJSValue{}\n}\n\nfunc _js_regexp_finalizer("
	_, err := fixUncatchable(bytes.Replace(b, []byte("\nfunc _js_regexp_finalizer("), []byte(take), 1))
	if err == nil || !strings.Contains(err.Error(), "_js_new_take (1)") {
		t.Fatalf("got %v, want the new function named", err)
	}

	body, err := funcBody(b, reThrowHead, "XJS_Throw")
	if err != nil {
		t.Fatal(err)
	}
	late := "\nfunc XJS_Throw(tls *libc.TLS, ctx1 uintptr, obj TJSValue) (r TJSValue) {\n" +
		"\tvar rt uintptr\n" +
		"\t_ = rt\n" +
		"\tXFreeValue(tls, ctx1, (*TJSRuntime)(unsafe.Pointer((*TJSContext)(unsafe.Pointer(ctx1)).Frt)).Fcurrent_exception)\n" +
		"\t(*TJSRuntime)(unsafe.Pointer((*TJSContext)(unsafe.Pointer(ctx1)).Frt)).Fcurrent_exception = obj\n" +
		"\trt = (*TJSContext)(unsafe.Pointer(ctx1)).Frt\n" +
		"\tlibc.SetBitFieldPtr16Int32(rt+1256, libc.Int32FromInt32(MFALSE), 0, 0xff)\n" +
		"\treturn jsvException()\n}\n"
	if _, err := fixUncatchable(bytes.Replace(b, body, []byte(late), 1)); err == nil {
		t.Fatal("a JS_Throw that replaces the exception before the guard went through")
	}
}
