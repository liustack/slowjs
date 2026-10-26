// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
package gen

import "testing"

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
