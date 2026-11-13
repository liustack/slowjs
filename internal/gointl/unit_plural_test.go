//go:build gointl_all

package intl_test

import (
	"testing"

	intl "github.com/liustack/quickjs/internal/gointl"
)

// A unit's or currency's name takes the plural of the number as written:
// rounded with its sign, which a directed rounding mode needs, and in
// scientific or compact notation the mantissa's with the exponent beside
// it, as ICU's LongNameHandler sees them. go-intl had rounded as if the
// number were positive, "-2 day", and ignored the exponent, "1E-3 metro".
// Node's answers (ISSUES.md NF-6, NF-7).
func TestUnitPluralSignAndNotation(t *testing.T) {
	d := intl.Digits
	day := func(mode intl.RoundingMode) intl.NumberFormatOptions {
		return intl.NumberFormatOptions{Style: intl.StyleUnit, Unit: "day", UnitDisplay: intl.UnitLong,
			MaximumFractionDigits: d(0), RoundingMode: mode}
	}
	meter := func(n intl.Notation) intl.NumberFormatOptions {
		return intl.NumberFormatOptions{Style: intl.StyleUnit, Unit: "meter", UnitDisplay: intl.UnitLong, Notation: n}
	}
	for _, c := range []struct {
		tag  string
		opts intl.NumberFormatOptions
		v    float64
		want string
	}{
		{"en", day(intl.Floor), -1.5, "-2 days"},
		{"en", day(intl.Ceil), -1.5, "-1 day"},
		{"en", day(intl.Floor), 1.5, "1 day"},
		{"pt", meter(intl.NotationScientific), 0.001, "1E-3 metros"},
		{"lv", meter(intl.NotationScientific), 0.001, "1E-3 metrs"},
		{"pl", meter(intl.NotationScientific), 2000, "2E3 metrów"},
		{"zu", meter(intl.NotationEngineering), 0.001, "1E-3 m"},
		{"fr", meter(intl.NotationCompact), 1500000, "1,5 M mètres"},
		{"en", intl.NumberFormatOptions{Style: intl.StyleCurrency, Currency: "USD", CurrencyDisplay: intl.CurrencyName,
			MaximumFractionDigits: d(0), RoundingMode: intl.Floor}, -0.5, "-1 US dollar"},
		{"en", intl.NumberFormatOptions{Style: intl.StyleCurrency, Currency: "USD", CurrencyDisplay: intl.CurrencyName,
			Notation: intl.NotationScientific}, 1, "1E0 US dollar"},
	} {
		loc, err := intl.ParseLocale(c.tag)
		if err != nil {
			t.Fatal(err)
		}
		if got := newNumber(t, loc, c.opts).Format(c.v); got != c.want {
			t.Errorf("%s %+v %v: %q, want %q", c.tag, c.opts, c.v, got, c.want)
		}
	}
}
