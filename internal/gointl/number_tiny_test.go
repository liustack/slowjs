//go:build gointl_all

package intl_test

import (
	"strings"
	"testing"
	"time"

	intl "github.com/liustack/quickjs/internal/gointl"
)

// TestTinyDecimals pins numbers given as strings far below the smallest
// float, which are held exactly: each is what node's ICU writes for it. The
// zeros after the point are not written out unless the format keeps them, so
// even one with a billion of them formats at once.
func TestTinyDecimals(t *testing.T) {
	zeros := func(n int, tail string) string { return "0." + strings.Repeat("0", n) + tail }
	cases := []struct {
		opts intl.NumberFormatOptions
		in   string
		want string
	}{
		{intl.NumberFormatOptions{}, "1e-400", "0"},
		{intl.NumberFormatOptions{}, "123.456e-420", "0"},
		{intl.NumberFormatOptions{MaximumSignificantDigits: intl.Digits(2)}, "1.5e-450", zeros(449, "15")},
		{intl.NumberFormatOptions{Notation: intl.NotationScientific}, "1e-400", "1E-400"},
		{intl.NumberFormatOptions{Notation: intl.NotationEngineering, MaximumFractionDigits: intl.Digits(0)}, "1e-400", "100E-402"},
		{intl.NumberFormatOptions{Notation: intl.NotationScientific}, "-1.5e-5000", "-1.5E-5000"},
		{intl.NumberFormatOptions{RoundingMode: intl.Ceil, MaximumFractionDigits: intl.Digits(2)}, "1e-999999999", "0.01"},
		{intl.NumberFormatOptions{RoundingMode: intl.Expand, MaximumFractionDigits: intl.Digits(0)}, "-1e-999999999", "-1"},
		{intl.NumberFormatOptions{RoundingMode: intl.Floor, MaximumFractionDigits: intl.Digits(2)}, "-1e-500", "-0.01"},
		{intl.NumberFormatOptions{Style: intl.StylePercent, MaximumFractionDigits: intl.Digits(3)}, "1e-500", "0%"},
		{intl.NumberFormatOptions{RoundingIncrement: 5, MinimumFractionDigits: intl.Digits(2),
			MaximumFractionDigits: intl.Digits(2), RoundingMode: intl.Ceil}, "1e-500", "0.05"},
		{intl.NumberFormatOptions{Notation: intl.NotationCompact}, "1e-500", zeros(499, "1")},
		{intl.NumberFormatOptions{SignDisplay: intl.SignExceptZero}, "-1e-500", "0"},
		{intl.NumberFormatOptions{MaximumSignificantDigits: intl.Digits(1), RoundingPriority: intl.MorePrecision}, "9.5e-420", zeros(418, "1")},
		{intl.NumberFormatOptions{Notation: intl.NotationScientific, MaximumFractionDigits: intl.Digits(1)}, "9.96e-999999990", "1E-999999989"},
	}
	for _, c := range cases {
		f := newFormat(t, "en", c.opts)
		start := time.Now()
		got := f.FormatDecimal(intl.ParseDecimal(c.in))
		if got != c.want {
			t.Errorf("%+v: %s = %q, want %q", c.opts, c.in, abbreviate(got), abbreviate(c.want))
		}
		if d := time.Since(start); d > time.Second {
			t.Errorf("%+v: %s took %v", c.opts, c.in, d)
		}
	}
}

// TestDecimalExponentBounds pins that an exponent too large to write a number
// out with is taken as an infinity or zero, as one too large for an int is,
// rather than overflowing.
func TestDecimalExponentBounds(t *testing.T) {
	f := newFormat(t, "en", intl.NumberFormatOptions{})
	for in, want := range map[string]string{
		"-1e9223372036854775807": "-∞",
		"1e9223372036854775807":  "∞",
		"1e-9223372036854775808": "0",
		"5e-4000000000":          "0",
		"5e4000000000":           "∞",
	} {
		if got := f.FormatDecimal(intl.ParseDecimal(in)); got != want {
			t.Errorf("%s = %q, want %q", in, got, want)
		}
	}
}

func abbreviate(s string) string {
	if len(s) <= 40 {
		return s
	}
	return s[:20] + "..." + s[len(s)-20:]
}
