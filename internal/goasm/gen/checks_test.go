// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
package gen

import (
	"strings"
	"testing"
)

// Each static rule has a small example it must reject.

func TestCheckIRRejects(t *testing.T) {
	cases := map[string]string{
		"variadic define":          `define i32 @f(ptr %a, ...) { ret i32 0 }`,
		"va_start":                 `  call void @llvm.va_start(ptr %ap)`,
		"byval":                    `define void @g(ptr byval(%struct.Big) %p) { ret void }`,
		"sret":                     `define void @h(ptr sret(%struct.Big) %r) { ret void }`,
		"long double":              `  %x = fadd x86_fp80 %a, %b`,
		"variadic with attributes": `define dso_local i64 @f(ptr readnone captures(none) %0, i64 noundef returned %1, ...) {`,
	}
	for name, ll := range cases {
		if err := CheckIR(ll); err == nil {
			t.Errorf("%s: expected rejection", name)
		}
	}
	if err := CheckIR(`define i64 @ok(ptr %sp, i64 %n) { ret i64 %n }`); err != nil {
		t.Errorf("valid IR rejected: %v", err)
	}
}

// A canonical function wrapper so LowerUnified reaches the body with the bad
// instruction between a valid prologue and epilogue.
func wrap(body string) string {
	// A harmless body instruction first so the bad one lands in the body, not
	// in the prologue's local-reservation scan.
	return "foo:\n\tpushq\t%rbp\n\tmovq\t%rsp, %rbp\n\tmovq\t%rdi, %rax\n\t" + body +
		"\n\tpopq\t%rbp\n\tretq\n.Lfunc_end0:\n"
}

func TestLowerUnifiedRejects(t *testing.T) {
	SetCallees([]FuncSpec{{Name: "foo", Kind: Unified, Sig: Sig{Params: []Type{Ptr}, Ret: I64}}})
	fn := FuncSpec{Name: "foo", Kind: Unified, Sig: Sig{Params: []Type{Ptr}, Ret: I64}}
	bad := map[string]string{
		"syscall":           "syscall",
		"rdtsc":             "rdtsc",
		"cpuid":             "cpuid",
		"rdrand":            "rdrand\t%rax",
		"SP write":          "subq\t$16, %rsp",
		"frame spill (rbp)": "movq\t%rax, -8(%rbp)",
		"escaping stack":    "leaq\t-8(%rsp), %rax",
		"indirect jump mem": "jmpq\t*8(%rax)",
		"indirect call mem": "callq\t*8(%rax)",
		"tail jump":         "jmp\tother",
		"SP outside locals": "movq\t%rax, 8(%rsp)",
		"read-only symbol":  "movq\ttable(%rip), %rax",
	}
	for name, body := range bad {
		if _, _, err := LowerUnified(AMD64(), wrap(body), fn); err == nil {
			t.Errorf("%s: expected rejection", name)
		}
	}
}

// With a 32-byte local area: a folded packed SSE operand on the stack needs
// 16-byte alignment the Go stack does not give, so it is rejected, while an
// aligned move is rewritten to its unaligned form.
func TestStackAlignment(t *testing.T) {
	SetCallees([]FuncSpec{{Name: "foo", Kind: Unified, Sig: Sig{Params: []Type{Ptr}, Ret: I64}}})
	fn := FuncSpec{Name: "foo", Kind: Unified, Sig: Sig{Params: []Type{Ptr}, Ret: I64}}
	wrapLocal := func(body string) string {
		return "foo:\n\tpushq\t%rbp\n\tmovq\t%rsp, %rbp\n\tsubq\t$32, %rsp\n\tmovq\t%rdi, %rax\n\t" + body +
			"\n\taddq\t$32, %rsp\n\tpopq\t%rbp\n\tretq\n.Lfunc_end0:\n"
	}
	if _, _, err := LowerUnified(AMD64(), wrapLocal("addpd\t16(%rsp), %xmm0"), fn); err == nil {
		t.Error("folded packed stack operand accepted")
	}
	text, _, err := LowerUnified(AMD64(), wrapLocal("movaps\t%xmm0, 16(%rsp)"), fn)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "movups %xmm0") || strings.Contains(text, "movaps") {
		t.Errorf("aligned spill not rewritten:\n%s", text)
	}
}
