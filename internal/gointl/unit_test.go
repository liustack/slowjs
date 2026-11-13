//go:build gointl_all

package intl_test

import (
	"testing"

	intl "github.com/liustack/quickjs/internal/gointl"
)

// The expectations are node's, across several locales rather than one: English
// composes a divided unit to the same string either way, so a single locale
// would have passed while the rule was wrong.
func TestUnitStyle(t *testing.T) {
	for _, c := range []struct {
		loc, unit string
		display   intl.UnitDisplay
		v         float64
		want      string
	}{
		{"en", "meter", intl.UnitLong, 1, "1 meter"},
		{"en", "meter", intl.UnitLong, 16, "16 meters"},
		{"en", "meter", intl.UnitShort, 16, "16 m"},
		{"en", "meter", intl.UnitNarrow, 16, "16m"},
		// The wording follows the count, and German has two of them.
		{"de", "hour", intl.UnitLong, 1, "1 Stunde"},
		{"de", "hour", intl.UnitLong, 2, "2 Stunden"},
		// Polish has four, and five metres takes the genitive plural.
		{"pl", "meter", intl.UnitLong, 5, "5 metrów"},
		// A unit over another.
		{"en", "kilometer-per-hour", intl.UnitShort, 987, "987 km/h"},
		{"en", "kilometer-per-hour", intl.UnitLong, 987, "987 kilometers per hour"},
		{"en", "liter-per-kilometer", intl.UnitLong, 5, "5 liters per kilometer"},
		// Chinese has a wording of its own for this pair. Composing the parts
		// would give "987公里/小时", which is not what ICU answers.
		{"zh", "kilometer-per-hour", intl.UnitShort, 987, "987 km/h"},
	} {
		f := newFormat(t, c.loc, intl.NumberFormatOptions{
			Style: intl.StyleUnit, Unit: c.unit, UnitDisplay: c.display,
		})
		if got := f.Format(c.v); got != c.want {
			t.Errorf("%s %s %v = %q, want %q", c.loc, c.unit, c.v, got, c.want)
		}
	}
}

// A unit ECMA-402 does not sanction is refused when the formatter is built,
// rather than formatted as a bare number with the unit silently dropped.
func TestUnsanctionedUnitIsRefused(t *testing.T) {
	loc, err := intl.ParseLocale("en")
	if err != nil {
		t.Fatal(err)
	}
	for _, unit := range []string{"", "furlong", "meter-per-furlong", "furlong-per-hour"} {
		if _, err := intl.NewNumberFormat(loc, intl.NumberFormatOptions{
			Style: intl.StyleUnit, Unit: unit,
		}); err == nil {
			t.Errorf("the unit %q was accepted", unit)
		}
	}
	for _, unit := range []string{"meter", "kilometer-per-hour", "percent"} {
		if !intl.HasUnit(unit) {
			t.Errorf("%q is sanctioned but HasUnit says otherwise", unit)
		}
	}
	if n := len(intl.SanctionedUnits()); n != 45 {
		t.Errorf("ECMA-402 sanctions 45 units, this has %d", n)
	}
}

// The pieces of a measurement keep the number apart from its unit, and the
// unit's name is a part of its own with the space before it a literal, as
// ICU marks it and Node reports it. This once expected " meters" as one
// literal, which was go-intl's output rather than Node's.
func TestUnitParts(t *testing.T) {
	f := newFormat(t, "en", intl.NumberFormatOptions{
		Style: intl.StyleUnit, Unit: "meter", UnitDisplay: intl.UnitLong,
	})
	got := f.FormatToParts(16)
	want := []intl.Part{
		{Kind: intl.PartInteger, Value: "16"},
		{Kind: intl.PartLiteral, Value: " "},
		{Kind: intl.PartUnit, Value: "meters"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d parts, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("part %d is %v, want %v", i, got[i], want[i])
		}
	}
}

// An unset unit display is ECMA-402's default, short: the zero value of every
// option is the ECMA-402 default. It was long until a range sweep against
// Node caught Arabic kilometres written out in full.
func TestUnitDisplayDefaultsToShort(t *testing.T) {
	loc, err := intl.ParseLocale("en")
	if err != nil {
		t.Fatal(err)
	}
	f, err := intl.NewNumberFormat(loc, intl.NumberFormatOptions{Style: intl.StyleUnit, Unit: "kilometer"})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := f.Format(16), "16 km"; got != want {
		t.Errorf("an unset unit display writes %q, want %q", got, want)
	}
}
