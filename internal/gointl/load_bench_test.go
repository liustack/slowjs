//go:build gointl_all

package intl_test

import (
	"testing"

	intl "github.com/liustack/quickjs/internal/gointl"
)

// What building each service costs: what a host pays on every
// toLocaleString and every Intl constructor, since nothing is cached.

var benchLocales = []string{"en", "de", "ja", "ar"}

func benchLocale(b *testing.B, tag string) intl.Locale {
	b.Helper()
	l, err := intl.ParseLocale(tag)
	if err != nil {
		b.Fatal(err)
	}
	return l
}

func benchEach(b *testing.B, build func(intl.Locale) error) {
	for _, tag := range benchLocales {
		loc := benchLocale(b, tag)
		b.Run(tag, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if err := build(loc); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkLoadNumberFormat(b *testing.B) {
	benchEach(b, func(l intl.Locale) error {
		_, err := intl.NewNumberFormat(l, intl.NumberFormatOptions{})
		return err
	})
}

func BenchmarkLoadNumberFormatCurrency(b *testing.B) {
	benchEach(b, func(l intl.Locale) error {
		_, err := intl.NewNumberFormat(l, intl.NumberFormatOptions{Style: intl.StyleCurrency, Currency: "EUR"})
		return err
	})
}

func BenchmarkLoadDateTimeFormat(b *testing.B) {
	benchEach(b, func(l intl.Locale) error {
		_, err := intl.NewDateTimeFormat(l, intl.DateTimeFormatOptions{})
		return err
	})
}

func BenchmarkLoadDateTimeFormatFull(b *testing.B) {
	benchEach(b, func(l intl.Locale) error {
		_, err := intl.NewDateTimeFormat(l, intl.DateTimeFormatOptions{
			DateStyle: intl.LengthFull, TimeStyle: intl.LengthFull, TimeZone: "America/New_York"})
		return err
	})
}

func BenchmarkLoadCollator(b *testing.B) {
	benchEach(b, func(l intl.Locale) error {
		_, err := intl.NewCollator(l, intl.CollatorOptions{})
		return err
	})
}

func BenchmarkLoadPluralRules(b *testing.B) {
	benchEach(b, func(l intl.Locale) error {
		_, err := intl.NewPluralRules(l, intl.PluralRulesOptions{})
		return err
	})
}

func BenchmarkLoadListFormat(b *testing.B) {
	benchEach(b, func(l intl.Locale) error {
		_, err := intl.NewListFormat(l, intl.ListFormatOptions{})
		return err
	})
}

func BenchmarkLoadRelativeTimeFormat(b *testing.B) {
	benchEach(b, func(l intl.Locale) error {
		_, err := intl.NewRelativeTimeFormat(l, intl.RelativeTimeFormatOptions{})
		return err
	})
}

func BenchmarkLoadDisplayNames(b *testing.B) {
	benchEach(b, func(l intl.Locale) error {
		_, err := intl.NewDisplayNames(l, intl.DisplayNamesOptions{Kind: intl.DisplayRegion})
		return err
	})
}

func BenchmarkLoadDurationFormat(b *testing.B) {
	benchEach(b, func(l intl.Locale) error {
		_, err := intl.NewDurationFormat(l, intl.DurationFormatOptions{})
		return err
	})
}

func BenchmarkLoadSegmenter(b *testing.B) {
	benchEach(b, func(l intl.Locale) error {
		_, err := intl.NewSegmenter(l, intl.SegmenterOptions{Granularity: intl.GranularityWord})
		return err
	})
}

func BenchmarkLoadLocaleMatcher(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := intl.NewLocaleMatcher(intl.Embedded, intl.ServiceNumberFormat); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkLoadCanonicalizer(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := intl.NewCanonicalizer(intl.Embedded, intl.CanonicalizeOptions{}); err != nil {
			b.Fatal(err)
		}
	}
}
