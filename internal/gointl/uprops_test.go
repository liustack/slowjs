//go:build gointl_all

package intl

import (
	"testing"
)

// The Unicode properties formatting reads are Unicode 17's, ICU 78.3's,
// whatever Go built the program. They had been Go's unicode tables, which
// in Go 1.24 are Unicode 15: a Garay digit, which Unicode 16 added, was no
// digit, and en-u-nu-gara lost the space between "CHF" and the number
// (ISSUES.md RU-3). Node's answers.
func TestUnicodePropertiesAreICUs(t *testing.T) {
	en, err := ParseLocale("en-u-nu-gara")
	if err != nil {
		t.Fatal(err)
	}
	for currency, want := range map[string]string{
		"CHF": "CHF \U00010D41\U00010D42.\U00010D45\U00010D40",
		"USD": "$\U00010D41\U00010D42.\U00010D45\U00010D40",
	} {
		f, err := NewNumberFormat(en, NumberFormatOptions{Style: StyleCurrency, Currency: currency})
		if err != nil {
			t.Fatal(err)
		}
		if got := f.Format(12.5); got != want {
			t.Errorf("%s: %+q, want %+q", currency, got, want)
		}
	}

	props, err := loadUnicodeProps(Embedded)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		set  string
		r    rune
		want bool
	}{
		{"gc Nd", 0x10D40, true}, // GARAY DIGIT ZERO, Unicode 16
		{"gc Nd", '7', true},
		{"gc Nd", 'a', false},
		{"gc L", 0x10D50, true}, // GARAY CAPITAL LETTER A, Unicode 16
		{"gc S", 0x20C1, true},  // SAUDI RIYAL SIGN, Unicode 17
		{"gc S", '$', true},
		{"gc Z", 0x3000, true},
		{"gc N", 0x00BD, true},
		{"gc Zs", 0x202F, true},
		{"gc Zs", '\t', false},
		{"Bidi_Control", 0x200E, true},
		{"Bidi_Control", 0x061C, true},
		{"Variation_Selector", 0x180F, true},
		{"Variation_Selector", 0xE01EF, true},
		{"sc Hebrew", 0x05D0, true},
		{"sc Hebrew", 0x05BE, true},
		{"sc Hebrew", 'a', false},
	} {
		if got := props.in(c.set, c.r); got != c.want {
			t.Errorf("%s U+%04X: %v, want %v", c.set, c.r, got, c.want)
		}
	}
	for _, r := range []rune{'\t', ' ', 0x00A0, 0x202F, 0x200F, 0xFE0F} {
		if !props.ignorable(r) {
			t.Errorf("U+%04X is not ignorable", r)
		}
	}

	he, err := ParseLocale("he")
	if err != nil {
		t.Fatal(err)
	}
	lf, err := NewListFormat(he, ListFormatOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		items []string
		want  string
	}{
		{[]string{"a", "א"}, "a וא"},
		{[]string{"א", "a"}, "א ו-a"},
	} {
		if got := lf.Format(c.items); got != c.want {
			t.Errorf("%q: %q, want %q", c.items, got, c.want)
		}
	}
}
