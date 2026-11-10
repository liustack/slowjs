//go:build gointl_all

package intl_test

import (
	"testing"

	intl "github.com/liustack/quickjs/internal/gointl"
)

// An accounting amount with signDisplay "never": ECMA-402 takes the
// accounting pattern's positive form, and V8 asks ICU for UNUM_SIGN_NEVER,
// which writes the plain currency pattern. Node's are its answers
// (ISSUES.md NF-17).
func TestAccountingNever(t *testing.T) {
	for _, c := range []struct {
		tag, currency string
		sign          intl.SignDisplay
		v             float64
		standard      string
		node          string
	}{
		{"nb", "EUR", intl.SignNever, -1, "€\u00a01,00", "1,00\u00a0€"},
		{"nb", "EUR", intl.SignNever, 1, "€\u00a01,00", "1,00\u00a0€"},
		{"nb", "EUR", intl.SignNever, 0, "€\u00a00,00", "0,00\u00a0€"},
		{"en", "USD", intl.SignNever, -1, "$1.00", "$1.00"},
		{"nb", "EUR", intl.SignAuto, -1, "(€\u00a01,00)", "(€\u00a01,00)"},
		{"nb", "EUR", intl.SignAlways, 1, "+€\u00a01,00", "+€\u00a01,00"},
	} {
		for _, side := range []struct {
			compat intl.Compat
			want   string
		}{{intl.Standard, c.standard}, {intl.AccountingNever, c.node}, {intl.NodeICU, c.node}} {
			loc, err := intl.ParseLocale(c.tag)
			if err != nil {
				t.Fatal(err)
			}
			f := newNumber(t, loc, intl.NumberFormatOptions{Style: intl.StyleCurrency, Currency: c.currency,
				CurrencySign: intl.CurrencySignAccounting, SignDisplay: c.sign, Compat: side.compat})
			if got := f.Format(c.v); got != side.want {
				t.Errorf("%s %v %v %v: %q, want %q", c.tag, c.sign, c.v, side.compat, got, side.want)
			}
		}
	}
}

// numeric "auto": ECMA-402 writes a word only for an offset that is a whole
// number, and ICU's formatRelativeImpl for any within 0.005 of one from -2
// to 2, by a hundred times the offset rounded half away from zero. Node's
// are its answers (ISSUES.md NF-17).
func TestRelativeEpsilon(t *testing.T) {
	for _, c := range []struct {
		tag      string
		v        float64
		unit     intl.RelativeTimeUnit
		standard string
		node     string
	}{
		{"en", 0.0001, intl.RelativeDay, "in 0 days", "today"},
		{"en", 0.004, intl.RelativeDay, "in 0.004 days", "today"},
		{"en", -0.004, intl.RelativeDay, "0.004 days ago", "today"},
		{"en", 0.005, intl.RelativeDay, "in 0.005 days", "in 0.005 days"},
		{"en", 0.995, intl.RelativeDay, "in 0.995 days", "tomorrow"},
		{"en", 1.004, intl.RelativeDay, "in 1.004 days", "tomorrow"},
		// A hundred times 1.005 is just short of 100.5 in a double.
		{"en", 1.005, intl.RelativeDay, "in 1.005 days", "tomorrow"},
		{"en", -1.004, intl.RelativeDay, "1.004 days ago", "yesterday"},
		{"en", 2.004, intl.RelativeDay, "in 2.004 days", "in 2.004 days"},
		{"en", 0.004, intl.RelativeSecond, "in 0.004 seconds", "now"},
		{"en", 0.004, intl.RelativeYear, "in 0.004 years", "this year"},
		{"fr", -1.999, intl.RelativeDay, "il y a 1,999 jour", "avant-hier"},
		{"en", 1, intl.RelativeDay, "tomorrow", "tomorrow"},
	} {
		for _, side := range []struct {
			compat intl.Compat
			want   string
		}{{intl.Standard, c.standard}, {intl.RelativeEpsilon, c.node}, {intl.NodeICU, c.node}} {
			loc, err := intl.ParseLocale(c.tag)
			if err != nil {
				t.Fatal(err)
			}
			f, err := intl.NewRelativeTimeFormat(loc, intl.RelativeTimeFormatOptions{Numeric: intl.RelativeAuto,
				Compat: side.compat})
			if err != nil {
				t.Fatal(err)
			}
			if got := f.Format(c.v, c.unit); got != side.want {
				t.Errorf("%s %v %v %v: %q, want %q", c.tag, c.v, c.unit, side.compat, got, side.want)
			}
		}
	}
}

// A double rounded to an increment other than 1 or 5: ECMA-402 rounds the
// double's decimal, and ICU the digits its fast reading of the double gives,
// past about sixteen not the double's. A decimal that was never a double is
// the same on both sides. Node's are its answers (ISSUES.md NF-17).
func TestApproximateIncrement(t *testing.T) {
	en, err := intl.ParseLocale("en")
	if err != nil {
		t.Fatal(err)
	}
	opts := func(frac, inc int) intl.NumberFormatOptions {
		return intl.NumberFormatOptions{MinimumFractionDigits: intl.Digits(frac),
			MaximumFractionDigits: intl.Digits(frac), RoundingIncrement: inc}
	}
	compact := opts(2, 2)
	compact.Notation = intl.NotationCompact
	for _, c := range []struct {
		opts     intl.NumberFormatOptions
		v        float64
		standard string
		node     string
	}{
		{opts(2, 2), 3.9967620239602476e27, "3,996,762,023,960,247,600,000,000,000.00",
			"3,996,762,023,960,248,000,000,000,000.00"},
		{opts(2, 10), 3.9967620239602476e27, "3,996,762,023,960,247,600,000,000,000.00",
			"3,996,762,023,960,248,000,000,000,000.00"},
		{opts(2, 25), 3.9967620239602476e27, "3,996,762,023,960,247,600,000,000,000.00",
			"3,996,762,023,960,248,000,000,000,000.00"},
		{opts(2, 5), 3.9967620239602476e27, "3,996,762,023,960,247,600,000,000,000.00",
			"3,996,762,023,960,247,600,000,000,000.00"},
		{opts(0, 2), 1.2345678901234567e20, "123,456,789,012,345,670,000", "123,456,789,012,345,660,000"},
		{opts(20, 2), 0.30000000000000004, "0.30000000000000004000", "0.30000000000000010000"},
		{opts(17, 2), 1.0 / 3, "0.33333333333333330", "0.33333333333333330"},
		{compact, 3.9967620239602476e27, "3,996,762,023,960,247.60T", "3,996,762,023,960,248.00T"},
	} {
		for _, side := range []struct {
			compat intl.Compat
			want   string
		}{{intl.Standard, c.standard}, {intl.ApproximateIncrement, c.node}, {intl.NodeICU, c.node}} {
			o := c.opts
			o.Compat = side.compat
			f := newNumber(t, en, o)
			if got := f.Format(c.v); got != side.want {
				t.Errorf("%+v %v %v: %q, want %q", c.opts, c.v, side.compat, got, side.want)
			}
			// The same digits, never a double, are ECMA-402's on both sides.
			if got := f.FormatDecimal(intl.ParseDecimal(intl.DecimalFromFloat(c.v).String())); got != c.standard {
				t.Errorf("%+v %v %v as a decimal: %q, want %q", c.opts, c.v, side.compat, got, c.standard)
			}
			// So are a range's ends, which ICU reads through a Formattable
			// that corrects the fast reading: Node writes formatRange(x, x)
			// as "~" and the standard's digits.
			if got, err := f.FormatRange(c.v, c.v); err != nil || got != "~"+c.standard {
				t.Errorf("%+v %v %v as a range: %q, %v, want %q", c.opts, c.v, side.compat, got, err, "~"+c.standard)
			}
		}
	}
}
