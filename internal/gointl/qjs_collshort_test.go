// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

package intl

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"
)

// shortCollators are collators of many locales with every combination of
// the options of Intl.Collator, with ICU's identical-prefix rule and
// without.
func shortCollators(tb testing.TB, locales []string) []*Collator {
	tb.Helper()
	t, f := true, false
	var out []*Collator
	for _, tag := range locales {
		loc, err := ParseLocale(tag)
		if err != nil {
			tb.Fatal(err)
		}
		for _, compat := range []Compat{NodeICU, 0} {
			for _, usage := range []CollatorUsage{UsageSort, UsageSearch} {
				for s := SensitivityDefault; s <= SensitivityVariant; s++ {
					for _, numeric := range []*bool{nil, &t, &f} {
						for cf := CaseFirstDefault; cf <= CaseFirstFalse; cf++ {
							for _, punct := range []*bool{nil, &t, &f} {
								c, err := NewCollator(loc, CollatorOptions{Usage: usage, Sensitivity: s, Numeric: numeric, CaseFirst: cf, IgnorePunctuation: punct, Compat: compat})
								if err != nil {
									tb.Fatal(err)
								}
								out = append(out, c)
							}
						}
					}
				}
			}
		}
	}
	return out
}

// shortEdgePairs are the edge cases, each with each: case, digits,
// punctuation, spaces, controls, the letters of contractions, a character
// that is not ASCII, and strings at and past shortMax.
func shortEdgePairs() [][2]string {
	edges := []string{"", "a", "A", "b", "B", "ab", "aB", "Ab", "a b", "a-b", "a_b", "ab ", " ab", "a\tb", "a\x00b", "\x00", "\x7f",
		"1", "2", "10", "02", "a1", "a10", "a2", "1a", "-1", "1.5", "1,5", "ch", "Ch", "cH", "CH", "c", "h", "cz", "d", "ll", "l", "dz", "lj", "nj",
		"aa", "Aa", "AA", "å", "Å", "a\u030a", "ä", "Ä", "ö", "ø", "æ", "ß", "é", "e\u0301", "è", "ł", "Ł", "ř", "č", "ı", "İ", "ŉ", "ſ", "đ", "ð", "þ", "ñ", "ü", "ő", "ç", "\u00a0", "\u00ad", "ÿ", "Ÿ", "ŀ", "ĳ", "Ǆ", "\xed\xa0\x80", "ae", "oe", "ss", "th", "i", "I", "dzs", "cs", "ny", "gy", "x", "y", "z", "Z", "item-1", "item-10", "item-2",
		"item-1xxxx", "Item-1", "item 1", "item_1", "item1", "a.b", "a,b", "a'b", "a\"b", "a!b", "a@b", "~", "`", "^", "{}", "[]", "()",
		strings.Repeat("a", shortMax), strings.Repeat("a", shortMax-1) + "b", strings.Repeat("a", shortMax+1)}
	var out [][2]string
	for _, x := range edges {
		for _, y := range edges {
			out = append(out, [2]string{x, y})
		}
	}
	return out
}

// shortPairs are n random pairs of strings, most of characters below U+0180, over an alphabet heavy in
// what collations treat apart, half of them sharing a prefix and a quarter
// the same but for one character.
func shortPairs(r *rand.Rand, n int) [][2]string {
	alphabet := []rune("aAbBcChHdDlLnNsSzZiIoOyY0123456789 -_.,'!?/\\#@\t\x00\x01\x7f~`^éÉäÄöÖåÅøæßñçłŁřčšžőüİıđĳſ\u00a0\u00ad\u0301\u0308")
	word := func() string {
		b := make([]rune, r.Intn(12))
		for i := range b {
			b[i] = alphabet[r.Intn(len(alphabet))]
		}
		return string(b)
	}
	var out [][2]string
	for len(out) < n {
		x, y := word(), word()
		if r.Intn(2) == 0 {
			p := word()
			x, y = p+x, p+y
		}
		if r.Intn(4) == 0 {
			b := []rune(x)
			if len(b) > 0 {
				b[r.Intn(len(b))] = alphabet[r.Intn(len(alphabet))]
			}
			y = string(b)
		}
		out = append(out, [2]string{x, y})
	}
	return out
}

// TestCompareShortAsGeneral holds the compare of short Latin strings to the
// general path, which builds the elements of the normal form: the same
// answer for every pair, collator and order, and the fast path taken where
// it should be. Each collator takes 40 random pairs and the next 40 edge
// pairs, round the list.
func TestCompareShortAsGeneral(t *testing.T) {
	locales := []string{"en", "und", "de", "fr-CA", "cs", "sk", "da", "sv", "fi", "es-u-co-trad", "hr", "hu", "lt", "tr", "cy",
		"th", "ja", "zh", "ko", "ar", "he", "ru", "vi", "en-u-kn-kf-upper", "en-u-ka-shifted", "de-u-co-phonebk", "ln", "smn"}
	if testing.Short() {
		locales = locales[:6]
	}
	edges := shortEdgePairs()
	r := rand.New(rand.NewSource(1))
	fast, all, next := 0, 0, 0
	for _, c := range shortCollators(t, locales) {
		table := c.makeShortTable()
		pairs := shortPairs(r, 40)
		for k := 0; k < 40; k++ {
			pairs = append(pairs, edges[next%len(edges)])
			next++
		}
		for _, p := range pairs {
			for _, ab := range [][2]string{p, {p[1], p[0]}} {
				a, b := ab[0], ab[1]
				if a == b {
					continue
				}
				all++
				got, ok := c.compareShort(table, a, b, nil)
				if !ok {
					continue
				}
				fast++
				if want := c.compareGeneral(a, b, nil); got != want {
					o := c.ResolvedOptions()
					t.Fatalf("%s %+v skipPrefix=%v: compare(%q, %q) = %d, the general path %d", o.Locale, o, c.skipPrefix, a, b, got, want)
				}
			}
		}
	}
	t.Logf("%d of %d compares took the short path, %d edge pairs of %d", fast, all, next, len(edges))
	if fast < all/4 {
		t.Errorf("only %d of %d compares took the short path", fast, all)
	}
	if !testing.Short() && next < len(edges) {
		t.Errorf("only %d of %d edge pairs were compared", next, len(edges))
	}
}

// TestShortTableBuiltLate checks that a collator builds its table at its
// shortAfter-th compare of two strings that differ, and answers the same
// before and after.
func TestShortTableBuiltLate(t *testing.T) {
	loc, _ := ParseLocale("en")
	c, err := NewCollator(loc, CollatorOptions{Compat: NodeICU})
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= shortAfter+2; i++ {
		if c.Compare("same", "same") != 0 {
			t.Fatal("a string differs from itself")
		}
		a, b := fmt.Sprintf("item-%d", i), fmt.Sprintf("Item-%d", i+1)
		if got, want := c.Compare(a, b), c.compareGeneral(a, b, nil); got != want {
			t.Fatalf("compare %d: Compare(%q, %q) = %d, the general path %d", i, a, b, got, want)
		}
		if built := c.short.Load() != nil; built != (i >= shortAfter) {
			t.Fatalf("after compare %d the table is built: %v", i, built)
		}
	}
}

// TestShortTableTakesASCII checks the ASCII characters the table of a
// collator holds: all in the root order, and where a collation gives a
// character a context, not that character.
func TestShortTableTakesASCII(t *testing.T) {
	for _, tc := range []struct {
		tag     string
		numeric bool
		without string
	}{
		{"en", false, ""},
		{"de", false, ""},
		{"en", true, "0123456789"},
		{"cs", false, "CRSZcrsz"},
		{"es-u-co-trad", false, "CLNcln"},
	} {
		loc, _ := ParseLocale(tc.tag)
		c, err := NewCollator(loc, CollatorOptions{Numeric: &tc.numeric})
		if err != nil {
			t.Fatal(err)
		}
		table := c.makeShortTable()
		var missing []string
		for b := 0; b < 128; b++ {
			if table.n[b] == 0 {
				missing = append(missing, fmt.Sprintf("%q", rune(b)))
			}
		}
		var want []string
		for _, r := range tc.without {
			want = append(want, fmt.Sprintf("%q", r))
		}
		if strings.Join(missing, " ") != strings.Join(want, " ") {
			t.Errorf("%s numeric=%v: the table leaves out %v, want %v", tc.tag, tc.numeric, missing, want)
		}
	}
	// Every character below tableEnd is itself or two code points in NFD.
	for r, d := range latinDecompositions() {
		if d.n == 0 {
			t.Errorf("%q decomposes into more than two code points", rune(r))
		}
	}
	// The letters of the names of a people list, in the root order.
	loc, _ := ParseLocale("en")
	c, err := NewCollator(loc, CollatorOptions{})
	if err != nil {
		t.Fatal(err)
	}
	table := c.makeShortTable()
	for _, r := range "ÉéÅåÇçŁłřØøäöüñő" {
		if table.n[r] == 0 {
			t.Errorf("en: the table leaves out %q", r)
		}
	}
}

// The compares of the study of React libraries: ids sorted as a table sorts
// them, and names with accents as a people list. The General ones take the
// path every compare took before the table, for the numbers before and after.
func BenchmarkCompareIDs(b *testing.B)        { benchCompare(b, compareIDs(), false) }
func BenchmarkCompareIDsGeneral(b *testing.B) { benchCompare(b, compareIDs(), true) }
func BenchmarkCompareNames(b *testing.B)      { benchCompare(b, compareNames(), false) }
func BenchmarkCompareNamesGeneral(b *testing.B) {
	benchCompare(b, compareNames(), true)
}

func benchCompare(b *testing.B, s []string, general bool) {
	loc, _ := ParseLocale("en-US")
	c, err := NewCollator(loc, CollatorOptions{Compat: NodeICU})
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	n := 0
	for i := 0; i < b.N; i++ {
		x, y := s[i%len(s)], s[(i*7+1)%len(s)]
		if general {
			if x != y {
				n += c.compareGeneral(x, y, nil)
			}
		} else {
			n += c.Compare(x, y)
		}
	}
	_ = n
}

func compareIDs() []string {
	ids := make([]string, 300)
	for i := range ids {
		ids[i] = fmt.Sprintf("item-%d", (i*7919)%1000)
		ids[i] += strings.Repeat("x", 12-len(ids[i]))
	}
	return ids
}

func compareNames() []string {
	base := []string{"Émile", "zoë", "Ana", "Ångström", "bob", "Çelik", "Dvořák", "eve", "Ørsted", "Łukasz", "Mia", "noah"}
	names := make([]string, 300)
	for i := range names {
		names[i] = fmt.Sprintf("%s %c%d", base[i%12], 'a'+(i*7)%26, i%37)
	}
	return names
}
