//go:build gointl_all

package intl_test

import (
	"reflect"
	"testing"

	intl "github.com/liustack/quickjs/internal/gointl"
)

// A percentage in compact notation is written as the unit percent, by the
// unit's short pattern, its sign a unit: V8 asks ICU for a percentage as
// that unit, and ICU writes a unit in compact notation with the unit's
// pattern (number_formatimpl.cpp isCldrUnit). go-intl had used the percent
// pattern: Turkish "-%1,3 B" where Node writes "%-1,3 B". Node's answers,
// parts and all (ISSUES.md NF-5).
func TestCompactPercent(t *testing.T) {
	for _, c := range []struct {
		tag  string
		v    float64
		opts intl.NumberFormatOptions
		want []intl.Part
	}{
		{"ar", -12.5, intl.NumberFormatOptions{Style: intl.StylePercent, Notation: intl.NotationCompact}, []intl.Part{{intl.PartLiteral, "‎"}, {intl.PartMinusSign, "-"}, {intl.PartInteger, "1"}, {intl.PartDecimal, "."}, {intl.PartFraction, "3"}, {intl.PartLiteral, " "}, {intl.PartCompact, "ألف"}, {intl.PartUnit, "٪"}}},
		{"tr", -12.5, intl.NumberFormatOptions{Style: intl.StylePercent, Notation: intl.NotationCompact}, []intl.Part{{intl.PartUnit, "%"}, {intl.PartMinusSign, "-"}, {intl.PartInteger, "1"}, {intl.PartDecimal, ","}, {intl.PartFraction, "3"}, {intl.PartLiteral, " "}, {intl.PartCompact, "B"}}},
		{"en", 12.5, intl.NumberFormatOptions{Style: intl.StylePercent, Notation: intl.NotationCompact}, []intl.Part{{intl.PartInteger, "1"}, {intl.PartDecimal, "."}, {intl.PartFraction, "3"}, {intl.PartCompact, "K"}, {intl.PartUnit, "%"}}},
		{"de", 12.5, intl.NumberFormatOptions{Style: intl.StylePercent, Notation: intl.NotationCompact}, []intl.Part{{intl.PartInteger, "1250"}, {intl.PartLiteral, " "}, {intl.PartUnit, "%"}}},
		{"fr", 0.5, intl.NumberFormatOptions{Style: intl.StylePercent, Notation: intl.NotationCompact}, []intl.Part{{intl.PartInteger, "50"}, {intl.PartLiteral, " "}, {intl.PartUnit, "%"}}},
		{"en", -12.5, intl.NumberFormatOptions{Style: intl.StylePercent, Notation: intl.NotationCompact, CompactDisplay: intl.CompactLong}, []intl.Part{{intl.PartMinusSign, "-"}, {intl.PartInteger, "1"}, {intl.PartDecimal, "."}, {intl.PartFraction, "3"}, {intl.PartLiteral, " "}, {intl.PartCompact, "thousand"}, {intl.PartUnit, "%"}}},
		{"ja", 123.45, intl.NumberFormatOptions{Style: intl.StylePercent, Notation: intl.NotationCompact}, []intl.Part{{intl.PartInteger, "1"}, {intl.PartDecimal, "."}, {intl.PartFraction, "2"}, {intl.PartCompact, "万"}, {intl.PartUnit, "%"}}},
		{"en", 0.123, intl.NumberFormatOptions{Style: intl.StylePercent, Notation: intl.NotationCompact}, []intl.Part{{intl.PartInteger, "12"}, {intl.PartUnit, "%"}}},
		{"he", 12.5, intl.NumberFormatOptions{Style: intl.StylePercent, Notation: intl.NotationCompact}, []intl.Part{{intl.PartInteger, "1"}, {intl.PartDecimal, "."}, {intl.PartFraction, "3"}, {intl.PartCompact, "K"}, {intl.PartLiteral, "‏"}, {intl.PartUnit, "%"}}},
		{"en", 12.5, intl.NumberFormatOptions{Style: intl.StylePercent, Notation: intl.NotationCompact, SignDisplay: intl.SignAlways}, []intl.Part{{intl.PartPlusSign, "+"}, {intl.PartInteger, "1"}, {intl.PartDecimal, "."}, {intl.PartFraction, "3"}, {intl.PartCompact, "K"}, {intl.PartUnit, "%"}}},
	} {
		loc, err := intl.ParseLocale(c.tag)
		if err != nil {
			t.Fatal(err)
		}
		if got := newNumber(t, loc, c.opts).FormatToParts(c.v); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s %v %+v:\n got  %q\n want %q", c.tag, c.v, c.opts, got, c.want)
		}
	}
}
