//go:build gointl_all

package intl_test

import (
	"testing"

	"github.com/liustack/quickjs/internal/gointl"
)

// Plural rules are looked up by truncating the locale, as ICU's PluralRules
// does, not along CLDR's parent locales: Serbian in Latin and Bosnian in
// Cyrillic, whose CLDR parent is the root, still count as their languages
// do. The answers are Node's.
func TestPluralRulesFallBackByTruncation(t *testing.T) {
	for _, c := range []struct {
		tag      string
		one, two intl.PluralCategory
	}{
		{"bs-Cyrl", "one", "few"},
		{"sr-Latn", "one", "few"},
		{"sr-Latn-ME", "one", "few"},
		{"ff-Adlm", "one", "other"},
		{"pt-PT", "one", "other"},
		{"zh-Hant", "other", "other"},
	} {
		loc, err := intl.ParseLocale(c.tag)
		if err != nil {
			t.Fatal(err)
		}
		p, err := intl.NewPluralRules(loc, intl.PluralRulesOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if got := p.Select(1); got != c.one {
			t.Errorf("%s: 1 is %s, want %s", c.tag, got, c.one)
		}
		if got := p.Select(2); got != c.two {
			t.Errorf("%s: 2 is %s, want %s", c.tag, got, c.two)
		}
	}
}
