// Copyright (c) 2026 The liustack/tifajs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

package intl

import (
	"math/rand/v2"
	"slices"
	"strings"
	"testing"
)

// sortElementsWhole is sortElementsFrom before it put in NFD only a window
// of the text before start: the whole of it.
func (c *Collator) sortElementsWhole(s string, start int, l *Limits) []uint64 {
	text := c.normalizedText(s[start:], l)
	from := 0
	if start > 0 {
		before := c.normalizedText(s[:start], l)
		from = len(before)
		text = append(before, text...)
	}
	ces := c.elementsFrom(text, from, l)
	if c.shifted {
		c.shiftVariables(ces, l)
	}
	return ces
}

// windowPieces are what the strings of TestSortWindowAsWhole are made of:
// accents, which NFD puts in order and which no boundary splits, Hangul
// syllables and jamo, the Japanese length and iteration marks, whose
// weight is that of what comes before them, digits, Thai prevowels, a
// character NFD takes apart, a lone surrogate and a character past the
// BMP.
var windowPieces = []string{
	"a", "b", "c", "e", "ab", "x", " ", "-", "́", "̣", "̀", "̣̂",
	"한", "ㅎ", "ᄀ", "ᅡ", "ᆨ", "カ", "か", "ー", "ゝ", "ゞ", "ヽ", "々", "キ", "ﾞ", "\u3099", "\u309a", "\u0323\u3099", "カ\u0323",
	"0", "1", "9", "٥", "เ", "ก", "แ", "ข", "ไ", "é", "ǖ", "ḉ", "ch", "ll", "\xed\xa0\x80", "😀",
}

// The elements of a string from where a comparison starts are those of the
// whole text before it put in NFD, with a window of it at any width, in
// collations with prefix contexts (Japanese), contractions (Spanish, Thai),
// backward accents, numeric sorting and ignored punctuation.
func TestSortWindowAsWhole(t *testing.T) {
	yes := true
	rnd := rand.New(rand.NewPCG(5, 6))
	for _, c := range []struct {
		tag  string
		opts CollatorOptions
	}{
		{"und", CollatorOptions{}}, {"en", CollatorOptions{}}, {"ja", CollatorOptions{}},
		{"es", CollatorOptions{}}, {"th", CollatorOptions{}}, {"ko", CollatorOptions{}},
		{"fr-CA", CollatorOptions{}}, {"en", CollatorOptions{Numeric: &yes}},
		{"ar", CollatorOptions{Numeric: &yes}}, {"en", CollatorOptions{IgnorePunctuation: &yes}},
		{"ja", CollatorOptions{Sensitivity: SensitivityAccent}},
	} {
		loc, err := ParseLocale(c.tag)
		if err != nil {
			t.Fatal(err)
		}
		opts := c.opts
		opts.Compat = NodeICU
		coll, err := NewCollator(loc, opts)
		if err != nil {
			t.Fatal(err)
		}
		random := func(n int) string {
			var b strings.Builder
			for range n {
				b.WriteString(windowPieces[rnd.IntN(len(windowPieces))])
			}
			return b.String()
		}
		for k := range 4000 {
			shared := random(rnd.IntN(60))
			if k%50 == 0 {
				shared = strings.Repeat(random(3), 40) + shared
			}
			a, b := shared+random(1+rnd.IntN(4)), shared+random(1+rnd.IntN(4))
			if a == b {
				continue
			}
			start := coll.comparisonStart(a, b, nil)
			for _, s := range []string{a, b} {
				want := coll.sortElementsWhole(s, start, nil)
				for _, w := range []int{0, 1, 2, 5, prefixWindow} {
					if got := coll.sortElementsWindow(s, start, w, nil); !slices.Equal(got, want) {
						t.Fatalf("%s %+v, %+q from %d, a window of %d: %x, want %x", c.tag, c.opts, s, start, w, got, want)
					}
				}
			}
			if got, want := coll.Compare(a, b), coll.compareElements(coll.sortElementsWhole(a, start, nil), coll.sortElementsWhole(b, start, nil), nil); got != want {
				t.Fatalf("%s %+v, %+q and %+q: %d, want %d", c.tag, c.opts, a, b, got, want)
			}
		}
	}
}

// A compare of two strings that share a long prefix puts in NFD and weighs
// only the text after it, and a window before it: it asks room for the
// work on that, whatever the length of the prefix. It normalized and
// copied the whole prefix of both, about 0.5 GiB for 16 Mi units, and
// 600 to 870 ms on an Apple M4.
func TestSharedPrefixWorkIsSmall(t *testing.T) {
	loc, err := ParseLocale("en")
	if err != nil {
		t.Fatal(err)
	}
	coll, err := NewCollator(loc, CollatorOptions{Compat: NodeICU})
	if err != nil {
		t.Fatal(err)
	}
	const n = 4 << 20
	for _, c := range []struct{ name, a, b string }{
		{"ASCII", strings.Repeat("ab", n/2) + "c", strings.Repeat("ab", n/2) + "d"},
		{"accents", strings.Repeat("é", n/2) + "c", strings.Repeat("é", n/2) + "d"},
		{"Japanese", strings.Repeat("カー", n/2) + "か", strings.Repeat("カー", n/2) + "き"},
	} {
		var asked int64
		l := &Limits{Check: func() {}, Room: func(n int64) { asked += n }}
		coll.CompareLimited(c.a, c.b, l)
		if asked > 64<<10 {
			t.Errorf("%s: a compare after a shared prefix of %d bytes asked room for %d KiB, want at most 64", c.name, len(c.a), asked>>10)
		}
	}
}
