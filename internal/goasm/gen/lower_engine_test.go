// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
package gen

import (
	"strings"
	"testing"
)

// Bodies as llc prints them for the whole engine, with the pass's markers.

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

const marker = "\t#APP\n\t# goasm.icall %s\n\t#NO_APP\n"

func mark(code string) string { return strings.Replace(marker, "%s", code, 1) }

// llc's tail merging moves the call of two marked sites into one block that
// both reach, each keeping its own marker.
func TestMarkerAcrossBlocks(t *testing.T) {
	body := "\ttestq\t%rsi, %rsi\n\tje\t.LBB1_2\n\tmovq\t8(%rsi), %rcx\n" + mark("ii>v") +
		"\tjmp\t.LBB1_3\n.LBB1_2:\n\tmovq\t16(%rsi), %rcx\n" + mark("ii>v") +
		".LBB1_3:\n\tcallq\t*%rcx\n"
	out, m, err := lowerEngine(t, body, Sig{Params: []Type{I64, I64}, Ret: Void}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if m.Frame < 16 || !strings.Contains(out, "*%rcx") {
		t.Fatalf("frame %d, body:\n%s", m.Frame, out)
	}
	conflict := strings.Replace(body, "\tjmp\t.LBB1_3\n.LBB1_2:\n\tmovq\t16(%rsi), %rcx\n"+mark("ii>v"),
		"\tjmp\t.LBB1_3\n.LBB1_2:\n\tmovq\t16(%rsi), %rcx\n"+mark("i>v"), 1)
	if _, _, err := lowerEngine(t, conflict, Sig{Params: []Type{I64, I64}, Ret: Void}, nil); err == nil ||
		!strings.Contains(err.Error(), "disagree") {
		t.Fatalf("disagreeing markers: %v", err)
	}
	unmarked := "\ttestq\t%rsi, %rsi\n\tje\t.LBB1_3\n\tmovq\t8(%rsi), %rcx\n" + mark("ii>v") + ".LBB1_3:\n\tcallq\t*%rcx\n"
	if _, _, err := lowerEngine(t, unmarked, Sig{Params: []Type{I64, I64}, Ret: Void}, nil); err == nil {
		t.Fatal("a path without a marker accepted")
	}
}

func TestMarkerFlow(t *testing.T) {
	// A marker flows across a direct jump into the call block, no indirect
	// jump anywhere: accepted.
	in, err := markerFlow(strings.Split("@icall i>v\njmp\t.LBB0_3\n.LBB0_5:\n@icall i>v\n.LBB0_3:\ncallq\t*%rcx\nretq", "\n"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if in[".LBB0_3"] != "i>v" {
		t.Fatalf("flow: %q", in)
	}
	if _, err := markerFlow(strings.Split("@icall i>v\nretq", "\n"), nil); err == nil {
		t.Fatal("marker before a return accepted")
	}
}

// The indirect-jump guard is decided per jump. A jump whose block reads no
// known table could reach any block, so a marker crossing a block edge is
// rejected. A switch dispatch that reads a known table reaches only that
// table's blocks, so a marker into a block the table does not name is safe.
func TestMarkerFlowIndirectJumpEntry(t *testing.T) {
	// A jmpq through a register with no table reference: targets unknown, so
	// the marker that crosses into .LBB0_3 is rejected.
	unknown := strings.Split("testq %rsi, %rsi\nje .LBB0_2\njmpq *%rdx\n.LBB0_2:\n@icall ii>v\njmp .LBB0_3\n.LBB0_3:\ncallq *%rax\nretq", "\n")
	if _, err := markerFlow(unknown, nil); err == nil || !strings.Contains(err.Error(), "indirect jump") {
		t.Fatalf("want rejection for the unknown indirect jump, got %v", err)
	}
	// The same unknown jump, but the marker is consumed in its own block:
	// nothing crosses a block edge, so it is accepted.
	consumed := strings.Split("jmpq *%rdx\n.LBB0_2:\n@icall ii>v\ncallq *%rax\nretq", "\n")
	if _, err := markerFlow(consumed, nil); err != nil {
		t.Fatalf("marker consumed in its own block rejected: %v", err)
	}
	// A switch dispatch that reads a known table .LJTI0_0, with a marker that
	// crosses into .LBB0_3. When the table does not target .LBB0_3 the marker
	// is safe (the shape of __JS_FreeValueRT: its switch lands on other
	// blocks); when it does, the block is reached unmarked, so reject.
	dispatch := strings.Split("testq %rsi, %rsi\nje .LBB0_2\nleaq .LJTI0_0(%rip), %rdx\nmovslq (%rdx,%rcx,4), %rcx\naddq %rdx, %rcx\njmpq *%rcx\n.LBB0_2:\n@icall ii>v\njmp .LBB0_3\n.LBB0_3:\ncallq *%rax\nretq", "\n")
	tables := func(targets ...string) map[string]map[string]bool {
		s := map[string]bool{}
		for _, tg := range targets {
			s[tg] = true
		}
		return map[string]map[string]bool{".LJTI0_0": s}
	}
	if _, err := markerFlow(dispatch, tables(".LBB0_8", ".LBB0_9")); err != nil {
		t.Fatalf("marker to a block the table does not target rejected: %v", err)
	}
	if _, err := markerFlow(dispatch, tables(".LBB0_3")); err == nil || !strings.Contains(err.Error(), "indirect jump") {
		t.Fatalf("marker to a jump-table target accepted: %v", err)
	}
	// The hole the old per-function guard left: a known table (.LJTI0_0,
	// landing on .LBB0_8) and a separate unknown jump (jmpq *%r8). The old
	// code trusted the union of known tables and, seeing .LBB0_3 absent from
	// it, accepted the crossing marker, though the unknown jump could reach
	// .LBB0_3 unmarked. Per jump, the unknown jump makes the targets unknown,
	// so the marker is rejected.
	mixed := strings.Split("testq %rsi, %rsi\nje .LBB0_2\nleaq .LJTI0_0(%rip), %rdx\nmovslq (%rdx,%rcx,4), %rcx\naddq %rdx, %rcx\njmpq *%rcx\n.LBB0_4:\njmpq *%r8\n.LBB0_2:\n@icall ii>v\njmp .LBB0_3\n.LBB0_3:\ncallq *%rax\nretq", "\n")
	if _, err := markerFlow(mixed, tables(".LBB0_8")); err == nil || !strings.Contains(err.Error(), "indirect jump") {
		t.Fatalf("a known table alongside an unknown jump accepted the crossing marker: %v", err)
	}
}

// The outgoing argument area must hold an indirect call's arguments even
// when the function makes no direct call (js_malloc_large wrote its
// callee's arguments over its own return address).
func TestFrameHoldsMarkedCall(t *testing.T) {
	body := "\tmovq\t1000(%rsi), %rax\n" + mark("iii>i") + "\tcallq\t*%rax\n"
	_, m, err := lowerEngine(t, body, Sig{Params: []Type{I64, I64, I64}, Ret: I64}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, ab := Layout(Sig{Params: []Type{I64, I64, I64}, Ret: I64}); m.Frame < ab {
		t.Fatalf("frame %d holds no %d-byte argument area", m.Frame, ab)
	}
}

// A pointer llc spilled to the frame is loaded into R11 and called through
// it with the arguments marshalled, not left as a call through the frame.
func TestCallThroughFrameSlot(t *testing.T) {
	body := "\tsubq\t$16, %rsp\n\tmovq\t%rsi, -8(%rbp)\n" + mark("ii>v") + "\tcallq\t*-8(%rbp)\n\taddq\t$16, %rsp\n"
	out, _, err := lowerEngine(t, body, Sig{Params: []Type{I64, I64}, Ret: Void}, nil)
	if err != nil {
		t.Fatal(err)
	}
	// The pointer is loaded into R11 and called through it, and both
	// arguments are marshalled into the Go outgoing area (rdi->0, rsi->8):
	// a fix that loads R11 but forgets the arguments must not pass.
	for _, want := range []string{"%r11", "call *%r11", "movq %rdi, 0(%rsp)", "movq %rsi, 8(%rsp)"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "(%rbp)") {
		t.Fatalf("base-pointer reference left:\n%s", out)
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
		"define void @caller(ptr %sp) {\n  %r = call i64 (ptr, ...) @var(ptr %sp, i64 1)\n  ret void\n}\n"
	got := map[string]string{}
	for _, v := range CheckIRFuncs(ll) {
		got[v.Func+" "+v.Rule] = v.Line
	}
	for _, want := range []string{"var variadic definition", "var va_start", "caller variadic call"} {
		if _, ok := got[want]; !ok {
			t.Errorf("missing %q in %v", want, got)
		}
	}
	for k := range got {
		if strings.HasPrefix(k, "ok ") {
			t.Errorf("valid function flagged: %s", k)
		}
	}
}
