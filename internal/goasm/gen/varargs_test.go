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

// TestVarargsBlock runs testdata/varargs through the pass with
// -shadow-varargs, checks no variadic function or call is left, and runs it
// natively. It needs LLVM 22.1.8 (SLOWJS_LLVM) and the built pass
// (SLOWJS_SHADOW_PASS) on linux/amd64.
func TestVarargsBlock(t *testing.T) {
	llvm, pass := os.Getenv("SLOWJS_LLVM"), os.Getenv("SLOWJS_SHADOW_PASS")
	if llvm == "" || pass == "" {
		t.Skip("set SLOWJS_LLVM and SLOWJS_SHADOW_PASS")
	}
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Skip("runs the result natively on linux/amd64")
	}
	work := t.TempDir()
	ir, shadow, obj := filepath.Join(work, "v.ll"), filepath.Join(work, "v.shadow.ll"), filepath.Join(work, "v.o")
	src := filepath.Join("testdata", "varargs")
	run := func(name string, args ...string) string {
		t.Helper()
		out, err := exec.Command(filepath.Join(llvm, name), args...).CombinedOutput()
		if err != nil {
			t.Fatalf("%s: %v\n%s", name, err, out)
		}
		return string(out)
	}
	run("clang", append(append([]string{"--target=x86_64-linux-gnu"}, EngineCFlags...), "-S", "-emit-llvm", "-o", ir, filepath.Join(src, "varargs.c"))...)
	rep := run("opt", "-load-pass-plugin", pass, "-passes=goasm-shadow-stack", "-shadow-add-sp", "-shadow-varargs", "-shadow-report", "-S", "-o", shadow, ir)
	r := ParseReport(rep)
	if strings.Join(r.Varargs, ",") != "sum,mixed" && strings.Join(r.Varargs, ",") != "mixed,sum" {
		t.Fatalf("rewritten definitions: %v", r.Varargs)
	}
	ll, err := os.ReadFile(shadow)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(ll), "...") || strings.Contains(string(ll), "llvm.va_start") {
		t.Fatalf("variadic code left:\n%s", ll)
	}
	for _, f := range []string{"sum", "mixed"} {
		if c := r.Sigs[f]; strings.Contains(c, ".") {
			t.Fatalf("%s still variadic: %s", f, c)
		}
	}
	run("llc", "-O2", "-relocation-model=pic", "-filetype=obj", "-o", obj, shadow)
	bin := filepath.Join(work, "v")
	run("clang", "-O1", "-o", bin, filepath.Join(src, "main.c"), obj)
	out, err := exec.Command(bin).CombinedOutput()
	if err != nil {
		t.Fatalf("run: %v\n%s", err, out)
	}
	// sum(1,2,3) = 6, mixed = 1+2.5+3+4.25 + 1+3 = 14.75, sum(1..8) = 36.
	if got := strings.TrimSpace(string(out)); got != "36006147" {
		t.Fatalf("got %s, want 36006147", got)
	}
}
