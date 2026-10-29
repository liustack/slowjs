// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
package gen

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestEmitLeafFrame: a leaf that saves a callee-saved register must be
// emitted with a frame for that save, through either emit entry, or its
// store would overwrite the return address. Needs LLVM 22.1.8 (SLOWJS_LLVM).
func TestEmitLeafFrame(t *testing.T) {
	llvm := os.Getenv("SLOWJS_LLVM")
	if llvm == "" {
		t.Skip("set SLOWJS_LLVM")
	}
	fn := FuncSpec{Name: "savingleaf", Sig: Sig{Params: []Type{I64}, Ret: I64}, Kind: Register}
	SetCallees([]FuncSpec{fn})
	asm := "savingleaf:\n\tpushq\t%rbx\n\tmovq\t%rdi, %rbx\n\tmovq\t%rbx, %rax\n\tpopq\t%rbx\n\tretq\n.Lfunc_end0:\n"
	text, m, err := LowerRegister(asm, fn)
	if err != nil {
		t.Fatal(err)
	}
	if m.Frame != 8 {
		t.Fatalf("frame %d, want 8", m.Frame)
	}
	work := t.TempDir()
	srcPath := filepath.Join(work, "l.s")
	if err := os.WriteFile(srcPath, []byte(".text\n.p2align 4\n.globl savingleaf\n.type savingleaf,@function\n"+
		text+".size savingleaf, .-savingleaf\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	obj := filepath.Join(work, "l.o")
	if out, err := exec.Command(filepath.Join(llvm, "clang"), "-c", srcPath, "-o", obj).CombinedOutput(); err != nil {
		t.Fatalf("assemble: %v\n%s", err, out)
	}
	asmGo, err := Emit(obj, []Meta{m}, NameMap{"savingleaf": m.GoName}, "amd64")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(asmGo, "TEXT ·savingleaf(SB), NOSPLIT, $8-0") {
		i := strings.Index(asmGo, "TEXT ·savingleaf")
		t.Fatalf("leaf frame not declared:\n%s", asmGo[i:i+60])
	}
}

// TestEmitNoStubs: a release build never emits a stub. EmitEngine stops on
// one before it reads the object (the path here does not exist), naming the
// functions, and without NoStubs the same input gets past that check.
func TestEmitNoStubs(t *testing.T) {
	e := EngineEmit{Stubs: []Meta{{Name: "js_atomics_pause"}, {Name: "other"}}, Arch: "amd64"}
	_, err := EmitEngine("no-such-object.o", e)
	if err == nil || strings.Contains(err.Error(), "release") {
		t.Fatalf("without NoStubs: %v", err)
	}
	e.NoStubs = true
	_, err = EmitEngine("no-such-object.o", e)
	if err == nil || !strings.Contains(err.Error(), "no stub") || !strings.Contains(err.Error(), "js_atomics_pause other") {
		t.Fatalf("with NoStubs: %v", err)
	}
}

// TestBuildExpr: the build line of a generated .s file is the architecture,
// and with a tag set it also requires the tag, so that a package whose files
// all need the tag has no file in a build without it.
func TestBuildExpr(t *testing.T) {
	for _, c := range []struct{ tag, arch, want string }{
		{"", "amd64", "amd64"},
		{"", "arm64", "arm64"},
		{"slowjs_cengine", "amd64", "slowjs_cengine && amd64"},
		{"slowjs_cengine", "arm64", "slowjs_cengine && arm64"},
	} {
		if got := BuildExpr(c.tag, c.arch); got != c.want {
			t.Errorf("BuildExpr(%q, %q) = %q, want %q", c.tag, c.arch, got, c.want)
		}
	}
}

// TestFuncPlace: a function starts in the lowered object where its body will
// sit in the binary, on 16 bytes or at its -align64 place in a line of 64, so
// that the alignment of its constant pool holds where the code runs.
func TestFuncPlace(t *testing.T) {
	a64 := map[string]int{"interp": 40, "matcher": 16, "zero": 0}
	for name, want := range map[string]string{
		"plain":   ".p2align 4\n",
		"interp":  ".p2align 6\n.skip 40\n",
		"matcher": ".p2align 6\n.skip 16\n",
		"zero":    ".p2align 6\n",
	} {
		if got := FuncPlace(a64, name); got != want {
			t.Errorf("%s: %q, want %q", name, got, want)
		}
	}
	if got := FuncPlace(nil, "interp"); got != ".p2align 4\n" {
		t.Errorf("no -align64: %q", got)
	}
}

// TestCheckBodyDataAlign: the generator stops at data in the body of a
// function that asks for more alignment than FuncPlace keeps (16 bytes, 64 for
// a body put on 64 bytes), also when the post-link check would find it on its
// alignment by luck.
func TestCheckBodyDataAlign(t *testing.T) {
	plan := &DataPlan{InBody: map[string][]*DataObj{
		"plain":   {{Label: ".LCPI0_0", Align: 4}, {Label: ".LJTI0_0", Align: 3}},
		"wide":    {{Label: ".LCPI1_0", Align: 5}},
		"placed":  {{Label: ".LCPI2_0", Align: 6}},
		"toowide": {{Label: ".LCPI3_0", Align: 7}},
		"other":   {{Label: ".LCPI4_0", Align: 6}}, // not lowered: not looked at
	}}
	a64 := map[string]int{"placed": 40, "toowide": 8}
	metas := func(names ...string) []Meta {
		var ms []Meta
		for _, n := range names {
			ms = append(ms, Meta{Name: n})
		}
		return ms
	}
	if err := CheckBodyDataAlign(plan, metas("plain", "placed"), a64); err != nil {
		t.Errorf("data within what is kept: %v", err)
	}
	for name, want := range map[string]string{
		"wide":    "wide: .LCPI1_0 in its body asks for 32 bytes of alignment, and the body keeps 16",
		"toowide": "toowide: .LCPI3_0 in its body asks for 128 bytes of alignment, and the body keeps 64",
	} {
		if err := CheckBodyDataAlign(plan, metas("plain", name), a64); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: %v, want %q", name, err, want)
		}
	}
	// A body put on 64 bytes keeps 64, a plain one 16.
	if got := [2]int{BodyDataAlign(a64, "placed"), BodyDataAlign(a64, "plain")}; got != [2]int{6, 4} {
		t.Errorf("kept alignments %v, want [6 4]", got)
	}
}
