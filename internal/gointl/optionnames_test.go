//go:build gointl_all

package intl_test

import (
	"fmt"
	"testing"

	intl "github.com/liustack/quickjs/internal/gointl"
)

// Every option value prints as JavaScript spells it: the option's string in
// ECMA-402, "undefined" for an option not given, and the type and number
// for a value no constant names. go-quickjs's tables from JavaScript's
// strings to these values, which test262 exercises, agree with every one.
func TestOptionNames(t *testing.T) {
	for _, c := range []struct {
		v    fmt.Stringer
		want string
	}{
		{intl.UsageSort, "sort"}, {intl.UsageSearch, "search"},
		{intl.SensitivityDefault, "undefined"}, {intl.SensitivityBase, "base"},
		{intl.SensitivityAccent, "accent"}, {intl.SensitivityCase, "case"},
		{intl.SensitivityVariant, "variant"},
		{intl.CaseFirstDefault, "undefined"}, {intl.CaseFirstUpper, "upper"},
		{intl.CaseFirstLower, "lower"}, {intl.CaseFirstFalse, "false"},

		{intl.LengthNone, "undefined"}, {intl.LengthFull, "full"}, {intl.LengthLong, "long"},
		{intl.LengthMedium, "medium"}, {intl.LengthShort, "short"},
		{intl.WidthNone, "undefined"}, {intl.WidthNumeric, "numeric"}, {intl.Width2Digit, "2-digit"},
		{intl.WidthLong, "long"}, {intl.WidthShort, "short"}, {intl.WidthNarrow, "narrow"},
		{intl.ZoneNone, "undefined"}, {intl.ZoneShort, "short"}, {intl.ZoneLong, "long"},
		{intl.ZoneShortOffset, "shortOffset"}, {intl.ZoneLongOffset, "longOffset"},
		{intl.ZoneShortGeneric, "shortGeneric"}, {intl.ZoneLongGeneric, "longGeneric"},
		{intl.HourCycleAuto, "undefined"}, {intl.H11, "h11"}, {intl.H12, "h12"}, {intl.H23, "h23"},
		{intl.H24, "h24"},
		{intl.SourceShared, "shared"}, {intl.SourceStartRange, "startRange"},
		{intl.SourceEndRange, "endRange"},

		{intl.DisplayLanguage, "language"}, {intl.DisplayRegion, "region"},
		{intl.DisplayScript, "script"}, {intl.DisplayCurrency, "currency"},
		{intl.DisplayCalendar, "calendar"}, {intl.DisplayDateTimeField, "dateTimeField"},
		{intl.DisplayLong, "long"}, {intl.DisplayShort, "short"}, {intl.DisplayNarrow, "narrow"},
		{intl.FallbackCode, "code"}, {intl.FallbackNone, "none"},
		{intl.LanguageDialect, "dialect"}, {intl.LanguageStandard, "standard"},

		{intl.DurationShort, "short"}, {intl.DurationLong, "long"}, {intl.DurationNarrow, "narrow"},
		{intl.DurationDigital, "digital"},
		{intl.DurationUnitDefault, "undefined"}, {intl.DurationUnitLong, "long"},
		{intl.DurationUnitShort, "short"}, {intl.DurationUnitNarrow, "narrow"},
		{intl.DurationUnitNumeric, "numeric"}, {intl.DurationUnitTwoDigit, "2-digit"},
		{intl.DurationDisplayDefault, "undefined"}, {intl.DurationDisplayAuto, "auto"},
		{intl.DurationDisplayAlways, "always"},

		{intl.Conjunction, "conjunction"}, {intl.Disjunction, "disjunction"}, {intl.UnitList, "unit"},
		{intl.ListLong, "long"}, {intl.ListShort, "short"}, {intl.ListNarrow, "narrow"},
		{intl.BestFit, "best fit"}, {intl.Lookup, "lookup"},
		{intl.NFC, "NFC"}, {intl.NFD, "NFD"}, {intl.NFKC, "NFKC"}, {intl.NFKD, "NFKD"},

		{intl.StyleDecimal, "decimal"}, {intl.StylePercent, "percent"},
		{intl.StyleCurrency, "currency"}, {intl.StyleUnit, "unit"},
		{intl.CurrencySymbol, "symbol"}, {intl.CurrencyNarrowSymbol, "narrowSymbol"},
		{intl.CurrencyCode, "code"}, {intl.CurrencyName, "name"},
		{intl.SignAuto, "auto"}, {intl.SignAlways, "always"}, {intl.SignExceptZero, "exceptZero"},
		{intl.SignNever, "never"}, {intl.SignNegative, "negative"},
		{intl.GroupingDefault, "undefined"}, {intl.GroupingAuto, "auto"}, {intl.GroupingNever, "false"},
		{intl.GroupingAlways, "always"},
		{intl.GroupingMin2, "min2"},
		{intl.NotationStandard, "standard"}, {intl.NotationCompact, "compact"},
		{intl.NotationScientific, "scientific"}, {intl.NotationEngineering, "engineering"},
		{intl.CurrencySignStandard, "standard"}, {intl.CurrencySignAccounting, "accounting"},
		{intl.CompactShort, "short"}, {intl.CompactLong, "long"},
		{intl.HalfExpand, "halfExpand"}, {intl.Ceil, "ceil"}, {intl.Floor, "floor"},
		{intl.Expand, "expand"}, {intl.Trunc, "trunc"}, {intl.HalfCeil, "halfCeil"},
		{intl.HalfFloor, "halfFloor"}, {intl.HalfTrunc, "halfTrunc"}, {intl.HalfEven, "halfEven"},
		{intl.TrailingZeroAuto, "auto"}, {intl.TrailingZeroStripIfInteger, "stripIfInteger"},
		{intl.PriorityAuto, "auto"}, {intl.MorePrecision, "morePrecision"},
		{intl.LessPrecision, "lessPrecision"},
		{intl.UnitShort, "short"}, {intl.UnitLong, "long"}, {intl.UnitNarrow, "narrow"},

		{intl.Cardinal, "cardinal"}, {intl.Ordinal, "ordinal"},
		{intl.RelativeYear, "year"}, {intl.RelativeQuarter, "quarter"}, {intl.RelativeMonth, "month"},
		{intl.RelativeWeek, "week"}, {intl.RelativeDay, "day"}, {intl.RelativeHour, "hour"},
		{intl.RelativeMinute, "minute"}, {intl.RelativeSecond, "second"},
		{intl.RelativeAlways, "always"}, {intl.RelativeAuto, "auto"},
		{intl.RelativeLong, "long"}, {intl.RelativeShort, "short"}, {intl.RelativeNarrow, "narrow"},
		{intl.GranularityGrapheme, "grapheme"}, {intl.GranularityWord, "word"},
		{intl.GranularitySentence, "sentence"},

		// No constant names these.
		{intl.Style(9), "Style(9)"}, {intl.HourCycle(-1), "HourCycle(-1)"},
		{intl.RelativeTimeUnit(8), "RelativeTimeUnit(8)"},
	} {
		if got := c.v.String(); got != c.want {
			t.Errorf("%T %d: %q, want %q", c.v, c.v, got, c.want)
		}
	}
}

// The names read back where the package parses them.
func TestOptionNamesParse(t *testing.T) {
	for k := intl.DisplayLanguage; k <= intl.DisplayDateTimeField; k++ {
		if got, ok := intl.ParseDisplayKind(k.String()); !ok || got != k {
			t.Errorf("ParseDisplayKind(%q) = %v, %v", k, got, ok)
		}
	}
	for u := intl.RelativeYear; u <= intl.RelativeSecond; u++ {
		if got, ok := intl.ParseRelativeTimeUnit(u.String()); !ok || got != u {
			t.Errorf("ParseRelativeTimeUnit(%q) = %v, %v", u, got, ok)
		}
	}
}
