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

// fixClassEscapes leaves a fixed file as it is, and puts the change back
// into a file without it, on every target.
func TestClassEscapesIdempotent(t *testing.T) {
	for _, fn := range libFiles(t) {
		b, err := readEngine(fn)
		if err != nil {
			t.Fatal(err)
		}
		name := filepath.Base(fn)
		if n := bytes.Count(b, []byte("(see fixClassEscapes)")); n != 4 {
			t.Fatalf("%s: %d changes of fixClassEscapes, want 4", name, n)
		}
		if n := bytes.Count(b, []byte("if inclass != 0 && (*TREParseState)(unsafe.Pointer(s)).Funicode_sets != 0")); n != 3 {
			t.Fatalf("%s: %d tests of inclass, want 3", name, n)
		}
		got, err := fixClassEscapes(bytes.Clone(b), name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !bytes.Equal(got, b) {
			t.Errorf("%s: a fixed file changed", name)
		}
		bare, err := unfixClassEscapes(bytes.Clone(b))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if bytes.Contains(bare, []byte("fixClassEscapes")) || bytes.Contains(bare, []byte("if inclass != 0 && (*TREParseState)")) ||
			!bytes.Contains(bare, []byte("_get_class_atom(tls, s1, libc.UintptrFromInt32(0), bp, int32(EFALSE))")) {
			t.Fatalf("%s: a change is still there", name)
		}
		if got, err = fixClassEscapes(bare, name); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !bytes.Equal(got, b) {
			t.Errorf("%s: the change put back differs", name)
		}
	}
}

// A change to the functions fixClassEscapes changes or relies on, or a
// change of its own that is not as fixlib writes it, stops it.
func TestClassEscapesRejects(t *testing.T) {
	b := string(engineFile(t))
	bareB, err := unfixClassEscapes([]byte(b))
	if err != nil {
		t.Fatal(err)
	}
	bare := string(bareB)
	for _, c := range []struct {
		name, in, old, new, err string
	}{
		{"a new escape", bare, "\tcase uint32('q'):\n\t\tgoto _59\n", "\tcase uint32('q'):\n\t\tgoto _59\n\tcase uint32('&'):\n\t\tgoto _42\n", "changed (fingerprint"},
		{"an escape upstream reads otherwise", bare, "\tcase uint32('v'):\n\t\tc3 = uint32('\\v')\n", "\tcase uint32('v'):\n\t\tc3 = uint32('\\f')\n", "changed (fingerprint"},
		{"upstream's inclass test", bare, "\tif (*TREParseState)(unsafe.Pointer(s)).Funicode_sets != 0 {\n\t\t/* invalid characters in unicode sets */\n", "\tif inclass != 0 && (*TREParseState)(unsafe.Pointer(s)).Funicode_sets != 0 {\n\t\t/* invalid characters in unicode sets */\n", "changed (fingerprint"},
		{"\\q{} read in a class", bare, "_get_class_atom(tls, s1, libc.UintptrFromInt32(0), bp, int32(EFALSE))", "_get_class_atom(tls, s1, libc.UintptrFromInt32(0), bp, int32(ETRUE))", "changed (fingerprint"},
		{"a test without its comment", b, classTestNote + "\tif inclass", "\tif inclass", "changed (fingerprint"},
		{"the escape test changed", b, "uint32('@'), uint32('`'), uint32('~'):\n\t\t\tgoto _61\n", "uint32('@'), uint32('`'):\n\t\t\tgoto _61\n", "not as fixlib writes it"},
		{"the \\q{} call changed", b, "\t\t\t" + classStringNote + "\t\t\tc = _get_class_atom(", "\t\t\t" + classStringNote + "\t\t\t\tc = _get_class_atom(", "not as fixlib writes it"},
	} {
		if !strings.Contains(c.in, c.old) {
			t.Fatalf("%s: no %q", c.name, c.old)
		}
		_, err := fixClassEscapes([]byte(strings.Replace(c.in, c.old, c.new, 1)), "ccgo_darwin_arm64.go")
		if err == nil || !strings.Contains(err.Error(), c.err) {
			t.Errorf("%s: %v, want %q", c.name, err, c.err)
		}
	}
}
