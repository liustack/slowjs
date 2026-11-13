//go:build gointl_all

package intl_test

import (
	"testing"

	intl "github.com/liustack/quickjs/internal/gointl"
)

// The expected values here are what node's ICU answers, so that the rounding
// modes are held to the same reference the corpus is.
func TestRoundingModes(t *testing.T) {
	cases := []struct {
		mode intl.RoundingMode
		name string
		want map[float64]string
	}{
		{intl.Ceil, "ceil", map[float64]string{1.4: "2", 1.5: "2", 2.5: "3", -1.5: "-1", -2.5: "-2"}},
		{intl.Floor, "floor", map[float64]string{1.4: "1", 1.5: "1", 2.5: "2", -1.5: "-2", -2.5: "-3"}},
		{intl.Expand, "expand", map[float64]string{1.4: "2", 1.5: "2", 2.5: "3", -1.5: "-2", -2.5: "-3"}},
		{intl.Trunc, "trunc", map[float64]string{1.4: "1", 1.5: "1", 2.5: "2", -1.5: "-1", -2.5: "-2"}},
		{intl.HalfCeil, "halfCeil", map[float64]string{1.4: "1", 1.5: "2", 2.5: "3", -1.5: "-1", -2.5: "-2"}},
		{intl.HalfFloor, "halfFloor", map[float64]string{1.4: "1", 1.5: "1", 2.5: "2", -1.5: "-2", -2.5: "-3"}},
		{intl.HalfExpand, "halfExpand", map[float64]string{1.4: "1", 1.5: "2", 2.5: "3", -1.5: "-2", -2.5: "-3"}},
		{intl.HalfTrunc, "halfTrunc", map[float64]string{1.4: "1", 1.5: "1", 2.5: "2", -1.5: "-1", -2.5: "-2"}},
		{intl.HalfEven, "halfEven", map[float64]string{1.4: "1", 1.5: "2", 2.5: "2", -1.5: "-2", -2.5: "-2"}},
	}
	for _, c := range cases {
		f := newFormat(t, "en", intl.NumberFormatOptions{
			MaximumFractionDigits: intl.Digits(0),
			RoundingMode:          c.mode,
		})
		for in, want := range c.want {
			if got := f.Format(in); got != want {
				t.Errorf("%s: Format(%v) = %q, want %q", c.name, in, got, want)
			}
		}
	}
}

func TestSignificantDigits(t *testing.T) {
	cases := []struct {
		minSig, maxSig int
		in             float64
		want           string
	}{
		{1, 2, 1234.5, "1,200"},
		{1, 3, 1234.5, "1,230"},
		{1, 2, 0.256, "0.26"},
		{1, 2, 0, "0"},
		{3, 3, 0, "0.00"},
		{3, 5, 1.5, "1.50"},
		{1, 21, 1.5, "1.5"},
		{2, 2, 0.05, "0.050"},
	}
	for _, c := range cases {
		f := newFormat(t, "en", intl.NumberFormatOptions{
			MinimumSignificantDigits: intl.Digits(c.minSig),
			MaximumSignificantDigits: intl.Digits(c.maxSig),
		})
		if got := f.Format(c.in); got != c.want {
			t.Errorf("sig %d..%d of %v = %q, want %q",
				c.minSig, c.maxSig, c.in, got, c.want)
		}
	}
}

// When both ways of counting are given, the priority says which wins.
func TestRoundingPriority(t *testing.T) {
	more := newFormat(t, "en", intl.NumberFormatOptions{
		MaximumFractionDigits:    intl.Digits(2),
		MaximumSignificantDigits: intl.Digits(2),
		RoundingPriority:         intl.MorePrecision,
	})
	less := newFormat(t, "en", intl.NumberFormatOptions{
		MaximumFractionDigits:    intl.Digits(2),
		MaximumSignificantDigits: intl.Digits(2),
		RoundingPriority:         intl.LessPrecision,
	})
	// Two decimals round at 0.01; two significant digits of 4.321 round at
	// 0.1. So more precision keeps the decimals and less keeps the digits.
	if got, want := more.Format(4.321), "4.32"; got != want {
		t.Errorf("morePrecision = %q, want %q", got, want)
	}
	if got, want := less.Format(4.321), "4.3"; got != want {
		t.Errorf("lessPrecision = %q, want %q", got, want)
	}
}

func TestTrailingZeroDisplay(t *testing.T) {
	f := newFormat(t, "en", intl.NumberFormatOptions{
		MinimumFractionDigits: intl.Digits(2),
		MaximumFractionDigits: intl.Digits(2),
		TrailingZeroDisplay:   intl.TrailingZeroStripIfInteger,
	})
	for _, c := range []struct {
		in   float64
		want string
	}{
		{1, "1"},
		{1.5, "1.50"},
		{1.25, "1.25"},
	} {
		if got := f.Format(c.in); got != c.want {
			t.Errorf("Format(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestRoundingIncrement(t *testing.T) {
	f := newFormat(t, "en", intl.NumberFormatOptions{
		MinimumFractionDigits: intl.Digits(2),
		MaximumFractionDigits: intl.Digits(2),
		RoundingIncrement:     5,
	})
	for _, c := range []struct {
		in   float64
		want string
	}{
		{1.01, "1.00"},
		{1.03, "1.05"},
		{1.06, "1.05"},
		{1.08, "1.10"},
	} {
		if got := f.Format(c.in); got != c.want {
			t.Errorf("Format(%v) = %q, want %q", c.in, got, c.want)
		}
	}

	// An increment needs a fixed number of decimals and no significant digits.
	loc, err := intl.ParseLocale("en")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := intl.NewNumberFormat(loc, intl.NumberFormatOptions{
		MinimumFractionDigits: intl.Digits(0),
		MaximumFractionDigits: intl.Digits(2),
		RoundingIncrement:     5,
	}); err == nil {
		t.Error("an increment with a range of decimals was accepted")
	}
}

func TestAccountingCurrencySign(t *testing.T) {
	f := newFormat(t, "en", intl.NumberFormatOptions{
		Style: intl.StyleCurrency, Currency: "USD",
		CurrencySign: intl.CurrencySignAccounting,
	})
	if got, want := f.Format(-1234.5), "($1,234.50)"; got != want {
		t.Errorf("accounting = %q, want %q", got, want)
	}
	if got, want := f.Format(1234.5), "$1,234.50"; got != want {
		t.Errorf("accounting positive = %q, want %q", got, want)
	}
}

// Scientific puts one digit before the point; engineering restricts the
// exponent to multiples of three, so the mantissa takes one, two or three.
// Both sets of expectations are node's.
func TestScientificAndEngineering(t *testing.T) {
	vals := []float64{0, 1, -1, 0.5, 42, 1234.5, 1234567.891, 0.000256, 123456789012}
	cases := []struct {
		notation intl.Notation
		name     string
		want     []string
	}{
		{intl.NotationScientific, "scientific", []string{
			"0E0", "1E0", "-1E0", "5E-1", "4.2E1", "1.235E3", "1.235E6",
			"2.56E-4", "1.235E11"}},
		{intl.NotationEngineering, "engineering", []string{
			"0E0", "1E0", "-1E0", "500E-3", "42E0", "1.235E3", "1.235E6",
			"256E-6", "123.457E9"}},
	}
	for _, c := range cases {
		f := newFormat(t, "en", intl.NumberFormatOptions{Notation: c.notation})
		for i, v := range vals {
			if got := f.Format(v); got != c.want[i] {
				t.Errorf("%s of %v = %q, want %q", c.name, v, got, c.want[i])
			}
		}
	}
}

// The exponent is its own set of pieces, so a caller can style it apart from
// the number.
func TestScientificParts(t *testing.T) {
	f := newFormat(t, "en", intl.NumberFormatOptions{Notation: intl.NotationScientific})
	want := []intl.Part{
		{Kind: intl.PartInteger, Value: "1"},
		{Kind: intl.PartDecimal, Value: "."},
		{Kind: intl.PartFraction, Value: "235"},
		{Kind: intl.PartExponentSeparator, Value: "E"},
		{Kind: intl.PartExponentInteger, Value: "3"},
	}
	got := f.FormatToParts(1234.5)
	if len(got) != len(want) {
		t.Fatalf("got %d parts, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("part %d is %v, want %v", i, got[i], want[i])
		}
	}
}

// ECMA-402 gives PluralRules the same digit options, because they decide how
// the number would be written and that is what decides the form.
func TestPluralRulesDigitOptions(t *testing.T) {
	loc, err := intl.ParseLocale("en")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		opts intl.PluralRulesOptions
		in   float64
		want intl.PluralCategory
	}{
		{intl.PluralRulesOptions{}, 1, intl.PluralOne},
		// Two significant digits write 1 as "1.0", which is not "one".
		{intl.PluralRulesOptions{
			MinimumSignificantDigits: intl.Digits(2),
			MaximumSignificantDigits: intl.Digits(2),
		}, 1, intl.PluralOther},
		{intl.PluralRulesOptions{MinimumFractionDigits: intl.Digits(1)}, 1, intl.PluralOther},
	} {
		p, err := intl.NewPluralRules(loc, c.opts)
		if err != nil {
			t.Fatal(err)
		}
		if got := p.Select(c.in); got != c.want {
			t.Errorf("%+v Select(%v) = %q, want %q", c.opts, c.in, got, c.want)
		}
	}
}

// A spelled-out currency name is chosen by the plural category of the amount
// *as written*, not of its value. Money is written with two decimals, so one
// dollar is "1.00" and English calls that "dollars". The expectations are
// node's.
func TestCurrencyDisplayName(t *testing.T) {
	for _, c := range []struct {
		loc, cur string
		v        float64
		want     string
	}{
		{"en", "USD", 1, "1.00 US dollars"},
		{"en", "USD", 2.5, "2.50 US dollars"},
		{"de", "EUR", 1, "1,00 Euro"},
		// The yen takes no decimals, so five of them really is "5".
		{"en", "JPY", 5, "5 Japanese yen"},
		{"pl", "USD", 5, "5,00 dolara amerykańskiego"},
	} {
		f := newFormat(t, c.loc, intl.NumberFormatOptions{
			Style: intl.StyleCurrency, Currency: c.cur,
			CurrencyDisplay: intl.CurrencyName,
		})
		if got := f.Format(c.v); got != c.want {
			t.Errorf("%s %s %v = %q, want %q", c.loc, c.cur, c.v, got, c.want)
		}
	}
}
