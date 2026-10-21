// Copyright (c) 2026 The liustack/quickjs Authors.
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

// unfixForInFile takes the change of fixForIn out of a whole file: the
// call and the constants.
func unfixForInFile(b []byte) []byte {
	return unfixForIn(cutConstants(b, forInLayoutMark))
}

// Every generated file asks forInNoEnum first and has its constants.
// fixForIn leaves a fixed file as it is, and puts the change back into a
// file without it.
func TestForInIdempotent(t *testing.T) {
	for _, fn := range libFiles(t) {
		b, err := readEngine(fn)
		if err != nil {
			t.Fatal(err)
		}
		name := filepath.Base(fn)
		if n := len(reForInNamesFixed.FindAll(b, -1)); n != 1 {
			t.Fatalf("%s: forInNoEnum is called %d times", name, n)
		}
		if n := bytes.Count(b, []byte(forInLayoutMark)); n != 1 {
			t.Fatalf("%s: the constants of lib/forin.go are there %d times", name, n)
		}
		got, err := fixForIn(bytes.Clone(b), name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !bytes.Equal(got, b) {
			t.Errorf("%s: a fixed file changed", name)
		}
		bare := unfixForInFile(bytes.Clone(b))
		if bytes.Contains(bare, []byte("forInNoEnum")) {
			t.Fatalf("%s: the call is still there", name)
		}
		if got, err = fixForIn(bare, name); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !bytes.Equal(got, b) {
			t.Errorf("%s: the change put back differs", name)
		}
	}
}

// The constants are the bit fields of each target: little-endian targets
// keep the flags of a shape property in the top 6 bits, s390x in the low 6.
func TestForInBits(t *testing.T) {
	for _, c := range []struct{ file, want string }{
		{"ccgo_linux_amd64.go", "forInFlagsMask  = 0xfc000000\n\tforInFlagsShift = 26\n"},
		{"ccgo_linux_386.go", "forInFlagsMask  = 0xfc000000\n\tforInFlagsShift = 26\n"},
		{"ccgo_linux_s390x.go", "forInFlagsMask  = 0x3f\n\tforInFlagsShift = 0\n"},
	} {
		b, err := readEngine(filepath.Join("..", "..", "lib", c.file))
		if err != nil {
			t.Fatal(err)
		}
		got, err := forInBits(b)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(got, []byte(c.want)) {
			t.Errorf("%s: %s, want %q", c.file, got, c.want)
		}
	}
}

// A change to the functions forInNoEnum repeats, or to the place it is
// called from, stops fixForIn.
func TestForInRejects(t *testing.T) {
	b := string(unfixForInFile(engineFile(t)))
	for _, c := range []struct {
		name, fn, old, new, err string
	}{
		{"another count", "_JS_GetOwnPropertyNamesInternal", "\t\t\t\t\tstr_keys_count = str_keys_count + 1\n", "\t\t\t\t\tstr_keys_count = str_keys_count + 2\n", "changed (fingerprint"},
		{"another flag", "_JS_GetOwnPropertyNamesInternal", "&(libc.Int32FromInt32(3)<<libc.Int32FromInt32(4))", "&(libc.Int32FromInt32(3)<<libc.Int32FromInt32(5))", "changed (fingerprint"},
		{"another free", "XJS_FreePropertyEnum", "\t\t\tXJS_FreeAtom(tls, ctx, (*(*TJSPropertyEnum)(unsafe.Pointer(tab + uintptr(i)*8))).Fatom)\n", "", "changed (fingerprint"},
		{"another poll", forInFunc, "if _js_poll_interrupts(tls, ctx2) != 0 {", "if false {", "changed (fingerprint"},
		{"another shape", "_get_shape_prop", "return sh + ", "return sh + 8 + ", "changed (fingerprint"},
	} {
		body, err := funcBody([]byte(b), funcHead(c.fn), c.fn)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(body), c.old) {
			t.Fatalf("%s: no %q in %s", c.name, c.old, c.fn)
		}
		_, err = fixForIn([]byte(strings.Replace(b, string(body), strings.Replace(string(body), c.old, c.new, 1), 1)), "ccgo_darwin_arm64.go")
		if err == nil || !strings.Contains(err.Error(), c.err) {
			t.Errorf("%s: %v, want %q", c.name, err, c.err)
		}
	}
	// A second listing of the names of a prototype is not one fixForIn
	// knows.
	body, err := funcBody([]byte(b), funcHead(forInFunc), forInFunc)
	if err != nil {
		t.Fatal(err)
	}
	line := reForInNames.Find(body)
	twice := strings.Replace(string(body), string(line), string(line)+"\t\t}\n"+string(line), 1)
	if _, err := forInCallFirst([]byte(twice)); err == nil || !strings.Contains(err.Error(), "want one listing") {
		t.Errorf("two listings: %v", err)
	}
}
