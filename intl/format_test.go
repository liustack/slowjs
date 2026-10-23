// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package intl

import (
	"math"
	"strings"
	"testing"
)

// Format and formatToParts share the same text contract, even though only
// the latter needs classification, trimming and merging of parts.
func TestFormatStringMatchesParts(t *testing.T) {
	join := func(parts []part) string {
		var b strings.Builder
		for _, p := range parts {
			b.WriteString(p.Value)
		}
		return b.String()
	}
	for _, locale := range []string{"en-US", "zh-CN", "ar-EG", "fr-FR", "bn-IN", "fa-IR"} {
		for _, options := range []map[string]any{
			{}, {"style": "currency", "currency": "USD"}, {"style": "currency", "currency": "EUR", "currencyDisplay": "code", "currencySign": "accounting"},
			{"style": "currency", "currency": "USD", "currencyDisplay": "name"}, {"style": "percent"}, {"style": "unit", "unit": "meter-per-second"},
			{"notation": "compact"}, {"notation": "scientific"}, {"signDisplay": "exceptZero"}, {"signDisplay": "negative"},
			{"roundingIncrement": float64(25), "minimumFractionDigits": float64(2), "maximumFractionDigits": float64(2)},
		} {
			f, err := resolveNumberFormat(request{Locales: []string{locale}, Options: options})
			if err != nil {
				t.Fatal(err)
			}
			for _, v := range []any{math.Inf(1), math.Inf(-1), math.NaN(), math.Copysign(0, -1), 0.0, -0.00001, 1.0, -1234567.891, 1234567.891, "123456789012345678901234567890.12345", int64(-9007199254740993)} {
				if got, want := f.formatString(v), join(f.format(v)); got != want {
					t.Errorf("%s %v %v: %q != %q", locale, options, v, got, want)
				}
			}
		}
		for _, options := range []map[string]any{
			{"dateStyle": "medium", "timeStyle": "short", "timeZone": "UTC"},
			{"dateStyle": "full", "timeStyle": "long", "timeZone": "Asia/Kathmandu"},
			{"dateStyle": "full", "timeStyle": "long", "timeZone": "America/New_York"},
			{"dateStyle": "full", "calendar": "hebrew", "timeZone": "UTC"},
			{"year": "numeric", "month": "long", "day": "numeric", "calendar": "chinese", "timeZone": "UTC"},
		} {
			f, err := resolveDateTimeFormat(request{Locales: []string{locale}, Options: options}, "UTC")
			if err != nil {
				t.Fatal(err)
			}
			for _, v := range []float64{0, 1755324309123, -2208988800000, -62198755200000, 1710053999999, 1710054000000} {
				parts, err := f.format(v)
				if err != nil {
					t.Fatal(err)
				}
				got, err := f.formatString(v)
				if err != nil {
					t.Fatal(err)
				}
				if want := join(parts); got != want {
					t.Errorf("%s %v %v: %q != %q", locale, options, v, got, want)
				}
			}
		}
	}
}
