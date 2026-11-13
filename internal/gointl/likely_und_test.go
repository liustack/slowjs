//go:build gointl_all

package intl_test

import (
	"testing"

	intl "github.com/liustack/quickjs/internal/gointl"
)

// An unknown language with a script and a region is looked up by its
// script before its region, as ICU's likely-subtags trie does: und-Cyrl-CN
// is Russian in China, where go-intl had made it Chinese in Cyrillic. Node's
// answers (ISSUES.md LO-3).
func TestLikelySubtagsUndScriptFirst(t *testing.T) {
	info, err := intl.NewLocaleInfo(intl.Embedded, intl.LocaleInfoOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ tag, max, min string }{
		{"und-Cyrl-CN", "ru-Cyrl-CN", "ru-CN"},
		{"und-Hans-RU", "zh-Hans-RU", "zh-RU"},
		{"und-Hant-RU", "zh-Hant-RU", "zh-Hant-RU"},
		{"und-Cyrl-419", "ru-Cyrl-419", "ru-419"},
		{"und-Latn-CN", "za-Latn-CN", "za"},
		{"und-CN", "zh-Hans-CN", "zh"},
		{"und-Cyrl", "ru-Cyrl-RU", "ru"},
		{"en-Cyrl-CN", "en-Cyrl-CN", "en-Cyrl-CN"},
	} {
		l, err := intl.ParseLocale(c.tag)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Maximize(l).String(); got != c.max {
			t.Errorf("%s maximized: %s, want %s", c.tag, got, c.max)
		}
		if got := info.Minimize(l).String(); got != c.min {
			t.Errorf("%s minimized: %s, want %s", c.tag, got, c.min)
		}
	}
}
