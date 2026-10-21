// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package intl // import "github.com/liustack/quickjs/intl"

import "testing"

func TestCLDRData(t *testing.T) {
	x := cldr()
	if got, want := x.supplemental().str("version"), "48.2.3"; got != want {
		t.Errorf("CLDR version %q, want %q", got, want)
	}

	for _, test := range []struct{ locale, key, want string }{
		{"en", "g/months/format/wide/1", "January"},
		{"zh", "g/months/format/abbreviated/8", "8月"},
		{"zh", "g/dateFormats/full", "y年M月d日EEEE"},
		{"zh-Hant", "g/dateTimeFormats/availableFormats/hm", "Bh:mm"},
		{"de", "n/symbols-numberSystem-latn/decimal", ","},
		{"ar-EG", "n/defaultNumberingSystem", "arab"},
		{"en", "c/USD/symbol", "$"},
		{"zh", "u/short/kilometer/unitPattern-count-other", "{0}公里"},
		{"en", "z/metazone/America_Eastern/short/standard", "EST"},
		// Inherited through en-001.
		{"en-GB", "g/months/format/wide/1", "January"},
		{"en-GB", "g/dateFormats/short", "dd/MM/y"},
	} {
		d := x.locale(test.locale)
		if d == nil {
			t.Errorf("no data for %s", test.locale)
			continue
		}

		if got := d.str(test.key); got != test.want {
			t.Errorf("%s %s = %q, want %q", test.locale, test.key, got, test.want)
		}
	}
	// Every chunk decodes.
	for i := range x.chunks {
		x.load(i)
	}
}
