// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package gen

import (
	"strings"
	"testing"
)

// a64JTCode is lowered code with the label .Lb at 0, n instructions, the
// label .Lt, and a last branch.
func a64JTCode(n int) []string {
	code := []string{".Lb:"}
	for i := 0; i < n; i++ {
		code = append(code, "add x0, x0, #1")
	}
	return append(code, ".Lt:", "b .Lgoret_f")
}

// a64JTAround is code with .Lb, before instructions, then line, then after
// instructions and .Lt.
func a64JTAround(before int, line string, after int) []string {
	return append(append(a64JTCode(before)[:before+1], line), a64JTCode(after)[1:]...)
}

type a64JTCase struct {
	name         string
	code         []string
	data         string
	start, align int
	want         string
}

// TestA64CheckJumpTables: each width holds the farthest distance it can and
// refuses one step more, after the code the lowering wrote, from where
// FuncPlace puts the function.
func TestA64CheckJumpTables(t *testing.T) {
	const jt = ".LJTI0_0:\n.byte\t(.Lt-.Lb)>>2"
	joined := func(n int) []string {
		return append([]string{".Lb:", strings.Repeat("add x0, x0, #1\n\t", n-1) + "add x0, x0, #1"}, a64JTCode(0)[1:]...)
	}
	ok := []a64JTCase{
		{name: "byte, 255 steps", code: a64JTCode(255), data: ".LJTI0_0:\n.byte\t(.Lb-.Lb)>>2\n.byte\t(.Lt-.Lb)>>2"},
		{name: "hword, 65535 steps", code: a64JTCode(65535), data: ".LJTI0_0:\n.hword\t(.Lt-.Lb)>>2"},
		{name: "word, either way", code: a64JTCode(70000), data: ".LJTI0_0:\n.word\t.Lt-.Lb\n.word\t.Lb-.Lt"},
		// two instructions in one line of the lowering count as two
		{name: "byte, joined lines", code: joined(255), data: jt},
		// .p2align 4 after 1 instruction pads 12 bytes: 4 + 12 + 4*251 = 255 steps
		{name: "byte, aligned", code: a64JTAround(1, ".p2align 4", 251), data: jt},
		// at 0, 252 instructions end on 16 bytes: 252 + 2 = 254 steps
		{name: "byte, aligned at 0", code: a64JTAround(252, ".p2align 4", 2), data: jt},
		// padding of 12 bytes past a max-skip of 4 is not made: 1 + 252 = 253 steps
		{name: "byte, max-skip", code: a64JTAround(1, ".p2align 4, , 4", 252), data: jt},
		{name: "byte, max-skip with a fill", code: a64JTAround(1, ".p2align 4, 0x0, 4", 252), data: jt},
		// data that names no label of the code is not a jump table
		{name: "other data", code: a64JTCode(1), data: ".LCPI0_0:\n.xword\t0\n.word\tdata-other\n.byte\t1"},
		{name: "no data", code: a64JTCode(1)},
		// the computed goto's table: whole distances from its own label
		{name: "word from data", code: a64JTCode(1), data: ".Ltable:\n.word\t.Lt-.Ltable"},
	}
	for _, c := range ok {
		if c.align == 0 {
			c.align = 4
		}
		if err := a64CheckJumpTables(c.code, c.data, c.start, c.align); err != nil {
			t.Errorf("%s: %v", c.name, err)
		}
	}
	bad := []a64JTCase{
		{name: "byte, 256 steps", code: a64JTCode(256), data: jt, want: "256 steps of 4 bytes after the lowering, past the 255 a .byte holds"},
		{name: "hword, 65536 steps", code: a64JTCode(65536), data: ".LJTI0_0:\n.hword\t(.Lt-.Lb)>>2", want: "65536 steps of 4 bytes after the lowering, past the 65535 a .hword holds"},
		{name: "hword of bytes, 32 KiB", code: a64JTCode(8192), data: ".LJTI0_0:\n.hword\t.Lt-.Lb", want: "32768 bytes after the lowering, past what a .hword holds"},
		{name: "byte, joined lines", code: joined(256), data: jt, want: "256 steps"},
		{name: "byte, aligned one past", code: a64JTAround(1, ".p2align 4", 252), data: jt, want: "256 steps"},
		// the same code put 8 bytes after 64 (Align64): 8 + 1008 pads 8 more, then 8
		{name: "byte, aligned at 8", code: a64JTAround(252, ".p2align 4", 2), data: jt, start: 8, align: 6, want: "256 steps"},
		{name: "byte, max-skip not passed", code: a64JTAround(1, ".p2align 4, , 12", 252), data: jt, want: "256 steps"},
		{name: "backward", code: a64JTCode(3), data: ".LJTI0_0:\n.byte\t(.Lb-.Lt)>>2", want: "not a whole number of 4-byte steps forward"},
		{name: "unknown label", code: a64JTCode(3), data: ".LJTI0_0:\n.byte\t(.Lx-.Lb)>>2", want: "names a label that is not in the code"},
		{name: "word shifted from data", code: a64JTCode(3), data: ".LJTI0_0:\n.word\t(.Lt-.LJTI0_0)>>2", want: "names a label that is not in the code"},
		{name: "unsized code", code: a64JTAround(0, ".inst 0xd503201f", 1), data: jt, want: "cannot size"},
		{name: "wide alignment", code: a64JTAround(0, ".p2align 5", 1), data: jt, want: "an alignment of 2^5 in the code, more than the 2^4"},
		{name: "wide alignment at 64", code: a64JTAround(0, ".p2align 7", 1), data: jt, align: 6, want: "an alignment of 2^7 in the code, more than the 2^6"},
	}
	for _, c := range bad {
		if c.align == 0 {
			c.align = 4
		}
		err := a64CheckJumpTables(c.code, c.data, c.start, c.align)
		if err == nil || !strings.Contains(err.Error(), c.want) || !strings.Contains(err.Error(), "jump table .LJTI0_0") {
			t.Errorf("%s: %v, want an error of jump table .LJTI0_0 with %q", c.name, err, c.want)
		}
	}
}

// TestLowerARM64EngineJumpTable: the lowering refuses a function whose jump
// table its own code made too narrow, with the function's name. llc's table
// of bytes spans 250 steps, under the 255 a byte holds; the call the
// lowering writes in place of bl adds instructions and takes it past.
func TestLowerARM64EngineJumpTable(t *testing.T) {
	fn, env := a64TestEnv()
	table := &DataObj{Label: ".LJTI0_0", Align: 0, Lines: []string{".byte\t(.LBB0_1-.LBB0_1)>>2", ".byte\t(.LBB0_2-.LBB0_1)>>2"}}
	env.Plan.InBody = map[string][]*DataObj{"f": {table}}
	env.Plan.Rename["f"][".LJTI0_0"] = ".LJTI0_0"
	body := func(calls int) []string {
		b := []string{"str x30, [sp, #-16]!", "adr x9, .LJTI0_0", "ldrb w8, [x9, x0]", "adr x10, .LBB0_1", "add x10, x10, x8, lsl #2", "br x10", ".LBB0_1:"}
		for i := 0; i < calls; i++ {
			b = append(b, "bl callee")
		}
		for i := 0; i < 250-calls; i++ {
			b = append(b, "add x0, x0, #1")
		}
		return append(b, ".LBB0_2:", "ldr x30, [sp], #16", "ret")
	}
	if _, _, err := LowerARM64EngineFunc(body(0), fn, env); err != nil {
		t.Fatalf("250 steps of plain code: %v", err)
	}
	_, _, err := LowerARM64EngineFunc(body(5), fn, env)
	if err == nil || !strings.HasPrefix(err.Error(), "f: jump table .LJTI0_0: ") || !strings.Contains(err.Error(), "past the 255 a .byte holds") {
		t.Errorf("5 calls in the table's span: %v, want the table refused", err)
	}
}

// TestA64FuncStart: a function FuncPlace puts at its Align64 place starts
// that far after a multiple of 64, any other at a multiple of 16.
func TestA64FuncStart(t *testing.T) {
	align64 := map[string]int{"g": 8, "h": 0}
	for _, c := range []struct {
		name         string
		start, align int
	}{{"f", 0, 4}, {"g", 8, 6}, {"h", 0, 6}} {
		if s, a := a64FuncStart(align64, c.name); s != c.start || a != c.align {
			t.Errorf("%s: %d after 2^%d, want %d after 2^%d", c.name, s, a, c.start, c.align)
		}
	}
	if s, a := a64FuncStart(nil, "f"); s != 0 || a != 4 {
		t.Errorf("no Align64: %d after 2^%d", s, a)
	}
	if !strings.Contains(FuncPlace(align64, "g"), ".skip 8") {
		t.Errorf("FuncPlace of g: %q", FuncPlace(align64, "g"))
	}
}
