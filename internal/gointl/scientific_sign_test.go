//go:build gointl_all

package intl_test

import (
	"testing"

	intl "github.com/liustack/quickjs/internal/gointl"
)

// In scientific notation signDisplay "exceptZero" asks whether the mantissa
// as written is zero, which a rounding increment can make it: 1 to the
// nearest 2.5 thousandths is "0.000E0", with no sign, where go-intl wrote
// "+0.000E0". Node's answers (ISSUES.md NF-13).
func TestScientificSignOfZero(t *testing.T) {
	loc, err := intl.ParseLocale("de-CH")
	if err != nil {
		t.Fatal(err)
	}
	f := newNumber(t, loc, intl.NumberFormatOptions{Notation: intl.NotationScientific,
		SignDisplay: intl.SignExceptZero, MinimumFractionDigits: intl.Digits(3),
		MaximumFractionDigits: intl.Digits(3), RoundingIncrement: 2500})
	for v, want := range map[float64]string{
		1: "0.000E0", -1: "0.000E0", 2: "+2.500E0", 0.5: "+5.000E-1", 5: "+5.000E0",
		1e-9: "0.000E-9", -0.7: "-7.500E-1",
	} {
		if got := f.Format(v); got != want {
			t.Errorf("%v: %q, want %q", v, got, want)
		}
	}
	en, err := intl.ParseLocale("en")
	if err != nil {
		t.Fatal(err)
	}
	g := newNumber(t, en, intl.NumberFormatOptions{Notation: intl.NotationEngineering,
		SignDisplay: intl.SignExceptZero, MaximumFractionDigits: intl.Digits(0)})
	for v, want := range map[float64]string{0.4: "+400E-3", -0.4: "-400E-3"} {
		if got := g.Format(v); got != want {
			t.Errorf("engineering %v: %q, want %q", v, got, want)
		}
	}
}
