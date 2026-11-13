// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package intl

import (
	"runtime"
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

// A string of digits in base 2, 8 or 16 is read in time linear in its
// length: one past the range of a float, 2**1024 and up, is an infinity
// before any of it is turned into decimal digits, and leading zeros cost
// nothing more. The answers are those of the conversion: the largest
// number under 2**1024 that is not an infinity, and the smallest that is.
func TestParseDecimalBaseLinear(t *testing.T) {
	f := strings.Repeat
	for _, c := range []struct {
		s, want string // want is "inf", "nan" or the integer
	}{
		{"0x" + f("f", 30000), "inf"},
		{"0x" + f("f", 524286), "inf"},
		{"0o" + f("7", 524286), "inf"},
		{"0b" + f("1", 524286), "inf"},
		{"0b1" + f("0", 1024), "inf"},
		{"0x" + f("f", 524285) + "g", "nan"},
		{"0b" + f("1", 524285) + "2", "nan"},
		{"0x" + f("0", 524285) + "1f", "31"},
		{"0b" + f("1", 1024), "inf"},
		{"0x" + f("f", 13) + "8" + f("0", 242), "179769313486231570814527423731704356798070567525844996598917476803157260780028538760589558632766878171540458953514382464234321326889464182768467546703537516986049910576551282076245490090389328944075868508455133942304583236903222948165808559332123348274797826204144723168738177180919299881250404026184124858368"},
		{"0x" + f("f", 13) + "c" + f("0", 242), "inf"},
	} {
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		d := ParseDecimal(c.s)
		runtime.ReadMemStats(&after)
		var got string
		switch {
		case d.IsNaN():
			got = "nan"
		case d.IsInf():
			got = "inf"
		default:
			got = d.m.integer
		}
		if got != c.want {
			t.Errorf("%.20q (%d bytes): %s, want %s", c.s, len(c.s), got, c.want)
		}
		if n := after.TotalAlloc - before.TotalAlloc; n > 16<<10 {
			t.Errorf("%.20q (%d bytes): Go allocated %d bytes", c.s, len(c.s), n)
		}
	}
}
