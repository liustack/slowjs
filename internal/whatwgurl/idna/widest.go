// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package idna

import gointl "github.com/liustack/quickjs/internal/gointl"

// Widest walks the tables of every code point and returns the two that grow
// the most in the IDNA of a domain, for each UTF-16 unit they take, with what
// one becomes, in UTF-16 units: mapping, the code point whose mapping is the
// longest, and decomposing, the code point that stays as it is in mapping and
// whose canonical decomposition, which normalization makes before it
// composes, is the longest. Tests build the worst domains from them, so that
// what they measure follows the tables.
func Widest() (mapping rune, mapsTo int, decomposing rune, decomposesTo int) {
	nfc("a")
	bestMap, bestDec := 0.0, 0.0
	for r := rune(0); r <= 0x10FFFF; r++ {
		if 0xD800 <= r && r <= 0xDFFF {
			continue
		}
		s := string(r)
		v, sz := trie.lookupString(s)
		if sz == 0 {
			continue
		}
		in := float64(units16(s))
		switch (&profile{}).simplify(info(v).category()) {
		case mapped:
			m := string(info(v).appendMapping(nil, s))
			if f := float64(units16(m)) / in; f > bestMap {
				bestMap, mapping, mapsTo = f, r, units16(m)
			}
		case valid:
			d := norm.Normalize(s, gointl.NFD)
			if f := float64(units16(d)) / in; f > bestDec {
				bestDec, decomposing, decomposesTo = f, r, units16(d)
			}
		}
	}
	return
}

// units16 is the length of s in UTF-16 units.
func units16(s string) int {
	n := 0
	for _, r := range s {
		n++
		if r >= 0x10000 {
			n++
		}
	}
	return n
}
