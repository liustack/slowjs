// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package gen

import "testing"

// TestFollowPages: a label whose address the code takes, or that a jump table
// names, is reached by the indirect jumps of the function with the registers
// they have, and a register holds a page there only if every way in agrees.
func TestFollowPages(t *testing.T) {
	body := []string{
		"cbz x0, .LBB0_3", // 0
		"adr x10, .LBB0_1",
		"br x10",
		".LBB0_3:",
		"adrp x8, .LCPI0_0+16", // 4
		"cbz x1, .LBB0_1",
		"b .LBB0_4",
		".LBB0_1:", // 7
		"nop",
		".LBB0_4:", // 9
		"nop",
	}
	page := func(indirect map[string]bool, line, reg int) (string, int64, string) {
		sym, add, def, held := a64FollowPages(body, indirect, nil).Page(line, reg)
		switch {
		case def:
			return sym, add, "page"
		case held:
			return "", 0, "maybe"
		}
		return "", 0, "none"
	}
	indirect := a64IndirectLabels(body, "")
	if !indirect[".LBB0_1"] || indirect[".LBB0_4"] {
		t.Fatalf("labels taken by adr: %v", indirect)
	}
	if s, a, k := page(indirect, 5, 8); s != ".LCPI0_0" || a != 16 || k != "page" {
		t.Errorf("after the adrp: %q %d %s", s, a, k)
	}
	// The indirect jump reaches .LBB0_1 with x8 as it is at the jump.
	if _, _, k := page(indirect, 8, 8); k != "maybe" {
		t.Errorf("at the label the jump reaches: %s", k)
	}
	if _, _, k := page(indirect, 10, 8); k != "maybe" {
		t.Errorf("a join with that label: %s", k)
	}
	if _, _, k := page(indirect, 8, 9); k != "none" {
		t.Errorf("a register no way holds a page in: %s", k)
	}
	// With no indirect label the jump reaches nothing.
	if s, _, k := page(map[string]bool{}, 8, 8); s != ".LCPI0_0" || k != "page" {
		t.Errorf("without the address taken: %q %s", s, k)
	}
	// A jump table names its labels in the data after the code.
	table := a64IndirectLabels([]string{"nop"}, ".LJTI0_0:\n\t.long .LBB0_2-.LJTI0_0")
	if !table[".LBB0_2"] || !table[".LJTI0_0"] {
		t.Errorf("labels a table names: %v", table)
	}
}

// TestFollowPagesTables: a jump table dispatch reaches the labels of its own
// table and the computed goto the labels of the other tables, so a page one
// holds at its jump does not reach a read in the targets of the other.
func TestFollowPagesTables(t *testing.T) {
	body := []string{
		"adrp x9, data", // 0
		"adr x10, .LBB0_5",
		"ldrb w11, [x2, x8]",
		"add x10, x10, x11, lsl #2",
		"br x10",
		".LBB0_5:", // 5
		"nop",
		".Ltmp0:", // 7
		"mov w9, #1",
		"ldrsw x8, [x20, w24, uxtw #2]",
		"add x8, x20, x8",
		"br x8",
		".Ltmp1:", // 12
		"nop",
	}
	data := ".LJTI0_0:\n\t.byte (.LBB0_5-.LBB0_5)>>2\ntable:\n\t.word .Ltmp0-dispatch.rel\n\t.word .Ltmp1-dispatch.rel"
	tables := a64LabelTables(data)
	if len(tables[".LBB0_5"]) != 1 || len(tables["dispatch.rel"]) != 2 {
		t.Fatalf("tables %v", tables)
	}
	f := a64FollowPages(body, a64IndirectLabels(body, data), tables)
	if sym, _, def, _ := f.Page(6, 9); !def || sym != "data" {
		t.Errorf("at the label of the jump table: %q %v", sym, def)
	}
	// .Ltmp1 is reached from the computed goto, after mov w9: no page. The
	// jump table's br does not come here.
	if _, _, _, held := f.Page(13, 9); held {
		t.Errorf("at the label of the computed goto, a page of the jump table's jump")
	}
	// A jump of no known shape goes to every label.
	body[3] = "mov x10, x11"
	h := a64FollowPages(body, a64IndirectLabels(body, data), tables)
	if _, _, _, held := h.Page(8, 9); !held {
		t.Errorf("a jump of no known shape does not reach the labels of the computed goto")
	}
}
