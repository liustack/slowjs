// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
package gen

import (
	"strconv"
	"strings"
	"testing"
)

// Bodies as llc prints them for the whole engine, with the plugin's labels.

func lowerEngine(t *testing.T, body string, sig Sig, env *LowerEnv) (string, Meta, error) {
	t.Helper()
	if env == nil {
		env = &LowerEnv{Plan: &DataPlan{}, Funcs: map[string]bool{}}
	}
	fn := FuncSpec{Name: "h", Sig: sig, Kind: Unified}
	SetCallees([]FuncSpec{fn, {Name: "printf", Sig: Sig{Params: []Type{I64, I64}, Ret: I64}, Kind: Unified}})
	lines := strings.Split("\tpushq\t%rbp\n\tmovq\t%rsp, %rbp\n"+body+"\tpopq\t%rbp\n\tretq\n", "\n")
	return LowerUnifiedFunc(AMD64(), lines, fn, env, "")
}

// label is the symbol llc writes at the n-th indirect call of h, with the
// type id of the signature LLVM bound to it.
func label(n int, code string) string {
	id, ok := ICallID(code)
	if !ok {
		id = ICallUnencodable
	}
	return ICallPrefix + "h." + strconv.Itoa(n) + "." + strconv.FormatUint(uint64(id), 10) + ":\n"
}

// Each indirect call is lowered with the signature of the label right
// before it, whatever block it sits in (llc's tail merging gives the merged
// call one label, since it merges only calls of one signature).
func TestICallLabel(t *testing.T) {
	sig := Sig{Params: []Type{I64, I64}, Ret: Void}
	body := "	testq\t%rsi, %rsi\n\tje\t.LBB1_2\n\tmovq\t8(%rsi), %rcx\n\tjmp\t.LBB1_3\n.LBB1_2:\n\tmovq\t16(%rsi), %rcx\n" +
		".LBB1_3:\n" + label(0, "ii>v") + "\tcallq\t*%rcx\n\tmovq\t%rsi, %rdi\n" + label(1, "i>j") + "\tcallq\t*%rcx\n"
	_, m, err := lowerEngine(t, body, sig, nil)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, c := range m.Rec.Calls {
		if c.Kind != CallReg || c.Target != "rcx" {
			t.Fatalf("call %+v", c)
		}
		got = append(got, sigCodeOf(c.Sig))
	}
	if strings.Join(got, " ") != "ii>v i>j" {
		t.Fatalf("calls lowered as %v", got)
	}
	for name, c := range map[string]struct{ body, want string }{
		"no label":           {"\tcallq\t*%rcx\n", "without LLVM's signature label"},
		"label off the call": {label(0, "ii>v") + "\tmovq\t%rsi, %rdi\n\tcallq\t*%rcx\n", "not on an indirect call"},
		"label on a direct":  {label(0, "ii>v") + "\tcallq\th\n", "not on an indirect call"},
		"label at the end":   {"\tcallq\th\n" + label(0, "ii>v"), "not on an indirect call"},
		"another function":   {strings.Replace(label(0, "ii>v"), ".h.", ".g.", 1) + "\tcallq\t*%rcx\n", "not one of this function"},
		"unencodable":        {label(0, "iB>v") + "\tcallq\t*%rcx\n", "encodes no signature"},
		"malformed":          {ICallPrefix + "h.x:\n\tcallq\t*%rcx\n", "not one of this function"},
	} {
		if _, _, err := lowerEngine(t, c.body, sig, nil); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v, want an error with %q", name, err, c.want)
		}
	}
}

// The outgoing argument area must hold an indirect call's arguments even
// when the function makes no direct call (js_malloc_large wrote its
// callee's arguments over its own return address).
func TestFrameHoldsMarkedCall(t *testing.T) {
	body := "\tmovq\t1000(%rsi), %rax\n" + label(0, "iii>i") + "\tcallq\t*%rax\n"
	_, m, err := lowerEngine(t, body, Sig{Params: []Type{I64, I64, I64}, Ret: I64}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, ab := Layout(Sig{Params: []Type{I64, I64, I64}, Ret: I64}); m.Frame < ab {
		t.Fatalf("frame %d holds no %d-byte argument area", m.Frame, ab)
	}
}

// llc built with +slow-two-mem-ops never calls through memory, so the
// lowering rejects any such call: one through a frame slot must not be taken
// for a plain frame access and left unmarshalled.
func TestCallThroughMemory(t *testing.T) {
	for _, call := range []string{"callq\t*-8(%rbp)", "callq\t*8(%rax)", "callq\t*(%r9,%rax,8)", "callq\t*fp(%rip)"} {
		body := "\tsubq\t$16, %rsp\n\tmovq\t%rsi, -8(%rbp)\n" + label(0, "ii>v") + "\t" + call + "\n\taddq\t$16, %rsp\n"
		_, _, err := lowerEngine(t, body, Sig{Params: []Type{I64, I64}, Ret: Void}, nil)
		if err == nil || !strings.Contains(err.Error(), "call through memory") {
			t.Fatalf("%s: %v", call, err)
		}
	}
}

func TestVariadicCall(t *testing.T) {
	env := &LowerEnv{Plan: &DataPlan{}, Funcs: map[string]bool{}, Variadic: map[string]bool{"printf": true}}
	sig := Sig{Params: []Type{I64, I64}, Ret: I64}
	_, _, err := lowerEngine(t, "\tcallq\tprintf\n", sig, env)
	if err == nil || !strings.Contains(err.Error(), "variadic call to printf") {
		t.Fatalf("variadic call: %v", err)
	}
	_, _, err = lowerEngine(t, "\tpushq\t$7\n\tcallq\tprintf\n\taddq\t$8, %rsp\n", sig, env)
	if err == nil || !strings.Contains(err.Error(), "stack arguments of the variadic call to printf") {
		t.Fatalf("stack arguments: %v", err)
	}
	env.LooseVarargs = true
	if _, _, err := lowerEngine(t, "\tcallq\tprintf\n", sig, env); err != nil {
		t.Fatalf("loose: %v", err)
	}
}

// musl's memmove saves RBX and calls memcpy: the push becomes a store into
// the leaf's NOSPLIT frame, the pop a load, and the call to the other leaf
// stays a plain call.
func TestLeafSavesAndCallsLeaf(t *testing.T) {
	memcpy := FuncSpec{Name: "memcpy", Sig: Sig{Params: []Type{I64, I64, I64}, Ret: I64}, Kind: Register}
	memmove := FuncSpec{Name: "memmove", Sig: memcpy.Sig, Kind: Register}
	SetCallees([]FuncSpec{memcpy, memmove})
	asm := "memmove:\n\tpushq\t%rbx\n\tmovq\t%rdi, %rbx\n\tcallq\tmemcpy\n\tmovq\t%rbx, %rax\n\tpopq\t%rbx\n\tretq\n.Lfunc_end1:\n"
	out, m, err := LowerRegister(asm, memmove)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"movq %rbx, 0(%rsp)", "call memcpy", "movq 0(%rsp), %rbx"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if m.Frame != 8 {
		t.Errorf("frame %d, want 8", m.Frame)
	}
	bad := strings.Replace(asm, "\tpopq\t%rbx\n", "\tpopq\t%r12\n", 1)
	if _, _, err := LowerRegister(bad, memmove); err == nil {
		t.Error("pop of a register the leaf did not save accepted")
	}
}

func TestCheckIRFuncs(t *testing.T) {
	ll := "define i64 @ok(ptr %sp) {\n  ret i64 0\n}\n" +
		"define i64 @var(ptr %sp, ...) {\n  call void @llvm.va_start.p0(ptr %ap)\n  ret i64 0\n}\n" +
		"define void @caller(ptr %sp) {\n  %r = call i64 (ptr, ...) @var(ptr %sp, i64 1)\n  ret void\n}\n" +
		"define double @okfp(double %a, double %b) {\n  %m = fmul double %a, %b\n  %s = fadd double %m, %a\n  %c = call fastcc double @okfp(double %s, double %b)\n  ret double %c\n}\n" +
		"define double @fused(double %a, double %b) {\n  %r = call double @llvm.fmuladd.f64(double %a, double %b, double %a)\n  ret double %r\n}\n" +
		"define double @fma(double %a, double %b) {\n  %r = call double @llvm.fma.f64(double %a, double %b, double %a)\n  ret double %r\n}\n" +
		"define double @reassoc(double %a, double %b) {\n  %r = fadd reassoc nsz double %a, %b\n  ret double %r\n}\n" +
		"define double @fastcall(double %a) {\n  %r = call fast double @llvm.sqrt.f64(double %a)\n  ret double %r\n}\n" +
		"define double @afn(double %a) #0 {\n  ret double %a\n}\n" +
		"attributes #0 = { \"unsafe-fp-math\"=\"true\" }\n"
	got := map[string]string{}
	for _, v := range CheckIRFuncs(ll) {
		got[v.Func+" "+v.Rule] = v.Line
	}
	for _, want := range []string{"var variadic definition", "var va_start", "caller variadic call", "fused fused multiply-add",
		"fma fused multiply-add", "reassoc fast-math", "fastcall fast-math", " fast-math"} {
		if _, ok := got[want]; !ok {
			t.Errorf("missing %q in %v", want, got)
		}
	}
	for k := range got {
		if strings.HasPrefix(k, "ok ") || strings.HasPrefix(k, "okfp ") {
			t.Errorf("valid function flagged: %s", k)
		}
	}
}
