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
	run("clang", append(append([]string{}, EngineCFlags...), "-S", "-emit-llvm", "-o", ir, filepath.Join("testdata", "varargs", "overaligned.c"))...)
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
	run("clang", append(append([]string{}, EngineCFlags...), "-S", "-emit-llvm", "-o", ir, filepath.Join("testdata", "varargs", "overaligned_helper.c"))...)
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
	run("clang", append(append([]string{}, EngineCFlags...), "-S", "-emit-llvm", "-o", ir, filepath.Join("testdata", "varargs", "escapes.c"))...)
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
// passed by value (AArch64 copies it with memcpy) and by reference, and a
// va_copy. The kept function's va_start must set the offsets that make
// every va_arg take the overflow path. callargs.ll passes arguments C would
// have promoted. offsets.ll.in writes the offset fields through phis,
// selects and a loop: every write that may wrap back into the register save
// area is refused, the ones va_arg makes are kept. A module of any other
// target is refused whole. Needs LLVM
// 22.1.8 (SLOWJS_LLVM) and the pass (SLOWJS_SHADOW_PASS).
func TestVarargsRules(t *testing.T) {
	const offsetWrite = "cannot prove keeps va_arg off the register save area"
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
		triple string   // for offsets.ll.in
		gp     string   // byte offset of the general-register offset field
		reg    string   // with regk, a compare on it that holds only on va_arg's register path
		regk   string
		fp     string // the same for the floating-point offset field
		fpreg  string
		fpregk string
	}{
		{"x86_64", append([]string{}, EngineCFlags...), []string{"store i32 48", "store i32 176"},
			"x86_64-unknown-linux-gnu", "0", "ult", "41", "4", "ult", "161"},
		{"aarch64", ARM64CFlags, []string{"store i32 0", "store i32 0"},
			"aarch64-unknown-linux-gnu", "24", "slt", "0", "28", "slt", "0"},
	} {
		ir, shadow := filepath.Join(work, tc.arch+".ll"), filepath.Join(work, tc.arch+".shadow.ll")
		must("clang", append(append([]string{}, tc.cflags...), "-S", "-emit-llvm", "-o", ir, rules)...)
		out, err := opt(ir, shadow)
		if err != nil {
			t.Fatalf("%s: %v\n%s", tc.arch, err, out)
		}
		rep := ParseReport(out)
		for fn, want := range map[string]string{
			"aggr":       "more than one 8-byte slot",
			"narrow":     "not a promoted int, i64, pointer or double",
			"resets":     offsetWrite,
			"bump_big":   offsetWrite,
			"bump_small": offsetWrite,
		} {
			if why := rep.Rejected[fn]; !strings.Contains(why, want) {
				t.Errorf("%s: %s: rejection %q, want one saying %q", tc.arch, fn, why, want)
			}
		}
		for _, fn := range []string{"kept", "by_value", "by_ref", "wraps", "use"} {
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
		// Offset writes through phis, selects and a loop, with and without
		// a path on which the add wraps.
		tmpl, err := os.ReadFile(filepath.Join("testdata", "varargs", "offsets.ll.in"))
		if err != nil {
			t.Fatal(err)
		}
		offsets := filepath.Join(work, tc.arch+".offsets.ll")
		if err := os.WriteFile(offsets, []byte(strings.NewReplacer("@TRIPLE@", tc.triple, "@GP@", tc.gp, "@REG@", tc.reg, "@REGK@", tc.regk,
			"@FP@", tc.fp, "@FPREG@", tc.fpreg, "@FPREGK@", tc.fpregk).Replace(string(tmpl))), 0o644); err != nil {
			t.Fatal(err)
		}
		oout, err := opt(offsets, os.DevNull)
		if err != nil {
			t.Fatalf("%s: offsets: %v\n%s", tc.arch, err, oout)
		}
		orep := ParseReport(oout)
		for _, fn := range []string{"wrap_phi", "wrap_select", "wrap_loop", "wrap_guard", "wrap_fp", "wrap_nondom", "wrap_left", "wrap_false"} {
			if why := orep.Rejected[fn]; !strings.Contains(why, offsetWrite) {
				t.Errorf("%s: %s: rejection %q, want one saying %q", tc.arch, fn, why, offsetWrite)
			}
		}
		for _, fn := range []string{"dead_store", "dead_phi", "dead_loop", "guarded", "dead_fp", "guarded_left", "guarded_false"} {
			if why, ok := orep.Rejected[fn]; ok {
				t.Errorf("%s: %s rejected: %s", tc.arch, fn, why)
			}
		}
		if t.Failed() {
			t.Logf("%s: pass output:\n%s\n%s", tc.arch, out, oout)
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
	// A va_list that goes through a select, a phi or memory is refused: the
	// pass follows it only straight into the functions that read it.
	flows := must("opt", "-load-pass-plugin", pass, "-passes=goasm-shadow-stack", "-shadow-add-sp", "-shadow-varargs",
		"-shadow-keep-going", "-shadow-report", "-S", "-o", os.DevNull, filepath.Join("testdata", "varargs", "flows.ll"))
	frep := ParseReport(flows)
	for fn, want := range map[string]string{
		"via_select": "uses a va_list in a way the pass cannot follow",
		"via_phi":    "uses a va_list in a way the pass cannot follow",
		"via_memory": "stores a va_list's address to memory",
	} {
		if why := frep.Rejected[fn]; !strings.Contains(why, want) {
			t.Errorf("%s: rejection %q, want one saying %q", fn, why, want)
		}
	}
	for _, fn := range []string{"direct", "take"} {
		if why, ok := frep.Rejected[fn]; ok {
			t.Errorf("%s rejected: %s", fn, why)
		}
	}
	if t.Failed() {
		t.Logf("pass output:\n%s", flows)
	}
	// Any other target: the va_list layout is not one the pass writes.
	for _, target := range []string{"x86_64-pc-linux-gnu", "x86_64-linux-musl", "aarch64-apple-darwin", "i686-linux-gnu"} {
		ir := filepath.Join(work, target+".ll")
		must("clang", "--target="+target, "-O2", "-S", "-emit-llvm", "-o", ir, rules)
		out, err := opt(ir, os.DevNull)
		if err == nil || !strings.Contains(out, "only for x86_64-linux-gnu and aarch64-linux-gnu") {
			t.Errorf("%s: %v\n%s", target, err, out)
		}
	}
}

// TestVarargsNative compiles testdata/varargs/diff.c twice for each
// target, plainly with the target's own variable arguments and through the
// pass's variadic block, links each with the freestanding driver
// diff_main.c and requires the same output (no variadic code may be left
// in the IR llc gets): interleaved types, more
// arguments than registers, none and forty, va_copy, helpers that take the
// va_list by reference and by value, reads after a host callback that
// dirties the stack, a formatter, a variadic call from a variadic function
// and a call through a pointer. On linux/amd64 the whole generator then
// turns diff.c into Go assembly (cmd/engine, body and pn) and a Go program
// runs it, with a host callback in Go that grows and so moves the goroutine
// stack between reads: it must print the same again. It needs LLVM 22.1.8
// (SLOWJS_LLVM), the pass (SLOWJS_SHADOW_PASS) and ld.lld (SLOWJS_LLD).
// x86_64 runs on a linux/amd64 host, aarch64 on a linux/arm64 host or under
// the user-mode emulator SLOWJS_QEMU_AARCH64.
func TestVarargsNative(t *testing.T) {
	llvm, pass, lld := os.Getenv("SLOWJS_LLVM"), os.Getenv("SLOWJS_SHADOW_PASS"), os.Getenv("SLOWJS_LLD")
	if llvm == "" || pass == "" || lld == "" {
		t.Skip("set SLOWJS_LLVM, SLOWJS_SHADOW_PASS and SLOWJS_LLD")
	}
	src := filepath.Join("testdata", "varargs")
	free := []string{"-ffreestanding", "-nostdinc", "-fno-builtin", "-fno-stack-protector", "-fno-asynchronous-unwind-tables"}
	native := map[string]string{}
	for _, tc := range []struct {
		arch, goarch, qemu string
		cflags             []string
	}{
		{"x86_64", "amd64", "", append(append([]string{}, EngineCFlags...), free...)},
		{"aarch64", "arm64", os.Getenv("SLOWJS_QEMU_AARCH64"), ARM64CFlags},
	} {
		t.Run(tc.arch, func(t *testing.T) {
			var runner []string
			switch {
			case runtime.GOOS == "linux" && runtime.GOARCH == tc.goarch:
			case tc.qemu != "":
				runner = []string{tc.qemu}
			default:
				t.Skip("no " + tc.arch + " linux host or emulator")
			}
			work := t.TempDir()
			tool := func(name string, args ...string) string {
				t.Helper()
				out, err := exec.Command(name, args...).CombinedOutput()
				if err != nil {
					t.Fatalf("%s: %v\n%s", filepath.Base(name), err, out)
				}
				return string(out)
			}
			at := func(n string) string { return filepath.Join(work, n) }
			clang := filepath.Join(llvm, "clang")
			plain := append([]string{"--target=" + tc.arch + "-linux-gnu", "-O2"}, free...)
			tool(clang, append(append([]string{}, plain...), "-c", "-o", at("native.o"), filepath.Join(src, "diff.c"))...)
			tool(clang, append(append([]string{}, tc.cflags...), "-S", "-emit-llvm", "-o", at("diff.ll"), filepath.Join(src, "diff.c"))...)
			rep := ParseReport(tool(filepath.Join(llvm, "opt"), "-load-pass-plugin", pass, "-passes=goasm-shadow-stack",
				"-shadow-add-sp", "-shadow-varargs", "-shadow-report", "-S", "-o", at("diff.shadow.ll"), at("diff.ll")))
			got := map[string]bool{}
			for _, f := range rep.Varargs {
				got[f] = true
			}
			for _, f := range []string{"mixed", "count", "twice", "helpers", "across", "fmt", "wrap"} {
				if !got[f] {
					t.Fatalf("%s not rewritten: %v", f, rep.Varargs)
				}
			}
			// No variadic function, call or intrinsic is left in what llc gets.
			shadow, err := os.ReadFile(at("diff.shadow.ll"))
			if err != nil {
				t.Fatal(err)
			}
			for _, l := range strings.Split(string(shadow), "\n") {
				if reDefine.MatchString(l) || reVarCall.MatchString(l) || strings.Contains(l, "llvm.va_start") {
					t.Fatalf("variadic code left after the pass: %s", strings.TrimSpace(l))
				}
			}
			tool(filepath.Join(llvm, "llc"), "-O2", "-filetype=obj", "-o", at("pass.o"), at("diff.shadow.ll"))
			tool(clang, append(append([]string{}, plain...), "-c", "-o", at("main.o"), filepath.Join(src, "diff_main.c"))...)
			tool(clang, append(append([]string{}, plain...), "-DPASS", "-c", "-o", at("main_pass.o"), filepath.Join(src, "diff_main.c"))...)
			tool(lld, "-static", "-e", "_start", "-o", at("native"), at("main.o"), at("native.o"))
			tool(lld, "-static", "-e", "_start", "-o", at("pass"), at("main_pass.o"), at("pass.o"))
			exe := func(bin string) string { return tool(append(runner, bin)[0], append(runner, bin)[1:]...) }
			want, have := exe(at("native")), exe(at("pass"))
			if n := strings.Count(want, "\n"); n != 16 {
				t.Fatalf("the native run printed %d results:\n%s", n, want)
			}
			if have != want {
				t.Fatalf("the pass's block gives\n%s\nnative variable arguments give\n%s", have, want)
			}
			t.Logf("%s: %s", tc.arch, strings.ReplaceAll(strings.TrimSpace(want), "\n", " "))
			native[tc.arch] = want
		})
	}
	t.Run("x86_64 through Go", func(t *testing.T) {
		want := native["x86_64"]
		if want == "" || runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
			t.Skip("needs the native x86_64 run and a linux/amd64 host")
		}
		work := t.TempDir()
		goEnv := append(os.Environ(), "GOWORK=off", "CGO_ENABLED=0")
		tool := func(dir, name string, args ...string) string {
			t.Helper()
			cmd := exec.Command(name, args...)
			cmd.Dir, cmd.Env = dir, goEnv
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("%s %v: %v\n%s", filepath.Base(name), args, err, out)
			}
			return string(out)
		}
		drv := filepath.Join(work, "engine")
		tool("", "go", "build", "-o", drv, "./cmd/engine")
		c, err := filepath.Abs(filepath.Join(src, "diff.c"))
		if err != nil {
			t.Fatal(err)
		}
		for _, mode := range []string{"body", "pn"} {
			mod := filepath.Join(work, mode)
			tool("", drv, "-src", c, "-llvm", llvm, "-pass", pass, "-mode", mode, "-varargs-block", "-allow", "host_cb", "-non-format-variadic", "mixed,count,twice,helpers,across,wrap,fmt",
				"-leaves", "", "-work", filepath.Join(work, "w-"+mode), "-out", filepath.Join(mod, "vd"), "-pkg", "vd")
			for name, text := range map[string]string{
				"go.mod":     "module vdiff\n\ngo 1.22\n",
				"main.go":    "package main\n\nimport (\n\t\"fmt\"\n\n\t\"vdiff/vd\"\n)\n\nfunc main() {\n\tfor _, v := range vd.Run() {\n\t\tfmt.Println(v)\n\t}\n}\n",
				"vd/host.go": varargsGoHost,
			} {
				if err := os.WriteFile(filepath.Join(mod, name), []byte(text), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			tool(mod, "go", "build", "-o", "run", ".")
			if have := tool(mod, filepath.Join(mod, "run")); have != want {
				t.Fatalf("%s: Go runs the generated code to\n%s\nnative variable arguments give\n%s", mode, have, want)
			}
		}
	})
}

// varargsGoHost runs diff.c's run on a shadow region and a result buffer
// outside the Go heap. Its host callback grows the goroutine stack, which
// copies it, generated frames and all, before it returns the driver's
// value. host_poll is the samples' (samplePoll).
const varargsGoHost = `package vd

import "syscall"
import "unsafe"

func goasmMissing(i int64) { panic("missing: " + missingNames[i]) }
` + samplePoll + `

//go:noinline
func grow(d int, x int64) int64 {
	var junk [512]int64
	for i := range junk {
		junk[i] = x ^ (0x5a5a5a5a5a5a5a5a + int64(i))
	}
	if d == 0 {
		return junk[3] & 0xff
	}
	return (grow(d-1, x+junk[d&63]) & 0xffff)
}

func host_cb(sp, x uintptr) uintptr { return uintptr(grow(200, int64(x)) % 97) }

func Run() []int64 {
	m, err := syscall.Mmap(-1, 0, 4<<20, syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_PRIVATE|syscall.MAP_ANON)
	if err != nil {
		panic(err)
	}
	base := (uintptr(unsafe.Pointer(&m[0])) + 1<<20 - 1) &^ (1<<20 - 1)
	r := base + 2<<20
	n := int(run(base+4096, r, 1))
	out := make([]int64, n)
	for i := range out {
		out[i] = *(*int64)(unsafe.Pointer(r + uintptr(8*i)))
	}
	return out
}
`
