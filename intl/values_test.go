// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package intl // import "github.com/liustack/slowjs/intl"

import (
	"slices"
	"testing"

	gointl "github.com/liustack/slowjs/internal/gointl"
)

// TestSupportedCollations checks unlistedCollations: the collations
// supportedValuesOf lists are those some language of Chromium's has in
// go-intl's data.
func TestSupportedCollations(t *testing.T) {
	info, err := gointl.NewLocaleInfo(gointl.Embedded, gointl.LocaleInfoOptions{Compat: goCompat})
	if err != nil {
		t.Fatal(err)
	}

	had := map[string]bool{}
	for _, tag := range chromiumLanguages("Collator") {
		l, err := gointl.ParseLocale(tag)
		if err != nil {
			t.Fatal(err)
		}

		c, err := info.Collations(l)
		if err != nil {
			t.Fatal(err)
		}

		for _, co := range c {
			had[co] = true
		}
	}
	values, err := supportedValues()
	if err != nil {
		t.Fatal(err)
	}

	want := slices.Sorted(func(yield func(string) bool) {
		for co := range had {
			if !yield(co) {
				return
			}
		}
	})
	if got := values["collation"]; !slices.Equal(got, want) {
		t.Errorf("supportedValuesOf('collation') is %v, the languages have %v", got, want)
	}
}
