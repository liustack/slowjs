// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

import (
	"bytes"
	"os"
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
	if bad := goasmRuleProblems(found, goasmInterpreterRules, string(cpatch)); len(bad) != 0 {
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
	got := goasmRuleProblems(map[string]string{"a.go": "x", "b.go": "x", "d.go": "x", "e.go": "names _JS_CallInternal"}, rules, "fixA")
	want := []string{
		"c.go: has a row in goasmInterpreterRules but no longer changes _JS_CallInternal",
		"d.go: internal/goasm/cpatch.py does not name fixD",
		"e.go: changes _JS_CallInternal (names _JS_CallInternal) but has no row in goasmInterpreterRules",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("got\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}
