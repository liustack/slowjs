//go:build gointl_all

package intl

import (
	"strings"
	"testing"
)

// The text a normalizer is given is mostly made of characters that do not
// decompose at all, which it must answer without searching the table.
var normalizeTexts = []struct {
	name, text string
}{
	{"ASCII", strings.Repeat("The quick brown fox jumps over the lazy dog. ", 23)},
	{"Latin", strings.Repeat("Les naïfs ægithales hâtifs pondant à Noël. ", 24)},
	{"CJK", strings.Repeat("日本語の文章です。中文句子。", 72)},
}

func BenchmarkNormalize(b *testing.B) {
	n, err := NewNormalizer()
	if err != nil {
		b.Fatal(err)
	}
	for _, t := range normalizeTexts {
		for _, form := range []struct {
			name string
			f    NormalizationForm
		}{{"NFD", NFD}, {"NFC", NFC}} {
			b.Run(t.name+"/"+form.name, func(b *testing.B) {
				runes := len([]rune(t.text))
				for i := 0; i < b.N; i++ {
					n.Normalize(t.text, form.f)
				}
				b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N*runes), "ns/char")
			})
		}
	}
}
