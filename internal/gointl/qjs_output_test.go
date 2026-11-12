// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package intl

import (
	"strings"
	"testing"
)

// Output is at least the digits FormatDecimal writes, for options that pad,
// round, carry, shift to a percent and round to an increment, in each
// notation, on numbers near one and far from it, and on the digits
// of BigInts.
func TestOutputBound(t *testing.T) {
	zero, one, two, three, five, twentyOne, hundred := 0, 1, 2, 3, 5, 21, 100
	opts := []NumberFormatOptions{
		{},
		{MinimumFractionDigits: &hundred, MaximumFractionDigits: &hundred},
		{MinimumSignificantDigits: &twentyOne, MaximumSignificantDigits: &twentyOne},
		{MaximumSignificantDigits: &three, MinimumIntegerDigits: twentyOne},
		{MaximumFractionDigits: &one},
		{MinimumFractionDigits: &five, MaximumSignificantDigits: &three, RoundingPriority: MorePrecision},
		{MinimumSignificantDigits: &twentyOne, RoundingPriority: LessPrecision},
		{Style: StylePercent},
		{Style: StylePercent, MaximumFractionDigits: &hundred},
		{MaximumFractionDigits: &zero, RoundingIncrement: 5000, RoundingMode: Ceil},
		{MinimumFractionDigits: &two, MaximumFractionDigits: &two, RoundingIncrement: 5000, RoundingMode: Ceil, Style: StylePercent},
		{Style: StyleUnit, Unit: "meter", UnitDisplay: UnitLong},
		{Style: StyleCurrency, Currency: "USD"},
	}
	en, err := ParseLocale("en")
	if err != nil {
		t.Fatal(err)
	}
	numbers := []string{"0", "1", "0.5", "9.99", "9.999", "999.95", "0.0000999", "1e-500", "1e500", "123456789e-30",
		"0." + strings.Repeat("9", 300), strings.Repeat("9", 300), "5e-450", "-1e-3000", "1e3000"}
	// A BigInt is kept as its digits, however many.
	exact := []string{strings.Repeat("9", 1000), "1" + strings.Repeat("0", 1000), "-" + strings.Repeat("5", 999)}
	for _, o := range opts {
		for _, n := range []Notation{NotationStandard, NotationScientific, NotationEngineering, NotationCompact} {
			o.Notation = n
			f, err := NewNumberFormat(en, o)
			if err != nil {
				t.Fatal(err)
			}
			for i, s := range append(numbers, exact...) {
				d := ParseDecimal(s)
				if i >= len(numbers) {
					d = ParseExactDecimal(s)
				}
				digits := 0
				for _, c := range f.FormatDecimal(d) {
					if c >= '0' && c <= '9' {
						digits++
					}
				}
				if out := f.Output(d); out < int64(digits) {
					t.Errorf("%+v, %s: Output %d, %d digits written", o, s, out, digits)
				}
			}
		}
	}
}

// Output tells, before anything is written, a number of a few characters
// whose written form is a thousand million digits from one that is short:
// only zeros after the point can be that many, as a number past the
// largest float is an infinity.
func TestOutputFar(t *testing.T) {
	en, err := ParseLocale("en")
	if err != nil {
		t.Fatal(err)
	}
	three, hundred := 3, 100
	for _, c := range []struct {
		opts NumberFormatOptions
		s    string
		long bool
	}{
		{NumberFormatOptions{MaximumSignificantDigits: &three}, "1e-1073741000", true},
		{NumberFormatOptions{Notation: NotationCompact}, "1e-1073741000", true},
		{NumberFormatOptions{}, "1e-1073741000", false},
		{NumberFormatOptions{Notation: NotationScientific}, "1e-1073741000", false},
		// lessPrecision takes the decimals' result, and morePrecision the
		// significant digits'.
		{NumberFormatOptions{MaximumFractionDigits: &hundred, RoundingPriority: LessPrecision}, "1e-1073741000", false},
		{NumberFormatOptions{MaximumFractionDigits: &hundred, RoundingPriority: MorePrecision}, "1e-1073741000", true},
		{NumberFormatOptions{Notation: NotationEngineering}, "1e1073741000", false},
		// Past the largest float a number is an infinity, as ToNumber makes it.
		{NumberFormatOptions{}, "1e1073741000", false},
		{NumberFormatOptions{Notation: NotationCompact}, "1e1073741000", false},
	} {
		f, err := NewNumberFormat(en, c.opts)
		if err != nil {
			t.Fatal(err)
		}
		if out := f.Output(ParseDecimal(c.s)); (out > 1<<20) != c.long {
			t.Errorf("%+v, %s: Output %d", c.opts, c.s, out)
		}
	}
}
