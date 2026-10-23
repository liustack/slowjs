// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

// fixYoungGC is its own exact inverse's inverse on every target, and stops on
// a changed collector, a partial or edited change, and a new function that
// uses gc_obj_list.
func TestYoungGCRewrite(t *testing.T) {
	for _, fn := range libFiles(t) {
		b, err := readAfter(fn, "fixYoungGC")
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
		for _, arg := range []string{"rt", "rt2", "runtime"} {
			t.Run(name+"/"+arg, func(t *testing.T) {
				reader := append(bytes.Clone(bare), "\nfunc _js_new_reader(tls *libc.TLS, "+arg+" uintptr) uintptr {\n\treturn (*Tlist_head)(unsafe.Pointer("+arg+" + "+off+")).Fnext\n}\n"...)
				if _, err := fixYoungGC(reader, name); err == nil || !strings.Contains(err.Error(), "_js_new_reader") {
					t.Fatalf("accepted a new function that uses gc_obj_list: %v", err)
				}
			})
		}
	}
}

// Numeric offsets alone do not identify a runtime. TLS slots and pointers
// whose declared struct has no gc_obj_list are unrelated, even with the
// same offset. Unknown uintptrs and mixed origins must still stop the audit.
func TestYoungReadersOrigins(t *testing.T) {
	for _, tc := range []struct {
		name, params, body string
		want               bool
	}{
		{"alias", "runtime uintptr", `alias := runtime; return (*Tlist_head)(unsafe.Pointer(alias + 1136)).Fnext`, true},
		{"compact", "runtime uintptr", `return (*Tlist_head)(unsafe.Pointer(runtime+1136)).Fnext`, true},
		{"reverse", "runtime uintptr", `return (*Tlist_head)(unsafe.Pointer(1136 + runtime)).Fnext`, true},
		{"unsafe_add", "runtime unsafe.Pointer", `return (*Tlist_head)(unsafe.Add(runtime, 1136)).Fnext`, true},
		{"field", "runtime *TJSRuntime", `return runtime.Fgc_obj_list.Fnext`, true},
		{"typed_runtime", "runtime *TJSRuntime", `return (*Tlist_head)(unsafe.Pointer(uintptr(unsafe.Pointer(runtime)) + 1136)).Fnext`, true},
		{"global_table", "", `p := unsafe.Pointer(&table); return (*Tlist_head)(unsafe.Add(p, 1136)).Fnext`, false},
		{"shadowed_table", "table uintptr", `p := uintptr(unsafe.Pointer(&table)); return (*Tlist_head)(unsafe.Pointer(p + 1136)).Fnext`, true},
		{"other_struct", "object *TOther", `return (*Tlist_head)(unsafe.Pointer(uintptr(unsafe.Pointer(object)) + 1136)).Fnext`, false},
		{"other_alias", "object *TOther", `p := uintptr(unsafe.Pointer(object)); alias := p; return (*Tlist_head)(unsafe.Pointer(alias + 1136)).Fnext`, false},
		{"other_cast_is_not_proof", "object uintptr", `_ = (*TOther)(unsafe.Pointer(object)); return (*Tlist_head)(unsafe.Pointer(object + 1136)).Fnext`, true},
		{"stack", "tls *libc.TLS", `stack := tls.Alloc(2048); return (*Tlist_head)(unsafe.Pointer(stack + 1136)).Fnext`, false},
		{"stack_reallocated", "tls *libc.TLS", `stack := tls.Alloc(2048); stack = tls.Alloc(2048); return (*Tlist_head)(unsafe.Pointer(stack + 1136)).Fnext`, false},
		{"stack_reassigned", "tls *libc.TLS, runtime uintptr", `stack := tls.Alloc(2048); stack = runtime; return (*Tlist_head)(unsafe.Pointer(stack + 1136)).Fnext`, true},
		{"escaped_stack", "tls *libc.TLS", `stack := tls.Alloc(2048); change(&stack); return (*Tlist_head)(unsafe.Pointer(stack + 1136)).Fnext`, true},
		{"comment", "runtime uintptr", "// runtime + 1136\n return 0", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := []byte("package p\ntype TOther = struct {\n padding [1136]byte\n list Tlist_head\n}\ntype TJSRuntime = struct {\n Fgc_obj_list Tlist_head\n}\nvar table = [100]TOther{}\nfunc probe(" + tc.params + ") uintptr {\n" + tc.body + "\n}\n")
			got, err := youngReaders(src, "1136")
			if err != nil {
				t.Fatal(err)
			}
			if (len(got) != 0) != tc.want {
				t.Fatalf("readers = %v, want reader %v", got, tc.want)
			}
		})
	}
}

func TestYoungReadersRepeatedInit(t *testing.T) {
	src := []byte(`package p
func init() {
 runtime := getRuntime()
 _ = (*Tlist_head)(unsafe.Pointer(runtime + 1136)).Fnext
}
func init() {
}
`)
	got, err := youngReaders(src, "1136")
	if err != nil || len(got) != 1 || got[0] != "init" {
		t.Fatalf("readers = %v, %v, want init", got, err)
	}
}
