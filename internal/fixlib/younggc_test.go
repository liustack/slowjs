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

// fixYoungGC is its own exact inverse's inverse on every target, and stops on
// a changed collector, a partial or edited change, and a new function that
// uses gc_obj_list.
func TestYoungGCRewrite(t *testing.T) {
	for _, fn := range libFiles(t) {
		b, err := os.ReadFile(fn)
		if err != nil {
			t.Fatal(err)
		}
		name := filepath.Base(fn)
		bare, err := restoreYoungGC(b, name)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(bare, []byte("young")) {
			t.Fatalf("%s: restoreYoungGC left a part of the change", name)
		}
		fixed, err := fixYoungGC(bare, name)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(fixed, b) {
			t.Fatalf("%s: fixYoungGC does not give the committed file", name)
		}
		if strings.Count(string(fixed), "youngDefer(tls, ") != 2 || strings.Count(string(fixed), "youngSplice(rt)\n") != len(youngSpliced) {
			t.Fatalf("%s: not every hook is in", name)
		}
		again, err := fixYoungGC(fixed, name)
		if err != nil || !bytes.Equal(fixed, again) {
			t.Fatalf("%s not idempotent: %v", name, err)
		}
		back, err := restoreYoungGC(fixed, name)
		if err != nil || !bytes.Equal(back, bare) {
			t.Fatalf("%s inverse: %v", name, err)
		}
		changed := bytes.Replace(bare, []byte("_gc_free_cycles(tls, rt)\n}"), []byte("_gc_free_cycles(tls, rt)\n\t_gc_free_cycles(tls, rt)\n}"), 1)
		if bytes.Equal(changed, bare) {
			t.Fatalf("%s: the collector change of the test does not apply", name)
		}
		if _, err := fixYoungGC(changed, name); err == nil {
			t.Fatal("accepted a changed collector")
		}
		partial := bytes.Replace(fixed, []byte(youngInitHook), nil, 1)
		if _, err := fixYoungGC(partial, name); err == nil {
			t.Fatal("accepted a partial rewrite")
		}
		edited := bytes.Replace(fixed, []byte("youngDefer(tls, rt, s)"), []byte("youngDefer(tls, rt, 0)"), 1)
		if _, err := fixYoungGC(edited, name); err == nil {
			t.Fatal("accepted an edited hook")
		}
		off, err := youngOldList(bare)
		if err != nil {
			t.Fatal(err)
		}
		reader := append(bytes.Clone(bare), "\nfunc _js_new_reader(tls *libc.TLS, rt uintptr) uintptr {\n\treturn (*Tlist_head)(unsafe.Pointer(rt + "+off+")).Fnext\n}\n"...)
		if _, err := fixYoungGC(reader, name); err == nil || !strings.Contains(err.Error(), "_js_new_reader") {
			t.Fatalf("accepted a new function that uses gc_obj_list: %v", err)
		}
	}
}
