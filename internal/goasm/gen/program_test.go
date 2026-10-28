// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
package gen

import (
	"encoding/binary"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func TestSigFromCode(t *testing.T) {
	s, err := SigFromCode("ijd>j")
	if err != nil {
		t.Fatal(err)
	}
	want := Sig{Params: []Type{I64, JSV, F64}, Ret: JSV}
	if !reflect.DeepEqual(s, want) {
		t.Fatalf("got %+v, want %+v", s, want)
	}
	if s, err := SigFromCode(">v"); err != nil || len(s.Params) != 0 || s.Ret != Void {
		t.Fatalf("empty signature: %+v %v", s, err)
	}
	for _, bad := range []string{"ii", "i...>i", "iB>v", "i?>v", "i>?", "in>v", "i>n", "if>v", "i>f"} {
		if _, err := SigFromCode(bad); err == nil {
			t.Errorf("%q: accepted", bad)
		}
	}
	// The negative build takes a narrow integer for a word.
	KeepNarrowSigs = true
	defer func() { KeepNarrowSigs = false }()
	if s, err := SigFromCode("in>n"); err != nil || !reflect.DeepEqual(s, Sig{Params: []Type{I64, I64}, Ret: I64}) {
		t.Fatalf("KeepNarrowSigs: %+v %v", s, err)
	}
}

func TestParseReport(t *testing.T) {
	r := ParseReport(strings.Join([]string{
		"SIG f ii>i def unified",
		"SIG memcpy iii>i def leaf",
		"SIG host_now iii>i decl unified",
		"ADDR f",
		"CALL f host_now",
		"ICALL f iii>v",
		"FRAMEADDR g",
		"PACKED h ijiiii>j",
		"PACKED-ICALL f",
		"PACKED-ICALL f",
		"REJECT k inline assembly",
		"GOTO-TABLE run",
		"MISMATCH f g",
		"VARARGS printf",
		"some other line",
	}, "\n"))
	if r.Sigs["f"] != "ii>i" || !r.Defined["f"] || r.Defined["host_now"] || !r.Leaf["memcpy"] || r.Leaf["f"] {
		t.Errorf("signatures: %+v %+v %+v", r.Sigs, r.Defined, r.Leaf)
	}
	if !r.AddrTaken["f"] || !reflect.DeepEqual(r.Calls["f"], []string{"host_now"}) || !reflect.DeepEqual(r.ICalls["f"], []string{"iii>v"}) {
		t.Errorf("graph: %+v %+v %+v", r.AddrTaken, r.Calls, r.ICalls)
	}
	if !r.FrameAddr["g"] || !r.Packed["h"] || r.PackedWas["h"] != "ijiiii>j" || r.PackedICalls["f"] != 2 || r.Rejected["k"] != "inline assembly" {
		t.Errorf("flags: %+v %+v %+v %+v %+v", r.FrameAddr, r.Packed, r.PackedWas, r.PackedICalls, r.Rejected)
	}
	if !reflect.DeepEqual(r.GotoTable, []string{"run"}) || !reflect.DeepEqual(r.Mismatch, [][2]string{{"f", "g"}}) ||
		!reflect.DeepEqual(r.Varargs, []string{"printf"}) {
		t.Errorf("lists: %v %v %v", r.GotoTable, r.Mismatch, r.Varargs)
	}
}

const unitAsm = `	.text
	.file	"engine.c"
	.globl	f                               # -- Begin function f
	.p2align	4
	.type	f,@function
f:                                      # @f
# %bb.0:
	pushq	%rbp
	movq	%rsp, %rbp
	movsd	.LCPI0_0(%rip), %xmm0           # xmm0 = mem[0],zero
	leaq	names(%rip), %rax
	leaq	.L.str(%rip), %rcx
	jmp	.LBB0_1
.LBB0_1:
	popq	%rbp
	retq
.Lfunc_end0:
	.size	f, .Lfunc_end0-f
	.section	.rodata.cst8,"aM",@progbits,8
	.p2align	3, 0x0
.LCPI0_0:
	.quad	0x8000000000000000              # double -0
	.section	.data.rel.ro,"aw",@progbits
	.globl	names
	.p2align	4, 0x0
names:
alias_of_names:
	.quad	.L.str
	.quad	f
	.quad	.L.str+2
	.long	.L.str-names
	.zero	4
	.section	.rodata.str1.1,"aMS",@progbits,1
.L.str:
	.asciz	"a b\t"
	.set	g, f
`

func TestSplitAsm(t *testing.T) {
	u, err := SplitAsm(unitAsm, "#")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(u.FuncOrder, []string{"f"}) || u.LabelFunc[".LBB0_1"] != "f" {
		t.Fatalf("functions: %v %v", u.FuncOrder, u.LabelFunc)
	}
	n := u.Data["names"]
	if n == nil || u.Data["alias_of_names"] != n || !n.Global || n.Align != 4 || n.Section != ".data.rel.ro" || len(n.Lines) != 5 {
		t.Fatalf("names: %+v", n)
	}
	if c := u.Data[".LCPI0_0"]; c == nil || c.Section != ".rodata.cst8" || c.Lines[0] != ".quad\t0x8000000000000000" {
		t.Fatalf("constant pool: %+v", c)
	}
	if u.Sets["g"] != "f" {
		t.Fatalf("sets: %v", u.Sets)
	}
}

func TestExprSymbols(t *testing.T) {
	cases := map[string][]string{
		".quad\t0x8000000000000000": nil,
		".quad\t0x7ff0000000000000": nil,
		".long\t12345":              nil,
		".quad\tfoo+8":              {"foo"},
		".long\t.Ltab-.Lbase+0x10":  {".Ltab", ".Lbase"},
		".asciz\t\"x8000 sym\"":     nil,
	}
	for l, want := range cases {
		if got := exprSymbols(l); !reflect.DeepEqual(got, want) {
			t.Errorf("%q: got %q, want %q", l, got, want)
		}
	}
}

func TestEncodeDirective(t *testing.T) {
	cases := []struct {
		line string
		at   int
		want []byte
	}{
		{".p2align\t3, 0x0", 3, make([]byte, 5)},
		{".asciz\t\"a b\\t\"", 0, []byte("a b\t\x00")},
		{".quad\t0x8000000000000000", 0, []byte{0, 0, 0, 0, 0, 0, 0, 0x80}},
		{".short\t-2", 0, []byte{0xfe, 0xff}},
		{".zero\t3,7", 0, []byte{7, 7, 7}},
	}
	for _, c := range cases {
		b, fx, err := encodeDirective(c.line, c.at)
		if err != nil || len(fx) != 0 || !reflect.DeepEqual(b, c.want) {
			t.Errorf("%q: got % x %v %v, want % x", c.line, b, fx, err, c.want)
		}
	}
	b, fx, err := encodeDirective(".quad\tfoo+8", 0)
	if err != nil || len(b) != 8 || len(fx) != 1 || fx[0].expr != "foo+8" {
		t.Errorf("symbol: % x %+v %v", b, fx, err)
	}
}

func TestPlanData(t *testing.T) {
	u, err := SplitAsm(unitAsm, "#")
	if err != nil {
		t.Fatal(err)
	}
	p := PlanData(u, map[string]bool{"f": true, "g": true})
	if len(p.Errors) != 0 {
		t.Fatalf("errors: %v", p.Errors)
	}
	// The constant pool goes into f's body, its double value untouched.
	body := p.BodyData("f")
	if !strings.Contains(body, ".LCPI0_0:") || !strings.Contains(body, "0x8000000000000000") {
		t.Fatalf("body data:\n%s", body)
	}
	// names and the string are LEA targets: both live in engine_rodata.
	no, ok1 := p.BlobOff["names"]
	so, ok2 := p.BlobOff[".L.str"]
	if !ok1 || !ok2 || p.BlobOff["alias_of_names"] != no {
		t.Fatalf("blob offsets: %v", p.BlobOff)
	}
	if string(p.Blob[so:so+5]) != "a b\t\x00" {
		t.Fatalf("string bytes: %q", p.Blob[so:so+5])
	}
	// .long .L.str-names is resolved here.
	if d := int32(binary.LittleEndian.Uint32(p.Blob[no+24:])); d != int32(so-no) {
		t.Fatalf("difference: got %d, want %d", d, so-no)
	}
	want := []BlobReloc{
		{Off: no, Add: int64(so), Label: ".L.str"},
		{Off: no + 8, Sym: "f", Func: true, Label: "f"},
		{Off: no + 16, Add: int64(so + 2), Label: ".L.str", LabelAdd: 2},
	}
	if !reflect.DeepEqual(p.Relocs, want) {
		t.Fatalf("relocations: %+v, want %+v", p.Relocs, want)
	}
	env := &LowerEnv{Plan: p, Funcs: map[string]bool{"f": true}}
	got, err := env.dataRefs("f", "leaq\tnames+8(%rip), %rax", nil)
	if err != nil || got != "leaq\t__goasm_rodata+"+strconv.Itoa(no+8)+"(%rip), %rax" {
		t.Fatalf("LEA of engine_rodata: %q %v", got, err)
	}
	if _, err := env.dataRefs("f", "movq\tnames(%rip), %rax", nil); err == nil {
		t.Fatal("non-LEA read of data holding pointers accepted")
	}
}

func TestEmitRodata(t *testing.T) {
	p := &DataPlan{Blob: []byte{1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0xab},
		Relocs: []BlobReloc{{Off: 8, Sym: "f.1", Func: true, Add: 4}}}
	out := EmitRodata(p, NameMap{"f.1": "f_1"}, "")
	for _, want := range []string{
		"GLOBL ·engine_rodata(SB), RODATA|NOPTR, $24",
		"DATA ·engine_rodata+0(SB)/8, $0x0000000000000001",
		"DATA ·engine_rodata+8(SB)/8, $·f_1+4(SB)",
		"DATA ·engine_rodata+16(SB)/8, $0x00000000000000ab",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestGoNames(t *testing.T) {
	m, err := GoNames([]string{"cr_add.156", "type", "f"})
	if err != nil {
		t.Fatal(err)
	}
	if m["cr_add.156"] != "cr_add_156" || m["type"] != "type_" || m["f"] != "f" {
		t.Fatalf("names: %v", m)
	}
	if _, err := GoNames([]string{"a.b", "a_b"}); err == nil {
		t.Fatal("collision accepted")
	}
}

func TestEvalExprRejectsUnknownOperators(t *testing.T) {
	p := &DataPlan{BlobOff: map[string]int{"table": 16, "base": 4}}
	// table+8 is a pointer into engine_rodata at offset 16+8.
	if _, r, err := p.evalExpr("table+8", 0, nil); err != nil || r == nil || r.Sym != "" || r.Add != 24 {
		t.Fatalf("table+8: %+v %v", r, err)
	}
	// table-base is a resolved constant difference.
	if k, r, err := p.evalExpr("table-base", 0, nil); err != nil || r != nil || k != 12 {
		t.Fatalf("table-base: %d %+v %v", k, r, err)
	}
	// An operator the evaluator does not implement must be rejected, not
	// silently dropped (table+8*2 is table+16, not table+10). Malformed
	// syntax with no operator between terms (1table, table+1 2) must be
	// rejected too, not reinterpreted as table+1 or table+12.
	for _, bad := range []string{"table+8*2", "table*2", "table<<3", "(table+8)", "table+8/2",
		"1table", "table 2", "table+1 2", "table+", "", "+"} {
		if _, _, err := p.evalExpr(bad, 0, nil); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestEncodeAlign(t *testing.T) {
	// A non-zero fill byte pads with that byte, not zero.
	b, _, err := encodeDirective(".p2align 3, 0x7f", 1)
	if err != nil || len(b) != 7 {
		t.Fatalf("fill length: % x %v", b, err)
	}
	for _, by := range b {
		if by != 0x7f {
			t.Fatalf("fill byte: % x", b)
		}
	}
	// A max-skip smaller than the padding drops the alignment.
	if b, _, err := encodeDirective(".p2align 4, 0, 3", 1); err != nil || len(b) != 0 {
		t.Fatalf("max-skip: % x %v", b, err)
	}
	// A max-skip larger than the padding keeps it.
	if b, _, err := encodeDirective(".p2align 4, 0, 99", 1); err != nil || len(b) != 15 {
		t.Fatalf("max-skip kept: %d %v", len(b), err)
	}
	// Plain power-of-two alignment to zero padding at an aligned offset.
	if b, _, err := encodeDirective(".p2align 3, 0x0", 8); err != nil || len(b) != 0 {
		t.Fatalf("aligned: % x %v", b, err)
	}
	// A fourth field is not a form we understand.
	if _, _, err := encodeDirective(".p2align 4, 0, 3, 9", 0); err == nil {
		t.Fatal("extra alignment field accepted")
	}
}

// Malformed alignment and count directives are errors, not panics: a negative
// or oversized power would shift or allocate wildly, and a non-positive
// max-skip is not a form the assembler accepts.
func TestEncodeDirectiveRejects(t *testing.T) {
	for _, bad := range []string{".p2align -1", ".p2align 64", ".p2align 3, 0, 0", ".p2align 3, 0, -4", ".zero -3"} {
		if _, _, err := encodeDirective(bad, 1); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

// A data object whose internal alignment exceeds its own start alignment
// cannot keep its layout once the blob places it at a different residue than
// llc did, so PlanData rejects it instead of shifting a byte the code reads
// at a fixed offset.
func TestDataObjectInternalAlignment(t *testing.T) {
	asm := ".text\n.globl f\nf:\n\tpushq\t%rbp\n\tmovq\t%rsp, %rbp\n\tleaq\ta(%rip), %rax\n" +
		"\tleaq\tb(%rip), %rcx\n\tpopq\t%rbp\n\tretq\n.Lfunc_end0:\n" +
		".section\t.rodata,\"a\",@progbits\na:\n\t.byte\t1\nb:\n\t.byte\t2\n\t.p2align\t3, 0x7f\n\t.byte\t3\n"
	u, err := SplitAsm(asm, "#")
	if err != nil {
		t.Fatal(err)
	}
	p := PlanData(u, map[string]bool{"f": true})
	if len(p.Errors) == 0 {
		t.Fatalf("an object with an unpreservable internal alignment was accepted; blob=% x", p.Blob)
	}
}

// A start alignment with a max-skip below its step may be dropped where llc
// placed the object, so the residue an alignment inside the object padded
// from is unknown: b sits at 1 in llc's section (the 7-byte pad exceeds the
// max-skip of 1), its inner .p2align pads to 8 and 03 lands at b+7, but the
// blob would place b at 8 and put 03 at b+8. PlanData must reject b.
func TestDataObjectConditionalStart(t *testing.T) {
	asm := ".text\n.globl f\nf:\n\tpushq\t%rbp\n\tmovq\t%rsp, %rbp\n\tleaq\ta(%rip), %rax\n" +
		"\tleaq\tb(%rip), %rcx\n\tpopq\t%rbp\n\tretq\n.Lfunc_end0:\n" +
		".section\t.rodata,\"a\",@progbits\na:\n\t.byte\t1\n\t.p2align\t3, 0, 1\nb:\n\t.byte\t2\n\t.p2align\t3, 0x7f\n\t.byte\t3\n"
	u, err := SplitAsm(asm, "#")
	if err != nil {
		t.Fatal(err)
	}
	p := PlanData(u, map[string]bool{"f": true})
	if !strings.Contains(fmt.Sprint(p.Errors), "data b ") {
		t.Fatalf("b with an inner alignment after a skippable start was accepted; blob=% x errors=%v", p.Blob, p.Errors)
	}
}

// An alignment before a label goes through the same checked parse as one
// inside an object: SplitAsm returns an error, never a power PlanData would
// shift by and panic on.
func TestSplitAsmRejectsBadStartAlignment(t *testing.T) {
	for _, al := range []string{"64", "-1", "x", "3, 0, 0", "3,0,1,2"} {
		asm := ".section\t.rodata,\"a\",@progbits\n\t.p2align\t" + al + "\na:\n\t.byte\t1\n"
		if _, err := SplitAsm(asm, "#"); err == nil {
			t.Errorf(".p2align %s before a label accepted", al)
		}
	}
}

func TestCheckShadowSizeLog2(t *testing.T) {
	for _, n := range []int{MinShadowSizeLog2, 20, 25, MaxShadowSizeLog2} {
		if err := CheckShadowSizeLog2(n); err != nil {
			t.Errorf("%d: %v", n, err)
		}
	}
	for _, n := range []int{-1, 0, 1, MinShadowSizeLog2 - 1, MaxShadowSizeLog2 + 1, 63, 64, 1 << 20} {
		if err := CheckShadowSizeLog2(n); err == nil || !strings.Contains(err.Error(), "-shadow-size-log2") {
			t.Errorf("%d: %v", n, err)
		}
	}
}
