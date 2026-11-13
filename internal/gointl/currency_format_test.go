//go:build gointl_all

package intl_test

import (
	"testing"

	intl "github.com/liustack/quickjs/internal/gointl"
)

// A currency a locale gives a pattern and separators of its own is written
// with them: Kabuverdianu's escudos with a "$" for the decimal point. go-intl
// had used the locale's for every currency. ICU (CurrencyFormats) also
// writes every other currency in the format of the locale's region's
// currency, and takes the pattern for the name and accounting forms too.
// Node's are its answers (ISSUES.md NF-10).
func TestCurrencyFormats(t *testing.T) {
	type form int
	const (
		symbol form = iota
		accounting
		name
		compact
	)
	for _, c := range []struct {
		tag, currency string
		form          form
		standard      string
		node          string
	}{
		{"kea", "CVE", symbol, "-1 234$50 ​", "-1 234$50 ​"},
		{"kea", "CVE", name, "-1 234$50 Skudu Kabuverdianu", "-1 234$50 ​ Skudu Kabuverdianu"},
		{"kea", "CVE", accounting, "(1 234$50 ​)", "-1 234$50 ​"},
		{"en-150", "EUR", symbol, "-€1,234.50", "-€1,234.50"},
		{"en-150-u-nu-arab", "EUR", symbol, "؜-€١,٢٣٤.٥٠", "؜-€١,٢٣٤.٥٠"},
		{"it", "ITL", symbol, "-ITL 1235", "-ITL 1235"},
		{"tr", "TRY", accounting, "(₺1.234,50)", "-₺1.234,50"},
		// The region's currency's format, for another currency.
		{"en-DE", "USD", symbol, "-1.234,50 US$", "-US$1,234.50"},
		{"en-DE", "USD", accounting, "-1.234,50 US$", "-US$1,234.50"},
		{"en-DE", "USD", name, "-1.234,50 US dollars", "-US$1,234.50 US dollars"},
		{"en-DE", "USD", compact, "-US$1,2M", "-US$1.2M"},
		{"pt-CV", "USD", symbol, "-1234,50 US$", "-1234$50 US$"},
		// Only a region the locale gives: Turkish has none.
		{"tr", "USD", accounting, "($1.234,50)", "($1.234,50)"},
		// ICU then writes "-CVE 1,234.5 Cape Verdean escudos0", putting
		// the name a character early, which is not reproduced.
		{"en-DE", "CVE", name, "-1.234,50 Cape Verdean escudos", "-CVE 1,234.50 Cape Verdean escudos"},
	} {
		loc, err := intl.ParseLocale(c.tag)
		if err != nil {
			t.Fatal(err)
		}
		for _, side := range []struct {
			compat intl.Compat
			want   string
		}{{intl.Standard, c.standard}, {intl.CurrencyFormats, c.node}, {intl.NodeICU, c.node}} {
			opts := intl.NumberFormatOptions{Style: intl.StyleCurrency, Currency: c.currency, Compat: side.compat}
			v := -1234.5
			switch c.form {
			case accounting:
				opts.CurrencySign = intl.CurrencySignAccounting
			case name:
				opts.CurrencyDisplay = intl.CurrencyName
			case compact:
				opts.Notation = intl.NotationCompact
				v = -1234567
			}
			if got := newNumber(t, loc, opts).Format(v); got != side.want {
				t.Errorf("%s %s %d %v: %+q, want %+q", c.tag, c.currency, c.form, side.compat, got, side.want)
			}
		}
	}
}
