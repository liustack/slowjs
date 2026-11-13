//go:build gointl_all

package intl_test

import (
	"errors"
	"strings"
	"testing"

	intl "github.com/liustack/quickjs/internal/gointl"
)

// Parsing settles spelling: an identifier written any of the ways a tag is
// written comes back in the one canonical form.
func TestParseLocaleCanonicalizes(t *testing.T) {
	cases := []struct{ in, want string }{
		{"en", "en"},
		{"EN", "en"},
		{"en-us", "en-US"},
		{"en_US", "en-US"},
		{"EN-US", "en-US"},
		{"zh-hant", "zh-Hant"},
		{"ZH-HANT-tw", "zh-Hant-TW"},
		{"es-419", "es-419"},
		{"de-ch-1901", "de-CH-1901"},
		// Variants are ordered, so two identifiers saying the same thing are
		// written the same way.
		{"sl-rozaj-biske", "sl-biske-rozaj"},
		{"sl-biske-rozaj", "sl-biske-rozaj"},
		// The undetermined language has one spelling.
		{"und", "und"},
		{"root", "und"},
		{"und-Latn-DE", "und-Latn-DE"},
		// Unicode extension: keywords ordered, a true value left unwritten.
		{"de-DE-u-ca-buddhist", "de-DE-u-ca-buddhist"},
		{"de-u-nu-latn-ca-gregory", "de-u-ca-gregory-nu-latn"},
		{"en-u-kn-true", "en-u-kn"},
		{"en-u-kn", "en-u-kn"},
		{"en-US-u-attr2-attr1-ca-gregory", "en-US-u-attr1-attr2-ca-gregory"},
		// Other extensions and private use survive.
		{"en-t-en-latn", "en-t-en-latn"},
		{"en-x-Private", "en-x-private"},
		{"en-u-ca-gregory-x-p", "en-u-ca-gregory-x-p"},
	}
	for _, c := range cases {
		l, err := intl.ParseLocale(c.in)
		if err != nil {
			t.Errorf("ParseLocale(%q): %v", c.in, err)
			continue
		}
		if got := l.String(); got != c.want {
			t.Errorf("ParseLocale(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// Writing an identifier out and reading it back has to give the same thing,
// or canonicalization is losing something.
func TestParseLocaleRoundTrips(t *testing.T) {
	for _, s := range []string{
		"en", "en-US", "zh-Hant-TW", "es-419", "sl-biske-rozaj",
		"de-DE-u-ca-buddhist-nu-latn", "en-u-kn", "en-t-en-latn", "en-x-private",
		"und", "und-Latn-DE",
	} {
		l, err := intl.ParseLocale(s)
		if err != nil {
			t.Errorf("ParseLocale(%q): %v", s, err)
			continue
		}
		again, err := intl.ParseLocale(l.String())
		if err != nil {
			t.Errorf("reparsing %q: %v", l.String(), err)
			continue
		}
		if got := again.String(); got != s {
			t.Errorf("%q wrote itself as %q", s, got)
		}
	}
}

// A tag that is not well formed is an error rather than a locale with pieces
// quietly dropped, because ECMA-402 answers this with a RangeError.
func TestParseLocaleRejectsMalformed(t *testing.T) {
	for _, s := range []string{
		"",
		"   ",
		"e",                         // a language is at least two letters
		"abcd",                      // four letters is a script's shape, not a language's
		"toolonglanguage",           // more than eight
		"en-",                       // an empty subtag
		"-en",                       // an empty subtag
		"en--US",                    // an empty subtag
		"en-US-",                    // an empty subtag
		"en-123456789",              // a variant is at most eight
		"en-u",                      // an extension with nothing after it
		"en-u-ca-gregory-u-nu-latn", // the same singleton twice
		"1234",                      // a language is letters
		"en-Lat1",                   // a script is letters
	} {
		if l, err := intl.ParseLocale(s); err == nil {
			t.Errorf("ParseLocale(%q) = %q, want an error", s, l)
		} else if !errors.Is(err, intl.ErrSyntax) {
			t.Errorf("ParseLocale(%q) gave %v, which is not ErrSyntax", s, err)
		}
	}
}

// The identifier and the key data is looked up by are different things: the
// extensions choose behavior and must not reach the table.
func TestDataLocaleDropsExtensions(t *testing.T) {
	l, err := intl.ParseLocale("de-CH-u-ca-buddhist-nu-latn-x-p")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := l.Data().String(), "de-CH"; got != want {
		t.Errorf("Data() = %q, want %q", got, want)
	}
	if v, ok := l.Keyword("ca"); !ok || v != "buddhist" {
		t.Errorf("Keyword(ca) = %q, %v", v, ok)
	}
	if _, ok := l.Keyword("co"); ok {
		t.Error("Keyword(co) found a setting that was not given")
	}
}

// A DataLocale holds no slices, so it can be a map key and compared with ==.
// That is the point of the fixed-size subtags.
func TestDataLocaleIsComparable(t *testing.T) {
	a, err := intl.ParseLocale("zh-Hant-TW-u-ca-roc")
	if err != nil {
		t.Fatal(err)
	}
	b, err := intl.ParseLocale("ZH-hant-tw")
	if err != nil {
		t.Fatal(err)
	}
	if a.Data() != b.Data() {
		t.Errorf("%v and %v are not the same data locale", a.Data(), b.Data())
	}
	seen := map[intl.DataLocale]int{a.Data(): 1}
	if seen[b.Data()] != 1 {
		t.Error("a data locale did not work as a map key")
	}
}

func TestFallbackChain(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"de-Latn-CH", []string{"de-Latn-CH", "de-Latn", "de", "und"}},
		{"de-CH", []string{"de-CH", "de", "und"}},
		{"zh-Hant", []string{"zh-Hant", "zh", "und"}},
		{"en", []string{"en", "und"}},
		{"und", []string{"und"}},
		{"und-DE", []string{"und-DE", "und"}},
		// The extensions play no part in where data is looked for.
		{"de-CH-u-ca-buddhist", []string{"de-CH", "de", "und"}},
	}
	for _, c := range cases {
		l, err := intl.ParseLocale(c.in)
		if err != nil {
			t.Errorf("ParseLocale(%q): %v", c.in, err)
			continue
		}
		var got []string
		for _, d := range l.Fallback() {
			got = append(got, d.String())
		}
		if strings.Join(got, ",") != strings.Join(c.want, ",") {
			t.Errorf("%q falls back through %v, want %v", c.in, got, c.want)
		}
	}
}

// The chain always ends at the root, which is where an answer is guaranteed.
func TestFallbackEndsAtRoot(t *testing.T) {
	for _, s := range []string{"de-Latn-CH", "en", "und", "es-419", "zh-Hant-TW"} {
		l, err := intl.ParseLocale(s)
		if err != nil {
			t.Fatal(err)
		}
		chain := l.Fallback()
		if len(chain) == 0 {
			t.Errorf("%q has an empty fallback chain", s)
			continue
		}
		if last := chain[len(chain)-1]; !last.IsRoot() {
			t.Errorf("%q ends its chain at %q rather than the root", s, last)
		}
		if first := chain[0]; first != l.Data() {
			t.Errorf("%q starts its chain at %q", s, first)
		}
	}
}
