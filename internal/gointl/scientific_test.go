//go:build gointl_all

package intl_test

import (
	"testing"

	intl "github.com/liustack/quickjs/internal/gointl"
)

// Rounding can carry a mantissa to the next power. Scientific notation then
// moves one power up; engineering notation only where the next power begins
// another group of three, rounding again from the start, as ICU's
// chooseMultiplierAndApply does: 999999.5 is 1E6, where it had been "100E4".
// Node's answers (ISSUES.md NF-1).
func TestScientificCarry(t *testing.T) {
	en, err := intl.ParseLocale("en")
	if err != nil {
		t.Fatal(err)
	}
	eng := intl.NumberFormatOptions{Notation: intl.NotationEngineering}
	sci := intl.NumberFormatOptions{Notation: intl.NotationScientific}
	for _, c := range []struct {
		opts intl.NumberFormatOptions
		v    float64
		want string
	}{
		{eng, 999999.5, "1E6"},
		{eng, 0.9999995, "1E0"},
		{eng, 99999.5, "100E3"},
		{eng, 999.9995, "1E3"},
		{eng, -999999.5, "-1E6"},
		{eng, 0.0009999995, "1E-3"},
		{intl.NumberFormatOptions{Notation: intl.NotationEngineering, MaximumSignificantDigits: intl.Digits(2)},
			999999, "1E6"},
		{intl.NumberFormatOptions{Notation: intl.NotationEngineering, MaximumSignificantDigits: intl.Digits(2)},
			99999, "100E3"},
		{intl.NumberFormatOptions{Notation: intl.NotationEngineering, MaximumFractionDigits: intl.Digits(0)},
			999.6, "1E3"},
		{sci, 9.9995, "1E1"},
		{sci, 99999.5, "1E5"},
	} {
		if got := newNumber(t, en, c.opts).Format(c.v); got != c.want {
			t.Errorf("%+v %v: %q, want %q", c.opts, c.v, got, c.want)
		}
	}

	fr, err := intl.ParseLocale("fr")
	if err != nil {
		t.Fatal(err)
	}
	pr, err := intl.NewPluralRules(fr, intl.PluralRulesOptions{Notation: intl.NotationEngineering})
	if err != nil {
		t.Fatal(err)
	}
	if got := pr.Select(0.99999); got != "many" {
		t.Errorf("fr engineering 0.99999: %q, want many", got)
	}
}
