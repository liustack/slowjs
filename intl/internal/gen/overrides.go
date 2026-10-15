// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import "fmt"

// browserOverrides are the values Chromium formats with where they differ from
// the CLDR release in cldrTag: Chromium's ICU data either comes from an older
// CLDR or was changed by Google. Each entry replaces one key of one locale, and
// of every locale under it that inherits the same value.
//
// When cldrTag or the Chromium the golden test was recorded with changes, run
// the golden test and drop the entries Chromium no longer needs.
var browserOverrides = []struct {
	locale, key, value string
	source             string // what Chromium does and what CLDR says
}{
	{"fr-CH", "n/symbols-numberSystem-latn/group", " ",
		"CLDR 48.2 changed the fr-CH group separator from U+202F to an apostrophe. Chromium 156 (ICU 78.2, CLDR 48.1), Safari and Node still use U+202F."},

	{"ko", "z/metazone/America_Eastern/long/generic", "북미 동부 시간", koNorthAmerica},
	{"ko", "z/metazone/America_Eastern/long/standard", "북미 동부 표준시", koNorthAmerica},
	{"ko", "z/metazone/America_Eastern/long/daylight", "북미 동부 하계 표준시", koNorthAmerica},
	{"ko", "z/metazone/America_Central/long/generic", "북미 중부 시간", koNorthAmerica},
	{"ko", "z/metazone/America_Central/long/standard", "북미 중부 표준시", koNorthAmerica},
	{"ko", "z/metazone/America_Central/long/daylight", "북미 중부 하계 표준시", koNorthAmerica},
	{"ko", "z/metazone/America_Mountain/long/generic", "북미 로키 산맥 시간", koNorthAmerica},
	{"ko", "z/metazone/America_Mountain/long/standard", "북미 로키 산맥 표준시", koNorthAmerica},
	{"ko", "z/metazone/America_Mountain/long/daylight", "북미 로키 산맥 하계 표준시", koNorthAmerica},
	{"ko", "z/metazone/America_Pacific/long/generic", "북미 태평양 시간", koNorthAmerica},
	{"ko", "z/metazone/America_Pacific/long/standard", "북미 태평양 표준시", koNorthAmerica},
	{"ko", "z/metazone/America_Pacific/long/daylight", "북미 태평양 하계 표준시", koNorthAmerica},

	{"uk", "c/UAH/symbol", "грн", uahVariant},
	{"ru", "c/UAH/symbol", "грн.", uahVariant},
}

const (
	koNorthAmerica = "Chromium 156 names the North American zones 북미 (North America) in Korean. CLDR 47 to 48.2 say 미 (US), and 산지/산악 for Mountain where Chromium says 로키 산맥."
	uahVariant     = "Chromium 156 writes the hryvnia as CLDR's symbol-alt-variant in Ukrainian and Russian. CLDR's symbol is ₴, which Chromium keeps for narrowSymbol."
)

// applyBrowserOverrides puts browserOverrides into the chunks' data. index
// maps a locale to its chunk.
func (g *gen) applyBrowserOverrides(index map[string]int) {
	for _, o := range browserOverrides {
		at, ok := index[o.locale]
		if !ok {
			fail(fmt.Errorf("browser override: no locale %s", o.locale))
		}

		cldr, ok := g.chunks[at].kv[o.key]
		if !ok {
			fail(fmt.Errorf("browser override: %s has no %s", o.locale, o.key))
		}

		if cldr == o.value {
			fail(fmt.Errorf("browser override: %s %s is %q in CLDR already, drop the override", o.locale, o.key, o.value))
		}

		for i := range g.chunks {
			c := &g.chunks[i]
			if g.descends(i, at) && c.kv[o.key] == cldr {
				c.kv[o.key] = o.value
			}
		}
	}
}

// descends reports whether chunk i is chunk at or one under it.
func (g *gen) descends(i, at int) bool {
	for ; i >= 0; i = g.chunks[i].parent {
		if i == at {
			return true
		}
	}
	return false
}
