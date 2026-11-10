//go:build gointl_all

package intl_test

import (
	"testing"
	"time"

	"github.com/liustack/quickjs/internal/gointl"
)

// A locale ICU opens another bundle for is written from that bundle, per
// tree, as Node writes it: "zh-TW" is traditional Chinese, "sr-ME" an alias
// of Serbian in Latin, "uz-AF" of Uzbek in Arabic, and "sr-Cyrl-ME" has
// Cyrillic dates and names but Latin units and currency names, ICU's unit and
// currency trees having no bundle of their own for it. The answers are
// Node's.
func TestICURedirects(t *testing.T) {
	for _, c := range []struct {
		tag                             string
		region, number, unit, cur, date string
	}{
		{"zh-TW", "法國", "1,234.5", "2 小時", "2.00 歐元", "1970年1月1日 星期四"},
		{"zh-HK", "法國", "1,234.5", "2 小時", "2.00 歐元", "1970年1月1日星期四"},
		{"sr-RS", "Француска", "1.234,5", "2 сата", "2,00 евра", "четвртак, 1. јануар 1970."},
		{"pa-PK", "FR", "۱٬۲۳۴٫۵", "۲ h", "۲٫۰۰ يورو", "جمعرات, ۰۱ جنوری ۱۹۷۰"},
		{"uz-AF", "FR", "۱٬۲۳۴٫۵", "۲ h", "۲٫۰۰ EUR", "AP ۱۳۴۸ Dey ۱۱, پنجشنبه"},
		{"sr-ME", "Francuska", "1.234,5", "2 sata", "2,00 evra", "četvrtak, 1. januar 1970."},
		{"sr-Cyrl-ME", "Француска", "1.234,5", "2 sata", "2,00 evra", "четвртак, 1. јануар 1970."},
	} {
		loc, err := intl.ParseLocale(c.tag)
		if err != nil {
			t.Fatal(err)
		}
		n, err := intl.NewDisplayNames(loc, intl.DisplayNamesOptions{Kind: intl.DisplayRegion})
		if err != nil {
			t.Fatal(err)
		}
		region, _ := n.Of("FR")
		nf := newNumber(t, loc, intl.NumberFormatOptions{})
		unit := newNumber(t, loc, intl.NumberFormatOptions{Style: intl.StyleUnit, Unit: "hour", UnitDisplay: intl.UnitLong})
		cur := newNumber(t, loc, intl.NumberFormatOptions{Style: intl.StyleCurrency, Currency: "EUR", CurrencyDisplay: intl.CurrencyName})
		d, err := intl.NewDateTimeFormat(loc, intl.DateTimeFormatOptions{DateStyle: intl.LengthFull, TimeZone: "UTC"})
		if err != nil {
			t.Fatal(err)
		}
		got := [5]string{region, nf.Format(1234.5), unit.Format(2), cur.Format(2), d.Format(time.Unix(0, 0))}
		want := [5]string{c.region, c.number, c.unit, c.cur, c.date}
		if got != want {
			t.Errorf("%s:\n\tgot  %q\n\twant %q", c.tag, got, want)
		}
	}
}

func newNumber(t *testing.T, loc intl.Locale, o intl.NumberFormatOptions) *intl.NumberFormat {
	t.Helper()
	f, err := intl.NewNumberFormat(loc, o)
	if err != nil {
		t.Fatal(err)
	}
	return f
}
