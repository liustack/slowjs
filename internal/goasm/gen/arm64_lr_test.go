// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package gen

import (
	"strings"
	"testing"
)

// a64LRPair saves x30 in a pair with a live value that it loads back after
// the call, and frees the frame with the load of x30. With L=32 (the call's
// area of 16 bytes) and a frame of 32, [sp, #k] of llc is 32+k(RSP).
var a64LRPair = []string{
	"stp x30, x20, [sp, #-32]!",
	"str x21, [sp, #16]",
	"bl callee",
	"ldr x20, [sp, #8]",
	"ldr x21, [sp, #16]",
	"ldr x30, [sp], #32",
	"ret",
}

// TestLowerARM64EngineLR: the lowering drops llc's saves and loads of x30
// in every form llc writes, keeps the other register of a pair, and keeps
// the frame as it was.
func TestLowerARM64EngineLR(t *testing.T) {
	for _, c := range []struct {
		name  string
		lines []string
		want  []string
		frame int
	}{
		{"pair that allocates, load that frees", a64LRPair,
			[]string{"str x20, [sp, #40]\n\tstr x21, [sp, #48]\n\tstr x20, [sp, #8]\n\tbl go_callee\n\tldr x0, [sp, #16]\n\tldr x20, [sp, #40]\n\tldr x21, [sp, #48]\n\tb .Lgoret_f"},
			32 + 32 - 8},
		{"LR second in a pair", []string{
			"sub sp, sp, #32",
			"stp x21, x30, [sp, #16]",
			"bl callee",
			"ldp x21, x30, [sp, #16]",
			"add sp, sp, #32",
			"ret",
		}, []string{"str x21, [sp, #48]\n\tstr x20, [sp, #8]", "ldr x0, [sp, #16]\n\tldr x21, [sp, #48]\n\tb .Lgoret_f"}, 32 + 32 - 8},
		{"single slot", []string{
			"sub sp, sp, #16",
			"str x30, [sp, #8]",
			"bl callee",
			"ldr x30, [sp, #8]",
			"add sp, sp, #16",
			"ret",
		}, []string{"ldr x0, [sp, #16]\n\tb .Lgoret_f"}, 32 + 16 - 8},
		{"pair that allocates with LR second", []string{
			"stp x22, x30, [sp, #-16]!",
			"bl callee",
			"ldp x22, x30, [sp], #16",
			"ret",
		}, []string{"str x22, [sp, #32]\n", "ldr x22, [sp, #32]\n\tb .Lgoret_f"}, 32 + 16 - 8},
		{"a leaf to Go", []string{
			"str x30, [sp, #-16]!",
			"mov x20, x21",
			"ldr x30, [sp], #16",
			"ret",
		}, []string{"mov x20, x21\n\tb .Lgoret_f"}, 16 + 16 - 8},
	} {
		fn, env := a64TestEnv()
		text, meta, err := LowerARM64EngineFunc(c.lines, fn, env)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if reA64LR.MatchString(text) {
			t.Errorf("%s: x30 left in\n%s", c.name, text)
		}
		for _, w := range c.want {
			if !strings.Contains(text, w) {
				t.Errorf("%s: no %q in\n%s", c.name, w, text)
			}
		}
		if meta.Frame != c.frame {
			t.Errorf("%s: frame %d, want %d (the slot of x30 stays in it)", c.name, meta.Frame, c.frame)
		}
	}
}

// TestLowerARM64EngineLRLabel: a label whose name ends in sp (llc names a
// constant of vsnprintf L__const.vsnprintf.sp) is no use of the stack.
// TestLowerARM64EngineLRTuple: the address of a tuple spill beside the slot,
// written again before the call, is no use of the slot.
func TestLowerARM64EngineLRTuple(t *testing.T) {
	fn, env := a64TestEnv()
	lines := []string{"sub sp, sp, #48", "str x30, [sp, #40]", "add x8, sp, #0", "st1 { v0.2d, v1.2d }, [x8]", "mov x8, x20",
		"bl callee", "ldr x30, [sp, #40]", "add sp, sp, #48", "ret"}
	text, _, err := LowerARM64EngineFunc(lines, fn, env)
	if err != nil || !strings.Contains(text, "add x8, sp, #32\n\tst1 { v0.2d, v1.2d }, [x8]\n") {
		t.Fatalf("%v\n%s", err, text)
	}
	lines[3] = "st1 { v0.2d, v1.2d, v2.2d }, [x8]"
	if _, _, err := LowerARM64EngineFunc(lines, fn, env); err == nil || !strings.Contains(err.Error(), "also reached") {
		t.Errorf("a tuple of 48 bytes over the slot at 40: %v", err)
	}
}

// TestLowerARM64EngineLRTuplePaths: the address of a tuple spill that lives
// past a branch or a label is followed on every path, as llc leaves it when
// a profile puts the reload at the end of a block (JS_GetOwnPropertyNamesInternal
// under clang PGO): it is no use of the slot when every path writes it again,
// calls or returns before reading it, and it is when a path reaches the slot
// through it.
func TestLowerARM64EngineLRTuplePaths(t *testing.T) {
	head := []string{"sub sp, sp, #48", "str x30, [sp, #40]", "add x8, sp, #0", "ld1 { v0.2d, v1.2d }, [x8]"}
	tail := []string{"bl callee", "ldr x30, [sp, #40]", "add sp, sp, #48", "ret"}
	for name, mid := range map[string][]string{
		"written on both paths of a conditional branch": {"cbz w0, .LBB0_2", "mov x8, x20", "b .LBB0_3", ".LBB0_2:", "mov x8, x21", ".LBB0_3:"},
		"a branch to the call":                          {"b .LBB0_2", ".LBB0_1:", "mov x8, x20", ".LBB0_2:"},
		"a loop before the call":                        {".LBB0_1:", "subs x9, x9, #1", "b.ne .LBB0_1"},
		"a return on one path":                          {"cbnz w0, .LBB0_1", "add sp, sp, #48", "ret", ".LBB0_1:"},
	} {
		fn, env := a64TestEnv()
		lines := append(append(append([]string{}, head...), mid...), tail...)
		if _, _, err := LowerARM64EngineFunc(lines, fn, env); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	fn, env := a64TestEnv()
	over := append(append([]string{}, head...), "cbz w0, .LBB0_1", "mov x8, #0", ".LBB0_1:", "st1 { v0.2d, v1.2d, v2.2d }, [x8]")
	if _, _, err := LowerARM64EngineFunc(append(over, tail...), fn, env); err == nil || !strings.Contains(err.Error(), "also reached") {
		t.Errorf("a tuple of 48 bytes over the slot at 40 where a branch goes: %v", err)
	}
}

func TestLowerARM64EngineLRLabel(t *testing.T) {
	fn, env := a64TestEnv()
	env.Plan.Rename["f"]["L__const.f.sp"] = ".Ldup.f.L__const.f.sp"
	lines := append([]string{a64LRPair[0], "ldr q0, L__const.f.sp"}, a64LRPair[1:]...)
	text, _, err := LowerARM64EngineFunc(lines, fn, env)
	if err != nil || !strings.Contains(text, "ldr q0, .Ldup.f.L__const.f.sp\n") {
		t.Fatalf("%v\n%s", err, text)
	}
}

// TestLowerARM64EngineLRRefused: each case is a use of LR, or of a slot of
// it, under which the drop would not be safe, and must be refused with its
// reason.
func TestLowerARM64EngineLRRefused(t *testing.T) {
	edit := func(base []string, at int, del int, add ...string) []string {
		out := append([]string{}, base[:at]...)
		out = append(out, add...)
		return append(out, base[at+del:]...)
	}
	single := []string{"sub sp, sp, #16", "str x30, [sp, #8]", "bl callee", "ldr x30, [sp, #8]", "add sp, sp, #16", "ret"}
	for name, c := range map[string]struct {
		lines []string
		want  string
	}{
		"LR in a w register":                     {edit(single, 1, 1, "str w30, [sp, #8]"), "LR used outside a stack save"},
		"LR twice in a pair":                     {edit(single, 1, 1, "stp x30, x30, [sp]"), "LR used outside a stack save"},
		"LR with a w register":                   {edit(single, 1, 1, "stp x30, w20, [sp]"), "LR used outside a stack save"},
		"LR in arithmetic":                       {edit(single, 2, 0, "add x30, x30, #4"), "LR used outside a stack save"},
		"LR loaded with writeback":               {edit(single, 3, 1, "ldr x30, [sp, #-8]!"), "LR used outside a stack save"},
		"LR stored with postindex":               {edit(single, 1, 1, "str x30, [sp], #16"), "LR used outside a stack save"},
		"LR through another base":                {edit(single, 1, 1, "str x30, [x8, #8]"), "LR used outside a stack save"},
		"LR beside the frame":                    {edit(single, 1, 1, "str x30, [sp, #16]"), "outside the LLVM frame"},
		"LR with the frame pointer":              {edit(a64LRPair, 0, 1, "stp x29, x30, [sp, #-32]!"), "reserved register"},
		"LR loaded from elsewhere":               {edit(single, 3, 1, "ldr x30, [sp]"), "never stored"},
		"slot read into another":                 {edit(single, 2, 0, "ldr x9, [sp, #8]"), "also reached"},
		"slot partly read":                       {edit(single, 2, 0, "ldrb w9, [sp, #15]"), "also reached"},
		"slot written by another":                {edit(single, 2, 0, "str w9, [sp, #12]"), "also reached"},
		"slot in a pair of another":              {edit(single, 2, 0, "stp x8, x9, [sp]"), "also reached"},
		"slot under a tuple address":             {edit(single, 2, 0, "add x8, sp, #0", "st1 { v0.2d }, [x8]", "mov x8, #0"), "also reached"},
		"writeback written apart":                {edit(single, 2, 0, "ldr x9, [sp, #0] !"), "a stack use beside"},
		"post-index written apart":               {edit(single, 2, 0, "ldr x9, [sp] , #16"), "a stack use beside"},
		"tuple address read past a label":        {edit(single, 2, 0, "add x8, sp, #0", "st1 { v0.2d }, [x8]", ".LBB0_1:", "ldr x0, [x8, #8]"), "is read by"},
		"tuple address read where a branch goes": {edit(single, 2, 0, "add x8, sp, #0", "st1 { v0.2d }, [x8]", "cbz w0, .LBB0_1", "mov x8, #0", ".LBB0_1:", "ldr x0, [x8, #8]"), "is read by"},
		"tuple address read by a branch":         {edit(single, 2, 0, "add x8, sp, #0", "st1 { v0.2d }, [x8]", "cbz x8, .LBB0_1", ".LBB0_1:", "mov x8, #0"), "is read by"},
		"tuple address past br":                  {edit(single, 2, 0, "add x8, sp, #0", "st1 { v0.2d }, [x8]", "br x9"), "may live past"},
		"tuple address to no label":              {edit(single, 2, 0, "add x8, sp, #0", "st1 { v0.2d }, [x8]", "b .Lnowhere"), "outside the function"},
		"tuple address read":                     {edit(single, 2, 0, "add x8, sp, #0", "st1 { v0.2d }, [x8]", "ldr x0, [x8, #8]"), "is read by"},
		"tuple address to the call":              {edit(single, 2, 0, "add x8, sp, #0", "st1 { v0.2d }, [x8]"), "also reached"},
		"tuple address a leaf keeps":             {edit(single, 2, 0, "add x19, sp, #0", "st1 { v0.2d }, [x19]"), "may live past"},
	} {
		fn, env := a64TestEnv()
		if _, _, err := LowerARM64EngineFunc(c.lines, fn, env); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v, want %q", name, err, c.want)
		}
	}
}

// TestLowerARM64EngineLRNegative: the negative build GEN_DROP_LR_PAIRS drops
// the save of x20 with x30's, which the proof must refuse, since x20 is
// loaded back after the call. Without the proof (=unchecked) the function
// is lowered without the store of x20, the mistake the engine's tests must
// then show.
func TestLowerARM64EngineLRNegative(t *testing.T) {
	fn, env := a64TestEnv()
	env.DropLRPairs = true
	if _, _, err := LowerARM64EngineFunc(a64LRPair, fn, env); err == nil || !strings.Contains(err.Error(), "the dropped slot 40(RSP) is also reached by \"ldr x20, [sp, #40]\"") {
		t.Errorf("GEN_DROP_LR_PAIRS=1: %v", err)
	}
	env.NoLRProof = true
	text, _, err := LowerARM64EngineFunc(a64LRPair, fn, env)
	if err != nil {
		t.Fatalf("GEN_DROP_LR_PAIRS=unchecked: %v", err)
	}
	if strings.Contains(text, "str x20, [sp, #40]") || !strings.Contains(text, "ldr x20, [sp, #40]") {
		t.Errorf("GEN_DROP_LR_PAIRS=unchecked kept the store of x20 or lost its load:\n%s", text)
	}
}
