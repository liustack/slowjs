//go:build gointl_all

package intl_test

import (
	"fmt"
	"testing"

	intl "github.com/liustack/quickjs/internal/gointl"
)

// Numbers given exactly are written exactly, with every digit, and those too
// large for a float are infinite. Several of the rules the exact path brought
// out hold for floats too: whether a number is zero, for the sign, is asked
// of it as written; rounding that carries into the next power of ten takes
// that power's compact pattern; French writes exactly a thousand as "mille",
// and minus a thousand does not count.
func TestFormatDecimal(t *testing.T) {
	two := intl.Digits(2)
	for _, c := range []struct {
		tag   string
		opts  intl.NumberFormatOptions
		input any
		want  string
	}{
		{"en", intl.NumberFormatOptions{}, "12345678901234567890.5", "12,345,678,901,234,567,890.5"},
		{"en", intl.NumberFormatOptions{MinimumFractionDigits: two, MaximumFractionDigits: two, RoundingIncrement: 5},
			"123456789012345678901234567890.123", "123,456,789,012,345,678,901,234,567,890.10"},
		{"en", intl.NumberFormatOptions{}, "1e400", "∞"},
		{"en", intl.NumberFormatOptions{}, "0x1F", "31"},
		{"en", intl.NumberFormatOptions{}, "abc", "NaN"},
		{"en", intl.NumberFormatOptions{SignDisplay: intl.SignExceptZero, MaximumFractionDigits: two}, 0.0001, "0"},
		{"en", intl.NumberFormatOptions{SignDisplay: intl.SignNegative, MaximumFractionDigits: two}, -0.0001, "0"},
		{"en", intl.NumberFormatOptions{Notation: intl.NotationCompact}, 999.5, "1K"},
		{"en", intl.NumberFormatOptions{Notation: intl.NotationCompact}, 999999.5, "1M"},
		{"fr", intl.NumberFormatOptions{Notation: intl.NotationCompact, CompactDisplay: intl.CompactLong}, 1000.0, "mille"},
		{"fr", intl.NumberFormatOptions{Notation: intl.NotationCompact, CompactDisplay: intl.CompactLong}, -1000.0, "-1 millier"},
	} {
		f := newFormat(t, c.tag, c.opts)
		var got string
		switch v := c.input.(type) {
		case string:
			got = f.FormatDecimal(intl.ParseDecimal(v))
		case float64:
			got = f.Format(v)
		}
		if got != c.want {
			t.Errorf("%s %v = %q, want %q", c.tag, c.input, got, c.want)
		}
	}

	// Compact notation's word is a part of its own, the space before it a
	// literal.
	f := newFormat(t, "fr", intl.NumberFormatOptions{Notation: intl.NotationCompact, CompactDisplay: intl.CompactLong})
	got := fmt.Sprint(f.FormatToParts(-1000))
	if want := "[{minusSign -} {integer 1} {literal  } {compact millier}]"; got != want {
		t.Errorf("parts = %s, want %s", got, want)
	}
}
