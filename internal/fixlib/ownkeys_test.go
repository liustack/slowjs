// Copyright (c) 2026 The slowjs Authors.
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

// Every target asks lib/ownkeys.go first in the three functions that list
// own keys, and drops a shape from the key cache when it leaves the shape
// hash table.
func TestOwnKeysEveryFile(t *testing.T) {
	for _, fn := range libFiles(t) {
		b, err := os.ReadFile(fn)
		if err != nil {
			t.Fatal(err)
		}
		name := filepath.Base(fn)
		for _, h := range ownKeysHooks {
			body, err := funcBody(b, funcHead(h.name), h.name)
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			if !bytes.Contains(body, []byte(h.head+h.text)) {
				t.Errorf("%s: %s does not start with %q", name, h.name, h.text)
			}
		}
		if n := len(reOwnKeysCall.FindAll(b, -1)); n != len(ownKeysHooks) {
			t.Errorf("%s: %d calls of lib/ownkeys.go, want %d", name, n, len(ownKeysHooks))
		}
	}
}

// fixOwnKeys leaves a fixed file as it is, and puts the change back into a
// file without it, on every target. restoreOwnKeys gives back the text
// without it.
func TestOwnKeysIdempotent(t *testing.T) {
	for _, fn := range libFiles(t) {
		b, err := readEngine(fn)
		if err != nil {
			t.Fatal(err)
		}
		name := filepath.Base(fn)
		if reOwnKeysCall.Match(b) {
			t.Fatalf("%s: readEngine leaves a call of lib/ownkeys.go in", name)
		}
		fixed, err := fixOwnKeys(bytes.Clone(b), name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if bytes.Equal(fixed, b) {
			t.Fatalf("%s: fixOwnKeys changed nothing", name)
		}
		again, err := fixOwnKeys(bytes.Clone(fixed), name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !bytes.Equal(again, fixed) {
			t.Errorf("%s: a fixed file changed", name)
		}
		back, err := restoreOwnKeys(fixed)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !bytes.Equal(back, b) {
			t.Errorf("%s: restoreOwnKeys does not give the text back", name)
		}
	}
}

// A change to what lib/ownkeys.go repeats or relies on, or a part of the
// change missing, stops fixOwnKeys.
func TestOwnKeysRejects(t *testing.T) {
	b := string(engineFile(t))
	fixed, err := fixOwnKeys([]byte(b), "ccgo_darwin_arm64.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name, in, old, new, err string
	}{
		{"another shape", b, "\tFshape_hash_next    uintptr\n", "\tFshape_hash_next    uintptr\n\tFextra              uintptr\n", "changed (fingerprint"},
		{"no hook", string(fixed), ownKeysHooks[1].text, "", "a part of the change"},
		{"no drop", string(fixed), ownKeysHooks[3].text, "", "a part of the change"},
		{"a call left", string(fixed), ownKeysHooks[3].text, "\townKeyCacheDrop(rt, sh)\n", "a call is left"},
		{"two hooks", string(fixed), ownKeysHooks[0].text, ownKeysHooks[0].text + ownKeysHooks[0].text, "2 copies"},
	} {
		if !strings.Contains(c.in, c.old) {
			t.Fatalf("%s: no %q", c.name, c.old)
		}
		_, err := fixOwnKeys([]byte(strings.Replace(c.in, c.old, c.new, 1)), "ccgo_darwin_arm64.go")
		if err == nil || !strings.Contains(err.Error(), c.err) {
			t.Errorf("%s: %v, want %q", c.name, err, c.err)
		}
	}
	// Each function lib/ownkeys.go repeats or relies on is pinned.
	for _, c := range []struct{ fn, old, new string }{
		{"_JS_GetOwnPropertyNamesInternal", "str_keys_count = str_keys_count + 1\n", "str_keys_count = str_keys_count + 2\n"},
		{"_JS_CopyDataProperties", "libc.Int32FromInt32(1)<<libc.Int32FromInt32(4)", "libc.Int32FromInt32(1)<<libc.Int32FromInt32(5)"},
		{"_JS_AtomIsArrayIndex", "*(*Tuint32_t)(unsafe.Pointer(pval)) = uint32(0)\n", ""},
		{"_add_shape_property", "\t\t_js_shape_hash_unlink(tls, rt, sh)\n", ""},
		{"_js_shape_prepare_update", "Fref_count != int32(1) {", "Fref_count != int32(2) {"},
		{"_js_free_shape0", "\t\t_js_shape_hash_unlink(tls, rt1, sh)\n", ""},
	} {
		body, err := funcBody([]byte(b), funcHead(c.fn), c.fn)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(body, []byte(c.old)) {
			t.Fatalf("%s: no %q", c.fn, c.old)
		}
		in := bytes.Replace([]byte(b), body, bytes.Replace(body, []byte(c.old), []byte(c.new), 1), 1)
		if _, err := fixOwnKeys(in, "ccgo_darwin_arm64.go"); err == nil || !strings.Contains(err.Error(), "changed (fingerprint") {
			t.Errorf("%s changed: %v, want %q", c.fn, err, "changed (fingerprint")
		}
	}
	// With the fingerprint of the change, a function with other parameters
	// still stops it.
	in := strings.Replace(b, ownKeysHooks[2].head, strings.Replace(ownKeysHooks[2].head, "obj1", "obj2", 1), 1)
	*ownKeysF = true
	_, err = fixOwnKeys([]byte(in), "ccgo_darwin_arm64.go")
	*ownKeysF = false
	if err == nil || !strings.Contains(err.Error(), "want one definition") {
		t.Errorf("other parameters: %v", err)
	}
}
