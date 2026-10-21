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

// Every target has the shape cache wired in, with gc_phase right after
// tmp_obj_list.
func TestShapeCacheEveryFile(t *testing.T) {
	for _, fn := range libFiles(t) {
		b, err := os.ReadFile(fn)
		if err != nil {
			t.Fatal(err)
		}
		name := filepath.Base(fn)
		for _, text := range []string{shapeKeepHook, shapeOffHook, shapeCloneNew, shapeFoundNew} {
			if n := bytes.Count(b, []byte(text)); n != 1 {
				t.Errorf("%s: %d copies of %q", name, n, text)
			}
		}
		if err := checkGCPhase(b); err != nil {
			t.Errorf("%s: %v", name, err)
		}
		// JS_FreeRuntime lets go of the kept shapes before it turns the
		// front cache off, so they go to the arenas.
		if i, j := bytes.Index(b, []byte(shapeOffHook)), bytes.Index(b, []byte(frontOffHook)); i < 0 || j < i {
			t.Errorf("%s: JS_FreeRuntime turns the front cache off first", name)
		}
	}
}

// fixShapeCache leaves a fixed file as it is, and puts the change back into
// a file without it, on every target. restoreShapeCache gives back the text
// ccgo writes.
func TestShapeCacheIdempotent(t *testing.T) {
	for _, fn := range libFiles(t) {
		b, err := readEngine(fn)
		if err != nil {
			t.Fatal(err)
		}
		name := filepath.Base(fn)
		for _, s := range []string{"shapeKeep(", "shapeKeepable(", "shapeFound(", "shapeCacheOff("} {
			if bytes.Contains(b, []byte(s)) {
				t.Fatalf("%s: readEngine leaves a call of %s in", name, s)
			}
		}
		fixed, err := fixShapeCache(bytes.Clone(b), name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if bytes.Equal(fixed, b) {
			t.Fatalf("%s: fixShapeCache changed nothing", name)
		}
		again, err := fixShapeCache(bytes.Clone(fixed), name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !bytes.Equal(again, fixed) {
			t.Errorf("%s: a fixed file changed", name)
		}
		back, err := restoreShapeCache(fixed)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !bytes.Equal(back, b) {
			t.Errorf("%s: restoreShapeCache does not give the text back", name)
		}
	}
}

// A change to what lib/shapecache.go relies on, or a part of the change
// missing, stops fixShapeCache.
func TestShapeCacheRejects(t *testing.T) {
	b := string(engineFile(t))
	fixed, err := fixShapeCache([]byte(b), "ccgo_darwin_arm64.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name, in, old, new, err string
	}{
		{"another resize", b, "if int32(2)*((*TJSRuntime)(unsafe.Pointer(rt)).Fshape_hash_count+int32(1)) > (*TJSRuntime)(unsafe.Pointer(rt)).Fshape_hash_size {", "if int32(3)*((*TJSRuntime)(unsafe.Pointer(rt)).Fshape_hash_count+int32(1)) > (*TJSRuntime)(unsafe.Pointer(rt)).Fshape_hash_size {", "changed (fingerprint"},
		{"another shape", b, "\tFshape_hash_next    uintptr\n", "\tFshape_hash_next    uintptr\n\tFextra              uintptr\n", "changed (fingerprint"},
		{"another atom", b, "\t(*TJSAtomStruct)(unsafe.Pointer(p)).Fhash_next = i /* atom_index */\n", "\t(*TJSAtomStruct)(unsafe.Pointer(p)).Fhash_next = 0 /* atom_index */\n", "changed (fingerprint"},
		{"no keep", string(fixed), shapeKeepHook, "", "a part of the change"},
		{"no credit", string(fixed), shapeFoundNew, shapeFoundOld, "a part of the change"},
		{"a found call left", string(fixed), shapeFoundNew, "\t\t\tshapeFound(tls, ctx)\n" + shapeFoundOld, "a call of shapeFound is left"},
		{"no copy", string(fixed), shapeCloneNew, shapeCloneOld, "a part of the change"},
		{"a call left", string(fixed), shapeOffHook, "\tshapeCacheOff(tls, rt1)\n", "a call of shapeCacheOff is left"},
	} {
		if !strings.Contains(c.in, c.old) {
			t.Fatalf("%s: no %q", c.name, c.old)
		}
		_, err := fixShapeCache([]byte(strings.Replace(c.in, c.old, c.new, 1)), "ccgo_darwin_arm64.go")
		if err == nil || !strings.Contains(err.Error(), c.err) {
			t.Errorf("%s: %v, want %q", c.name, err, c.err)
		}
	}
	// The atom functions that write hash_next are pinned: shapeHasSymbol
	// tells a symbol by it.
	for _, c := range []struct{ fn, old, new string }{
		{"_JS_ResizeAtomHash", "(*TJSAtomStruct)(unsafe.Pointer(p)).Fhash_next = *(*Tuint32_t)(unsafe.Pointer(new_hash + uintptr(j)*4))", "(*TJSAtomStruct)(unsafe.Pointer(p)).Fhash_next = h"},
		{"_JS_FreeAtomStruct", "i = (*TJSAtomStruct)(unsafe.Pointer(p)).Fhash_next /* atom_index */", "i = h0 /* atom_index */"},
		{"_js_get_atom_index", "i = (*TJSAtomStruct)(unsafe.Pointer(p)).Fhash_next /* atom_index */", "i = 0 /* atom_index */"},
	} {
		body, err := funcBody([]byte(b), funcHead(c.fn), c.fn)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(body, []byte(c.old)) {
			t.Fatalf("%s: no %q", c.fn, c.old)
		}
		in := bytes.Replace([]byte(b), body, bytes.Replace(body, []byte(c.old), []byte(c.new), 1), 1)
		if _, err := fixShapeCache(in, "ccgo_darwin_arm64.go"); err == nil || !strings.Contains(err.Error(), "changed (fingerprint") {
			t.Errorf("%s changed: %v, want %q", c.fn, err, "changed (fingerprint")
		}
	}
	// gc_phase anywhere but right after tmp_obj_list stops the change,
	// whatever the fingerprint.
	moved := strings.Replace(b, "\tFtmp_obj_list                          Tlist_head\n", "\tFtmp_obj_list                          Tlist_head\n\tFgc_pad                                uint8\n", 1)
	if moved == b {
		t.Fatal("no tmp_obj_list")
	}
	if err := checkGCPhase([]byte(moved)); err == nil || !strings.Contains(err.Error(), "no byte right after tmp_obj_list") {
		t.Errorf("gc_phase moved: %v", err)
	}
	other := strings.Replace(b, "\tlibc.SetBitFieldPtr8Int32(rt+1184, int32(EJS_GC_PHASE_REMOVE_CYCLES), 0, 0xff)\n", "\tlibc.SetBitFieldPtr8Int32(rt+1185, int32(EJS_GC_PHASE_REMOVE_CYCLES), 0, 0xff)\n", 1)
	if other == b {
		t.Fatal("gc_free_cycles does not set gc_phase")
	}
	if err := checkGCPhase([]byte(other)); err == nil || !strings.Contains(err.Error(), "not the byte right after") {
		t.Errorf("gc_phase elsewhere: %v", err)
	}
}
