//go:build gointl_all

package intl_test

import (
	"reflect"
	"testing"

	intl "github.com/liustack/quickjs/internal/gointl"
)

// A currency written by its name is the pattern's text around the number,
// the name put into it, all of it the currency but for the spaces at its
// ends, as ICU's LongNameHandler builds it: Romanian's "de" is part of the
// currency, where go-intl had made it a literal. Node's answers
// (ISSUES.md NF-14).
func TestCurrencyNameParts(t *testing.T) {
	for _, c := range []struct {
		tag, currency string
		want          []intl.Part
	}{
		{"ro", "CHF", []intl.Part{{intl.PartInteger, "100"}, {intl.PartLiteral, " "},
			{intl.PartCurrency, "de franci elvețieni"}}},
		{"en", "USD", []intl.Part{{intl.PartInteger, "100"}, {intl.PartLiteral, " "},
			{intl.PartCurrency, "US dollars"}}},
		{"fr", "EUR", []intl.Part{{intl.PartInteger, "100"}, {intl.PartLiteral, " "},
			{intl.PartCurrency, "euros"}}},
		{"ar", "EGP", []intl.Part{{intl.PartInteger, "100"}, {intl.PartLiteral, " "},
			{intl.PartCurrency, "جنيه مصري"}}},
	} {
		loc, err := intl.ParseLocale(c.tag)
		if err != nil {
			t.Fatal(err)
		}
		f := newNumber(t, loc, intl.NumberFormatOptions{Style: intl.StyleCurrency, Currency: c.currency,
			CurrencyDisplay: intl.CurrencyName, MaximumFractionDigits: intl.Digits(0)})
		if got := f.FormatToParts(100); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s %s: %q, want %q", c.tag, c.currency, got, c.want)
		}
	}
}
