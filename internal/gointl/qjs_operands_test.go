// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package intl

import (
	"runtime"
	"testing"
)

// shiftedOperands gives the operands that operandsFor gives of the digits
// shiftDigits writes, for digits of every shape moved near the point and far
// from it, and holds a few hundred bytes for an exponent of a thousand
// million, where shiftDigits writes as many zeros.
func TestShiftedOperands(t *testing.T) {
	for _, d := range []struct{ integer, fraction string }{
		{"1", ""}, {"0", ""}, {"0", "5"}, {"1", "5"}, {"10", ""}, {"1", "50"}, {"123", "456"},
		{"0001", "2"}, {"9", "99999999999999999999"}, {"1", "000000000000000000001"},
		{"12345678901234567890", "123"}, {"100", "00"}, {"0", "000"},
	} {
		for exponent := -200; exponent <= 200; exponent++ {
			integer, fraction := shiftDigits(d.integer, d.fraction, exponent)
			want := operandsFor(integer, fraction, exponent)
			if got := shiftedOperands(d.integer, d.fraction, exponent); got != want {
				t.Fatalf("%s.%s moved %d: %+v, want %+v", d.integer, d.fraction, exponent, got, want)
			}
		}
	}
	for _, exponent := range []int{-1 << 30, 1 << 30} {
		var a, b runtime.MemStats
		runtime.ReadMemStats(&a)
		o := shiftedOperands("1", "5", exponent)
		runtime.ReadMemStats(&b)
		if n := b.TotalAlloc - a.TotalAlloc; n > 1<<10 {
			t.Errorf("moved %d: %d bytes", exponent, n)
		}
		if o.c != exponent {
			t.Errorf("moved %d: %+v", exponent, o)
		}
	}
}

// A unit name in long form, in scientific or engineering notation, takes its
// plural from the digits moved back by the exponent: a number of 13
// characters whose exponent is a thousand million is formatted with no zeros
// written out for it. (Compact notation writes the zeros out in its output,
// as ICU does.) format('1e-1073741000') of meters in long form took 4.3
// seconds and allocated 3.2 GB.
func TestShiftedOperandsFormat(t *testing.T) {
	loc, err := ParseLocale("en")
	if err != nil {
		t.Fatal(err)
	}
	for _, notation := range []Notation{NotationScientific, NotationEngineering} {
		f, err := NewNumberFormat(loc, NumberFormatOptions{Style: StyleUnit, Unit: "meter", UnitDisplay: UnitLong, Notation: notation})
		if err != nil {
			t.Fatal(err)
		}
		for _, s := range []string{"1e-1073741000", "1.5e-1073741000", "1e1073741000"} {
			var a, b runtime.MemStats
			runtime.ReadMemStats(&a)
			out := f.FormatDecimal(ParseDecimal(s))
			runtime.ReadMemStats(&b)
			if n := b.TotalAlloc - a.TotalAlloc; n > 64<<10 {
				t.Errorf("notation %d, %s: %d bytes for %d", notation, s, n, len(out))
			}
		}
	}
}
