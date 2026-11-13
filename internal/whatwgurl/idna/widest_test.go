// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package idna

import "testing"

// TestWidest checks what Widest finds in the tables of Unicode 17, which the
// limits of docs/changes/url.md name: a code point that maps to 18, U+FDFA,
// and one that decomposes to 3, U+01D6. New tables that change them change
// what the docs say.
func TestWidest(t *testing.T) {
	m, mn, d, dn := Widest()
	t.Logf("mapping %U to %d units, decomposing %U to %d", m, mn, d, dn)
	if m != 0xFDFA || mn != 18 || d != 0x01D6 || dn != 3 {
		t.Errorf("Widest = %U %d %U %d, want U+FDFA 18 U+01D6 3", m, mn, d, dn)
	}
}
