// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
package gen

import (
	"strings"
	"testing"
)

// A register-entry leaf must not touch RBP: with frame-pointer=none llc could
// otherwise use it as a value register while Go owns it as the frame pointer.
func TestLeafRejectsBasePointer(t *testing.T) {
	fn := FuncSpec{Name: "bpleaf", Sig: Sig{Params: []Type{I64}, Ret: I64}, Kind: Register}
	SetCallees([]FuncSpec{fn})
	asm := "bpleaf:\n\tpushq\t%rbp\n\tmovq\t%rdi, %rbp\n\tmovq\t%rbp, %rax\n\tpopq\t%rbp\n\tretq\n.Lfunc_end0:\n"
	if _, _, err := LowerRegister(asm, fn); err == nil {
		t.Fatal("leaf saving and using RBP accepted")
	}
	body := "bpleaf2:\n\tmovq\t%rdi, %rbp\n\tmovq\t%rbp, %rax\n\tretq\n.Lfunc_end0:\n"
	fn.Name = "bpleaf2"
	SetCallees([]FuncSpec{fn})
	if _, _, err := LowerRegister(body, fn); err == nil || !strings.Contains(err.Error(), "base pointer") {
		t.Fatalf("leaf using RBP as a value register accepted: %v", err)
	}
}
