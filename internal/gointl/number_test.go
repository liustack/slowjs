//go:build gointl_all

package intl_test

import (
	"strings"
	"testing"

	intl "github.com/liustack/quickjs/internal/gointl"
)

func newFormat(t *testing.T, tag string, opts intl.NumberFormatOptions) *intl.NumberFormat {
	t.Helper()
	loc, err := intl.ParseLocale(tag)
	if err != nil {
		t.Fatalf("ParseLocale(%q): %v", tag, err)
	}
	f, err := intl.NewNumberFormat(loc, opts)
	if err != nil {
		t.Fatalf("NewNumberFormat(%q): %v", tag, err)
	}
	return f
}

// The parts are not a convenience: they cannot be recovered from a finished
// string, so producing them is what keeps the formatter building a number out
// of a pattern rather than looking an answer up. Concatenating them has to
// give exactly what Format gives.
func TestFormatToPartsRebuildsTheString(t *testing.T) {
	cases := []struct {
		tag  string
		opts intl.NumberFormatOptions
	}{
		{"en", intl.NumberFormatOptions{}},
		{"de", intl.NumberFormatOptions{}},
		{"en-IN", intl.NumberFormatOptions{}},
		{"ar-EG", intl.NumberFormatOptions{}},
		{"en", intl.NumberFormatOptions{Style: intl.StyleCurrency, Currency: "USD"}},
		{"ja", intl.NumberFormatOptions{Style: intl.StyleCurrency, Currency: "JPY"}},
		{"de", intl.NumberFormatOptions{Style: intl.StylePercent}},
		{"en", intl.NumberFormatOptions{SignDisplay: intl.SignAlways}},
	}
	values := []float64{0, 1, -1, 0.5, 1234.5, -1234567.891, 123456789012}
	for _, c := range cases {
		f := newFormat(t, c.tag, c.opts)
		for _, v := range values {
			var b strings.Builder
			for _, p := range f.FormatToParts(v) {
				if p.Value == "" {
					t.Errorf("%s %v: an empty %s part", c.tag, v, p.Kind)
				}
				b.WriteString(p.Value)
			}
			if got, want := b.String(), f.Format(v); got != want {
				t.Errorf("%s %v: the parts join to %q but Format gives %q",
					c.tag, v, got, want)
			}
		}
	}
}

// The pieces have to be labelled correctly, not merely add up.
func TestFormatToPartsLabels(t *testing.T) {
	f := newFormat(t, "en", intl.NumberFormatOptions{
		Style: intl.StyleCurrency, Currency: "USD",
	})
	got := f.FormatToParts(-1234.5)
	want := []intl.Part{
		{Kind: intl.PartMinusSign, Value: "-"},
		{Kind: intl.PartCurrency, Value: "$"},
		{Kind: intl.PartInteger, Value: "1"},
		{Kind: intl.PartGroup, Value: ","},
		{Kind: intl.PartInteger, Value: "234"},
		{Kind: intl.PartDecimal, Value: "."},
		{Kind: intl.PartFraction, Value: "50"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d parts, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("part %d is %v, want %v", i, got[i], want[i])
		}
	}
}

// Rounding goes away from zero, not to even, which is where Go's own
// formatting would have given a different answer.
func TestRoundingIsHalfAwayFromZero(t *testing.T) {
	f := newFormat(t, "en", intl.NumberFormatOptions{
		MaximumFractionDigits: intl.Digits(0),
	})
	for _, c := range []struct {
		in   float64
		want string
	}{
		{0.5, "1"},        // strconv would give "0"
		{1.5, "2"},        // strconv would give "2"
		{2.5, "3"},        // strconv would give "2"
		{1234.5, "1,235"}, // strconv would give "1,234"
		{-0.5, "-1"},
		{-2.5, "-3"},
	} {
		if got := f.Format(c.in); got != c.want {
			t.Errorf("Format(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

// CLDR puts a space between a currency and a number that would otherwise run
// into it, and only then: a dollar sign is a symbol and needs none.
func TestCurrencySpacing(t *testing.T) {
	code := newFormat(t, "en", intl.NumberFormatOptions{
		Style: intl.StyleCurrency, Currency: "USD",
		CurrencyDisplay: intl.CurrencyCode,
	})
	if got, want := code.Format(1), "USD 1.00"; got != want {
		t.Errorf("a currency code gives %q, want %q", got, want)
	}
	symbol := newFormat(t, "en", intl.NumberFormatOptions{
		Style: intl.StyleCurrency, Currency: "USD",
	})
	if got, want := symbol.Format(1), "$1.00"; got != want {
		t.Errorf("a currency symbol gives %q, want %q", got, want)
	}
}

// India groups by two above the first three, which is a property of the
// locale's pattern rather than of the formatter.
func TestIndianGrouping(t *testing.T) {
	f := newFormat(t, "en-IN", intl.NumberFormatOptions{})
	if got, want := f.Format(123456789012), "1,23,45,67,89,012"; got != want {
		t.Errorf("en-IN gives %q, want %q", got, want)
	}
}

// A locale whose digits are not the ASCII ones writes its own.
func TestOtherNumberingSystem(t *testing.T) {
	f := newFormat(t, "ar-EG", intl.NumberFormatOptions{})
	got := f.Format(1234.5)
	for _, r := range got {
		if r >= '0' && r <= '9' {
			t.Errorf("ar-EG wrote an ASCII digit in %q", got)
			break
		}
	}
	if resolved := f.ResolvedOptions(); resolved.NumberingSystem != "arab" {
		t.Errorf("ar-EG resolved to the %q digits", resolved.NumberingSystem)
	}
}

// A formatter never changes after it is built, so sharing one is safe. This
// is the property that replaces the warm-up entry points and the process-wide
// caches of the package go-intl replaces.
func TestFormatterIsSafeToShare(t *testing.T) {
	f := newFormat(t, "de", intl.NumberFormatOptions{})
	want := f.Format(1234567.891)

	done := make(chan string, 8)
	for i := 0; i < 8; i++ {
		go func() { done <- f.Format(1234567.891) }()
	}
	for i := 0; i < 8; i++ {
		if got := <-done; got != want {
			t.Errorf("a shared formatter gave %q, want %q", got, want)
		}
	}
}

// What is refused says so, rather than formatting something plausible and
// wrong. This test has now caught three options arriving -- compact notation,
// scientific notation and currency names -- which is what it is for.
func TestUnimplementedOptionsAreRefused(t *testing.T) {
	loc, err := intl.ParseLocale("en")
	if err != nil {
		t.Fatal(err)
	}
	for _, opts := range []intl.NumberFormatOptions{
		{Style: intl.StyleCurrency},
		// A rounding increment needs a fixed number of decimals.
		{RoundingIncrement: 5, MaximumFractionDigits: intl.Digits(2)},
	} {
		if _, err := intl.NewNumberFormat(loc, opts); err == nil {
			t.Errorf("%+v was accepted", opts)
		}
	}
}

// A source of the caller's own is enough to format with: the embedded data is
// a default, not a requirement.
func TestNumberFormatFromAnotherSource(t *testing.T) {
	loc, err := intl.ParseLocale("de")
	if err != nil {
		t.Fatal(err)
	}
	f, err := intl.NewNumberFormatFrom(intl.Embedded, loc, intl.NumberFormatOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := f.Format(1234.5), "1.234,5"; got != want {
		t.Errorf("de gives %q, want %q", got, want)
	}
}

// Compact notation picks its wording by the plural category of the divided
// amount, which is why it arrived with the plural rules rather than with the
// rest of NumberFormat.
func TestCompactNotation(t *testing.T) {
	short := newFormat(t, "en", intl.NumberFormatOptions{Notation: intl.NotationCompact})
	long := newFormat(t, "en", intl.NumberFormatOptions{
		Notation: intl.NotationCompact, CompactDisplay: intl.CompactLong,
	})
	for _, c := range []struct {
		in          float64
		short, long string
	}{
		{42, "42", "42"},
		{1234.5, "1.2K", "1.2 thousand"},
		{12345678, "12M", "12 million"},
		// Two significant digits would give 120B; no decimals gives 123B, and
		// ECMA-402 keeps whichever holds more.
		{123456789012, "123B", "123 billion"},
		{0.256, "0.26", "0.26"},
	} {
		if got := short.Format(c.in); got != c.short {
			t.Errorf("compact %v = %q, want %q", c.in, got, c.short)
		}
		if got := long.Format(c.in); got != c.long {
			t.Errorf("compact long %v = %q, want %q", c.in, got, c.long)
		}
	}
}

// A compact number is grouped only when the leading group has two digits of
// its own, which ECMA-402 calls min2.
func TestCompactGroupingNeedsTwoDigits(t *testing.T) {
	f := newFormat(t, "ja", intl.NumberFormatOptions{Notation: intl.NotationCompact})
	if got, want := f.Format(12345678), "1235万"; got != want {
		t.Errorf("ja compact = %q, want %q", got, want)
	}
}

func TestCurrencyDigits(t *testing.T) {
	for code, want := range map[string]int{"USD": 2, "jpy": 0, "KWD": 3, "XXX": 2, "ZZZ": 2} {
		if got, err := intl.CurrencyDigits(intl.Embedded, code); err != nil || got != want {
			t.Errorf("%s: %d, %v; want %d", code, got, err, want)
		}
	}
}
