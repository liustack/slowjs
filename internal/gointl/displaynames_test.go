//go:build gointl_all

package intl_test

import (
	"testing"

	intl "github.com/liustack/quickjs/internal/gointl"
)

// The corpus asks only in English and only for the long style, so these go
// where it does not. The expectations are node's.
func TestDisplayNamesBeyondTheCorpus(t *testing.T) {
	for _, c := range []struct {
		loc, kind, code string
		style           intl.DisplayStyle
		lang            intl.LanguageDisplay
		want            string
	}{
		{"ja", "language", "fr", intl.DisplayLong, intl.LanguageDialect, "フランス語"},
		{"de", "region", "FR", intl.DisplayLong, intl.LanguageDialect, "Frankreich"},
		{"fr", "script", "Cyrl", intl.DisplayLong, intl.LanguageDialect, "cyrillique"},
		// A language with a region has a name of its own, until it is asked
		// for the standard form, which builds it from the parts instead.
		{"en", "language", "en-GB", intl.DisplayLong, intl.LanguageDialect, "British English"},
		{"en", "language", "en-GB", intl.DisplayLong, intl.LanguageStandard, "English (United Kingdom)"},
		{"en", "language", "zh-Hans", intl.DisplayLong, intl.LanguageDialect, "Simplified Chinese"},
		{"en", "region", "GB", intl.DisplayShort, intl.LanguageDialect, "UK"},
		{"en", "dateTimeField", "weekday", intl.DisplayShort, intl.LanguageDialect, "day of wk."},
		// Three of ECMA-402's fields are CLDR's under other names.
		{"en", "dateTimeField", "weekOfYear", intl.DisplayShort, intl.LanguageDialect, "wk."},
		{"de", "dateTimeField", "weekOfYear", intl.DisplayNarrow, intl.LanguageDialect, "W"},
		{"de", "dateTimeField", "dayPeriod", intl.DisplayLong, intl.LanguageDialect, "Tageshälfte"},
		{"ja", "dateTimeField", "timeZoneName", intl.DisplayLong, intl.LanguageDialect, "タイムゾーン"},
		// ECMA-402 spells this calendar "roc" and CLDR files it as "roc" too,
		// but "gregory" and "gregorian" differ, so both spellings are stored.
		{"en", "calendar", "roc", intl.DisplayLong, intl.LanguageDialect, "Minguo Calendar"},
		{"en", "calendar", "gregory", intl.DisplayLong, intl.LanguageDialect, "Gregorian Calendar"},
		// A currency's plain name, not the wording that goes beside an amount.
		{"de", "currency", "USD", intl.DisplayLong, intl.LanguageDialect, "US-Dollar"},
		{"en", "currency", "USD", intl.DisplayLong, intl.LanguageDialect, "US Dollar"},
	} {
		loc, err := intl.ParseLocale(c.loc)
		if err != nil {
			t.Fatal(err)
		}
		kind, ok := intl.ParseDisplayKind(c.kind)
		if !ok {
			t.Fatalf("%q is not a kind", c.kind)
		}
		d, err := intl.NewDisplayNames(loc, intl.DisplayNamesOptions{
			Kind: kind, Style: c.style, LanguageDisplay: c.lang,
		})
		if err != nil {
			t.Fatal(err)
		}
		if got, _ := d.Of(c.code); got != c.want {
			t.Errorf("%s %s %s = %q, want %q", c.loc, c.kind, c.code, got, c.want)
		}
	}
}

// A code with no name is answered with the code or with nothing, by the
// fallback option, and the second result says which happened either way.
//
// The code has to be one CLDR really has no name for. ZZ looks like one and is
// not: it is "Unknown Region", which is a name.
func TestDisplayNamesFallback(t *testing.T) {
	loc, err := intl.ParseLocale("en")
	if err != nil {
		t.Fatal(err)
	}
	kind, _ := intl.ParseDisplayKind("region")

	code, err := intl.NewDisplayNames(loc, intl.DisplayNamesOptions{Kind: kind})
	if err != nil {
		t.Fatal(err)
	}
	if got, found := code.Of("QQ"); got != "QQ" || found {
		t.Errorf("Of(QQ) = %q, %v; want %q, false", got, found, "QQ")
	}

	none, err := intl.NewDisplayNames(loc, intl.DisplayNamesOptions{
		Kind: kind, Fallback: intl.FallbackNone,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, found := none.Of("QQ"); got != "" || found {
		t.Errorf("Of(QQ) with no fallback = %q, %v; want empty, false", got, found)
	}
	if got, found := none.Of("FR"); got != "France" || !found {
		t.Errorf("Of(FR) = %q, %v; want France, true", got, found)
	}
}
