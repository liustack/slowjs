// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
package gen

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestArgBlockShared checks that the calls of one function share one argument
// block: three(x) makes three packed calls of 56 bytes of arguments, so its
// shadow frame is one 64-byte block. It then runs the result natively to
// confirm the shared block carries the right values, including a call whose
// argument is itself a packed call (nested). It needs LLVM 22.1.8
// (SLOWJS_LLVM) and the built pass (SLOWJS_SHADOW_PASS) on linux/amd64.
func TestArgBlockShared(t *testing.T) {
	llvm, pass := os.Getenv("SLOWJS_LLVM"), os.Getenv("SLOWJS_SHADOW_PASS")
	if llvm == "" || pass == "" {
		t.Skip("set SLOWJS_LLVM and SLOWJS_SHADOW_PASS")
	}
	work := t.TempDir()
	ir, shadow, obj := filepath.Join(work, "a.ll"), filepath.Join(work, "a.shadow.ll"), filepath.Join(work, "a.o")
	run := func(name string, args ...string) string {
		t.Helper()
		out, err := exec.Command(filepath.Join(llvm, name), args...).CombinedOutput()
		if err != nil {
			t.Fatalf("%s: %v\n%s", name, err, out)
		}
		return string(out)
	}
	run("clang", append(append([]string{}, EngineCFlags...), "-S", "-emit-llvm", "-o", ir, filepath.Join("testdata", "argblock.c"))...)
	out := run("opt", "-load-pass-plugin", pass, "-passes=goasm-shadow-stack",
		"-shadow-add-sp", "-shadow-max-int-args=6", "-shadow-report", "-S", "-o", shadow, ir)
	if !strings.Contains(out, "PACKED many") {
		t.Fatalf("many not packed:\n%s", out)
	}
	// withval's value of two words, which C passes in memory, is the pair
	// the Go host passes (gen.PackedGo), not a byval it cannot.
	if !strings.Contains(out, "PACKED withval iiiiiiij>i\n") {
		t.Fatalf("want withval packed with its value as a pair:\n%s", out)
	}
	if !strings.Contains(out, "SHADOW three 64\n") {
		t.Fatalf("want one 64-byte block for three:\n%s", out)
	}
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Skip("runs the result natively on linux/amd64")
	}
	run("llc", "-O2", "-relocation-model=pic", "-filetype=obj", "-o", obj, shadow)
	bin := filepath.Join(work, "a")
	run("clang", "-O1", "-o", bin, filepath.Join("testdata", "argblock_main.c"), obj)
	got, err := exec.Command(bin).CombinedOutput()
	if err != nil {
		t.Fatalf("run: %v\n%s", err, got)
	}
	// three(5) = 3*(5+21) = 78. nested(10) = many(10,0..) + 1..6 = 10+21 = 31.
	// callval(7) = 1..6 + 7*3 = 42.
	if s := strings.TrimSpace(string(got)); s != "78 31 42" {
		t.Fatalf("got %q, want %q", s, "78 31 42")
	}
}
