// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
package gen

import "testing"

// A misplaced data object (for example a byte shifted by wrong alignment
// padding) differs from llc's object in a literal byte, which diffData flags.
func TestDiffDataCatchesMisplacedByte(t *testing.T) {
	llc := []byte{1, 2, 3, 4}
	no := make([]bool, 4)
	if err := diffData("x", llc, no, llc, no); err != nil {
		t.Fatalf("identical data flagged: %v", err)
	}
	moved := []byte{1, 2, 9, 4}
	if diffData("x", llc, no, moved, no) == nil {
		t.Fatal("a shifted literal byte was not caught")
	}
	if diffData("x", llc, no, []byte{1, 2, 3}, make([]bool, 3)) == nil {
		t.Fatal("a length difference was not caught")
	}
}

// A wrong relocation: llc keeps a relocation where the generator wrote a
// literal, so the parser turned a pointer into a constant.
func TestDiffDataCatchesWrongRelocation(t *testing.T) {
	llc := []byte{0, 0, 0, 0}
	my := []byte{7, 0, 0, 0}
	allReloc := []bool{true, true, true, true}
	noFix := []bool{false, false, false, false}
	if diffData("x", llc, allReloc, my, noFix) == nil {
		t.Fatal("a literal where llc relocates was not caught")
	}
	// When the generator also marks the slot, both agree (a real pointer).
	if err := diffData("x", llc, allReloc, my, allReloc); err != nil {
		t.Fatalf("a matching relocation was flagged: %v", err)
	}
}

// A corrupted function byte outside a call displacement is caught; the four
// displacement bytes of a call are filled by the linker and are not.
func TestDiffBodyCatchesCorruptedByte(t *testing.T) {
	orig := []byte{0x90, 0xe8, 0, 0, 0, 0, 0x90} // nop; call rel32; nop
	linked := append([]byte(nil), orig...)
	linked[2], linked[3], linked[4], linked[5] = 0x11, 0x22, 0x33, 0x44 // linker filled the displacement
	if err := diffBody(orig, linked, []int{1}); err != nil {
		t.Fatalf("a filled call displacement was flagged: %v", err)
	}
	bad := append([]byte(nil), orig...)
	bad[0] = 0xcc
	if diffBody(orig, bad, []int{1}) == nil {
		t.Fatal("a corrupted code byte was not caught")
	}
}
