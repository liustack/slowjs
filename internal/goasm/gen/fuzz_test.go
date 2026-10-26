// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
package gen

import "testing"

// FuzzEncodeDirective feeds arbitrary bytes to the data-directive parser. It
// may return an error, but it must never panic: a bad alignment, count,
// string or number from the input is rejected, not crashed on.
func FuzzEncodeDirective(f *testing.F) {
	for _, s := range []string{
		".byte 1", ".quad 0x8000000000000000", ".short -2", ".long foo+8",
		".zero 3,7", ".zero -3", ".ascii \"a b\\t\"", ".asciz \"\\x41\\0\"",
		".p2align 3, 0x7f", ".p2align 4, 0, 3", ".p2align -1", ".p2align 64",
		".p2align 3, 0, 0", ".string \"\"", "", ".byte", ".quad 1table",
	} {
		f.Add(s, 1)
	}
	f.Fuzz(func(t *testing.T, line string, at int) {
		if at < 0 || at > 1<<20 {
			at &= 0xffff
		}
		_, _, _ = encodeDirective(line, at)
	})
}

// FuzzEvalExpr feeds arbitrary expressions to the symbol-expression parser.
// It must never panic: anything it does not understand is an error.
func FuzzEvalExpr(f *testing.F) {
	for _, s := range []string{"table+8", "table-base", "table+8*2", "1table",
		"table+1 2", ".-foo", "foo+0x10-bar", "", "+", "-", "+-+", "0x"} {
		f.Add(s)
	}
	p := &DataPlan{BlobOff: map[string]int{"table": 16, "base": 4, "foo": 0, "bar": 8}}
	f.Fuzz(func(t *testing.T, e string) {
		_, _, _ = p.evalExpr(e, 0, map[string]bool{})
	})
}

// FuzzParse feeds arbitrary text to the whole text pipeline the generator
// runs on llc's output: SplitAsm, PlanData and both lowerings of every
// function it finds. Each stage may reject the input, but none may panic.
func FuzzParse(f *testing.F) {
	for _, s := range []string{
		".section .rodata\n.p2align 64\na:\n.byte 1\n",
		".section .rodata\n.p2align -1\na:\n.byte 1\n",
		".text\n.globl f\nf:\n\tpushq\t%rbp\n\tmovq\t%rsp, %rbp\n\tleaq\ta(%rip), %rax\n\tpopq\t%rbp\n\tretq\n.Lfunc_end0:\n.section .rodata\na:\n.byte 1\n.p2align 3, 0, 1\nb:\n.quad a+8\n",
		".text\nf:\n.Lfunc_end0:\n",
		".text\nf:\n\tpushq\t%rbp\n.Lfunc_end0:\n",
		".text\nf:\n\tpushq\t%rbp\n\tmovq\t%rsp, %rbp\n\tsubq\t$8, %rsp\n\tmovq\t-8(%rbp), %rax\n\tjmpq\t*%rax\n.Lfunc_end0:\n",
		".section .rodata\na:\n.ascii \"\\x\"\n",
		".set0", ":", ".globl", ".section", ".p2alignx 3",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, asm string) {
		u, err := SplitAsm(asm, "#")
		if err != nil {
			return
		}
		funcs := map[string]bool{}
		for _, n := range u.FuncOrder {
			funcs[n] = true
		}
		p := PlanData(u, funcs)
		env := &LowerEnv{Plan: p, Funcs: funcs}
		SetCallees(nil)
		for _, n := range u.FuncOrder {
			_, _, _ = LowerUnifiedFunc(AMD64(), u.Funcs[n].Lines, FuncSpec{Name: n, Sig: Sig{Params: []Type{Ptr}, Ret: I64}}, env, "")
			_, _, _ = LowerRegister(asm, FuncSpec{Name: n, Sig: Sig{Params: []Type{Ptr, I64}, Ret: Ptr}, Kind: Register})
		}
	})
}
