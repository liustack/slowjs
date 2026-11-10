//go:build gointl_all

package intl_test

import (
	"testing"

	intl "github.com/liustack/quickjs/internal/gointl"
)

// Under a roundingPriority the two ways of counting are compared by where
// each rounded, and the significant digits rounded where the rounded result
// says: 0.99999 to three significant digits is 1.00, rounded at the
// hundredths, so morePrecision with three decimals takes the decimals' "1",
// where it had taken the significant digits' "1.0". ECMA-402's
// ToRawPrecision and ICU's number_rounding.cpp; Node's answers
// (ISSUES.md NF-2).
func TestRoundingPriorityCarry(t *testing.T) {
	en, err := intl.ParseLocale("en")
	if err != nil {
		t.Fatal(err)
	}
	d := intl.Digits
	for _, c := range []struct {
		opts   intl.NumberFormatOptions
		v      float64
		want   string
		plural string
	}{
		{intl.NumberFormatOptions{MaximumSignificantDigits: d(3), MinimumSignificantDigits: d(2),
			MaximumFractionDigits: d(3), MinimumFractionDigits: d(0), RoundingPriority: intl.MorePrecision},
			0.99999, "1", "one"},
		{intl.NumberFormatOptions{MaximumSignificantDigits: d(3), MinimumSignificantDigits: d(1),
			MaximumFractionDigits: d(3), MinimumFractionDigits: d(2), RoundingPriority: intl.LessPrecision},
			0.99999, "1", "one"},
		{intl.NumberFormatOptions{Notation: intl.NotationScientific, MaximumSignificantDigits: d(1),
			MinimumSignificantDigits: d(1), MaximumFractionDigits: d(1), MinimumFractionDigits: d(1),
			RoundingPriority: intl.MorePrecision}, 9.99, "1.0E1", "other"},
		{intl.NumberFormatOptions{Notation: intl.NotationCompact, MaximumSignificantDigits: d(1),
			MinimumSignificantDigits: d(1), MaximumFractionDigits: d(1), MinimumFractionDigits: d(1),
			RoundingPriority: intl.MorePrecision}, 999.99, "1.0K", "other"},
		{intl.NumberFormatOptions{MaximumSignificantDigits: d(2), MaximumFractionDigits: d(0),
			RoundingPriority: intl.MorePrecision}, 99.6, "100", "other"},
		{intl.NumberFormatOptions{MaximumSignificantDigits: d(2), MaximumFractionDigits: d(0),
			RoundingPriority: intl.LessPrecision}, 99.6, "100", "other"},
		{intl.NumberFormatOptions{MaximumSignificantDigits: d(2), MaximumFractionDigits: d(1),
			RoundingPriority: intl.MorePrecision}, 0, "0", "other"},
		{intl.NumberFormatOptions{MaximumSignificantDigits: d(1), MaximumFractionDigits: d(2),
			RoundingPriority: intl.LessPrecision}, 0.0099, "0.01", "other"},
	} {
		if got := newNumber(t, en, c.opts).Format(c.v); got != c.want {
			t.Errorf("%+v %v: %q, want %q", c.opts, c.v, got, c.want)
		}
		o := c.opts
		pr, err := intl.NewPluralRules(en, intl.PluralRulesOptions{Notation: o.Notation,
			MinimumFractionDigits: o.MinimumFractionDigits, MaximumFractionDigits: o.MaximumFractionDigits,
			MinimumSignificantDigits: o.MinimumSignificantDigits, MaximumSignificantDigits: o.MaximumSignificantDigits,
			RoundingPriority: o.RoundingPriority})
		if err != nil {
			t.Fatal(err)
		}
		if got := pr.Select(c.v); string(got) != c.plural {
			t.Errorf("%+v %v: plural %q, want %q", c.opts, c.v, got, c.plural)
		}
	}
}
