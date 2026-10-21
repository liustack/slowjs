// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Every target calls funcShape first in js_function_set_properties, marks
// the shapes it holds with the shape of arrays and lets them go before it.
func TestFuncShapeEveryFile(t *testing.T) {
	for _, fn := range libFiles(t) {
		b, err := os.ReadFile(fn)
		if err != nil {
			t.Fatal(err)
		}
		name := filepath.Base(fn)
		for _, c := range []struct{ fn, text string }{
			{"_js_function_set_properties", funcShapeHook},
			{"_JS_MarkContext", funcShapeMarkAt + funcShapeMarkHook},
			{"XJS_FreeContext", funcShapeFreeHook + funcShapeFreeAt},
		} {
			body, err := funcBody(b, funcHead(c.fn), c.fn)
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			if n := bytes.Count(body, []byte(c.text)); n != 1 {
				t.Errorf("%s: %s has %d copies of %q", name, c.fn, n, c.text)
			}
		}
		if n := bytes.Count(b, []byte("funcShape")); n != 3 {
			t.Errorf("%s: %d calls of funcShape, funcShapesMark and funcShapesFree, want 3", name, n)
		}
	}
}

// fixFuncShape leaves a fixed file as it is, and puts the change back into
// a file without it, on every target. restoreFuncShape gives back the text
// without it.
func TestFuncShapeIdempotent(t *testing.T) {
	for _, fn := range libFiles(t) {
		b, err := readEngine(fn)
		if err != nil {
			t.Fatal(err)
		}
		name := filepath.Base(fn)
		if bytes.Contains(b, []byte("funcShape")) {
			t.Fatalf("%s: readEngine leaves a call of funcShape in", name)
		}
		fixed, err := fixFuncShape(bytes.Clone(b), name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if bytes.Equal(fixed, b) {
			t.Fatalf("%s: fixFuncShape changed nothing", name)
		}
		again, err := fixFuncShape(bytes.Clone(fixed), name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !bytes.Equal(again, fixed) {
			t.Errorf("%s: a fixed file changed", name)
		}
		back, err := restoreFuncShape(fixed)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !bytes.Equal(back, b) {
			t.Errorf("%s: restoreFuncShape does not give the text back", name)
		}
	}
}

// A change to what lib/funcshape.go repeats or relies on, or a part of the
// change missing, stops fixFuncShape.
func TestFuncShapeRejects(t *testing.T) {
	b := string(engineFile(t))
	fixed, err := fixFuncShape([]byte(b), "ccgo_darwin_arm64.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name, in, old, new, err string
	}{
		{"another length", b, "XJS_DefinePropertyValue(tls, ctx1, func_obj, uint32(EJS_ATOM_length), v1, libc.Int32FromInt32(1)<<libc.Int32FromInt32(0))", "XJS_DefinePropertyValue(tls, ctx1, func_obj, uint32(EJS_ATOM_length), v1, libc.Int32FromInt32(1)<<libc.Int32FromInt32(1))", "changed (fingerprint"},
		{"another shape", b, "\tFshape_hash_next    uintptr\n", "\tFshape_hash_next    uintptr\n\tFextra              uintptr\n", "changed (fingerprint"},
		{"no mark of the shape of arrays", b, funcShapeMarkAt, "", "changed (fingerprint"},
		{"no hook", string(fixed), funcShapeHook, "", "a part of the change"},
		{"no mark", string(fixed), funcShapeMarkHook, "", "a part of the change"},
		{"no free", string(fixed), funcShapeFreeHook, "", "a part of the change"},
		{"a call left", string(fixed), funcShapeFreeHook, "\tfuncShapesFree(tls, ctx1)\n", "a call of funcShapesFree is left"},
		{"two hooks", string(fixed), funcShapeHook, funcShapeHook + funcShapeHook, "2 copies"},
	} {
		if !strings.Contains(c.in, c.old) {
			t.Fatalf("%s: no %q", c.name, c.old)
		}
		_, err := fixFuncShape([]byte(strings.Replace(c.in, c.old, c.new, 1)), "ccgo_darwin_arm64.go")
		if err == nil || !strings.Contains(err.Error(), c.err) {
			t.Errorf("%s: %v, want %q", c.name, err, c.err)
		}
	}
	// Each function funcShape repeats or relies on is pinned.
	for _, c := range []struct{ fn, old, new string }{
		{"_add_property", "if (*TJSShape)(unsafe.Pointer(new_sh)).Fprop_size != (*TJSShape)(unsafe.Pointer(sh)).Fprop_size {", "if (*TJSShape)(unsafe.Pointer(new_sh)).Fprop_size > (*TJSShape)(unsafe.Pointer(sh)).Fprop_size {"},
		{"_js_shape_prepare_update", "Fref_count != int32(1) {", "Fref_count != int32(2) {"},
		{"XJS_FreeContext", funcShapeFreeAt, ""},
	} {
		body, err := funcBody([]byte(b), funcHead(c.fn), c.fn)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(body, []byte(c.old)) {
			t.Fatalf("%s: no %q", c.fn, c.old)
		}
		in := bytes.Replace([]byte(b), body, bytes.Replace(body, []byte(c.old), []byte(c.new), 1), 1)
		if _, err := fixFuncShape(in, "ccgo_darwin_arm64.go"); err == nil || !strings.Contains(err.Error(), "changed (fingerprint") {
			t.Errorf("%s changed: %v, want %q", c.fn, err, "changed (fingerprint")
		}
	}
	// With the fingerprint of the change, a missing place to put it still
	// stops it.
	for _, c := range []struct{ name, at, err string }{
		{"mark", funcShapeMarkAt, "_JS_MarkContext: 0 places"},
		{"free", funcShapeFreeAt, "XJS_FreeContext: 0 places"},
	} {
		in := []byte(strings.Replace(b, c.at, strings.Replace(c.at, "Farray_shape", "Farray_shape ", -1), 1))
		*fnShape = true
		_, err := fixFuncShape(in, "ccgo_darwin_arm64.go")
		*fnShape = false
		if err == nil || !strings.Contains(err.Error(), c.err) {
			t.Errorf("%s moved: %v, want %q", c.name, err, c.err)
		}
	}
}
