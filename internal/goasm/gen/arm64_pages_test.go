// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package gen

import "testing"

// TestFollowOwnPages: a label whose address the code takes, or that a jump
// table names, can be reached with any registers, so it starts with no page
// even when every branch to it carries one.
func TestFollowOwnPages(t *testing.T) {
	own := func(s string) bool { return s == ".LCPI0_0" }
	body := []string{
		"adrp x8, .LCPI0_0",
		"adr x10, .LBB0_1",
		"cbz x0, .LBB0_1",
		"cbz x0, .LBB0_2",
		".LBB0_1:",
		"nop",
		".LBB0_2:",
		"nop",
	}
	at := func(indirect map[string]bool, line, reg int) string {
		return a64FollowOwnPages(body, own, indirect).At(line, reg)
	}
	indirect := a64IndirectLabels(body, "")
	if !indirect[".LBB0_1"] || indirect[".LBB0_2"] {
		t.Fatalf("labels taken by adr: %v", indirect)
	}
	if got := at(indirect, 5, 8); got != "" {
		t.Errorf("after an address-taken label: %q", got)
	}
	if got := at(indirect, 7, 8); got != "" {
		t.Errorf("a join with the address-taken label's way: %q", got)
	}
	if got := at(map[string]bool{}, 5, 8); got != ".LCPI0_0" {
		t.Errorf("without the address taken: %q", got)
	}
	// A jump table names its labels in the data after the code.
	table := a64IndirectLabels([]string{"nop"}, ".LJTI0_0:\n\t.long .LBB0_2-.LJTI0_0")
	if !table[".LBB0_2"] || !table[".LJTI0_0"] {
		t.Errorf("labels a table names: %v", table)
	}
}
