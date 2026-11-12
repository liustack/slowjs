// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package intl

import (
	"strings"
	"testing"
)

// A numeric string is read as StrDecimalLiteral reads it, as Number does: an
// exponent has one sign at most, and a mantissa that is no number makes the
// string NaN, and a zero one makes it zero, however large its exponent.
func TestParseDecimalExponent(t *testing.T) {
	huge := "99999999999999999999999"
	for _, s := range []string{
		"1e+-5", "1e--5", "1e-+5", "1e++5", "1e+-" + huge, "1e--" + huge, "1e+", "1e-", "1e",
		"1xe1073741825", "1xe-1073741825", "1xe" + huge, "1xe-" + huge, ".e" + huge, "e" + huge,
		"1.2.3e" + huge, "1.2.3e-1073741825", "-.e1073741825", "1e1x", "1e" + huge + "x",
	} {
		if d := ParseDecimal(s); !d.IsNaN() {
			t.Errorf("%q: %v, want NaN", s, d)
		}
	}
	for _, s := range []string{
		"0e" + huge, "0e1073741825", "0.000e-" + huge, "000.0e-1073741825", "-0e" + huge, "-0.0e1073741825",
	} {
		d := ParseDecimal(s)
		if d.IsNaN() || d.IsInf() || !d.isZero() || d.neg != strings.HasPrefix(s, "-") {
			t.Errorf("%q: %v, want zero", s, d)
		}
	}
	for _, c := range []struct {
		s   string
		inf bool
	}{
		{"1e" + huge, true}, {"1e+" + huge, true}, {"-1e1073741825", true}, {"1e-" + huge, false},
		{"1e-1073741825", false}, {"-1.5e-" + huge, false},
	} {
		d := ParseDecimal(c.s)
		if c.inf && !d.IsInf() || !c.inf && (!d.isZero() || d.IsNaN()) || d.neg != strings.HasPrefix(c.s, "-") {
			t.Errorf("%q: %v, want %s", c.s, d, map[bool]string{true: "an infinity", false: "zero"}[c.inf])
		}
	}
	for _, c := range [][2]string{{"1e+5", "100000"}, {"1e-0005", "0.00001"}, {"1.5e+2", "150"}, {"001e002", "100"}} {
		d := ParseDecimal(c[0])
		got := d.m.integer
		if d.m.fraction != "" {
			got += "." + d.m.written(1000)
		}
		if d.IsNaN() || d.IsInf() || got != c[1] {
			t.Errorf("%q: %v, want %s", c[0], d, c[1])
		}
	}
}
