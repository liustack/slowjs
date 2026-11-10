//go:build gointl_all

package intl_test

import (
	"errors"
	"testing"

	intl "github.com/liustack/quickjs/internal/gointl"
)

// NumberFormat and PluralRules refuse what ECMA-402's
// SetNumberFormatDigitOptions and InitializeNumberFormat refuse, with
// ErrOption: digits out of their ranges, an increment not in its list, an
// ill-formed currency code or unit, and a value no option has. They had
// been accepted, and MaximumFractionDigits -1 then panicked in Format
// (ISSUES.md API-1).
func TestNumberOptionsRefused(t *testing.T) {
	en, err := intl.ParseLocale("en")
	if err != nil {
		t.Fatal(err)
	}
	bad := map[string]intl.NumberFormatOptions{
		"fraction -1":         {MaximumFractionDigits: intl.Digits(-1)},
		"fraction 101":        {MinimumFractionDigits: intl.Digits(101)},
		"significant 0":       {MinimumSignificantDigits: intl.Digits(0)},
		"significant 22":      {MaximumSignificantDigits: intl.Digits(22)},
		"integer 22":          {MinimumIntegerDigits: 22},
		"integer -1":          {MinimumIntegerDigits: -1},
		"increment 3":         {RoundingIncrement: 3, MinimumFractionDigits: intl.Digits(2), MaximumFractionDigits: intl.Digits(2)},
		"currency USDX":       {Style: intl.StyleCurrency, Currency: "USDX"},
		"currency U1D":        {Currency: "U1D"},
		"unit furlong":        {Unit: "furlong"},
		"unit style, no unit": {Style: intl.StyleUnit},
		"style 9":             {Style: intl.Style(9)},
		"sign display 9":      {SignDisplay: intl.SignDisplay(9)},
		"rounding mode 99":    {RoundingMode: intl.RoundingMode(99)},
		"grouping 9":          {UseGrouping: intl.Grouping(9)},
		"min above max":       {MinimumFractionDigits: intl.Digits(3), MaximumFractionDigits: intl.Digits(2)},
	}
	for name, opts := range bad {
		if _, err := intl.NewNumberFormat(en, opts); !errors.Is(err, intl.ErrOption) {
			t.Errorf("%s: %v, want ErrOption", name, err)
		}
	}
	good := map[string]intl.NumberFormatOptions{
		"fraction 100":   {MinimumFractionDigits: intl.Digits(100), MaximumFractionDigits: intl.Digits(100)},
		"significant 21": {MinimumSignificantDigits: intl.Digits(21), MaximumSignificantDigits: intl.Digits(21)},
		"integer 21":     {MinimumIntegerDigits: 21},
		"increment 5000": {RoundingIncrement: 5000, MinimumFractionDigits: intl.Digits(2), MaximumFractionDigits: intl.Digits(2)},
		"currency usd":   {Style: intl.StyleCurrency, Currency: "usd"},
	}
	for name, opts := range good {
		f, err := intl.NewNumberFormat(en, opts)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		f.Format(1.5)
	}
	for name, opts := range map[string]intl.PluralRulesOptions{
		"type 5":         {Type: intl.PluralType(5)},
		"significant 22": {MaximumSignificantDigits: intl.Digits(22)},
		"fraction -1":    {MaximumFractionDigits: intl.Digits(-1)},
		"notation 9":     {Notation: intl.Notation(9)},
	} {
		if _, err := intl.NewPluralRules(en, opts); !errors.Is(err, intl.ErrOption) {
			t.Errorf("plural rules %s: %v, want ErrOption", name, err)
		}
	}
}
