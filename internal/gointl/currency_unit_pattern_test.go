//go:build gointl_all

package intl_test

import (
	"reflect"
	"testing"

	intl "github.com/liustack/quickjs/internal/gointl"
)

// A currency's name is joined to the amount by the pattern ICU's curr tree
// gives, which Burmese inherits from the root, "{0} {1}". cldr-json resolves
// Burmese's as "{1} {0}", and go-intl had written the name first. Node's
// answers (ISSUES.md NF-12).
func TestCurrencyUnitPatterns(t *testing.T) {
	for _, c := range []struct {
		tag, currency string
		v             float64
		want          string
	}{
		{"my", "GBP", -700.3, "-၇၀၀.၃၀ ဗြိတိသျှ ပေါင်"},
		{"my", "USD", 1, "၁.၀၀ အမေရိကန် ဒေါ်လာ"},
		{"my-u-nu-latn", "EUR", 2.5, "2.50 ယူရို"},
	} {
		loc, err := intl.ParseLocale(c.tag)
		if err != nil {
			t.Fatal(err)
		}
		f := newNumber(t, loc, intl.NumberFormatOptions{Style: intl.StyleCurrency, Currency: c.currency,
			CurrencyDisplay: intl.CurrencyName})
		if got := f.Format(c.v); got != c.want {
			t.Errorf("%s %s %v: %q, want %q", c.tag, c.currency, c.v, got, c.want)
		}
	}
	loc, err := intl.ParseLocale("my")
	if err != nil {
		t.Fatal(err)
	}
	f := newNumber(t, loc, intl.NumberFormatOptions{Style: intl.StyleCurrency, Currency: "GBP",
		CurrencyDisplay: intl.CurrencyName})
	want := []intl.Part{{intl.PartMinusSign, "-"}, {intl.PartInteger, "၇၀၀"}, {intl.PartDecimal, "."},
		{intl.PartFraction, "၃၀"}, {intl.PartLiteral, " "}, {intl.PartCurrency, "ဗြိတိသျှ ပေါင်"}}
	if got := f.FormatToParts(-700.3); !reflect.DeepEqual(got, want) {
		t.Errorf("parts: %q, want %q", got, want)
	}
}
