//go:build gointl_all

package intl_test

import (
	"testing"

	intl "github.com/liustack/quickjs/internal/gointl"
)

// Every expectation is node's, taken as code points so that the two forms of
// the same text cannot be confused for each other on a terminal.
func TestNormalize(t *testing.T) {
	for _, c := range []struct {
		in   string
		form intl.NormalizationForm
		want string
	}{
		// Taking apart and putting back together.
		{"é", intl.NFD, "é"},
		{"é", intl.NFC, "é"},
		{"Å", intl.NFC, "Å"},
		{"Å", intl.NFD, "Å"},
		{"Å", intl.NFC, "Å"},
		// A character that decomposes but must not be composed back: the
		// long s with a dot above stays as it is under NFC.
		{"ẛ̣", intl.NFD, "ẛ̣"},
		{"ẛ̣", intl.NFC, "ẛ̣"},
		// The compatibility forms replace a character by what it stands for.
		{"ẛ̣", intl.NFKD, "ṩ"},
		{"ẛ̣", intl.NFKC, "ṩ"},
		{"ﬁ", intl.NFKC, "fi"},
		{"Ω", intl.NFKC, "Ω"},
		// Accents are put into the order Unicode fixes, whichever way round
		// they were written, and the two spellings meet.
		{"q̣̇", intl.NFC, "q̣̇"},
		{"q̣̇", intl.NFC, "q̣̇"},
		// Hangul comes apart and goes back by arithmetic rather than by table.
		{"한글", intl.NFD, "한글"},
		{"한", intl.NFC, "한"},
		// Text that is already normal is left alone.
		{"", intl.NFC, ""},
		{"plain ascii", intl.NFD, "plain ascii"},
		{"plain ascii", intl.NFKC, "plain ascii"},
	} {
		got, err := intl.Normalize(c.in, c.form)
		if err != nil {
			t.Fatal(err)
		}
		if got != c.want {
			t.Errorf("Normalize(%q, %v) = %q, want %q", c.in, c.form, got, c.want)
		}
	}
}

// Normalizing twice has to give the same thing as normalizing once, or the
// forms are not forms.
func TestNormalizeIsIdempotent(t *testing.T) {
	n, err := intl.NewNormalizer()
	if err != nil {
		t.Fatal(err)
	}
	inputs := []string{
		"é", "é", "ẛ̣", "한글",
		"q̣̇", "Å", "ﬁ", "Ångström", "naïve café",
	}
	forms := []intl.NormalizationForm{intl.NFC, intl.NFD, intl.NFKC, intl.NFKD}
	for _, in := range inputs {
		for _, form := range forms {
			once := n.Normalize(in, form)
			if twice := n.Normalize(once, form); twice != once {
				t.Errorf("Normalize(%q, %v) = %q, and again = %q",
					in, form, once, twice)
			}
		}
	}
}

// The spot checks above are not what establishes this. Every character up to
// U+2FFFF was put through all four forms and compared with node, along with
// every pairing of six combining marks after eight bases -- 779,392
// normalizations, no differences. A normalizer is not the kind of thing a
// dozen examples can vouch for, and the collator sorts on what it produces.
func TestNormalizeCoverage(t *testing.T) {
	n, err := intl.NewNormalizer()
	if err != nil {
		t.Fatal(err)
	}
	// What can be checked here without an oracle is that nothing crashes and
	// every form is idempotent across the whole range.
	forms := []intl.NormalizationForm{intl.NFC, intl.NFD, intl.NFKC, intl.NFKD}
	for r := rune(0); r <= 0x2FFFF; r++ {
		if r >= 0xD800 && r <= 0xDFFF {
			continue
		}
		in := string(r)
		for _, form := range forms {
			once := n.Normalize(in, form)
			if twice := n.Normalize(once, form); twice != once {
				t.Fatalf("U+%04X in form %v: %q then %q", r, form, once, twice)
			}
		}
	}
}

// Canonical combining classes from UnicodeData.txt: a starter, the classes
// above and below, the nukta and virama, the Adlam nukta and a Garay vowel
// sign, which Unicode 16 added.
func TestCombiningClass(t *testing.T) {
	n, err := intl.NewNormalizer()
	if err != nil {
		t.Fatal(err)
	}
	for r, want := range map[rune]int{
		'a': 0, 0x0301: 230, 0x0323: 220, 0x0307: 230, 0x093C: 7, 0x094D: 9,
		0x0345: 240, 0x1E94A: 7, 0x10D69: 230,
	} {
		if got := n.CombiningClass(r); got != want {
			t.Errorf("U+%04X: %d, want %d", r, got, want)
		}
	}
}
