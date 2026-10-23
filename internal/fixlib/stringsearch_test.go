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

// On every target, the loops of js_string_includes and js_string_indexOf
// each call stringFind once, at their start.
// fixStringSearch leaves a fixed file as it is and puts the change back
// into a file without it, and restoreStringSearch gives back the text
// without it, byte for byte.
func TestStringSearchEveryFile(t *testing.T) {
	for _, fn := range libFiles(t) {
		b, err := readEngine(fn)
		if err != nil {
			t.Fatal(err)
		}
		name := filepath.Base(fn)
		for k, fn := range []string{"_js_string_includes", "_js_string_indexOf"} {
			body, err := funcBody(b, funcHead(fn), fn)
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			if n := bytes.Count(body, []byte("stringFind(")); n != 1 || !bytes.Contains(body, []byte("\t\ti = start\n\t\tfor {\n"+searchCalls[k])) {
				t.Errorf("%s: %s: %d calls of stringFind, want one at the start of the loop", name, fn, n)
			}
		}
		if n := bytes.Count(b, []byte("stringFind(")); n != 2 {
			t.Errorf("%s: %d calls of stringFind, want 2", name, n)
		}

		again, err := fixStringSearch(bytes.Clone(b), name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !bytes.Equal(again, b) {
			t.Errorf("%s: a fixed file changed", name)
		}
		plain, err := restoreStringSearch(bytes.Clone(b))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if bytes.Contains(plain, []byte("stringFind")) || bytes.Contains(plain, []byte("fixStringSearch")) {
			t.Fatalf("%s: restoreStringSearch leaves part of the change", name)
		}
		fixed, err := fixStringSearch(bytes.Clone(plain), name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !bytes.Equal(fixed, b) {
			t.Errorf("%s: fixStringSearch of the text without the change does not give the file", name)
		}
	}
}

// A change to a function stringFind repeats or is called from, or to a
// loop fixStringSearch reads, stops it.
func TestStringSearchRejects(t *testing.T) {
	fixed := string(engineFile(t))
	plainB, err := restoreStringSearch([]byte(fixed))
	if err != nil {
		t.Fatal(err)
	}
	plain := string(plainB)
	for _, c := range []struct {
		name, in, old, new, err string
	}{
		{"compare", plain, "\t\tif v2 != v3 {\n\t\t\treturn c1 - c2\n", "\t\tif v2 != v3+0 {\n\t\t\treturn c1 - c2\n", "changed (fingerprint"},
		{"includes", plain, "\t\t\t\tret = int32(1)\n\t\t\t\tbreak\n", "\t\t\t\tret = int32(1)\n\t\t\t\tret = ret\n\t\t\t\tbreak\n", "_js_string_includes: want one loop"},
		{"includes step", plain, "\t\t\tif i == stop {\n\t\t\t\tbreak\n\t\t\t}\n\t\t\tgoto _10\n\t\t_10:\n\t\t\t;\n\t\t\ti = i + 1\n", "\t\t\tif i == stop {\n\t\t\t\tbreak\n\t\t\t}\n\t\t\tgoto _10\n\t\t_10:\n\t\t\t;\n\t\t\ti = i + inc\n", "the loop moves the index by inc, want 1"},
		{"indexOf", plain, "\t\t\t\tret = i\n\t\t\t\tbreak\n", "\t\t\t\tret = i + 0\n\t\t\t\tbreak\n", "_js_string_indexOf: want one loop"},
		{"get", plain, "\t\tv1 = libc.Int32FromUint16(*(*Tuint16_t)(unsafe.Pointer(p + 12 + uintptr(idx)*2)))\n", "\t\tv1 = libc.Int32FromUint16(*(*Tuint16_t)(unsafe.Pointer(p + 14 + uintptr(idx)*2)))\n", "changed (fingerprint"},
		{"a call left", fixed, searchCalls[1], strings.Replace(searchCalls[1], "stop, inc", "0, inc", 1), "a call of stringFind is left"},
	} {
		if !strings.Contains(c.in, c.old) {
			t.Fatalf("%s: no %q", c.name, c.old)
		}
		in := strings.Replace(c.in, c.old, c.new, 1)
		var err error
		if c.in == plain {
			_, err = fixStringSearch([]byte(in), "ccgo_darwin_arm64.go")
		} else {
			_, err = restoreStringSearch([]byte(in))
		}
		if err == nil || !strings.Contains(err.Error(), c.err) {
			t.Errorf("%s: %v, want %q", c.name, err, c.err)
		}
	}
}
