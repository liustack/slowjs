// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
package gen

import "testing"

// Each static rule has a small example it must reject.

func TestCheckIRRejects(t *testing.T) {
	cases := map[string]string{
		"variadic define": `define i32 @f(ptr %a, ...) { ret i32 0 }`,
		"va_start":        `  call void @llvm.va_start(ptr %ap)`,
		"byval":           `define void @g(ptr byval(%struct.Big) %p) { ret void }`,
		"sret":            `define void @h(ptr sret(%struct.Big) %r) { ret void }`,
		"long double":     `  %x = fadd x86_fp80 %a, %b`,
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
	}
	for name, body := range bad {
		if _, _, err := LowerUnified(AMD64(), wrap(body), fn); err == nil {
			t.Errorf("%s: expected rejection", name)
		}
	}
}
