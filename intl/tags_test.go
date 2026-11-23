// Copyright (c) 2026 The liustack/beastjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

package intl // import "github.com/liustack/beastjs/intl"

import (
	"testing"

	"github.com/liustack/beastjs/internal/intlhook"
	"github.com/liustack/beastjs/internal/stopsweep"
)

// manyVariants makes l, a list of N / 9615 tags of 9,615 units each, of ten
// variants that are aliases and 1,590 that are not, each tag apart from the
// others. Each alias replaced starts the search for aliases again, from the
// first variant.
const manyVariants = `var mk = (j) => 'en-aaland-arevela-arevmda-heploc-polytoni-bokmal-nynorsk-hakka-lojban-saaho-' +
	Array.from({length: 1590}, (_, i) => 'v' + (i + j * 7).toString(36).padStart(4, '0')).join('-');
var l = Array.from({length: N / 9615}, (_, j) => mk(j))`

// tagLists are calls on a list of locales of tags at about the most units a
// tag may have.
var tagLists = []struct{ name, call string }{
	{"getCanonicalLocales", `Intl.getCanonicalLocales(l).length`},
	{"a constructor", `new Intl.Collator(l).resolvedOptions().locale`},
	{"supportedLocalesOf", `Intl.Collator.supportedLocalesOf(l).length`},
}

// A list of locales is checked after every tag of intlhook.MaxTag units:
// the work on one tag at the cap is short, and a check every
// intlhook.Piece units, four such tags, let 64 tags of many variants hold
// the call 44 ms on an Apple M4 with no check.
func TestTagListChecksEveryTag(t *testing.T) {
	const n = 64 * 9615
	for _, c := range tagLists {
		t.Run(c.name, func(t *testing.T) {
			vm := newVM(t)
			setUp(t, vm, manyVariants, n)
			s, err := measureStops(t, vm, c.call)
			if err != nil {
				t.Fatalf("%s: %v", c.call, err)
			}
			if s.stops < n/intlhook.MaxTag {
				t.Errorf("%s read vmhook.Stop %d times for %d units, want at least %d", c.call, s.stops, n, n/intlhook.MaxTag)
			}
		})
	}
}

// The calls on a list of tags of many variants stop within the grace of a
// check every tag at the cap, at every timeout: one such tag took 7 ms to
// put in canonical form, and 18 ms to maximize in Intl.Locale, on an Apple
// M4, looking each of its variants up for an alias again after each alias
// replaced.
func TestTagListsStopAtEveryTimeout(t *testing.T) {
	const n = 64 * 9615
	for _, c := range tagLists {
		t.Run(c.name, func(t *testing.T) {
			vm := newVM(t)
			setUp(t, vm, manyVariants, n)
			stopsweep.Sweep(t, vm, c.call, n, intlhook.MaxTag)
		})
	}
}
