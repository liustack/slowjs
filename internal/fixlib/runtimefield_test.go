// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

import (
	"bytes"
	"go/format"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Every target has the field at the end of JSRuntime, allocated and cleared
// with it, and the struct is formatted as gofmt formats it.
func TestRuntimeFieldEveryFile(t *testing.T) {
	for _, fn := range libFiles(t) {
		b, err := os.ReadFile(fn)
		if err != nil {
			t.Fatal(err)
		}
		name := filepath.Base(fn)
		st, err := structBody(b, reRuntimeStruct, "TJSRuntime")
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.HasSuffix(st, []byte(forkField+"}\n")) {
			t.Errorf("%s: Ffork is not the last field of TJSRuntime", name)
		}
		if n := bytes.Count(b, []byte(forkSize)); n != 2 {
			t.Errorf("%s: %d sizes of the runtime with Ffork, want 2", name, n)
		}
		src := append([]byte("package p\n"), st...)
		if out, err := format.Source(src); err != nil || !bytes.Equal(out, src) {
			t.Errorf("%s: TJSRuntime is not as gofmt writes it (%v)", name, err)
		}
	}
}

// fixRuntimeField leaves a fixed file as it is, and puts the change back
// into a file without it, on every target. restoreRuntimeField gives back
// the text ccgo writes.
func TestRuntimeFieldIdempotent(t *testing.T) {
	for _, fn := range libFiles(t) {
		b, err := readEngine(fn)
		if err != nil {
			t.Fatal(err)
		}
		name := filepath.Base(fn)
		if bytes.Contains(b, []byte("Ffork")) || bytes.Contains(b, []byte("forkRuntimeSize")) {
			t.Fatalf("%s: readEngine leaves a part of the field in", name)
		}
		fixed, err := fixRuntimeField(bytes.Clone(b), name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		again, err := fixRuntimeField(bytes.Clone(fixed), name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !bytes.Equal(again, fixed) {
			t.Errorf("%s: a fixed file changed", name)
		}
		back, err := restoreRuntimeField(fixed)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !bytes.Equal(back, b) {
			t.Errorf("%s: restoreRuntimeField does not give the text back", name)
		}
	}
}

// A change to JSRuntime or to how JS_NewRuntime2 allocates it, or a part
// of the change missing, stops fixRuntimeField.
func TestRuntimeFieldRejects(t *testing.T) {
	b := string(engineFile(t))
	fixed, err := fixRuntimeField([]byte(b), "ccgo_darwin_arm64.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name, in, old, new, err string
	}{
		{"another size", b, "uint64(1440))\n", "uint64(1448))\n", "changed (fingerprint"},
		{"another first field", b, "\tFmalloc_ctx                            TJSMallocContext\n", "\tFrt_info0                              uintptr\n\tFmalloc_ctx                            TJSMallocContext\n", "changed (fingerprint"},
		{"no field", string(fixed), forkField, "", "want 1 and 2"},
		{"one size", string(fixed), forkSize, ")", "want 1 and 2"},
	} {
		if !strings.Contains(c.in, c.old) {
			t.Fatalf("%s: no %q", c.name, c.old)
		}
		_, err := fixRuntimeField([]byte(strings.Replace(c.in, c.old, c.new, 1)), "ccgo_darwin_arm64.go")
		if err == nil || !strings.Contains(err.Error(), c.err) {
			t.Errorf("%s: %v, want %q", c.name, err, c.err)
		}
	}
	// Without the fingerprint check, a size that the allocation and the
	// memset do not agree on, or a first field other than malloc_ctx, stops
	// the change itself.
	for _, c := range []struct{ name, old, new, err string }{
		{"two sizes", "uint64(1440))\n", "uint64(1448))\n", "allocates"},
		{"first field", "\tFmalloc_ctx                            TJSMallocContext\n", "\tFrt_info0                              uintptr\n\tFmalloc_ctx                            TJSMallocContext\n", "not the first field"},
	} {
		_, err := addRuntimeField([]byte(strings.Replace(b, c.old, c.new, 1)))
		if err == nil || !strings.Contains(err.Error(), c.err) {
			t.Errorf("%s: %v, want %q", c.name, err, c.err)
		}
	}
}
