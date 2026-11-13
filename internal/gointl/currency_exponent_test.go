//go:build gointl_all

package intl_test

import (
	"testing"

	intl "github.com/liustack/quickjs/internal/gointl"
)

// Currency spacing goes after an exponent's digits as after an integer's:
// ICU asks only whether the character beside the currency is a digit.
// go-intl had left it out after an exponent: Bengali "১.২৩৪E৩USD" where Node
// writes "১.২৩৪E৩ USD". Node's answers (ISSUES.md NF-9).
func TestCurrencySpacingAfterExponent(t *testing.T) {
	code := func(n intl.Notation, cur string) intl.NumberFormatOptions {
		return intl.NumberFormatOptions{Style: intl.StyleCurrency, Currency: cur, CurrencyDisplay: intl.CurrencyCode,
			Notation: n}
	}
	accounting := code(intl.NotationScientific, "USD")
	accounting.CurrencySign = intl.CurrencySignAccounting
	for _, c := range []struct {
		tag  string
		opts intl.NumberFormatOptions
		v    float64
		want string
	}{
		{"bn", code(intl.NotationScientific, "USD"), 1234, "১.২৩৪E৩ USD"},
		{"ar", accounting, -1234, "(؜1.234E3 USD)"},
		{"de", code(intl.NotationScientific, "EUR"), 0.001234, "1,234E-3 EUR"},
		{"en", code(intl.NotationEngineering, "USD"), 1234, "USD 1.234E3"},
		{"fr", intl.NumberFormatOptions{Style: intl.StyleCurrency, Currency: "EUR", Notation: intl.NotationScientific},
			1234, "1,234E3 €"},
	} {
		loc, err := intl.ParseLocale(c.tag)
		if err != nil {
			t.Fatal(err)
		}
		if got := newNumber(t, loc, c.opts).Format(c.v); got != c.want {
			t.Errorf("%s %v: %+q, want %+q", c.tag, c.v, got, c.want)
		}
	}
	bn, err := intl.ParseLocale("bn")
	if err != nil {
		t.Fatal(err)
	}
	got, err := newNumber(t, bn, code(intl.NotationScientific, "USD")).FormatRange(1234, 5678)
	if err != nil {
		t.Fatal(err)
	}
	if want := "১.২৩৪E৩ – ৫.৬৭৮E৩ USD"; got != want {
		t.Errorf("bn range: %+q, want %+q", got, want)
	}
}
