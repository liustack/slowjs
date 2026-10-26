// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
package gen

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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
	// sum and mixed read scalars, wrap is a variadic function that itself
	// makes a variadic call: all three rewritten.
	for _, f := range []string{"sum", "mixed", "wrap"} {
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
	// wrap: 7 + sum(2,100,200)=307.
	if got := strings.TrimSpace(string(out)); got != "36006147 307" {
		t.Fatalf("got %q, want %q", got, "36006147 307")
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

// TestVarargsOverAlignedHelperRejected: the over-aligned read sits in a plain
// helper the variadic function passes its va_list to, not in the variadic
// function itself. The pass must follow the va_list into the helper and
// reject it, otherwise a caller gets a block the helper reads wrong. Needs
// LLVM 22.1.8 (SLOWJS_LLVM) and the pass (SLOWJS_SHADOW_PASS).
func TestVarargsOverAlignedHelperRejected(t *testing.T) {
	llvm, pass := os.Getenv("SLOWJS_LLVM"), os.Getenv("SLOWJS_SHADOW_PASS")
	if llvm == "" || pass == "" {
		t.Skip("set SLOWJS_LLVM and SLOWJS_SHADOW_PASS")
	}
	work := t.TempDir()
	ir := filepath.Join(work, "h.ll")
	run := func(name string, args ...string) string {
		t.Helper()
		out, err := exec.Command(filepath.Join(llvm, name), args...).CombinedOutput()
		if err != nil {
			t.Fatalf("%s: %v\n%s", name, err, out)
		}
		return string(out)
	}
	run("clang", append(append([]string{"--target=x86_64-linux-gnu"}, EngineCFlags...), "-S", "-emit-llvm", "-o", ir, filepath.Join("testdata", "varargs", "overaligned_helper.c"))...)
	out := run("opt", "-load-pass-plugin", pass, "-passes=goasm-shadow-stack", "-shadow-add-sp", "-shadow-varargs", "-shadow-keep-going", "-S", "-o", os.DevNull, ir)
	rep := ParseReport(out)
	if why := rep.Rejected["read_s"]; !strings.Contains(why, "over-aligned") {
		t.Fatalf("the helper read_s was not rejected: %q\n%s", why, out)
	}
}

// TestVarargsEscapesRejected: a va_list, or a pointer read from one, that
// goes where the pass cannot follow it (a va_copy into a helper, an indirect
// or external call, a store to memory, a va_copy into a global) must reject
// the function that sends it, since the pass cannot prove the reads there
// keep to the block's slots. A va_copy read by a scalar helper stays. Needs
// LLVM 22.1.8 (SLOWJS_LLVM) and the pass (SLOWJS_SHADOW_PASS).
func TestVarargsEscapesRejected(t *testing.T) {
	llvm, pass := os.Getenv("SLOWJS_LLVM"), os.Getenv("SLOWJS_SHADOW_PASS")
	if llvm == "" || pass == "" {
		t.Skip("set SLOWJS_LLVM and SLOWJS_SHADOW_PASS")
	}
	work := t.TempDir()
	ir := filepath.Join(work, "e.ll")
	run := func(name string, args ...string) string {
		t.Helper()
		out, err := exec.Command(filepath.Join(llvm, name), args...).CombinedOutput()
		if err != nil {
			t.Fatalf("%s: %v\n%s", name, err, out)
		}
		return string(out)
	}
	run("clang", append(append([]string{"--target=x86_64-linux-gnu"}, EngineCFlags...), "-S", "-emit-llvm", "-o", ir, filepath.Join("testdata", "varargs", "escapes.c"))...)
	out := run("opt", "-load-pass-plugin", pass, "-passes=goasm-shadow-stack", "-shadow-add-sp", "-shadow-varargs", "-shadow-keep-going", "-shadow-report", "-S", "-o", os.DevNull, ir)
	rep := ParseReport(out)
	for fn, want := range map[string]string{
		"read_over":       "over-aligned",
		"through_pointer": "cannot follow",
		"external":        "cannot follow",
		"stores_list":     "stores a va_list",
		"stores_block":    "stores a pointer into the variadic block",
		"copies_out":      "not a local",
	} {
		if why := rep.Rejected[fn]; !strings.Contains(why, want) {
			t.Errorf("%s: rejection %q, want one saying %q", fn, why, want)
		}
	}
	for _, fn := range []string{"plain", "read_long", "copied"} {
		if why, ok := rep.Rejected[fn]; ok {
			t.Errorf("%s rejected: %s", fn, why)
		}
	}
	if t.Failed() {
		t.Logf("pass output:\n%s", out)
	}
}

// TestVarargsRules runs testdata/varargs/rules.c through the pass for both
// targets. Each varargs rule has a function that breaks it, which the pass
// must reject for that rule, and the uses it must keep stay: a va_list
// passed by reference and a va_copy. The kept function's va_start must set the offsets that make
// every va_arg take the overflow path. callargs.ll passes arguments C would
// have promoted. Needs LLVM
// 22.1.8 (SLOWJS_LLVM) and the pass (SLOWJS_SHADOW_PASS).
func TestVarargsRules(t *testing.T) {
	llvm, pass := os.Getenv("SLOWJS_LLVM"), os.Getenv("SLOWJS_SHADOW_PASS")
	if llvm == "" || pass == "" {
		t.Skip("set SLOWJS_LLVM and SLOWJS_SHADOW_PASS")
	}
	work := t.TempDir()
	run := func(name string, args ...string) (string, error) {
		out, err := exec.Command(filepath.Join(llvm, name), args...).CombinedOutput()
		return string(out), err
	}
	must := func(name string, args ...string) string {
		t.Helper()
		out, err := run(name, args...)
		if err != nil {
			t.Fatalf("%s: %v\n%s", name, err, out)
		}
		return out
	}
	opt := func(ir, out string) (string, error) {
		return run("opt", "-load-pass-plugin", pass, "-passes=goasm-shadow-stack", "-shadow-add-sp", "-shadow-varargs",
			"-shadow-keep-going", "-shadow-report", "-S", "-o", out, ir)
	}
	rules := filepath.Join("testdata", "varargs", "rules.c")
	for _, tc := range []struct {
		arch   string
		cflags []string
		start  []string // what va_start writes into the offset fields
	}{
		{"x86_64", append([]string{"--target=x86_64-linux-gnu"}, EngineCFlags...), []string{"store i32 48", "store i32 176"}},
		{"aarch64", ARM64CFlags, []string{"store i32 0", "store i32 0"}},
	} {
		ir, shadow := filepath.Join(work, tc.arch+".ll"), filepath.Join(work, tc.arch+".shadow.ll")
		must("clang", append(append([]string{}, tc.cflags...), "-S", "-emit-llvm", "-o", ir, rules)...)
		out, err := opt(ir, shadow)
		if err != nil {
			t.Fatalf("%s: %v\n%s", tc.arch, err, out)
		}
		rep := ParseReport(out)
		for fn, want := range map[string]string{
			"aggr":   "more than one 8-byte slot",
			"narrow": "not a promoted int, i64, pointer or double",
		} {
			if why := rep.Rejected[fn]; !strings.Contains(why, want) {
				t.Errorf("%s: %s: rejection %q, want one saying %q", tc.arch, fn, why, want)
			}
		}
		for _, fn := range []string{"kept", "by_ref", "use"} {
			if why, ok := rep.Rejected[fn]; ok {
				t.Errorf("%s: %s rejected: %s", tc.arch, fn, why)
			}
		}
		ll, err := os.ReadFile(shadow)
		if err != nil {
			t.Fatal(err)
		}
		kept := regexp.MustCompile(`(?s)\ndefine [^\n]*@kept\(.*?\n}\n`).FindString(string(ll))
		for _, w := range tc.start {
			if strings.Count(kept, w) < strings.Count(strings.Join(tc.start, "\n"), w) {
				t.Errorf("%s: kept's va_start does not %s:\n%s", tc.arch, w, kept)
			}
		}
		if t.Failed() {
			t.Logf("%s: pass output:\n%s", tc.arch, out)
		}
	}
	callargs := filepath.Join("testdata", "varargs", "callargs.ll")
	out := must("opt", "-load-pass-plugin", pass, "-passes=goasm-shadow-stack", "-shadow-add-sp", "-shadow-varargs",
		"-shadow-keep-going", "-shadow-report", "-S", "-o", os.DevNull, callargs)
	rep := ParseReport(out)
	for _, fn := range []string{"passes_float", "passes_byte"} {
		if why := rep.Rejected[fn]; !strings.Contains(why, "not a promoted int, i64, pointer or double") {
			t.Errorf("%s: rejection %q", fn, why)
		}
	}
	if why, ok := rep.Rejected["passes_promoted"]; ok {
		t.Errorf("passes_promoted rejected: %s", why)
	}
}
