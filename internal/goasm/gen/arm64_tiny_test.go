// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package gen

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestLowerARM64EngineTiny: in the tiny code model each reference to data is
// one instruction that names the whole address, and each becomes the same
// address on its own: an adr of other data or of a function is the
// MOVD $sym(SB) of Go, an adr of the function's own data stays, and a load of
// a label reads the copy in the function's body.
func TestLowerARM64EngineTiny(t *testing.T) {
	wrap := func(mid ...string) []string {
		out := []string{"str x30, [sp, #-16]!"}
		out = append(out, mid...)
		return append(out, "ldr x30, [sp], #16", "ret")
	}
	lower := func(lines []string) (string, error) {
		fn, env := a64TestEnv()
		env.Plan.Rename["f"]["copy"] = ".Ldup.f.copy"
		text, _, err := LowerARM64EngineFunc(lines, fn, env)
		return text, err
	}
	text, err := lower(wrap("adr x8, data+8", "ldr x9, [x8, #4]", "adr x10, .LCPI0_0", "ldr d0, .LCPI0_0+8",
		"ldr q1, copy+16", "adr x11, callee", "ldrsw x12, .LCPI0_0"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"adrp x8, __goasm_rodata+24\n\tadd x8, x8, :lo12:__goasm_rodata+24\n\tldr x9, [x8, #4]\n",
		"adr x10, .LCPI0_0\n",
		"ldr d0, .LCPI0_0+8\n",
		"ldr q1, .Ldup.f.copy+16\n",
		"adrp x11, go_callee\n\tadd x11, x11, :lo12:go_callee\n",
		"ldrsw x12, .LCPI0_0\n",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("no %q in\n%s", want, text)
		}
	}
	for name, c := range map[string]struct {
		lines []string
		want  string
	}{
		"load of data not in the body": {wrap("ldr d0, data"), "not in the body"},
		"unplaced data":                {wrap("adr x8, other"), "unplaced data"},
		"offset from a function":       {wrap("adr x8, callee+4"), "address of the function"},
		"GOT":                          {wrap("ldr x0, :got:data"), "unreviewed relocation form"},
		"prefetch of a label":          {wrap("prfm pldl1keep, data"), "unreviewed reference"},
	} {
		if _, err := lower(c.lines); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v, want %q", name, err, c.want)
		}
	}
}

// TestPlanDataARM64Tiny: an adr takes the address of data, which goes into
// engine_rodata, and a load of a label reads it, which puts a copy of it in
// the body of each function that loads it, as a load through the PC does on
// amd64.
func TestPlanDataARM64Tiny(t *testing.T) {
	asm := ".text\n.globl f\nf:\n\tadr\tx8, tbl\n\tldr\td0, cst+8\n\tldr\tq1, .LCPI0_0\n\tret\n.Lfunc_end0:\n" +
		".section\t.rodata.cst16,\"aM\",@progbits,16\n\t.p2align\t4, 0x0\n.LCPI0_0:\n\t.xword\t1\n\t.xword\t2\n" +
		".section\t.rodata,\"a\",@progbits\n\t.p2align\t3, 0x0\ntbl:\n\t.xword\t3\ncst:\n\t.xword\t4\n\t.xword\t5\n"
	u, err := SplitAsm(asm, "//")
	if err != nil {
		t.Fatal(err)
	}
	p := PlanDataARM64(u, map[string]bool{"f": true})
	if len(p.Errors) != 0 {
		t.Fatalf("errors: %v", p.Errors)
	}
	if _, ok := p.BlobOff["tbl"]; !ok {
		t.Errorf("tbl, whose address is taken, is not in engine_rodata: %v", p.BlobOff)
	}
	if _, ok := p.BlobOff["cst"]; ok {
		t.Errorf("cst, which is only loaded, is in engine_rodata: %v", p.BlobOff)
	}
	if got := p.Rename["f"]["cst"]; got != ".Ldup.f.cst" {
		t.Errorf("cst in f's body as %q", got)
	}
	if got := p.Rename["f"][".LCPI0_0"]; got != ".LCPI0_0" {
		t.Errorf(".LCPI0_0 in f's body as %q", got)
	}
}

// TestCheckObjectFunctionSections: llc writes the arm64 object with a
// section for each function (an adr between two functions of the tiny code
// model may be too far to resolve in one section), so the check of the
// object reads the relocations of every section of code, not only .text. It
// needs clang (SLOWJS_LLVM).
func TestCheckObjectFunctionSections(t *testing.T) {
	llvm := os.Getenv("SLOWJS_LLVM")
	if llvm == "" {
		t.Skip("set SLOWJS_LLVM")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "f.c")
	os.WriteFile(src, []byte("extern int ext(int);\nint f(int x) { return ext(x) + 1; }\n"), 0o644)
	obj := filepath.Join(dir, "f.o")
	if out, err := exec.Command(filepath.Join(llvm, "clang"), "--target=aarch64-linux-gnu", "-O2", "-fPIC",
		"-fno-optimize-sibling-calls", "-ffunction-sections", "-c", "-o", obj, src).CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if err := CheckObject(obj, map[string]bool{"f": true}); err == nil || !strings.Contains(err.Error(), "outside the allow list") {
		t.Errorf("a call in .text.f to a name off the list: %v", err)
	}
	if err := CheckObject(obj, map[string]bool{"f": true, "ext": true}); err != nil {
		t.Errorf("a call to a name on the list: %v", err)
	}
}

// TestPlanDataARM64RefusesPages: the page forms of the small code model stop
// the plan, whose references would otherwise miss the data they reach.
func TestPlanDataARM64RefusesPages(t *testing.T) {
	for _, l := range []string{"adrp\tx8, tbl", "add\tx8, x8, :lo12:tbl", "ldr\tx0, [x8, :lo12:tbl]"} {
		asm := ".text\n.globl f\nf:\n\t" + l + "\n\tret\n.Lfunc_end0:\n" +
			".section\t.rodata,\"a\",@progbits\n\t.p2align\t3, 0x0\ntbl:\n\t.xword\t3\n"
		u, err := SplitAsm(asm, "//")
		if err != nil {
			t.Fatal(err)
		}
		p := PlanDataARM64(u, map[string]bool{"f": true})
		if !strings.Contains(fmt.Sprint(p.Errors), "tiny code model") {
			t.Errorf("%q: %v", l, p.Errors)
		}
	}
}
