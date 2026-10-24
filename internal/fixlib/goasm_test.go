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

func TestGoasmRewrite(t *testing.T) {
	const name = "ccgo_linux_amd64.go"
	b, err := os.ReadFile("../../lib/" + name)
	if err != nil {
		t.Fatal(err)
	}
	bare, err := restoreGoasm(b, name)
	if err != nil {
		t.Fatal(err)
	}
	fixed, err := fixGoasm(bare, name)
	if err != nil || !bytes.Equal(fixed, b) {
		t.Fatalf("rewrite differs: %v", err)
	}
	back, err := restoreGoasm(fixed, name)
	if err != nil || !bytes.Equal(back, bare) {
		t.Fatalf("inverse differs: %v", err)
	}
	changed := bytes.Replace(bare, []byte("func _JS_CallInternal("), []byte("func _JS_CallInternalChanged("), 1)
	if _, err := fixGoasm(changed, name); err == nil {
		t.Fatal("accepted changed interpreter")
	}
	corrupt := bytes.Replace(fixed, []byte("return goasmCall(tls, caller_ctx"), []byte("return goasmCall(tls, 0"), 1)
	if _, err := fixGoasm(corrupt, name); err == nil {
		t.Fatal("accepted corrupted hook")
	}
	duplicate := append(append([]byte{}, fixed...), []byte(goasmMark)...)
	if _, err := restoreGoasm(duplicate, name); err == nil {
		t.Fatal("accepted duplicate hook")
	}
	other, err := fixGoasm(bare, "ccgo_darwin_arm64.go")
	if err != nil || !bytes.Equal(other, bare) {
		t.Fatalf("changed fallback target: %v", err)
	}
}

// TestGoasmCoversInterpreterRules checks that every file of fixlib that
// changes _JS_CallInternal has a row in goasmInterpreterRules, that every
// row names such a file, and that a rule the generated interpreter takes
// over in C is named in internal/goasm/cpatch.py. A file changes the
// interpreter when its code names _JS_CallInternal, uses a package variable
// whose value does, or defines a change that left a "(see fixX)" mark in the
// interpreter of lib/ccgo_linux_amd64.go.
func TestGoasmCoversInterpreterRules(t *testing.T) {
	found, err := goasmRuleFiles(".", "../../lib/ccgo_linux_amd64.go")
	if err != nil {
		t.Fatal(err)
	}
	cpatch, err := os.ReadFile("../goasm/cpatch.py")
	if err != nil {
		t.Fatal(err)
	}
	if bad := goasmRuleProblems(found, goasmInterpreterRules, string(cpatch), "goasmInterpreterRules", "_JS_CallInternal"); len(bad) != 0 {
		t.Fatalf("fixlib and the generated interpreter disagree (see internal/fixlib/goasm.go):\n%s", strings.Join(bad, "\n"))
	}
}

// The check fails on a missing row, a stale row, and a C rule cpatch.py
// does not name.
func TestGoasmRuleProblems(t *testing.T) {
	rules := map[string]goasmRule{
		"a.go": {goasmCPatch, "fixA"},
		"b.go": {goasmGoOnly, "Go's compiler"},
		"c.go": {goasmGoOnly, "gone"},
		"d.go": {goasmCPatch, "fixD"},
	}
	got := goasmRuleProblems(map[string]string{"a.go": "x", "b.go": "x", "d.go": "x", "e.go": "names _JS_CallInternal"}, rules, "fixA", "goasmInterpreterRules", "_JS_CallInternal")
	want := []string{
		"c.go: has a row in goasmInterpreterRules but no longer changes _JS_CallInternal",
		"d.go: internal/goasm/cpatch.py does not name fixD",
		"e.go: changes _JS_CallInternal (names _JS_CallInternal) but has no row in goasmInterpreterRules",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("got\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// TestGoasmCoversHelperRules checks the same for the helpers that the
// generated interpreter compiles in from the C, listed by the generator in
// lib/goasm_helpers.txt: a change of fixlib to the Go copy of one of them
// does not reach the assembly, so each needs a row in goasmHelperRules.
func TestGoasmCoversHelperRules(t *testing.T) {
	helpers, err := goasmHelpers("../../lib/goasm_helpers.txt")
	if err != nil {
		t.Fatal(err)
	}
	if len(helpers) == 0 {
		t.Fatal("lib/goasm_helpers.txt lists no helpers")
	}
	found, err := goasmHelperFiles(".", "../../lib/ccgo_linux_amd64.go", helpers)
	if err != nil {
		t.Fatal(err)
	}
	cpatch, err := os.ReadFile("../goasm/cpatch.py")
	if err != nil {
		t.Fatal(err)
	}
	if bad := goasmRuleProblems(found, goasmHelperRules, string(cpatch), "goasmHelperRules", goasmHelperWhat); len(bad) != 0 {
		t.Fatalf("fixlib and the generated interpreter disagree (see internal/fixlib/goasm.go):\n%s", strings.Join(bad, "\n"))
	}
}

// A helper's Go copy is changed by a file whose code names the function on
// its own, as a list of functions does, or holds its first line, as a hook
// does, by one that uses a package variable with such a value, and by one
// whose change left a mark in it. A call of the helper in another
// function's text, a helper with no Go function of its own, which ccgo
// wrote into its callers, and a test file are not.
func TestGoasmHelperFiles(t *testing.T) {
	dir := t.TempDir()
	write := func(name, text string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("a.go", "package main\n\nvar fooFuncs = []string{\"_foo\", \"_other\"}\n")
	write("a2.go", "package main\n\nvar s = \"\\tx := _foo(tls, p)\\n\"\n")
	write("b.go", "package main\n\nfunc fixBar() {}\n")
	write("c.go", "package main\n\nvar bazHook = \"func Xbaz(tls *libc.TLS) {\\n\"\n")
	write("d.go", "package main\n\nfunc d() { _ = bazHook }\n")
	write("e.go", "package main\n\nvar t = []string{\"_foo_lazy\", \"_nogo\"}\n")
	write("a_test.go", "package main\n\nvar u = \"_foo\"\n")
	engine := filepath.Join(dir, "engine.go")
	write("engine.go", "package x\n\nfunc _foo(tls *libc.TLS, p uintptr) (r int32) {\n\t// Changed by internal/fixlib: x (see fixBar).\n\treturn 0\n}\n\nfunc Xbaz(tls *libc.TLS) {\n}\n")
	got, err := goasmHelperFiles(dir, engine, []string{"foo", "baz", "nogo"})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"foo in a.go": "names _foo",
		"foo in b.go": "left (see fixBar) in _foo",
		"baz in c.go": "names Xbaz",
		"baz in d.go": "uses bazHook",
	}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}
