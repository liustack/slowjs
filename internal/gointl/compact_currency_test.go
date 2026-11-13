//go:build gointl_all

package intl_test

import (
	"reflect"
	"testing"

	intl "github.com/liustack/quickjs/internal/gointl"
)

// A compact currency is written with CLDR's currency compact patterns, each
// the whole pattern with the currency sign in it, in place of the currency
// or accounting pattern (ICU's TYPE_CURRENCY); below a thousand, or where a
// magnitude's pattern is "0", the currency pattern writes it in full; a
// currency written by its name keeps the decimal patterns. go-intl had put
// the decimal pattern inside the currency pattern: "$1.2 thousand" where
// Node writes "$1.2K". Node's answers, parts and all (ISSUES.md NF-4).
func TestCompactCurrency(t *testing.T) {
	for _, c := range []struct {
		tag  string
		opts intl.NumberFormatOptions
		v    float64
		want []intl.Part
	}{
		{"en", intl.NumberFormatOptions{Style: intl.StyleCurrency, Currency: "USD", Notation: intl.NotationCompact, CompactDisplay: intl.CompactLong}, 1234, []intl.Part{{intl.PartCurrency, "$"}, {intl.PartInteger, "1"}, {intl.PartDecimal, "."}, {intl.PartFraction, "2"}, {intl.PartCompact, "K"}}},
		{"en", intl.NumberFormatOptions{Style: intl.StyleCurrency, Currency: "USD", Notation: intl.NotationCompact, CurrencySign: intl.CurrencySignAccounting}, -1234, []intl.Part{{intl.PartMinusSign, "-"}, {intl.PartCurrency, "$"}, {intl.PartInteger, "1"}, {intl.PartDecimal, "."}, {intl.PartFraction, "2"}, {intl.PartCompact, "K"}}},
		{"en", intl.NumberFormatOptions{Style: intl.StyleCurrency, Currency: "USD", Notation: intl.NotationCompact, CurrencySign: intl.CurrencySignAccounting}, -12, []intl.Part{{intl.PartLiteral, "("}, {intl.PartCurrency, "$"}, {intl.PartInteger, "12"}, {intl.PartLiteral, ")"}}},
		{"en", intl.NumberFormatOptions{Style: intl.StyleCurrency, Currency: "USD", Notation: intl.NotationCompact}, 12.345, []intl.Part{{intl.PartCurrency, "$"}, {intl.PartInteger, "12"}}},
		{"nl", intl.NumberFormatOptions{Style: intl.StyleCurrency, Currency: "USD", Notation: intl.NotationCompact}, -1234567, []intl.Part{{intl.PartMinusSign, "-"}, {intl.PartCurrency, "US$"}, {intl.PartLiteral, " "}, {intl.PartInteger, "1"}, {intl.PartDecimal, ","}, {intl.PartFraction, "2"}, {intl.PartLiteral, " "}, {intl.PartCompact, "mln."}}},
		{"fa", intl.NumberFormatOptions{Style: intl.StyleCurrency, Currency: "USD", Notation: intl.NotationCompact}, 1234, []intl.Part{{intl.PartLiteral, "‎"}, {intl.PartCurrency, "$"}, {intl.PartLiteral, " "}, {intl.PartInteger, "۱"}, {intl.PartDecimal, "٫"}, {intl.PartFraction, "۲"}, {intl.PartLiteral, " "}, {intl.PartCompact, "هزار"}}},
		{"en", intl.NumberFormatOptions{Style: intl.StyleCurrency, Currency: "EUR", Notation: intl.NotationCompact, CurrencyDisplay: intl.CurrencyName}, 1234, []intl.Part{{intl.PartInteger, "1"}, {intl.PartDecimal, "."}, {intl.PartFraction, "2"}, {intl.PartCompact, "K"}, {intl.PartLiteral, " "}, {intl.PartCurrency, "euros"}}},
		{"en", intl.NumberFormatOptions{Style: intl.StyleCurrency, Currency: "EUR", Notation: intl.NotationCompact, CurrencyDisplay: intl.CurrencyCode}, 1234, []intl.Part{{intl.PartCurrency, "EUR"}, {intl.PartLiteral, " "}, {intl.PartInteger, "1"}, {intl.PartDecimal, "."}, {intl.PartFraction, "2"}, {intl.PartCompact, "K"}}},
		{"de", intl.NumberFormatOptions{Style: intl.StyleCurrency, Currency: "EUR", Notation: intl.NotationCompact}, 1234, []intl.Part{{intl.PartInteger, "1234"}, {intl.PartLiteral, " "}, {intl.PartCurrency, "€"}}},
		{"de", intl.NumberFormatOptions{Style: intl.StyleCurrency, Currency: "EUR", Notation: intl.NotationCompact}, 1234567, []intl.Part{{intl.PartInteger, "1"}, {intl.PartDecimal, ","}, {intl.PartFraction, "2"}, {intl.PartLiteral, " "}, {intl.PartCompact, "Mio."}, {intl.PartLiteral, " "}, {intl.PartCurrency, "€"}}},
		{"ja", intl.NumberFormatOptions{Style: intl.StyleCurrency, Currency: "JPY", Notation: intl.NotationCompact}, 123456789, []intl.Part{{intl.PartCurrency, "￥"}, {intl.PartInteger, "1"}, {intl.PartDecimal, "."}, {intl.PartFraction, "2"}, {intl.PartCompact, "億"}}},
		{"sw", intl.NumberFormatOptions{Style: intl.StyleCurrency, Currency: "TZS", Notation: intl.NotationCompact}, -1234, []intl.Part{{intl.PartCurrency, "TSh"}, {intl.PartLiteral, " "}, {intl.PartCompact, "elfu"}, {intl.PartLiteral, " "}, {intl.PartMinusSign, "-"}, {intl.PartInteger, "1"}, {intl.PartDecimal, "."}, {intl.PartFraction, "2"}}},
		{"en", intl.NumberFormatOptions{Style: intl.StyleCurrency, Currency: "USD", Notation: intl.NotationCompact, SignDisplay: intl.SignAlways}, 1234, []intl.Part{{intl.PartPlusSign, "+"}, {intl.PartCurrency, "$"}, {intl.PartInteger, "1"}, {intl.PartDecimal, "."}, {intl.PartFraction, "2"}, {intl.PartCompact, "K"}}},
		{"ar", intl.NumberFormatOptions{Style: intl.StyleCurrency, Currency: "USD", Notation: intl.NotationCompact}, -1234, []intl.Part{{intl.PartLiteral, "‎"}, {intl.PartMinusSign, "-"}, {intl.PartLiteral, "‏"}, {intl.PartInteger, "1"}, {intl.PartDecimal, "."}, {intl.PartFraction, "2"}, {intl.PartLiteral, " "}, {intl.PartCompact, "ألف"}, {intl.PartLiteral, " "}, {intl.PartCurrency, "US$"}}},
		{"he", intl.NumberFormatOptions{Style: intl.StyleCurrency, Currency: "ILS", Notation: intl.NotationCompact}, 1234, []intl.Part{{intl.PartLiteral, "‏"}, {intl.PartInteger, "1"}, {intl.PartDecimal, "."}, {intl.PartFraction, "2"}, {intl.PartCompact, "K"}, {intl.PartLiteral, "‏ ‏"}, {intl.PartCurrency, "₪"}}},
		{"lo", intl.NumberFormatOptions{Style: intl.StyleCurrency, Currency: "LAK", Notation: intl.NotationCompact}, 1234567, []intl.Part{{intl.PartCurrency, "₭"}, {intl.PartInteger, "1"}, {intl.PartDecimal, ","}, {intl.PartFraction, "2"}, {intl.PartLiteral, " "}, {intl.PartCompact, "ລ້ານ"}}},
		{"de-CH", intl.NumberFormatOptions{Style: intl.StyleCurrency, Currency: "CHF", Notation: intl.NotationCompact}, 1234567, []intl.Part{{intl.PartCurrency, "CHF"}, {intl.PartLiteral, " "}, {intl.PartInteger, "1"}, {intl.PartDecimal, "."}, {intl.PartFraction, "2"}, {intl.PartLiteral, " "}, {intl.PartCompact, "Mio."}}},
		{"fr", intl.NumberFormatOptions{Style: intl.StyleCurrency, Currency: "EUR", Notation: intl.NotationCompact, CompactDisplay: intl.CompactLong}, 1000, []intl.Part{{intl.PartInteger, "1"}, {intl.PartLiteral, " "}, {intl.PartCompact, "k"}, {intl.PartLiteral, " "}, {intl.PartCurrency, "€"}}},
	} {
		loc, err := intl.ParseLocale(c.tag)
		if err != nil {
			t.Fatal(err)
		}
		if got := newNumber(t, loc, c.opts).FormatToParts(c.v); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s %+v %v:\n got  %q\n want %q", c.tag, c.opts, c.v, got, c.want)
		}
	}
}
