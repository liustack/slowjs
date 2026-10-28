// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
package gen

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestNegativeBuild: GEN_KEEP_NARROW takes the value 1 and no other, and its
// package is marked. Any other value but an empty one stops the driver
// before it does any work (a build that read 0 as on would be the mistake the
// switch exists to show). A negative build over a fixture writes a package
// that does not compile without the tag goasmnegative, and the next build
// into the same folder removes the mark. It needs LLVM 22.1.8 (SLOWJS_LLVM),
// the built pass (SLOWJS_SHADOW_PASS) and the go tool.
func TestNegativeBuild(t *testing.T) {
	llvm, pass := os.Getenv("SLOWJS_LLVM"), os.Getenv("SLOWJS_SHADOW_PASS")
	if llvm == "" || pass == "" {
		t.Skip("set SLOWJS_LLVM and SLOWJS_SHADOW_PASS")
	}
	work := t.TempDir()
	run := driverRunner(t, work, llvm, pass)
	clean := fixture(t, "run/alias.c")

	for _, v := range []string{"0", "true"} {
		out, err := run("narrow-"+v, clean, []string{"GEN_KEEP_NARROW=" + v})
		if err == nil || !strings.Contains(out, `GEN_KEEP_NARROW="`+v+`"`) {
			t.Errorf("GEN_KEEP_NARROW=%s: %v\n%s", v, err, out)
		}
		if _, err := os.Stat(filepath.Join(work, "w-narrow-"+v)); err == nil {
			t.Errorf("GEN_KEEP_NARROW=%s: the driver made its work folder before it refused", v)
		}
	}

	out, err := run("neg", clean, []string{"GEN_KEEP_NARROW=1"})
	if err != nil {
		t.Fatalf("a negative build: %v\n%s", err, out)
	}
	decl, err := os.ReadFile(filepath.Join(work, "out-neg", "engine_decl_amd64.go"))
	if err != nil || !strings.Contains(string(decl), "NEGATIVE BUILD") {
		t.Fatalf("engine_decl_amd64.go of a negative build: %v\n%s", err, decl)
	}
	if err := os.WriteFile(filepath.Join(work, "go.mod"), []byte("module negmod\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gobuild := func(tags ...string) (string, error) {
		cmd := exec.Command("go", append(append([]string{"build"}, tags...), "./out-neg")...)
		cmd.Dir, cmd.Env = work, append(os.Environ(), "GOWORK=off", "CGO_ENABLED=0")
		o, err := cmd.CombinedOutput()
		return string(o), err
	}
	if o, err := gobuild(); err == nil || !strings.Contains(o, "negative_build_of_the_generator_is_not_an_engine") {
		t.Fatalf("a negative build compiled without the tag: %v\n%s", err, o)
	}
	if o, err := gobuild("-tags", "goasmnegative"); err != nil {
		t.Fatalf("a negative build with the tag: %v\n%s", err, o)
	}
	// The same folder, built normally: the mark goes.
	if out, err := run("neg", clean, nil); err != nil {
		t.Fatalf("a normal build over it: %v\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(work, "out-neg", "engine_negative_amd64.go")); err == nil {
		t.Fatal("a normal build left engine_negative_amd64.go of the negative build before it")
	}
	if o, err := gobuild(); err != nil {
		t.Fatalf("the normal build does not compile without the tag: %v\n%s", err, o)
	}
}

// fixture returns the absolute path of a file under testdata.
func fixture(t *testing.T, rel string) string {
	t.Helper()
	p, err := filepath.Abs(filepath.Join("testdata", rel))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// driverRunner builds the engine driver into work and returns a function that
// runs it over one C file (-src), as named run, writing its package to
// work/out-NAME. env is added to an environment without GEN_KEEP_NARROW or
// GEN_EDIT_LOWERED from outside. It returns the driver's output.
func driverRunner(t *testing.T, work, llvm, pass string) func(name, src string, env []string, extra ...string) (string, error) {
	t.Helper()
	drv := filepath.Join(work, "engine")
	build := exec.Command("go", "build", "-o", drv, "./cmd/engine")
	build.Env = append(os.Environ(), "GOWORK=off", "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	var base []string
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, "GEN_KEEP_NARROW=") && !strings.HasPrefix(e, "GEN_EDIT_LOWERED=") {
			base = append(base, e)
		}
	}
	return func(name, src string, env []string, extra ...string) (string, error) {
		cmd := exec.Command(drv, append([]string{"-src", src, "-llvm", llvm, "-pass", pass, "-leaves", "",
			"-work", filepath.Join(work, "w-"+name), "-out", filepath.Join(work, "out-"+name), "-pkg", "nw"}, extra...)...)
		cmd.Env = append(append([]string{}, base...), env...)
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
}
