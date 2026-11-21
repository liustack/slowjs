// Copyright (c) 2026 The liustack/slowjs Authors.
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

// Canonicalizer.aliasVariants leaves out only lookups that miss: random tags
// of the variants the alias tables name and of others, with the languages
// and regions of their keys, are canonicalized as when every variant is
// looked up.
func TestAliasVariantsSkipOnlyMisses(t *testing.T) {
	for _, compat := range []Compat{Standard, NodeICU} {
		fast, err := NewCanonicalizer(Embedded, CanonicalizeOptions{Compat: compat})
		if err != nil {
			t.Fatal(err)
		}
		all := *fast
		all.aliasVariants = nil
		var named []string
		for v := range fast.aliasVariants {
			if len(v) >= 5 || len(v) == 4 && v[0] >= '0' && v[0] <= '9' {
				named = append(named, v)
			}
		}
		slices.Sort(named)
		if len(named) < 10 {
			t.Fatalf("the alias tables name %d variants: %v", len(named), named)
		}
		plain := []string{"fonipa", "hepburn", "valencia", "1994", "biske", "abcde", "posix", "polyton", "alalc97", "x1234"}
		langs := []string{"en", "und", "sgn", "art", "zh", "hy", "no", "aa", "cel", "i", "de", "ja"}
		regions := []string{"", "", "US", "GB", "BR", "AX", "830", "DE"}
		rnd := rand.New(rand.NewPCG(1, 2))
		for k := range 20000 {
			var vs []string
			for range rnd.IntN(8) {
				if rnd.IntN(2) == 0 {
					vs = append(vs, named[rnd.IntN(len(named))])
				} else {
					vs = append(vs, plain[rnd.IntN(len(plain))])
				}
			}
			if k%100 == 0 {
				for i := range 300 {
					vs = append(vs, "v"+strings.Repeat("0", 3)+string(rune('a'+i%26))+string(rune('a'+i/26)))
				}
			}
			slices.Sort(vs)
			vs = slices.Compact(vs)
			rnd.Shuffle(len(vs), func(i, j int) { vs[i], vs[j] = vs[j], vs[i] })
			parts := []string{langs[rnd.IntN(len(langs))]}
			if r := regions[rnd.IntN(len(regions))]; r != "" {
				parts = append(parts, r)
			}
			tag := strings.Join(append(parts, vs...), "-")
			want, errWant := all.Canonicalize(tag)
			got, errGot := fast.Canonicalize(tag)
			if (errWant == nil) != (errGot == nil) || got.String() != want.String() {
				t.Fatalf("%v %s: %q, %v, want %q, %v", compat, tag, got.String(), errGot, want.String(), errWant)
			}
		}
	}
}
