// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
package gen

import (
	"debug/elf"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/arch/x86/x86asm"
)

func TestICallID(t *testing.T) {
	for _, code := range []string{">v", "i>i", "ii>i", "iid>d", "iiii>j", "ij>v", "if>f", "iiiiiiiiiiii>j"} {
		id, ok := ICallID(code)
		if !ok || id>>31 != 1 {
			t.Fatalf("%s: id %#x, %v", code, id, ok)
		}
		back, err := ICallCode(id)
		if err != nil || back != code {
			t.Fatalf("%s: %#x decodes as %q, %v", code, id, back, err)
		}
	}
	// Different codes, different ids.
	a, _ := ICallID("ii>i")
	b, _ := ICallID("i>i")
	c, _ := ICallID("id>i")
	if a == b || a == c || b == c {
		t.Fatalf("ids collide: %#x %#x %#x", a, b, c)
	}
	for _, code := range []string{"iiiiiiiiiiiii>i", "iB>i", "i...>i", "i?>i", "i>?", "i", "i>ii"} {
		if id, ok := ICallID(code); ok || id != ICallUnencodable {
			t.Fatalf("%s: encoded as %#x", code, id)
		}
	}
	// An id the pass gives no signature, or the unencodable one, decodes
	// to an error.
	for _, id := range []uint32{0, ICallUnencodable, 0x7fffffff, 1<<31 | 1<<27 | 5<<24, 1<<31 | 1<<27 | 1<<24 | 1<<2} {
		if code, err := ICallCode(id); err == nil {
			t.Fatalf("%#x decodes as %q", id, code)
		}
	}
	for name, want := range map[string]string{
		"goasm.icall.f.0.2164260865":     "f 0 2164260865",
		"goasm.icall.cr_op.156.3.42":     "cr_op.156 3 42",
		"goasm.icall.f.x.1":              "",
		"goasm.icall.f.1":                "",
		"goasm.icall..0.1":               "",
		"other.f.0.1":                    "",
		"goasm.icall.f.0.99999999999999": "",
	} {
		fn, n, id, ok := ParseICallSymbol(name)
		got := ""
		if ok {
			got = fn + " " + strconv.Itoa(n) + " " + strconv.FormatUint(uint64(id), 10)
		}
		if got != want {
			t.Fatalf("%s: %q, want %q", name, got, want)
		}
	}
}

// TestICallSymbols compiles testdata/icall through the pass with
// -shadow-icall-sigs and llc with the plugin. Every indirect call in llc's
// object must have a symbol at its address carrying the signature the pass
// reported for that function's indirect calls, the text must have the label
// right before the call, and no symbol may sit anywhere else. Without the
// pass option, llc with the plugin must stop. It needs LLVM 22.1.8
// (SLOWJS_LLVM) and the built pass (SLOWJS_SHADOW_PASS).
func TestICallSymbols(t *testing.T) {
	llvm, pass := os.Getenv("SLOWJS_LLVM"), os.Getenv("SLOWJS_SHADOW_PASS")
	if llvm == "" || pass == "" {
		t.Skip("set SLOWJS_LLVM and SLOWJS_SHADOW_PASS")
	}
	if runtime.GOARCH != "amd64" {
		t.Skip("decodes amd64 code")
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
	ir, shadow := filepath.Join(work, "c.ll"), filepath.Join(work, "c.shadow.ll")
	must("clang", append(append([]string{"--target=x86_64-linux-gnu"}, EngineCFlags...), "-S", "-emit-llvm", "-o", ir,
		filepath.Join("testdata", "icall", "icall.c"))...)
	for _, mode := range []string{"body", "pn"} {
		args := []string{"-load-pass-plugin", pass, "-passes=goasm-shadow-stack", "-shadow-add-sp", "-shadow-report", "-shadow-icall-sigs"}
		if mode == "pn" {
			args = append(args, "-shadow-cc=preserve_none")
		}
		rep := ParseReport(must("opt", append(args, "-S", "-o", shadow, ir)...))
		asm, obj := filepath.Join(work, mode+".s"), filepath.Join(work, mode+".o")
		must("llc", "-O2", "-relocation-model=pic", "-enable-shrink-wrap=false", "-load", pass, "-o", asm, shadow)
		must("llc", "-O2", "-relocation-model=pic", "-enable-shrink-wrap=false", "-load", pass, "-filetype=obj", "-o", obj, shadow)
		text, err := os.ReadFile(asm)
		if err != nil {
			t.Fatal(err)
		}
		// The label is the line right before its call.
		labels := regexp.MustCompile(`(?m)^(goasm\.icall\.[\w.]+):\n\s*(\S+)\s+(\S+)`).FindAllStringSubmatch(string(text), -1)
		if len(labels) == 0 {
			t.Fatalf("%s: no indirect-call label in llc's text:\n%s", mode, text)
		}
		for _, m := range labels {
			if m[2] != "callq" || !strings.HasPrefix(m[3], "*%") {
				t.Fatalf("%s: %s labels %s %s, not an indirect call", mode, m[1], m[2], m[3])
			}
		}
		f, err := elf.Open(obj)
		if err != nil {
			t.Fatal(err)
		}
		syms, err := f.Symbols()
		if err != nil {
			t.Fatal(err)
		}
		code, err := f.Section(".text").Data()
		if err != nil {
			t.Fatal(err)
		}
		at := map[uint64]string{}
		got := map[string]map[string]bool{}
		for _, s := range syms {
			if !strings.HasPrefix(s.Name, ICallPrefix) {
				continue
			}
			fn, _, id, ok := ParseICallSymbol(s.Name)
			c, err := ICallCode(id)
			if !ok || err != nil || elf.ST_BIND(s.Info) != elf.STB_LOCAL || f.Sections[s.Section].Name != ".text" {
				t.Fatalf("%s: symbol %s: %v", mode, s.Name, err)
			}
			at[s.Value] = s.Name
			if got[fn] == nil {
				got[fn] = map[string]bool{}
			}
			got[fn][c] = true
		}
		calls := 0
		for pc := 0; pc < len(code); {
			in, err := x86asm.Decode(code[pc:], 64)
			if err != nil {
				t.Fatalf("%s: decode at %#x: %v", mode, pc, err)
			}
			_, reg := in.Args[0].(x86asm.Reg)
			name, sym := at[uint64(pc)]
			if in.Op == x86asm.CALL && reg {
				calls++
				if !sym {
					t.Fatalf("%s: the indirect call at %#x has no symbol", mode, pc)
				}
			} else if sym {
				t.Fatalf("%s: %s at %#x is on %v, not an indirect call", mode, name, pc, in)
			}
			pc += in.Len
		}
		if calls != len(at) || calls != len(labels) {
			t.Fatalf("%s: %d indirect calls, %d symbols, %d labels", mode, calls, len(at), len(labels))
		}
		for _, fn := range []string{"branches", "mixed", "pair", "loop"} {
			want := map[string]bool{}
			for _, c := range rep.ICalls[fn] {
				want[c] = true
			}
			if strings.Join(SortedKeys(got[fn]), " ") != strings.Join(SortedKeys(want), " ") || len(want) == 0 {
				t.Fatalf("%s: %s's calls carry %v, the pass reported %v", mode, fn, SortedKeys(got[fn]), SortedKeys(want))
			}
		}
		if n := len(got["branches"]); n != 2 {
			t.Fatalf("%s: branches has %d signatures, want 2", mode, n)
		}
	}
	// Without ids the plugin stops llc.
	must("opt", "-load-pass-plugin", pass, "-passes=goasm-shadow-stack", "-shadow-add-sp", "-S", "-o", shadow, ir)
	out, err := run("llc", "-O2", "-load", pass, "-filetype=obj", "-o", filepath.Join(work, "bare.o"), shadow)
	if err == nil || !strings.Contains(out, "indirect call without a signature id") {
		t.Fatalf("llc with the plugin took indirect calls without ids: %v\n%s", err, out)
	}
}
