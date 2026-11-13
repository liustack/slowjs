//go:build gointl_all

package intl_test

import (
	"testing"

	intl "github.com/liustack/quickjs/internal/gointl"
)

// TestLeftToRightDirection pins getTextInfo's direction on both sides: the
// standard answers no direction for a script that has none of its own or
// that no one has registered, or a locale whose likely script cannot be
// found (test262's script-metadata-rtl-is-unknown,
// script-unregistered-or-privateuse, script-subtag-missing and
// language-subtag-with-more-than-three-letters); ICU, and so Node v26.10,
// answers "ltr" for each of them. A script known either way is answered
// alike, a variant like its script.
func TestLeftToRightDirection(t *testing.T) {
	for _, c := range []struct {
		locale         string
		standard, node string
	}{
		{"und-Zyyy", "", "ltr"},
		{"en-Zinh", "", "ltr"},
		{"ar-Zzzz", "", "ltr"},
		{"en-Brai", "", "ltr"},
		{"ar-Zxxx", "", "ltr"},
		{"und-Aaaa", "", "ltr"},
		{"en-Qaaq", "", "ltr"},
		{"tlh", "", "ltr"},
		{"qfz", "", "ltr"},
		{"abcdefgh", "", "ltr"},
		{"abcdefgh-Latn", "ltr", "ltr"},
		{"abcdefgh-Arab", "rtl", "rtl"},
		{"und", "ltr", "ltr"},
		{"en", "ltr", "ltr"},
		{"ar", "rtl", "rtl"},
		{"pa", "ltr", "ltr"},
		{"pa-PK", "rtl", "rtl"},
		{"he", "rtl", "rtl"},
		{"ja", "ltr", "ltr"},
		{"tlh-Latn", "ltr", "ltr"},
		{"ur-Aran", "rtl", "ltr"},
		{"de-Latf", "ltr", "ltr"},
	} {
		l, err := intl.ParseLocale(c.locale)
		if err != nil {
			t.Fatal(err)
		}
		for _, side := range []struct {
			compat intl.Compat
			want   string
		}{{intl.Standard, c.standard}, {intl.LeftToRightDirection, c.node}} {
			info, err := intl.NewLocaleInfo(intl.Embedded, intl.LocaleInfoOptions{Compat: side.compat})
			if err != nil {
				t.Fatal(err)
			}
			if got, err := info.Direction(l); err != nil || got != side.want {
				t.Errorf("%v: %s answers %q, %v, want %q", side.compat, c.locale, got, err, side.want)
			}
		}
	}
}
