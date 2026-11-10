//go:build gointl_all

package intl_test

import (
	"strings"
	"testing"

	"github.com/liustack/quickjs/internal/gointl"
)

// ResolveLocale and SupportedLocales by lookup, with Node's answers: the
// first request the service has, cut short, with its Unicode extension (the
// service keeps only the keywords it reads: Node's NumberFormat drops
// "-u-ca-buddhist" from "fr"), else the default.
func TestLocaleMatcherResolve(t *testing.T) {
	m, err := intl.NewLocaleMatcher(intl.Embedded, intl.ServiceNumberFormat)
	if err != nil {
		t.Fatal(err)
	}
	def, _ := intl.ParseLocale("en-US")
	for _, c := range []struct {
		requested []string
		resolved  string
		supported string
	}{
		{[]string{"xx", "de-CH-u-nu-thai"}, "de-CH-u-nu-thai", "de-CH-u-nu-thai"},
		{[]string{"ht"}, "en-US", ""},
		{[]string{"az-Arab-u-nu-latn"}, "az-u-nu-latn", "az-Arab-u-nu-latn"},
		{[]string{"en-t-ja"}, "en", "en-t-ja"},
		{[]string{"zh-Hant-SG"}, "zh-Hant", "zh-Hant-SG"},
		{[]string{"sr-Latn-XK"}, "sr-Latn-XK", "sr-Latn-XK"},
		{[]string{"qaa", "fr-XY-u-ca-buddhist"}, "fr-u-ca-buddhist", "fr-XY-u-ca-buddhist"},
	} {
		var requested []intl.Locale
		for _, tag := range c.requested {
			l, err := intl.ParseLocale(tag)
			if err != nil {
				t.Fatal(err)
			}
			requested = append(requested, l)
		}
		for _, kind := range []intl.MatcherKind{intl.BestFit, intl.Lookup} {
			if got := m.Resolve(requested, kind, def).String(); got != c.resolved {
				t.Errorf("Resolve(%v) = %s, want %s", c.requested, got, c.resolved)
			}
			var supported []string
			for _, l := range m.Supported(requested, kind) {
				supported = append(supported, l.String())
			}
			if got := strings.Join(supported, " "); got != c.supported {
				t.Errorf("Supported(%v) = %q, want %q", c.requested, got, c.supported)
			}
		}
	}
}

// A service's available locales, as data/available.bin records V8's: each
// one the matcher finds, in order, and no locale ICU has no data for. V8's
// "no-NO-NY", ICU's legacy alias, is kept for matching but not listed,
// since it is not a language tag.
func TestLocaleMatcherLocales(t *testing.T) {
	for service, want := range map[intl.Service]int{
		intl.ServiceDateTimeFormat: 969,
		intl.ServiceNumberFormat:   961,
		intl.ServiceCollator:       154,
		intl.ServiceSegmenter:      971,
	} {
		m, err := intl.NewLocaleMatcher(intl.Embedded, service)
		if err != nil {
			t.Fatal(err)
		}
		tags := m.Locales()
		if len(tags) != want {
			t.Errorf("%s: %d locales, want %d", service, len(tags), want)
		}
		for i, tag := range tags {
			if !m.Available(tag) || i > 0 && tags[i-1] >= tag {
				t.Errorf("%s: %q unavailable or out of order", service, tag)
			}
			if tag == "ht" || tag == "az-Arab" {
				t.Errorf("%s: lists %q, which ICU has no data for", service, tag)
			}
			if _, err := intl.ParseLocale(tag); err != nil {
				t.Errorf("%s: lists %q, which is not a tag", service, tag)
			}
		}
		if service == intl.ServiceDateTimeFormat && !m.Available("no-NO-NY") {
			t.Errorf("%s: V8's no-NO-NY is not available", service)
		}
	}
}
