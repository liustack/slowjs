// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

package intl

import (
	"sync"
	"unicode/utf8"
)

// The compare of two short Latin strings, without the normal form and with
// no allocation.
//
// The characters below U+0180, ASCII, Latin-1 and Latin Extended-A, are all
// starters, and the normal form of each is the character, or a letter and
// one accent: a string of them is in NFD once each character is, with no
// accent to put in order across two characters. In most collations the
// letter of each weighs a few collation elements of its own, which no
// context changes (no contraction starts with it and no prefix decides its
// weight), and an accent weighs one element from the table of diacritics.
// Where every character of two strings is such a character, the elements of
// each string are those of its characters, one after another, which a table
// built with the collator holds. compareShort builds the two lists from it,
// on the stack, and compares them with compareElements: the same function
// over the same lists as the general path, so every answer is the same. A
// character that is not simple in the collation (a digit with numeric
// sorting, the start of a contraction as "ch" is in Czech or "r" before a
// caron, U+0000 in a tailoring that maps it) sends the compare down the
// general path.
//
// The general path made both strings NFD, turned them into runes and built
// their elements on the heap, twice for the shared prefix with ICU's
// identical-prefix rule: 2.1 µs and 20 allocations for two ids of 12
// characters on an AMD EPYC, 73% of it the normal form.
//
// The table costs a few compares to build, so a collator builds it at its
// shortAfter-th compare: one made for a compare or two, as localeCompare
// with options makes one each call, never builds it. The decompositions it
// reads are those of the data built in, read once: Unicode never changes the
// canonical decomposition of a character it has assigned, and all of these
// are, so every collator's normalizer has the same.

// shortAfter is the compare at which a collator builds its table.
const shortAfter = 8

// shortMax is the longest string, in bytes, compareShort takes.
const shortMax = 128

// tableEnd is the first character the table does not cover.
const tableEnd = 0x180

// tableElements is the most elements the table holds for a character.
const tableElements = 3

// smallElements is the most elements of a string the compare of short
// strings builds in a list of its own frame. A longer one takes the lists of
// compareLong, which has room for shortMax characters of tableElements.
const smallElements = 32

// shortTable is, for each character below tableEnd, its collation elements
// in a collator: n of them, 0 for a character the table leaves out.
type shortTable struct {
	ce [tableEnd][tableElements]uint64
	n  [tableEnd]uint8
}

// shortTableOf is the collator's table, or nil before its shortAfter-th
// compare. Two goroutines may both build one: they build the same table.
func (c *Collator) shortTableOf() *shortTable {
	if t := c.short.Load(); t != nil {
		return t
	}
	if c.compares.Add(1) != shortAfter {
		return nil
	}
	t := c.makeShortTable()
	c.short.Store(t)
	return t
}

// A decomposition is n code points.
type decomposition struct {
	n    int
	code [2]rune
}

// latinDecompositions are the canonical decompositions of the characters
// below tableEnd: n code points, n is 1 for a character that has none and 0
// for one the table must leave out. It is nil where the data built in has
// no normalization tables.
var latinDecompositions = sync.OnceValue(func() *[tableEnd]decomposition {
	n, err := NewNormalizerFrom(Embedded)
	if err != nil {
		return nil
	}
	out := new([tableEnd]decomposition)
	for r := rune(0); r < tableEnd; r++ {
		to, ok := n.tables.Decompose(r)
		if !ok {
			out[r].n, out[r].code[0] = 1, r
			continue
		}
		for _, x := range to {
			if out[r].n == len(out[r].code) {
				out[r].n = 0
				break
			}
			out[r].code[out[r].n] = x
			out[r].n++
		}
	}
	return out
})

// makeShortTable builds the collator's table, nil where there are no
// decompositions to build it from.
func (c *Collator) makeShortTable() *shortTable {
	dec := latinDecompositions()
	if dec == nil {
		return nil
	}
	t := new(shortTable)
	var buf [tableElements * 2]uint64
	for r := rune(0); r < tableEnd; r++ {
		ces, ok := buf[:0], dec[r].n > 0
		for i := 0; ok && i < dec[r].n; i++ {
			ces, ok = c.simpleElements(ces, dec[r].code[i], i > 0)
			ok = ok && len(ces) <= tableElements
		}
		if ok && len(ces) > 0 && len(ces) <= tableElements {
			copy(t.ce[r][:], ces)
			t.n[r] = uint8(len(ces))
		}
	}
	return t
}

// simpleElements appends the elements of r to out as elementsFrom makes
// them with no context: an accent, after the letter it follows, from the
// table of diacritics, and anything else through appendCE32's tags. It is
// false where elementsFrom could make anything else of r: a context, a
// numeric digit, a table that does not hold what it points to, a lone
// surrogate, or a dot above Lithuanian may drop.
func (c *Collator) simpleElements(out []uint64, r rune, accent bool) ([]uint64, bool) {
	if r >= 0xd800 && r <= 0xdfff || c.lithuanianDotAbove && r == 0x307 {
		return out, false
	}
	if k := int(r) - 0x300; k >= 0 && k < len(c.diacritics) {
		return append(out, uint64(c.diacritics[k])<<16|commonTertiaryCE), accent
	}
	d, ce32 := c.lookup(r)
	for steps := 0; steps <= 16; steps++ {
		if ce, ok := simpleCE(ce32); ok {
			return append(out, ce), true
		}
		switch tagOf(ce32) {
		case tagFallback:
			d = &c.root.Data
			ce32 = d.Trie.Get(r)
		case tagLatinExpansion:
			return append(out,
				uint64(ce32&0xff000000)<<32|commonSecondaryCE|uint64(ce32&0xff0000)>>8,
				uint64(ce32&0xff00)<<16|commonTertiaryCE), true
		case tagExpansion32:
			index, length := int(ce32>>13), int(ce32>>8&31)
			if length > tableElements || !d.CE32s.Has(index, length) {
				return out, false
			}
			for k := index; k < index+length; k++ {
				ce, ok := simpleCE(d.CE32s.At(k))
				if !ok {
					return out, false
				}
				out = append(out, ce)
			}
			return out, true
		case tagExpansion:
			index, length := int(ce32>>13), int(ce32>>8&31)
			if length > tableElements || !d.CEs.Has(index, length) {
				return out, false
			}
			for k := index; k < index+length; k++ {
				out = append(out, d.CEs.At(k))
			}
			return out, true
		case tagDigit:
			if c.numeric || !d.CE32s.Has(int(ce32>>13), 1) {
				return out, false
			}
			ce32 = d.CE32s.At(int(ce32 >> 13))
		case tagU0000:
			if !d.CE32s.Has(0, 1) {
				return out, false
			}
			ce32 = d.CE32s.At(0)
		default:
			return out, false
		}
	}
	return out, false
}

// compareShort is CompareLimited of a and b where both have at most
// shortMax bytes, all characters of the table, and false otherwise. It
// compares what the general path compares: the elements from where ICU
// starts, with the variable ones shifted when punctuation is ignored.
func (c *Collator) compareShort(t *shortTable, a, b string, l *Limits) (int, bool) {
	if len(a) > shortMax || len(b) > shortMax {
		return 0, false
	}
	na, ok := t.count(a)
	if !ok {
		return 0, false
	}
	nb, ok := t.count(b)
	if !ok {
		return 0, false
	}
	start := 0
	if c.skipPrefix {
		start = c.comparisonStart(a, b, l)
	}
	if na > smallElements || nb > smallElements {
		return c.compareLong(t, a[start:], b[start:], l), true
	}
	var left, right [smallElements + 1]uint64
	return c.compareElements(c.shortElements(t, left[:0], a[start:], l), c.shortElements(t, right[:0], b[start:], l), l), true
}

// compareLong is compareShort's compare of two strings one of which has
// more than smallElements elements, apart so that a compare of two shorter
// ones does not clear these lists.
//
//go:noinline
func (c *Collator) compareLong(t *shortTable, a, b string, l *Limits) int {
	var left, right [shortMax*tableElements + 1]uint64
	return c.compareElements(c.shortElements(t, left[:0], a, l), c.shortElements(t, right[:0], b, l), l)
}

// count is the number of elements of s when every character of s is
// one the table holds, and false otherwise. An invalid byte, and a lone
// surrogate in WTF-8, are not.
func (t *shortTable) count(s string) (int, bool) {
	n := 0
	for i := 0; i < len(s); {
		r, size := rune(s[i]), 1
		if r >= utf8.RuneSelf {
			r, size = utf8.DecodeRuneInString(s[i:])
		}
		if r >= tableEnd || t.n[r] == 0 {
			return 0, false
		}
		n += int(t.n[r])
		i += size
	}
	return n, true
}

// shortElements appends the elements of s, of characters the table holds,
// and the end of text to out, as sortElementsFrom makes them.
func (c *Collator) shortElements(t *shortTable, out []uint64, s string, l *Limits) []uint64 {
	for i := 0; i < len(s); {
		r, size := rune(s[i]), 1
		if r >= utf8.RuneSelf {
			r, size = utf8.DecodeRuneInString(s[i:])
		}
		out = append(out, t.ce[r][:t.n[r]]...)
		i += size
	}
	out = append(out, noCE)
	if c.shifted {
		c.shiftVariables(out, l)
	}
	return out
}
