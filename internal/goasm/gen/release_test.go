// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
package gen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestReleaseBuild runs the driver with -release over fixtures (-src). A clean
// one is built without a stub. One the generator refuses (a variadic
// function) stops the run with nothing written, while the same input as a
// survey (-keep-going) still gives its package with a stub. -keep-going with
// -release, and GEN_KEEP_NARROW with it, are refused before any work is done.
// It needs LLVM 22.1.8 (SLOWJS_LLVM), the built pass (SLOWJS_SHADOW_PASS) and
// the go tool.
func TestReleaseBuild(t *testing.T) {
	llvm, pass := os.Getenv("SLOWJS_LLVM"), os.Getenv("SLOWJS_SHADOW_PASS")
	if llvm == "" || pass == "" {
		t.Skip("set SLOWJS_LLVM and SLOWJS_SHADOW_PASS")
	}
	work := t.TempDir()
	at := func(n string) string { return filepath.Join(work, n) }
	run := driverRunner(t, work, llvm, pass)
	clean, variadic := fixture(t, "run/alias.c"), fixture(t, "reject/variadic.c")

	if out, err := run("clean", clean, nil, "-release"); err != nil {
		t.Fatalf("-release over a clean fixture: %v\n%s", err, out)
	}
	if _, err := os.Stat(at("out-clean/engine_stubs_amd64.s")); err == nil {
		t.Fatal("a release build wrote a stub file")
	}
	if _, err := os.Stat(at("out-clean/engine_decl.go")); err != nil {
		t.Fatalf("a release build wrote no package: %v", err)
	}

	out, err := run("refused", variadic, nil, "-release")
	if err == nil || !strings.Contains(out, "a release build has no stub") {
		t.Fatalf("-release over a variadic function: %v\n%s", err, out)
	}
	if _, err := os.Stat(at("out-refused")); err == nil {
		t.Fatal("a refused release build wrote a package")
	}
	if out, err := run("survey", variadic, nil, "-keep-going"); err != nil {
		t.Fatalf("-keep-going over it: %v\n%s", err, out)
	}
	if b, err := os.ReadFile(at("out-survey/engine_stubs_amd64.s")); err != nil || !strings.Contains(string(b), "goasmMissing") {
		t.Fatalf("the survey package has no stub: %v", err)
	}

	for _, tc := range []struct {
		name  string
		env   []string
		extra []string
		want  string
	}{
		{"keep-going", nil, []string{"-release", "-keep-going"}, "-release refuses -keep-going"},
		{"narrow", []string{"GEN_KEEP_NARROW=1"}, []string{"-release"}, "-release refuses GEN_KEEP_NARROW"},
		{"poison", nil, []string{"-release", "-poison"}, "-release refuses -poison"},
	} {
		out, err := run(tc.name, variadic, tc.env, tc.extra...)
		if err == nil || !strings.Contains(out, tc.want) {
			t.Errorf("%s: %v\n%s", tc.name, err, out)
		}
		if _, err := os.Stat(at("w-" + tc.name)); err == nil {
			t.Errorf("%s: the driver made its work folder before it refused", tc.name)
		}
	}
}
