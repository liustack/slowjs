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
