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

// unfixEqFile takes the change of fixEq out of a whole file, named name.
func unfixEqFile(t *testing.T, b []byte, name string) []byte {
	t.Helper()
	bits, err := eqBits(b)
	if err == nil && cellFiles[name] {
		bits, err = eqFrame(b, bits)
	}
	if err != nil {
		t.Fatal(err)
	}
	body, err := funcBody(b, funcHead("_JS_CallInternal"), "_JS_CallInternal")
	if err != nil {
		t.Fatal(err)
	}
	bare, err := unfixEq(body, bits)
	if err != nil {
		t.Fatal(err)
	}
	return bytes.Replace(b, body, bare, 1)
}

// The interpreter of every 64-bit target writes out the four comparisons,
// and that of the 32-bit targets does not. fixEq leaves a fixed file as it
// is, and puts the change back into a file without it.
func TestEqIdempotent(t *testing.T) {
	for _, fn := range libFiles(t) {
		b, err := readEngine(fn)
		if err != nil {
			t.Fatal(err)
		}
		name := filepath.Base(fn)
		want := 0
		if cellFiles[name] {
			want = 4
		}
		if n := len(reEqFixed.FindAll(b, -1)); n != want {
			t.Fatalf("%s: the comparisons are written out %d times, want %d", name, n, want)
		}
		got, err := fixEq(bytes.Clone(b), name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !bytes.Equal(got, b) {
			t.Errorf("%s: a fixed file changed", name)
		}
		bare := unfixEqFile(t, bytes.Clone(b), name)
		if reEqFixed.Match(bare) {
			t.Fatalf("%s: the text of fixEq is still there", name)
		}
		if got, err = fixEq(bare, name); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !bytes.Equal(got, b) {
			t.Errorf("%s: the change put back differs", name)
		}
	}
}

// The bit fields of the string length, the atom type and IsHTMLDDA.
func TestEqBits(t *testing.T) {
	for _, c := range []struct{ file, len, atom, htmldda string }{
		{"ccgo_linux_amd64.go", "&0x7fffffff>>0", "+ 4))&0xc0000000>>30", "+ 17))&0x1 != 0"},
	} {
		b, err := readEngine(filepath.Join("..", "..", "lib", c.file))
		if err != nil {
			t.Fatal(err)
		}
		l, err := eqBits(b)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(l.len, c.len) || !strings.Contains(l.atom, c.atom) || !strings.Contains(l.htmldda, c.htmldda) {
			t.Errorf("%s: %+v, want %s, %s and %s", c.file, l, c.len, c.atom, c.htmldda)
		}
	}
}

// The layout of a value is the same on every 64-bit target: the reference
// count 4 bytes below the pointer, 16 bytes a value, and a boolean tagged
// int64(EJS_TAG_BOOL).
func TestEqFrame(t *testing.T) {
	for _, fn := range libFiles(t) {
		name := filepath.Base(fn)
		if !cellFiles[name] {
			continue
		}
		b, err := readEngine(fn)
		if err != nil {
			t.Fatal(err)
		}
		l, err := eqFrame(b, eqLayout{})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if l.ref != 4 || l.val != 16 || l.boolTag != "int64(EJS_TAG_BOOL)" {
			t.Errorf("%s: %+v", name, l)
		}
	}
}

// A change to the functions the text of fixEq stands for, or to the calls
// of the slow paths in the interpreter, stops fixEq.
func TestEqRejects(t *testing.T) {
	b := string(unfixEqFile(t, engineFile(t), "ccgo_darwin_arm64.go"))
	for _, c := range []struct {
		name, fn, old, new, err string
	}{
		{"another strict", "_js_strict_eq2", "case int32(EJS_TAG_SYMBOL):\n\t\tgoto _6", "case int32(EJS_TAG_SYMBOL):\n\t\tgoto _7", "changed (fingerprint"},
		{"another loose", "_js_eq_slow", "tag2 == uint32(EJS_TAG_NULL) && tag1 == uint32(EJS_TAG_UNDEFINED)", "tag2 == uint32(EJS_TAG_NULL) && tag1 == uint32(EJS_TAG_BOOL)", "changed (fingerprint"},
		{"another result", "_js_strict_eq_slow", "res^is_neq", "res|is_neq", "changed (fingerprint"},
		{"another free", "x__JS_FreeValue", "(*TJSContext)(unsafe.Pointer(ctx)).Frt", "(*TJSContext)(unsafe.Pointer(ctx)).Frt+8", "changed (fingerprint"},
		{"another string", "_js_string_eq", "if p1 == p2 {", "if p1 != p2 {", "changed (fingerprint"},
		{"another call", "_JS_CallInternal", "if _js_strict_eq_slow(tls, ctx10, sp, 0) != 0 {", "if _js_strict_eq_slow(tls, ctx10, sp, 0) < 0 {", "want 4 calls"},
		{"one twice", "_JS_CallInternal", "if _js_strict_eq_slow(tls, ctx10, sp, int32(1)) != 0 {", "if _js_strict_eq_slow(tls, ctx10, sp, 0) != 0 {", "not one for each"},
		{"another pop", "_JS_CallInternal", "if _js_strict_eq_slow(tls, ctx10, sp, 0) != 0 {\n\t\t\t\tgoto exception\n\t\t\t}\n\t\t\tsp -= 16\n", "if _js_strict_eq_slow(tls, ctx10, sp, 0) != 0 {\n\t\t\t\tgoto exception\n\t\t\t}\n\t\t\tsp -= 8\n", "pops 8 bytes"},
		{"another drop", "XFreeValue", "if v4 <= libc.Int32FromInt32(0) {", "if v4 < libc.Int32FromInt32(0) {", "JS_FreeValue changed shape"},
	} {
		body, err := funcBody([]byte(b), funcHead(c.fn), c.fn)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(body), c.old) {
			t.Fatalf("%s: no %q in %s", c.name, c.old, c.fn)
		}
		_, err = fixEq([]byte(strings.Replace(b, string(body), strings.Replace(string(body), c.old, c.new, 1), 1)), "ccgo_darwin_arm64.go")
		if err == nil || !strings.Contains(err.Error(), c.err) {
			t.Errorf("%s: %v, want %q", c.name, err, c.err)
		}
	}
	// Text in the place of fixEq's that it does not write is not taken
	// out.
	fixed, err := fixEq([]byte(b), "ccgo_darwin_arm64.go")
	if err != nil {
		t.Fatal(err)
	}
	changed := strings.Replace(string(fixed), "eqRes = eqPA == eqPB\n", "eqRes = true\n", 1)
	if _, err := fixEq([]byte(changed), "ccgo_darwin_arm64.go"); err == nil || !strings.Contains(err.Error(), "is not what fixEq writes") {
		t.Errorf("changed text: %v", err)
	}
}
