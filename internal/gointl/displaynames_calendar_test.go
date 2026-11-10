//go:build gointl_all

package intl_test

import (
	"testing"

	intl "github.com/liustack/quickjs/internal/gointl"
)

// A calendar is named by CLDR's key, or by the two BCP 47 spellings V8 maps
// to it, "gregory" and "ethioaa"; "islamicc", a deprecated alias of
// "islamic-civil", has no name. go-intl had named it. Node's answers
// (ISSUES.md NF-18).
func TestCalendarDisplayNames(t *testing.T) {
	for _, c := range []struct {
		tag, code, want string // "" for no name
	}{
		{"en", "islamicc", ""},
		{"ar", "islamicc", ""},
		{"en", "islamic-civil", "Hijri Calendar (tabular, civil epoch)"},
		{"en", "gregory", "Gregorian Calendar"},
		{"en", "ethioaa", "Ethiopic Amete Alem Calendar"},
		{"en", "ethiopic-amete-alem", "Ethiopic Amete Alem Calendar"},
		{"en", "islamic-umalqura", "Hijri Calendar (Umm al-Qura)"},
	} {
		loc, err := intl.ParseLocale(c.tag)
		if err != nil {
			t.Fatal(err)
		}
		d, err := intl.NewDisplayNames(loc, intl.DisplayNamesOptions{Kind: intl.DisplayCalendar,
			Fallback: intl.FallbackNone})
		if err != nil {
			t.Fatal(err)
		}
		got, ok := d.Of(c.code)
		if !ok {
			got = ""
		}
		if got != c.want {
			t.Errorf("%s %s: %q, want %q", c.tag, c.code, got, c.want)
		}
	}
}
