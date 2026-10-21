// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The report counts at the final stage, whether the input was copied from
// upstream or already generated. An outdated expectSites must not stop it.
func TestExpectReportUpstream(t *testing.T) {
	out, err := exec.Command("go", "env", "GOMODCACHE").Output()
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(strings.TrimSpace(string(out)), "modernc.org", "libquickjs@v0.13.1")
	if _, err := os.Stat(dir); err != nil {
		t.Skip("libquickjs v0.13.1 is not in the module cache")
	}
	lib := filepath.Join("..", "..", "lib")
	hw, err := handWritten(lib)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"ccgo_darwin_arm64.go", "ccgo_linux_386.go", "ccgo_windows.go"} {
		t.Run(name, func(t *testing.T) {
			want := expectSites[name]
			expectSites[name] = want + 1
			t.Cleanup(func() { expectSites[name] = want })
			for _, input := range []struct{ name, dir string }{{"upstream", dir}, {"generated", lib}} {
				b, err := readFinalInput(filepath.Join(input.dir, name), hw)
				if err != nil {
					t.Fatal(err)
				}
				_, n, err := expectRewrite(b)
				if err != nil || n != want {
					t.Errorf("%s: got %d, %v, want %d", input.name, n, err, want)
				}
			}
		})
	}
}

// Every test of the form of __builtin_expect in the generated files is
// written through expect64 or expect32, as many as expectSites has, and
// fixExpect makes each file again from what restoreExpect gives.
func TestExpectEveryFile(t *testing.T) {
	for _, fn := range libFiles(t) {
		name := filepath.Base(fn)
		raw, err := os.ReadFile(fn)
		if err != nil {
			t.Fatal(err)
		}
		if edits, err := expectEdits(raw); err != nil || len(edits) != 0 {
			t.Errorf("%s: %d tests of the form of __builtin_expect left (%v)", name, len(edits), err)
		}
		b, err := restoreExpect(raw, name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if bytes.Contains(b, []byte("expect64(")) || bytes.Contains(b, []byte("expect32(")) {
			t.Fatalf("%s: restoreExpect leaves a call of expect64 or expect32", name)
		}
		out, err := fixExpect(bytes.Clone(b), name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !bytes.Equal(out, raw) {
			t.Errorf("%s: fixExpect does not make the file again", name)
		}
		if n := bytes.Count(raw, []byte("expect64(")) + bytes.Count(raw, []byte("expect32(")); n != expectSites[name] || n < 700 {
			t.Errorf("%s: %d tests changed, expectSites has %d", name, n, expectSites[name])
		}
	}
}

const expectBefore = `package p

func f() {
	if libc.BoolInt64(!!(a != 0 && b < c)) != 0 {
		g()
	}
	if !(libc.BoolInt64(!!(x == 0)) != 0) {
		g()
	}
	if libc.BoolInt64(!!(libc.BoolInt64(!!(y)) != 0 || z)) != 0 {
		g()
	}
	if libc.BoolInt32(!!!(p != 0)) != 0 {
		g()
	}
	v = int32(libc.BoolInt64(!!(t != 0)))
	if libc.BoolInt64(a < b) != 0 {
		g()
	}
	if libc.BoolInt64(!!(a < b)) == 0 {
		g()
	}
}
`

const expectAfter = `package p

func f() {
	if expect64(!!(a != 0 && b < c)) {
		g()
	}
	if !(expect64(!!(x == 0))) {
		g()
	}
	if expect64(!!(expect64(!!(y)) || z)) {
		g()
	}
	if expect32(!!!(p != 0)) {
		g()
	}
	v = int32(libc.BoolInt64(!!(t != 0)))
	if libc.BoolInt64(a < b) != 0 {
		g()
	}
	if libc.BoolInt64(!!(a < b)) == 0 {
		g()
	}
}
`

// Only a test of the form of __builtin_expect changes, nested ones too, and
// restoreExpect gives the text back. A value made of one, a test without
// the !! of __builtin_expect and a test for 0 stay as they are.
func TestExpectShapes(t *testing.T) {
	out, n, err := expectRewrite([]byte(expectBefore))
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != expectAfter || n != 5 {
		t.Fatalf("%d changes, got\n%s\nwant\n%s", n, out, expectAfter)
	}
	back, err := restoreExpect(out, "ccgo_test.go")
	if err != nil {
		t.Fatal(err)
	}
	if string(back) != expectBefore {
		t.Fatalf("restoreExpect gives\n%s", back)
	}
}

// fixExpect stops at a file that has its change already, and at a number
// of tests other than expectSites has.
func TestExpectRejects(t *testing.T) {
	name := "ccgo_linux_amd64.go"
	if _, err := fixExpect([]byte(expectAfter), name); err == nil || !strings.Contains(err.Error(), "restoreExpect goes first") {
		t.Errorf("a file with the change: %v", err)
	}
	if _, err := fixExpect([]byte(expectBefore), name); err == nil || !strings.Contains(err.Error(), fmt.Sprintf("expectSites has %d", expectSites[name])) {
		t.Errorf("another number of tests: %v", err)
	}
	if _, err := restoreExpect([]byte("package p\n\n// expect64(x)\n"), "ccgo_test.go"); err == nil {
		t.Error("restoreExpect of expect64 not called: no error")
	}
}
