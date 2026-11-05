package intl

import (
	"strconv"

	"github.com/liustack/quickjs/internal/gointl/internal/reltimedata"
)

// The names of the option values, as JavaScript spells them: what String
// returns for each, which is the string the option takes in ECMA-402 and
// what resolvedOptions reports. A value that stands for an option not given
// is "undefined", as the option is in JavaScript; a value no constant names
// is its type and number, "Style(9)". Only the types whose values
// JavaScript sees have one.

// optionName is the name at v's place in names, or the type and number.
func optionName(typ string, v int, names ...string) string {
	if v >= 0 && v < len(names) {
		return names[v]
	}
	return typ + "(" + strconv.Itoa(v) + ")"
}

// Collator

func (u CollatorUsage) String() string { return optionName("CollatorUsage", int(u), "sort", "search") }

func (s Sensitivity) String() string {
	return optionName("Sensitivity", int(s), "undefined", "base", "accent", "case", "variant")
}

func (c CaseFirst) String() string {
	return optionName("CaseFirst", int(c), "undefined", "upper", "lower", "false")
}

// DateTimeFormat

func (l DateTimeLength) String() string {
	return optionName("DateTimeLength", int(l), "undefined", "full", "long", "medium", "short")
}

func (w FieldWidth) String() string {
	return optionName("FieldWidth", int(w), "undefined", "numeric", "2-digit", "long", "short", "narrow")
}

func (z ZoneStyle) String() string {
	return optionName("ZoneStyle", int(z), "undefined", "short", "long", "shortOffset", "longOffset",
		"shortGeneric", "longGeneric")
}

func (h HourCycle) String() string {
	return optionName("HourCycle", int(h), "undefined", "h11", "h12", "h23", "h24")
}

// String is the type of a formatToRange part's source.
func (s RangeSource) String() string {
	return optionName("RangeSource", int(s), "shared", "startRange", "endRange")
}

// DisplayNames

func (k DisplayKind) String() string {
	return optionName("DisplayKind", int(k), "language", "region", "script", "currency", "calendar",
		"dateTimeField")
}

func (s DisplayStyle) String() string {
	return optionName("DisplayStyle", int(s), "long", "short", "narrow")
}

func (f DisplayFallback) String() string {
	return optionName("DisplayFallback", int(f), "code", "none")
}

func (d LanguageDisplay) String() string {
	return optionName("LanguageDisplay", int(d), "dialect", "standard")
}

// DurationFormat

func (s DurationStyle) String() string {
	return optionName("DurationStyle", int(s), "short", "long", "narrow", "digital")
}

func (s DurationUnitStyle) String() string {
	return optionName("DurationUnitStyle", int(s), "undefined", "long", "short", "narrow", "numeric",
		"2-digit")
}

func (d DurationDisplay) String() string {
	return optionName("DurationDisplay", int(d), "undefined", "auto", "always")
}

// ListFormat

func (t ListType) String() string {
	return optionName("ListType", int(t), "conjunction", "disjunction", "unit")
}

func (s ListStyle) String() string { return optionName("ListStyle", int(s), "long", "short", "narrow") }

// Locale negotiation and normalization

// String is the localeMatcher option's value.
func (k MatcherKind) String() string { return optionName("MatcherKind", int(k), "best fit", "lookup") }

// String is String.prototype.normalize's name for the form.
func (f NormalizationForm) String() string {
	return optionName("NormalizationForm", int(f), "NFC", "NFD", "NFKC", "NFKD")
}

// NumberFormat

func (s Style) String() string {
	return optionName("Style", int(s), "decimal", "percent", "currency", "unit")
}

func (d CurrencyDisplay) String() string {
	return optionName("CurrencyDisplay", int(d), "symbol", "narrowSymbol", "code", "name")
}

func (s SignDisplay) String() string {
	return optionName("SignDisplay", int(s), "auto", "always", "exceptZero", "never", "negative")
}

// String is the useGrouping option's value, "false" for GroupingNever,
// which the option takes as false.
func (g Grouping) String() string {
	return optionName("Grouping", int(g), "undefined", "auto", "false", "always", "min2")
}

func (n Notation) String() string {
	return optionName("Notation", int(n), "standard", "compact", "scientific", "engineering")
}

func (s CurrencySign) String() string {
	return optionName("CurrencySign", int(s), "standard", "accounting")
}

func (d CompactDisplay) String() string {
	return optionName("CompactDisplay", int(d), "short", "long")
}

func (m RoundingMode) String() string {
	return optionName("RoundingMode", int(m), "halfExpand", "ceil", "floor", "expand", "trunc",
		"halfCeil", "halfFloor", "halfTrunc", "halfEven")
}

func (d TrailingZeroDisplay) String() string {
	return optionName("TrailingZeroDisplay", int(d), "auto", "stripIfInteger")
}

func (p RoundingPriority) String() string {
	return optionName("RoundingPriority", int(p), "auto", "morePrecision", "lessPrecision")
}

func (d UnitDisplay) String() string {
	return optionName("UnitDisplay", int(d), "short", "long", "narrow")
}

// PluralRules and RelativeTimeFormat

func (t PluralType) String() string { return optionName("PluralType", int(t), "cardinal", "ordinal") }

// String is the unit's singular name, "day", as format's unit argument
// takes it.
func (u RelativeTimeUnit) String() string {
	return optionName("RelativeTimeUnit", int(u), reltimedata.Names[:]...)
}

func (n RelativeTimeNumeric) String() string {
	return optionName("RelativeTimeNumeric", int(n), "always", "auto")
}

func (s RelativeTimeStyle) String() string {
	return optionName("RelativeTimeStyle", int(s), "long", "short", "narrow")
}

// Segmenter

func (g Granularity) String() string {
	return optionName("Granularity", int(g), "grapheme", "word", "sentence")
}
