// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package intl

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/liustack/slowjs/internal/gointl/internal/blob"
	"github.com/liustack/slowjs/internal/gointl/internal/layout"
	"github.com/liustack/slowjs/internal/gointl/internal/numdata"
)

// Raw sources resolve number and currency trees independently. A generated
// Chromium pack has already selected each resource and declares that fact.
func TestQJSNumberSourceAliases(t *testing.T) {
	files := fstest.MapFS{}
	pool := blob.NewPool(numdata.Version)
	for _, tag := range []string{"und", "en", "fr", "en-GB"} {
		l := &numdata.Locale{System: numdata.System{NumberingSystem: "latn"}, Currencies: []numdata.Currency{{Code: "EUR", DisplayName: tag}}}
		if tag == "en-GB" {
			l.NumberingSystem = "arab"
		}
		files["numbers/"+tag+".bin"] = &fstest.MapFile{Data: numdata.Encode(l, pool)}
	}
	files["numbersshared.bin"] = &fstest.MapFile{Data: pool.Bytes()}
	for _, name := range []string{"parentlocales", "likelysubtags"} {
		files[name+".bin"] = &fstest.MapFile{Data: []byte{layout.Pairs}}
	}
	for tree, target := range map[string]string{"locales": "en", "curr": "fr"} {
		b, err := blob.BuildIndex(layout.ICUTree, map[string][]byte{
			"bundle root": nil, "bundle en_GB": nil, "bundle " + target: nil,
			"alias en_GB": []byte(target),
		})
		if err != nil {
			t.Fatal(err)
		}
		files["icutree-"+tree+".bin"] = &fstest.MapFile{Data: b}
	}
	for _, c := range []struct{ name, marker, system, currency string }{
		{"raw", "", "latn", "fr"},
		{"resolved", "1", "arab", "en-GB"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if c.marker != "" {
				files["qjsnumbers.bin"] = &fstest.MapFile{Data: []byte(c.marker)}
			}
			l, err := loadNumbers(NewFS(files), qjsLocale("en-GB"))
			if err != nil {
				t.Fatal(err)
			}
			currency, ok := l.Currency("EUR")
			if !ok || l.NumberingSystem != c.system || currency.DisplayName != c.currency {
				t.Errorf("got %s, %q, want %s, %q", l.NumberingSystem, currency.DisplayName, c.system, c.currency)
			}
		})
	}
	files["qjsnumbers.bin"] = &fstest.MapFile{Data: []byte("unknown")}
	if _, err := loadNumbers(NewFS(files), qjsLocale("en-GB")); err == nil || !strings.Contains(err.Error(), "number resource version") {
		t.Fatalf("unknown version: %v", err)
	}
}
