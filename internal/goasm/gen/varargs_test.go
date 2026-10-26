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
	rewritten := map[string]bool{}
	for _, f := range r.Varargs {
		rewritten[f] = true
	}
	// sum and mixed read scalars, aggr reads an 8-aligned struct, wrap is a
	// variadic function that itself makes a variadic call: all four rewritten.
	for _, f := range []string{"sum", "mixed", "aggr", "wrap"} {
		if !rewritten[f] {
			t.Fatalf("%s not rewritten: %v", f, r.Varargs)
		}
	}
	ll, err := os.ReadFile(shadow)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(ll), "...") || strings.Contains(string(ll), "llvm.va_start") {
		t.Fatalf("variadic code left:\n%s", ll)
	}
	run("llc", "-O2", "-relocation-model=pic", "-filetype=obj", "-o", obj, shadow)
	bin := filepath.Join(work, "v")
	run("clang", "-O1", "-o", bin, filepath.Join(src, "main.c"), obj)
	out, err := exec.Command(bin).CombinedOutput()
	if err != nil {
		t.Fatalf("run: %v\n%s", err, out)
	}
	// use: sum(1,2,3)=6, mixed=1+2.5+3+4.25+1+3=14.75->147, sum(1..8)=36.
	// aggr: 100*1+2 + 100*3+4 = 406. wrap: 7 + sum(2,100,200)=307.
	if got := strings.TrimSpace(string(out)); got != "36006147 406 307" {
		t.Fatalf("got %q, want %q", got, "36006147 406 307")
	}
}

// TestVarargsOverAlignedRejected: a variadic function reading a 16-aligned
// aggregate cannot be held in the 8-byte-slot block and must be rejected,
// not miscompiled. Needs LLVM 22.1.8 (SLOWJS_LLVM) and the pass
// (SLOWJS_SHADOW_PASS).
func TestVarargsOverAlignedRejected(t *testing.T) {
	llvm, pass := os.Getenv("SLOWJS_LLVM"), os.Getenv("SLOWJS_SHADOW_PASS")
	if llvm == "" || pass == "" {
		t.Skip("set SLOWJS_LLVM and SLOWJS_SHADOW_PASS")
	}
	work := t.TempDir()
	ir := filepath.Join(work, "o.ll")
	run := func(name string, args ...string) string {
		t.Helper()
		out, err := exec.Command(filepath.Join(llvm, name), args...).CombinedOutput()
		if err != nil {
			t.Fatalf("%s: %v\n%s", name, err, out)
		}
		return string(out)
	}
	run("clang", append(append([]string{"--target=x86_64-linux-gnu"}, EngineCFlags...), "-S", "-emit-llvm", "-o", ir, filepath.Join("testdata", "varargs", "overaligned.c"))...)
	out := run("opt", "-load-pass-plugin", pass, "-passes=goasm-shadow-stack", "-shadow-add-sp", "-shadow-varargs", "-shadow-keep-going", "-S", "-o", os.DevNull, ir)
	rep := ParseReport(out)
	if why := rep.Rejected["take"]; !strings.Contains(why, "over-aligned") {
		t.Fatalf("take not rejected for over-alignment: %q\n%s", why, out)
	}
}
