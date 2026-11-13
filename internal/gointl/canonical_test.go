//go:build gointl_all

package intl_test

import (
	"testing"

	"github.com/liustack/quickjs/internal/gointl"
)

// V8 answers a lone two-letter language without consulting ICU's aliases;
// ECMA-402, and the Standard profile, replace them. Every other tag is
// canonicalized alike by both.
func TestCanonicalizeTwoLetterFastPath(t *testing.T) {
	for _, c := range []struct {
		tag, standard, node string
	}{
		{"bh", "bho", "bh"},
		{"tw", "ak", "tw"},
		{"iw", "he", "he"},
		{"sh", "sr-Latn", "sr-Latn"},
		{"en", "en", "en"},
		{"bh-IN", "bho-IN", "bho-IN"},
	} {
		for _, p := range []struct {
			compat intl.Compat
			want   string
		}{{intl.Standard, c.standard}, {intl.NodeICU, c.node}} {
			canon, err := intl.NewCanonicalizer(intl.Embedded, intl.CanonicalizeOptions{Compat: p.compat})
			if err != nil {
				t.Fatal(err)
			}
			got, err := canon.Canonicalize(c.tag)
			if err != nil || got.String() != p.want {
				t.Errorf("%v: %s = %q, %v; want %q", p.compat, c.tag, got.String(), err, p.want)
			}
		}
	}
}

// A few shapes, with Node's answers: legacy tags rewritten first, the
// extensions in singleton order, a keyword given twice meaning what it said
// first, private-use variants.
func TestCanonicalizeShapes(t *testing.T) {
	canon, err := intl.NewCanonicalizer(intl.Embedded, intl.CanonicalizeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ tag, want string }{
		{"zh-hakka-hakka", "hak"},
		{"sgn-NO", "nsl"},
		{"cnr-BA", "sr-BA"},
		{"hy-SU", "hy-AM"},
		{"en-a-bar-u-baz-x-u-foo", "en-a-bar-u-baz-x-u-foo"},
		{"da-u-ca-gregory-ca-buddhist", "da-u-ca-gregory"},
		{"en-u-kn-yes", "en-u-kn"},
		{"en-u-ca-islamicc", "en-u-ca-islamic-civil"},
		{"en-x-lvariant-posix", "en-u-va-posix"},
		{"en-US-posix", "en-US-u-va-posix"},
		{"und-Latn-t-und-hani-m0-names", "und-Latn-t-und-hani-m0-prprname"},
	} {
		got, err := canon.Canonicalize(c.tag)
		if err != nil || got.String() != c.want {
			t.Errorf("%s = %q, %v; want %q", c.tag, got.String(), err, c.want)
		}
	}
	for _, tag := range []string{"en_GB", "i-klingon", "de-1901-1901", "en-t-zh-t-ja", "root", "x-private"} {
		if got, err := canon.Canonicalize(tag); err == nil {
			t.Errorf("%s was accepted as %q", tag, got.String())
		}
	}
}

// A tag with a letter outside ASCII is not well formed, however it folds:
// Unicode's lowercase of "İ" is an ASCII "i", and the Kelvin sign's a "k",
// but neither makes a subtag of one. test262's Temporal calendar-case-
// insensitive tests refuse "İSO8601" as a calendar.
func TestCanonicalizeRefusesNonASCII(t *testing.T) {
	canon, err := intl.NewCanonicalizer(intl.Embedded, intl.CanonicalizeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, tag := range []string{"und-u-ca-İso8601", "İt", "en-Kelvin", "en-u-ca-ıso8601", "dİe"} {
		if l, err := canon.Canonicalize(tag); err == nil {
			t.Errorf("%q is %q, want an error", tag, l.String())
		}
	}
}
