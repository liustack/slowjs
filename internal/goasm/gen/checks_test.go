// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
package gen

import (
	"sort"
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

// With a 32-byte local area: an aligned move or a folded packed SSE operand
// on the stack needs 16-byte alignment the Go stack does not give, so both
// are rejected, while an unaligned move is kept.
func TestStackAlignment(t *testing.T) {
	SetCallees([]FuncSpec{{Name: "foo", Kind: Unified, Sig: Sig{Params: []Type{Ptr}, Ret: I64}}})
	fn := FuncSpec{Name: "foo", Kind: Unified, Sig: Sig{Params: []Type{Ptr}, Ret: I64}}
	wrapLocal := func(body string) string {
		return "foo:\n\tpushq\t%rbp\n\tmovq\t%rsp, %rbp\n\tsubq\t$32, %rsp\n\tmovq\t%rdi, %rax\n\t" + body +
			"\n\taddq\t$32, %rsp\n\tpopq\t%rbp\n\tretq\n.Lfunc_end0:\n"
	}
	for _, body := range []string{"addpd\t16(%rsp), %xmm0", "movaps\t%xmm0, 16(%rsp)", "movdqa\t-16(%rbp), %xmm1"} {
		if _, _, err := LowerUnified(AMD64(), wrapLocal(body), fn); err == nil || !strings.Contains(err.Error(), "16-byte alignment") {
			t.Errorf("%s: %v", body, err)
		}
	}
	text, _, err := LowerUnified(AMD64(), wrapLocal("movups\t%xmm0, 16(%rsp)"), fn)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(strings.Fields(text), " "), "movups %xmm0, 16(%rsp)") {
		t.Errorf("unaligned spill not kept:\n%s", text)
	}
}

// forbiddenAMD64 are the instructions the plan bans, as llc writes them
// (AT&T, with the operand-size suffix of the register forms), and the ones
// beside them that reach the system or read the machine.
var forbiddenAMD64 = map[string]string{
	"syscall":  "syscall",
	"sysenter": "sysenter",
	"int 0x80": "int\t$0x80",
	"int 3":    "int3",
	"rdtsc":    "rdtsc",
	"rdtscp":   "rdtscp",
	"rdrand":   "rdrandq\t%rax",
	"rdrand32": "rdrandl\t%eax",
	"rdseed":   "rdseedq\t%rax",
	"rdseed32": "rdseedl\t%eax",
	"cpuid":    "cpuid",
	"rdpmc":    "rdpmc",
	"sysretq":  "sysretq",
	// The traps: SIGILL, SIGTRAP and a general protection fault, which end a Go
	// process or (hlt) cannot be told from a bad pointer.
	"ud2": "ud2",
	"ud1": "ud1\t%eax, %eax",
	"ud0": "ud0\t%eax, %eax",
	"hlt": "hlt",
}

// Each forbidden instruction is rejected in the body of a unified function
// and in a register-entry leaf.
func TestForbiddenInstructionsAMD64(t *testing.T) {
	fn := FuncSpec{Name: "foo", Kind: Unified, Sig: Sig{Params: []Type{Ptr}, Ret: I64}}
	for name, ins := range forbiddenAMD64 {
		SetCallees([]FuncSpec{fn})
		if _, _, err := LowerUnified(AMD64(), wrap(ins), fn); err == nil || !strings.Contains(err.Error(), "forbidden instruction") {
			t.Errorf("%s in a unified function: %v", name, err)
		}
		leaf := FuncSpec{Name: "foo", Kind: Register, Sig: fn.Sig}
		SetCallees([]FuncSpec{leaf})
		asm := "foo:\n\tmovq\t%rdi, %rax\n\t" + ins + "\n\tretq\n.Lfunc_end0:\n"
		if _, _, err := LowerRegister(asm, leaf); err == nil || !strings.Contains(err.Error(), "forbidden instruction") {
			t.Errorf("%s in a leaf: %v", name, err)
		}
	}
}

// arm64: the instructions that call the system or read machine state (svc,
// the other exception calls, and mrs and msr, which also read the counter
// and the random number register), and the traps (brk, udf, hlt), which
// are SIGTRAP and SIGILL and end a Go process.
func TestForbiddenInstructionsARM64(t *testing.T) {
	fn := FuncSpec{Name: "foo", Kind: Unified, Sig: Sig{Params: []Type{I64}, Ret: I64}}
	SetCallees([]FuncSpec{fn})
	for name, ins := range map[string]string{
		"svc": "svc\t#0", "hvc": "hvc\t#0", "smc": "smc\t#0",
		"mrs counter": "mrs\tx0, CNTVCT_EL0", "mrs random": "mrs\tx0, RNDR", "msr": "msr\tTPIDR_EL0, x0",
		"brk": "brk\t#0x1", "udf": "udf\t#0", "hlt": "hlt\t#0", "dcps1": "dcps1",
	} {
		asm := "foo:\n\tmov\tx1, x0\n\t" + ins + "\n\tret\n.Lfunc_end0:\n"
		_, err := LowerARM64(asm, "", []FuncSpec{fn}, ARM64SaveBody, false)
		if err == nil || !strings.Contains(err.Error(), "forbidden instruction") {
			t.Errorf("%s: %v", name, err)
		}
	}
	ok := "foo:\n\tmov\tx1, x0\n\tadd\tx0, x1, #1\n\tret\n.Lfunc_end0:\n"
	if _, err := LowerARM64(ok, "", []FuncSpec{fn}, ARM64SaveBody, false); err != nil {
		t.Errorf("a plain function: %v", err)
	}
}

// A trap does not end a function any more: the code before the constant pool of
// a function must end in a branch, and a brk there is refused like any other.
func TestTrapBeforePoolARM64(t *testing.T) {
	fn := FuncSpec{Name: "foo", Kind: Unified, Sig: Sig{Params: []Type{I64}, Ret: I64}}
	SetCallees([]FuncSpec{fn})
	asm := "foo:\n\tadrp\tx8, .LCPI0_0\n\tldr\td0, [x8, :lo12:.LCPI0_0]\n\tbrk\t#0x1\n.Lfunc_end0:\n" +
		"\t.p2align\t3\n.LCPI0_0:\n\t.xword\t0x4000000000000000\n"
	if _, err := LowerARM64(asm, "", []FuncSpec{fn}, ARM64SaveBody, false); err == nil || !strings.Contains(err.Error(), "forbidden instruction") {
		t.Errorf("a brk ending a function with a pool: %v", err)
	}
}

// The words of the assembled object: EmitARM64 refuses the exception-generating
// class and udf, whatever the text said.
func TestForbiddenWordsARM64(t *testing.T) {
	for _, c := range []struct {
		name string
		w    uint32
		bad  bool
	}{
		{"brk #0x1", 0xd4200020, true},
		{"brk #0", 0xd4200000, true},
		{"udf #0", 0x00000000, true},
		{"udf #0xffff", 0x0000ffff, true},
		{"hlt #0", 0xd4400000, true},
		{"svc #0", 0xd4000001, true},
		{"hvc #0", 0xd4000002, true},
		{"smc #0", 0xd4000003, true},
		{"dcps1", 0xd4a00001, true},
		{"nop", 0xd503201f, false},
		{"ret", 0xd65f03c0, false},
		{"add x0, x1, #1", 0x91000420, false},
		{"ldr x0, [sp, #8]", 0xf94007e0, false},
		{"b .+4", 0x14000001, false},
		{"bl .+4", 0x94000001, false},
		{"mov w0, #1", 0x52800020, false},
		{"br x0", 0xd61f0000, false},
	} {
		if why := a64ForbiddenWord(c.w); (why != "") != c.bad {
			t.Errorf("%s (%#08x): %q, bad is %v", c.name, c.w, why, c.bad)
		}
	}
}

// The C flags name their target, so a build does not follow the machine it
// runs on: the variable-argument layout the pass writes is one of two.
func TestCFlagsTargets(t *testing.T) {
	for _, tc := range []struct {
		name   string
		flags  []string
		target string
	}{{"EngineCFlags", EngineCFlags, "--target=x86_64-linux-gnu"}, {"ARM64CFlags", ARM64CFlags, "--target=aarch64-linux-gnu"}} {
		n := 0
		for _, f := range tc.flags {
			if strings.HasPrefix(f, "--target") {
				n++
				if f != tc.target {
					t.Errorf("%s has %s, want %s", tc.name, f, tc.target)
				}
			}
		}
		if n != 1 {
			t.Errorf("%s has %d --target flags, want one", tc.name, n)
		}
	}
}

// The closed list of host callbacks is the plan's five. The one more that the
// generator puts in by itself is on a list of its own, so that C cannot name it.
func TestHostCallbacksList(t *testing.T) {
	want := "host_call host_memory_grow host_now host_poll host_release"
	got := append([]string{}, HostCallbacks...)
	sort.Strings(got)
	if strings.Join(got, " ") != want {
		t.Fatalf("HostCallbacks = %v, the plan's five are %s", got, want)
	}
	if g := strings.Join(GeneratorCallbacks, " "); g != "host_trap" {
		t.Fatalf("GeneratorCallbacks = %v, want host_trap", GeneratorCallbacks)
	}
}
