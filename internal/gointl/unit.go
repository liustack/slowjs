package intl

import (
	"fmt"
	"strings"

	"github.com/liustack/slowjs/internal/gointl/internal/unitdata"
)

// Measurements: "987 km/h", "1 Stunde", "2 Stunden".
//
// A unit is written by a pattern, one per plural category, so which wording is
// used depends on how the amount is written -- the same rule the currency
// names follow, and for the same reason.
//
// A unit may be one thing divided by another. Where the locale has a wording
// for the divisor on its own ("{0}/m") that is used; where it does not, the
// two are joined by the locale's compound pattern ("{0} per {1}"), which is
// why both are carried.

// UnitDisplay is how much room the unit's name takes.
type UnitDisplay int

const (
	// UnitShort writes "16 L", and is the default, as in ECMA-402.
	UnitShort UnitDisplay = iota
	// UnitLong writes "16 litres".
	UnitLong
	// UnitNarrow writes "16L".
	UnitNarrow
)

// SanctionedUnits are the measurements ECMA-402 allows, chosen because every
// language has a name for each of them.
func SanctionedUnits() []string {
	out := make([]string, 0, len(sanctionedUnits))
	for name := range sanctionedUnits {
		out = append(out, name)
	}
	sortStrings(out)
	return out
}

// HasUnit reports whether a name is one a number may be written in, including
// the compound forms: "kilometer-per-hour" is two sanctioned units.
func HasUnit(name string) bool {
	numerator, denominator, compound := strings.Cut(name, "-per-")
	if !sanctionedUnits[numerator] {
		return false
	}
	return !compound || sanctionedUnits[denominator]
}

var sanctionedUnits = map[string]bool{
	"acre": true, "bit": true, "byte": true, "celsius": true,
	"centimeter": true, "day": true, "degree": true, "fahrenheit": true,
	"fluid-ounce": true, "foot": true, "gallon": true, "gigabit": true,
	"gigabyte": true, "gram": true, "hectare": true, "hour": true,
	"inch": true, "kilobit": true, "kilobyte": true, "kilogram": true,
	"kilometer": true, "liter": true, "megabit": true, "megabyte": true,
	"meter": true, "microsecond": true, "mile": true, "mile-scandinavian": true,
	"milliliter": true, "millimeter": true, "millisecond": true, "minute": true,
	"month": true, "nanosecond": true, "ounce": true, "percent": true,
	"petabyte": true, "pound": true, "second": true, "stone": true,
	"terabit": true, "terabyte": true, "week": true, "yard": true, "year": true,
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

// loadUnits reads a locale's measurement patterns.
func loadUnits(src Source, loc Locale) (*unitdata.Locale, error) {
	chain := loc.Fallback()
	if f, err := NewFallbacker(src); err == nil {
		chain = f.ChainIn(treeUnit, loc.Data())
	}
	pool, err := openShared(src, MarkerUnitsShared, unitdata.Version)
	if err != nil {
		return nil, err
	}
	for _, d := range chain {
		b, err := src.Open(MarkerUnits, d)
		if err != nil {
			continue
		}
		l, err := unitdata.Decode(b, pool)
		if err != nil {
			return nil, fmt.Errorf("intl: the units for %s: %w", d, err)
		}
		return l, nil
	}
	return nil, fmt.Errorf("intl: no units for %s: %w", loc, ErrNotFound)
}

// applyUnit puts the amount into the unit's pattern.
func (f *NumberFormat) applyUnit(parts []Part, count string) []Part {
	pattern, ok := f.unitPattern(count)
	if !ok {
		return parts
	}
	// The pattern's text is the unit, as ICU marks it; the spaces round it
	// are trimmed off into literals when the parts are finished.
	return fillPatternAs(pattern, PartUnit, parts, nil)
}

// unitPattern builds the pattern for the formatter's unit, joining a divisor
// where the unit is one thing over another.
func (f *NumberFormat) unitPattern(count string) (string, bool) {
	width := f.units.Width(f.unitWidth)

	// A pair the locale has a wording of its own for is used as it stands.
	// Composing the parts would give "987公里/小时" in Chinese where ICU says
	// "987 km/h".
	if direct, ok := width.Unit(f.unit); ok {
		if pattern := direct.Pattern(count); pattern != "" {
			return pattern, true
		}
	}

	numerator, denominator, compound := strings.Cut(f.unit, "-per-")
	top, ok := width.Unit(numerator)
	if !ok {
		return "", false
	}
	pattern := top.Pattern(count)
	if pattern == "" || !compound {
		return pattern, pattern != ""
	}

	bottom, ok := width.Unit(denominator)
	if !ok {
		return "", false
	}
	// A locale that has a wording for the divisor on its own uses it, so that
	// "kilometer-per-hour" is "km/h" rather than "km per hour".
	if bottom.PerUnit != "" {
		return strings.ReplaceAll(bottom.PerUnit, "{0}", pattern), true
	}
	joined := width.Compound
	if joined == "" {
		joined = "{0} per {1}"
	}
	// The divisor is named without an amount, which is what the "one" wording
	// with its placeholder removed gives.
	name := strings.TrimSpace(strings.ReplaceAll(bottom.Pattern("one"), "{0}", ""))
	if name == "" {
		name = strings.TrimSpace(strings.ReplaceAll(bottom.Pattern(count), "{0}", ""))
	}
	joined = strings.ReplaceAll(joined, "{1}", name)
	return strings.ReplaceAll(joined, "{0}", pattern), true
}

// fillPattern substitutes a pattern's placeholders with pieces, keeping what
// came from the number apart from what came from the pattern.
func fillPattern(pattern string, first, second []Part) []Part {
	return fillPatternAs(pattern, PartLiteral, first, second)
}

// fillPatternAs is fillPattern with the pattern's own text marked as a kind
// of part.
func fillPatternAs(pattern string, kind PartKind, first, second []Part) []Part {
	var out []Part
	literal := func(s string) {
		if s != "" {
			out = append(out, Part{kind, s})
		}
	}
	rest := pattern
	for {
		at := strings.IndexByte(rest, '{')
		if at < 0 || at+2 >= len(rest) || rest[at+2] != '}' {
			break
		}
		literal(rest[:at])
		switch rest[at+1] {
		case '0':
			out = append(out, first...)
		case '1':
			if second != nil {
				out = append(out, second...)
			}
		default:
			literal(rest[at : at+3])
		}
		rest = rest[at+3:]
	}
	literal(rest)
	return out
}
