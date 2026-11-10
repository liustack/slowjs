//go:build gointl_all

package intl_test

import (
	"testing"

	intl "github.com/liustack/quickjs/internal/gointl"
)

// A language the likely subtags do not know is not maximized, as ICU leaves
// it and Node answers: "gss" is "gss", not "gss-Latn-US" by way of "und",
// and what depends on its region is the world's. "und" itself is.
func TestMaximizeUnknownLanguage(t *testing.T) {
	info, err := intl.NewLocaleInfo(intl.Embedded, intl.LocaleInfoOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for tag, want := range map[string]string{
		"gss": "gss", "xyz": "xyz", "xyz-DE": "xyz-DE", "xyz-Cyrl": "xyz-Cyrl", "qaa": "qaa",
		"und": "en-Latn-US", "und-DE": "de-Latn-DE", "und-Cyrl": "ru-Cyrl-RU", "zzj": "zzj-Hani-CN",
	} {
		l, err := intl.ParseLocale(tag)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Maximize(l).String(); got != want {
			t.Errorf("%s maximizes to %s, want %s", tag, got, want)
		}
	}
	l, _ := intl.ParseLocale("gss")
	if got, err := info.HourCycles(l); err != nil || len(got) != 1 || got[0] != "h23" {
		t.Errorf("gss's hour cycles are %v, %v, want [h23]", got, err)
	}
	if first, _, err := info.WeekInfo(l); err != nil || first != 1 {
		t.Errorf("gss's week starts on %d, %v, want 1", first, err)
	}
}

// The unknown script "Zzzz" and region "ZZ" count as left out, as UTS #35's
// Add Likely Subtags removes them first; under UnknownSubtags, as ICU has
// it, a locale that names all three is kept. Minimizing removes them either
// way, as ICU does too.
func TestLikelyUnknownSubtags(t *testing.T) {
	for _, c := range []struct {
		tag, standard, node, min string
	}{
		{"und-Zzzz-ZZ", "en-Latn-US", "en-Latn-US", "en"},
		{"en-Zzzz-US", "en-Latn-US", "en-Zzzz-US", "en"},
		{"en-Latn-ZZ", "en-Latn-US", "en-Latn-ZZ", "en"},
		{"sr-Zzzz-ME", "sr-Latn-ME", "sr-Zzzz-ME", "sr-ME"},
		{"zh-Zzzz-TW", "zh-Hant-TW", "zh-Zzzz-TW", "zh-TW"},
		{"en-Zzzz-001", "en-Latn-001", "en-Zzzz-001", "en-001"},
		{"und-Zzzz-ME", "sr-Latn-ME", "sr-Latn-ME", "sr-ME"},
		{"zh-Zzzz", "zh-Hans-CN", "zh-Hans-CN", "zh"},
		{"und-ZZ", "en-Latn-US", "en-Latn-US", "en"},
		{"und-Cyrl-ZZ", "ru-Cyrl-RU", "ru-Cyrl-RU", "ru"},
		{"xyz-Zzzz-DE", "xyz-Zzzz-DE", "xyz-Zzzz-DE", "xyz-Zzzz-DE"},
		{"en-Zzzz-ZZ-u-ca-gregory", "en-Latn-US-u-ca-gregory", "en-Zzzz-ZZ-u-ca-gregory", "en-u-ca-gregory"},
	} {
		l, err := intl.ParseLocale(c.tag)
		if err != nil {
			t.Fatal(err)
		}
		for _, compat := range []intl.Compat{intl.Standard, intl.UnknownSubtags} {
			info, err := intl.NewLocaleInfo(intl.Embedded, intl.LocaleInfoOptions{Compat: compat})
			if err != nil {
				t.Fatal(err)
			}
			want := c.standard
			if compat != intl.Standard {
				want = c.node
			}
			if got := info.Maximize(l).String(); got != want {
				t.Errorf("%s maximizes to %s under %v, want %s", c.tag, got, compat, want)
			}
			if got := info.Minimize(l).String(); got != c.min {
				t.Errorf("%s minimizes to %s under %v, want %s", c.tag, got, compat, c.min)
			}
		}
	}
}

// What Maximize and Minimize return is the caller's own: a Locale is a
// struct of four slices, so returning a copy of the one given would hand
// back its arrays, and a caller that changed the answer would change the
// question. CanonicalizeLocale already clones; these had not.
func TestMaximizeAndMinimizeCopyTheirInput(t *testing.T) {
	info, err := intl.NewLocaleInfo(intl.Embedded, intl.LocaleInfoOptions{})
	if err != nil {
		t.Fatal(err)
	}
	// Every path out of the two: the locale the likely subtags fill in,
	// one they know nothing of and leave as it is, and the shortest form.
	for _, c := range []struct {
		name string
		tag  string
		fn   func(intl.Locale) intl.Locale
	}{
		{"Maximize", "en-GB-fonipa-u-attr-ca-japanese-rg-gbzzzz-t-de", info.Maximize},
		{"Maximize", "xyz-GB-fonipa-u-attr-ca-japanese-rg-gbzzzz-t-de", info.Maximize},
		{"Minimize", "zh-Hant-TW-fonipa-u-attr-ca-chinese-rg-twzzzz-t-de", info.Minimize},
	} {
		in, err := intl.ParseLocale(c.tag)
		if err != nil {
			t.Fatal(err)
		}
		before := in.String()
		out := c.fn(in)
		out.Variants[0] = intl.Variant{}
		out.Attributes[0] = "changed"
		out.Keywords[0].Value = "changed"
		out.Extensions[0].Value = "changed"
		if got := in.String(); got != before {
			t.Errorf("%s(%s): the locale given reads %s after its answer was changed", c.name, c.tag, got)
		}
	}
}
