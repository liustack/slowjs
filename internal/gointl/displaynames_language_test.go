//go:build gointl_all

package intl_test

import (
	"testing"

	intl "github.com/liustack/quickjs/internal/gointl"
)

// A language is named as ICU's LocaleDisplayNamesImpl::localeDisplayName
// names it: the code in canonical form, a name for the language with its
// script and region where there is one, and beside it what that does not
// name, the variants among them, the code standing in for a part with no
// name when the fallback is the code. go-intl had dropped the variants and
// the parts with no name: "ca-ES-valencia" was "Catalan (Spain)". Node's
// answers (ISSUES.md NF-16).
func TestLanguageDisplayNames(t *testing.T) {
	standard := intl.DisplayNamesOptions{Kind: intl.DisplayLanguage, LanguageDisplay: intl.LanguageStandard}
	none := intl.DisplayNamesOptions{Kind: intl.DisplayLanguage, Fallback: intl.FallbackNone}
	dialect := intl.DisplayNamesOptions{Kind: intl.DisplayLanguage}
	for _, c := range []struct {
		tag  string
		opts intl.DisplayNamesOptions
		code string
		want string // "" for no name
	}{
		{"en", dialect, "en-GB-oxendict", "British English (Oxford English Dictionary spelling)"},
		{"en", dialect, "en-US-posix", "American English (Computer)"},
		{"en", dialect, "en-posix", "English (Computer)"},
		{"en", dialect, "en-u-va-posix", "English (Computer)"},
		{"en", standard, "en-US-posix", "English (United States, Computer)"},
		{"en", dialect, "ca-ES-valencia", "Catalan (Spain, Valencian)"},
		{"en", dialect, "de-CH-1996", "Swiss High German (German orthography of 1996)"},
		{"en", dialect, "ja-hepburn-heploc", "Japanese (ALA-LC Romanization, 1997 edition)"},
		// ICU asks for a name for all three or none.
		{"en", dialect, "en-Latn-GB", "English (Latin, United Kingdom)"},
		{"en", dialect, "zh-Hant-HK", "Chinese (Traditional, Hong Kong SAR China)"},
		{"en", dialect, "yue-Hans", "Cantonese (Simplified)"},
		// The code in canonical form.
		{"en", dialect, "mo", "Romanian"},
		{"en", dialect, "iw-IL", "Hebrew (Israel)"},
		{"en", dialect, "en-840", "American English"},
		{"en", dialect, "art-lojban", "Lojban"},
		// Parentheses inside become brackets.
		{"en", dialect, "en-MM", "English (Myanmar [Burma])"},
		{"zh", dialect, "fr-CD", "法语（刚果［金］）"},
		{"zh", dialect, "ca-ES-valencia", "加泰罗尼亚语（西班牙，巴伦西亚文）"},
		// A variant alternate is not the name.
		{"ja", dialect, "hi-Latn", "ヒンディー語 (ラテン文字)"},
		// A part with no name.
		{"en", dialect, "en-xyzzy", "English (XYZZY)"},
		{"en", none, "en-xyzzy", ""},
		{"en", dialect, "xyz-GB", "xyz (United Kingdom)"},
		{"en", none, "xyz-GB", ""},
		{"en", dialect, "en-QM", "English (QM)"},
		{"ja", dialect, "en-GB-oxendict", "イギリス英語 (OXENDICT)"},
		{"en", dialect, "sl-rozaj-biske", "Slovenian (BISKE_ROZAJ)"},
		{"en", dialect, "sl-rozaj", "Slovenian (Resian)"},
		{"en", dialect, "sl-posix-rozaj", "Slovenian (POSIX_ROZAJ)"},
		{"en", none, "sl-posix-rozaj", ""},
		{"en", dialect, "und", "root"},
		{"en", none, "und", ""},
		{"en", dialect, "und-GB", "root (United Kingdom)"},
		// ICU reads four letters after the language as a script.
		{"en", dialect, "de-baku1926", "German (Baku)"},
		{"en", dialect, "de-baku1926-rozaj", "German (Baku)"},
		{"en", none, "de-baku1926", ""},
		{"en", none, "de-DE-baku1926", "German (Germany, Unified Turkic Latin Alphabet)"},
		{"en", dialect, "de-1996-colb1945", "German (1996_COLB1945)"},
	} {
		loc, err := intl.ParseLocale(c.tag)
		if err != nil {
			t.Fatal(err)
		}
		d, err := intl.NewDisplayNames(loc, c.opts)
		if err != nil {
			t.Fatal(err)
		}
		got, ok := d.Of(c.code)
		if !ok {
			got = ""
		}
		if got != c.want {
			t.Errorf("%s %+v %s: %q, want %q", c.tag, c.opts, c.code, got, c.want)
		}
	}
}
