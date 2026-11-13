//go:build gointl_all

package intl_test

import (
	"math"
	"testing"

	intl "github.com/liustack/quickjs/internal/gointl"
)

// NaN and the infinities are "other" in every language and either type, as
// ICU's plural rules answer for a number that is not finite. go-intl had
// tested them against the rules: French NaN was "one", Welsh "few". Node's
// answers (ISSUES.md NF-8).
func TestPluralNonFinite(t *testing.T) {
	for _, tag := range []string{"fr", "hi", "cy", "en", "ar"} {
		loc, err := intl.ParseLocale(tag)
		if err != nil {
			t.Fatal(err)
		}
		for _, typ := range []intl.PluralType{intl.Cardinal, intl.Ordinal} {
			for _, n := range []intl.Notation{intl.NotationStandard, intl.NotationCompact} {
				pr, err := intl.NewPluralRules(loc, intl.PluralRulesOptions{Type: typ, Notation: n})
				if err != nil {
					t.Fatal(err)
				}
				for _, v := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
					if got := pr.Select(v); got != intl.PluralOther {
						t.Errorf("%s %v %v %v: %q, want other", tag, typ, n, v, got)
					}
				}
			}
		}
	}
}
