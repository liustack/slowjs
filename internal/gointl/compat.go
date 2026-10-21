package intl

import (
	"slices"
	"strings"
)

// Which behavior to reproduce where Node and the standard disagree.
//
// Node, and so ICU4C, differs from ECMA-402 as written in a handful of
// observable places. A host that cares about conformance wants the standard; a
// host that cares about matching Node wants Node. Both are legitimate, so the
// choice is made when a formatter is built, and it is not ambient state: two
// formatters in one program may answer differently.
//
// The choice is made divergence by divergence, because a host may want Node
// in most places and the standard in a few: go-quickjs answers as Node does
// but for the Japanese twelve-hour clock, the Islamic eras, Temporal's
// formats, the keyword value "yes", the currencies DisplayNames names, a
// duration's fraction of a second, the separator a digital duration's
// minutes take, the fields a date pattern's literals seem to write, the
// deprecated Islamic calendars, when a resolved locale keeps its hour cycle,
// the zone a plain Temporal value is read in, the time zone names a
// DateTimeFormat takes and reports, a coptic year before the era, the days
// of the Chinese and Korean calendars, the region of a locale's hour
// cycles, the time zones Intl.supportedValuesOf lists, and two roundings
// that Temporal's normative fixes of May 2026 changed after the
// temporal_rs Node has, the patterns of Uzbek in Afghanistan, the digit
// options PluralRules reports, the unknown subtags a locale maximizes
// past, the pattern an accounting amount with no sign takes, the offsets a
// relative time names in words, the digits of a double rounded to an
// increment, which currencies a currency's own format is used for, the
// empty items of a list, the hour cycle keyword a time style is made with,
// what Intl.Locale answers for a keyword with no value, and a text
// direction no one knows. Standard and NodeICU are the two ends.
//
// The rule that keeps this honest is that every divergence is a named,
// documented entry with a test on both sides. It is a short list, not a
// licence to be bug-compatible with all of ICU.
//
// If the list ever passes about twenty entries, that is a sign the standard
// behavior is wrong somewhere and this is hiding it, rather than a sign that
// Node has drifted further. The Temporal entries are the exception to read
// past: each is a fix the standard made that Node's temporal_rs predates,
// and each goes when Node takes it.

// Compat is the divergences in which to answer as Node does rather than as
// the standard: a set of the flags below. The zero value is the standard in
// every one.
type Compat uint64

const (
	// NarrowSpace writes a plain space where ICU writes U+202F.
	NarrowSpace Compat = 1 << iota
	// TwoLetterTags leaves a tag of two lowercase letters as it came.
	TwoLetterTags
	// CollationKeyword gives a collation chosen by option to the resolved
	// locale.
	CollationKeyword
	// DurationOverflow sums a duration's fraction of a second as V8 does,
	// in an int64.
	DurationOverflow
	// TwelveHourCycle takes V8's twelve-hour clock for hour12: h11 in
	// Japan's region and h12 everywhere else.
	TwelveHourCycle
	// IslamicEras writes an Islamic year before the Hijrah as ICU4C does,
	// a negative year of the era after it.
	IslamicEras
	// TemporalFormats writes a Temporal value with V8's formats, which
	// count an era as a field asked for and keep no hour cycle.
	TemporalFormats
	// YesValues leaves "yes" out of any Unicode extension keyword, as ICU
	// does, rather than only where it stands for "true", and, with
	// TrueKeywords, answers "yes" for a keyword with no value where
	// Intl.Locale's getCalendars and the like give the keyword's value.
	YesValues
	// CurrencyNames names every currency CLDR has a name for, where the
	// standard names only those Intl.supportedValuesOf lists.
	CurrencyNames
	// DurationSeparator joins numeric minutes to whatever came before them
	// by the time separator, as V8 does, when the hours were left out.
	DurationSeparator
	// LiteralFields reads the fields DateTimeFormat's resolvedOptions
	// reports from its pattern's quoted literals as well as its fields.
	LiteralFields
	// IslamicFallback keeps the calendars islamic and islamic-rgsa for a
	// DateTimeFormat, where the standard settles them on islamic-civil,
	// and lists them among the calendars.
	IslamicFallback
	// HourCycleKeyword keeps a DateTimeFormat's -u-hc keyword where
	// hour12 or hourCycle was given only if the hour cycle it settled on is
	// the keyword's.
	HourCycleKeyword
	// PlainValueZone writes a plain Temporal value as the instant it names
	// in a DateTimeFormat's zone, rather than as its wall-clock fields.
	PlainValueZone
	// ZoneIdentifiers takes any time zone ICU knows for a DateTimeFormat,
	// and reports ICU's canonical name for it rather than the one given.
	ZoneIdentifiers
	// CopticEra writes a coptic year before the era as ICU4C does, in an
	// era no data names.
	CopticEra
	// ChineseAstronomy reckons a DateTimeFormat's Chinese calendar and
	// Dangi by ICU4C's astronomy, rather than as Temporal does.
	ChineseAstronomy
	// SubdivisionHourCycles passes over a locale's "-u-sd-" subdivision
	// for the region of Intl.Locale's getHourCycles.
	SubdivisionHourCycles
	// RegionZones lists only the time zones in a region, where the
	// standard lists UTC and Etc/GMT's as well.
	RegionZones
	// RoundingWindow starts the window a duration is rounded in at its
	// origin wherever none of the smallest unit is in it, as temporal_rs
	// 0.2.3 does, rather than only where the duration it starts with is
	// zero.
	RoundingWindow
	// RepeatedMidnight refuses to round a ZonedDateTime to the day where
	// its day ended before it, when the clock went back past midnight, as
	// temporal_rs 0.2.3 does.
	RepeatedMidnight
	// PatternCalendar writes the few locales ICU's pattern generator reads
	// another calendar's patterns for, when no calendar is asked for, with
	// that calendar's patterns: uz-AF's Persian dates in Gregorian ones.
	PatternCalendar
	// PluralRulesDigits reports only the significant digits among a
	// PluralRules' resolved options where both they and the decimals were
	// settled on, as V8 does.
	PluralRulesDigits
	// UnknownSubtags keeps a locale that names a language, a script and a
	// region as it is when it maximizes, though the script is "Zzzz" or the
	// region "ZZ", as ICU does.
	UnknownSubtags
	// AccountingNever writes an accounting amount with signDisplay "never"
	// by the plain currency pattern, as V8 asks ICU to.
	AccountingNever
	// RelativeEpsilon names in words, numeric "auto", any offset within
	// 0.005 of a whole one from -2 to 2, as ICU does.
	RelativeEpsilon
	// ApproximateIncrement rounds a double to an increment other than 1 or
	// 5 from the digits ICU's fast conversion gives it, which past about
	// sixteen are not the double's.
	ApproximateIncrement
	// CurrencyFormats writes every currency in the format of the currency
	// of the locale's region, where that has one of its own and the
	// currency asked for does not, and takes a currency's own pattern for
	// its name and accounting forms too, as ICU does.
	CurrencyFormats
	// EmptyListItems leaves an empty item out of a list's parts, as ICU
	// does.
	EmptyListItems
	// HourCycleStyles makes a time style with the pattern generator
	// wherever the locale has a -u-hc keyword, even one hour12 or
	// hourCycle overrode, as V8 gives ICU the locale's keyword.
	HourCycleStyles
	// IdenticalPrefix compares two strings from after the prefix they
	// share, backed up only as far as ICU's unsafe-backward set says, as
	// ICU's RuleBasedCollator does.
	IdenticalPrefix
	// HostAbbreviations guesses a host zone that nothing names from the C
	// library's abbreviations and offset, as ICU's uprv_tzname does.
	HostAbbreviations
	// DoubleRangeIdentity writes a range whose ends ICU's Formattable holds
	// equal, the same double, as one number marked approximate, as ICU's
	// NumberRangeFormatter does.
	DoubleRangeIdentity
	// TrueKeywords answers a Unicode extension keyword with no value, which
	// stands for "true", as "true" -- or, with YesValues, ICU's "yes" -- where
	// Intl.Locale's getCalendars and the like give the keyword's value.
	TrueKeywords
	// LeftToRightDirection answers getTextInfo's direction "ltr" for a script
	// whose direction is unknown, or a locale whose script cannot be found,
	// as ICU's uloc_isRightToLeft does.
	LeftToRightDirection
)

const (
	// Standard is ECMA-402 as written, in every divergence.
	Standard Compat = 0
	// NodeICU reproduces Node's and ICU4C's observable behavior in every
	// divergence.
	NodeICU = NarrowSpace | TwoLetterTags | CollationKeyword | DurationOverflow |
		TwelveHourCycle | IslamicEras | TemporalFormats | YesValues | CurrencyNames |
		DurationSeparator | LiteralFields | IslamicFallback | HourCycleKeyword | PlainValueZone |
		ZoneIdentifiers | CopticEra | ChineseAstronomy | SubdivisionHourCycles | RegionZones |
		RoundingWindow | RepeatedMidnight | PatternCalendar | PluralRulesDigits |
		UnknownSubtags | AccountingNever | RelativeEpsilon | ApproximateIncrement | CurrencyFormats |
		EmptyListItems | HourCycleStyles | IdenticalPrefix | HostAbbreviations | DoubleRangeIdentity |
		TrueKeywords | LeftToRightDirection
)

// Has reports whether Node's behavior is chosen for a divergence.
func (c Compat) Has(d Compat) bool { return c&d != 0 }

func (c Compat) String() string {
	switch c {
	case Standard:
		return "Standard"
	case NodeICU:
		return "NodeICU"
	}
	var names []string
	for _, d := range divergences {
		if c.Has(d.Flag) {
			names = append(names, d.Name)
		}
	}
	return strings.Join(names, "|")
}

// Divergences are the places the two profiles differ, listed so that the set
// can be read rather than discovered, with the flag that chooses Node's side
// of each. The list is the caller's own, a copy.
func Divergences() []Divergence { return slices.Clone(divergences) }

var divergences = []Divergence{
	{
		Name: "NarrowSpace", Flag: NarrowSpace,
		Area:     "DateTimeFormat",
		What:     "the space before a day period, and every other narrow no-break space",
		Standard: "CLDR's character, U+202F, as ICU writes it: \"3:04\u202fPM\"",
		Node: "a plain space: V8 replaces U+202F in everything it formats, " +
			"reverting ICU 72's change for the web's sake",
	},
	{
		Name: "TwoLetterTags", Flag: TwoLetterTags,
		Area: "Locale",
		What: "canonicalizing a tag that is two lowercase letters alone",
		Standard: "its aliases are replaced like any other tag's: \"bh\" is \"bho\" and " +
			"\"tw\" is \"ak\"",
		Node: "V8 answers it as it came, without consulting ICU, unless it is one of in, iw, ji, " +
			"jw, mo, sh, tl and no: \"bh\" stays \"bh\"",
	},
	{
		Name: "CollationKeyword", Flag: CollationKeyword,
		Area: "Collator",
		What: "a collation chosen by option, in the resolved locale",
		Standard: "the locale gains no keyword: ResolveLocale keeps a keyword only for " +
			"a value the locale itself asked for",
		Node: "unless the locale had a -u-co keyword it honoured, the locale gains one " +
			"for the option: new Intl.Collator(\"de\", {collation: \"eor\"}) resolves " +
			"to \"de-u-co-eor\"",
	},
	{
		Name: "DurationOverflow", Flag: DurationOverflow,
		Area: "DurationFormat",
		What: "a fraction of a second summed past 2**63 nanoseconds",
		Standard: "the sum is exact, as the proposal's arithmetic is: 1e20 nanoseconds in " +
			"digital style are 0:00:100000000000",
		Node: "V8 sums the smaller units in a double and converts it to an int64, which C++ " +
			"leaves undefined past 2**63 and x86-64 makes INT64_MIN: " +
			"0:00:9223372036.854775808, with a minus sign where it is the first unit written",
	},
	{
		Name: "TwelveHourCycle", Flag: TwelveHourCycle,
		Area: "DateTimeFormat",
		What: "the hour cycle hour12 chooses",
		Standard: "ECMA-402's [[HourCycle12]] and [[HourCycle24]] of the locale's data: the " +
			"first twelve- or twenty-four-hour cycle CLDR's time data allows for the locale's " +
			"region, so Japanese counts twelve hours from 0, h11: \"午前0:00\"",
		Node: "V8 takes h11 only for a locale whose region is Japan and h12 for every " +
			"other, so \"ja\" alone is h12: \"午前12:00\"",
	},
	{
		Name: "IslamicEras", Flag: IslamicEras,
		Area: "DateTimeFormat",
		What: "an Islamic year before the Hijrah",
		Standard: "CLDR's era before the Hijrah, the year counted back from it: 500 CE is " +
			"\"127 Before Hijrah\" (UTS #35, CLDR 48.2)",
		Node: "ICU4C's Islamic calendars have one era, and write the year before 1 AH " +
			"as a year of it: \"-126 Anno Hegirae\"",
	},
	{
		Name: "TemporalFormats", Flag: TemporalFormats,
		Area: "DateTimeFormat",
		What: "the format a Temporal value is written with",
		Standard: "the Temporal proposal's GetDateTimeFormat: an era alone is not a field " +
			"asked for, so the kind's defaults are written with it (\"5/2/2000 A\"), and the " +
			"resolved hour cycle is the value's too (hour12: false writes \"00:00:00\")",
		Node: "V8 counts an era among the fields asked for, writing \"A\" alone, and makes " +
			"the value's format with no hour cycle, writing \"12:00:00 AM\" for hour12: false",
	},
	{
		Name: "YesValues", Flag: YesValues,
		Area: "Locale",
		What: "a Unicode extension keyword whose value is \"yes\"",
		Standard: "UTS #35 replaces \"yes\" by its canonical \"true\", and leaves \"true\" out, " +
			"only for the keys whose BCP 47 data has that alias -- kb, kc, kh, kk and kn -- " +
			"so \"und-u-ka-yes\" stays as it is",
		Node: "ICU takes \"yes\" for any key as \"true\" and leaves it out: " +
			"\"und-u-ka-yes\" is \"und-u-ka\"; and where a key has no value, which stands " +
			"for \"true\", getCalendars and the like answer ICU's own spelling of it: " +
			"\"en-u-ca\" has [\"yes\"], where its calendar is \"true\"",
	},
	{
		Name: "CurrencyNames", Flag: CurrencyNames,
		Area: "DisplayNames",
		What: "a currency Intl.supportedValuesOf does not list",
		Standard: "ECMA-402's AvailableCurrencies are the currencies DisplayNames and NumberFormat " +
			"provide for, so DisplayNames names only those: the Andorran peseta, ADP, has no name " +
			"(test262's currencies-accepted-by-DisplayNames)",
		Node: "V8 lists ICU's current currencies but names every one CLDR does: " +
			"\"Andorran Peseta\"",
	},
	{
		Name: "DurationSeparator", Flag: DurationSeparator,
		Area: "DurationFormat",
		What: "numeric minutes when numeric hours are left out",
		Standard: "the proposal's PartitionDurationFormatPattern joins the minutes to the hours " +
			"only where the hours were written, so the minutes start a group of their own: " +
			"\"1 day, 01:02\" (test262's digital-style-with-hours-display-auto-with-zero-hour)",
		Node: "V8 joins them to whatever was written last by the time separator: " +
			"\"1 day:01:02\"",
	},
	{
		Name: "LiteralFields", Flag: LiteralFields,
		Area: "DateTimeFormat",
		What: "resolvedOptions of a pattern whose quoted literal holds a field's letter",
		Standard: "CreateDateTimeFormat keeps the fields of the format it chose, and resolvedOptions " +
			"reports those: Portuguese {month: \"long\", year: \"numeric\"}, written " +
			"\"MMMM 'de' y\", reports a month and a year",
		Node: "V8 looks for each field's letters anywhere in the pattern's text, and finds the d " +
			"of 'de': it reports day: \"numeric\" too",
	},
	{
		Name: "IslamicFallback", Flag: IslamicFallback,
		Area: "DateTimeFormat",
		What: "the calendars islamic and islamic-rgsa",
		Standard: "CreateDateTimeFormat settles either on a calendar AvailableCalendars lists, " +
			"implementation-defined; go-intl takes islamic-civil, and writes in it " +
			"(test262's constructor-options-calendar-islamic-fallback), and Intl.supportedValuesOf " +
			"lists neither (calendars-accepted-by-DateTimeFormat)",
		Node: "V8 keeps them, and ICU writes in its astronomical Islamic calendar: " +
			"resolvedOptions().calendar is \"islamic\", and both are listed",
	},
	{
		Name: "HourCycleKeyword", Flag: HourCycleKeyword,
		Area: "DateTimeFormat",
		What: "the -u-hc keyword of a resolved locale where hour12 or hourCycle was given",
		Standard: "ResolveLocale drops it for any hour12, which makes the option null, and for an " +
			"hourCycle that is another: \"en-u-hc-h23\" with {hour: \"numeric\", hour12: false} " +
			"is \"en\", and with {hourCycle: \"h23\"} alone \"en-u-hc-h23\"",
		Node: "V8 drops it where the hour cycle the formatter settled on, none where it writes " +
			"no hour, is another: the first is \"en-u-hc-h23\" and the second \"en\"",
	},
	{
		Name: "PlainValueZone", Flag: PlainValueZone,
		Area: "DateTimeFormat",
		What: "a plain Temporal value whose wall-clock time the formatter's zone skips",
		Standard: "the proposal writes a plain value's fields as they are, whatever the zone: " +
			"PlainDate 2011-12-30 in Pacific/Apia, a day Samoa skipped, is \"12/30/2011\" " +
			"(test262's PlainDate/prototype/toLocaleString/ignore-timezone)",
		Node: "V8 reads the value as the instant it names in the zone, with \"compatible\", " +
			"and writes that instant there: \"12/31/2011\"",
	},
	{
		Name: "ZoneIdentifiers", Flag: ZoneIdentifiers,
		Area: "DateTimeFormat",
		What: "the time zone names a DateTimeFormat takes, and the one resolvedOptions reports",
		Standard: "the IANA database's names, reported as given in its case: \"Asia/Calcutta\" is " +
			"\"Asia/Calcutta\", and \"ACT\" a RangeError (test262's timezone-not-canonicalized, " +
			"canonicalize-timezone and timezone-legacy-non-iana)",
		Node: "V8 takes any name ICU knows and reports ICU's canonical one: \"Asia/Kolkata\" is " +
			"\"Asia/Calcutta\", and \"ACT\" is \"Australia/Darwin\"",
	},
	{
		Name: "CopticEra", Flag: CopticEra,
		Area: "DateTimeFormat",
		What: "a coptic year before the era",
		Standard: "the calendar has one era, as Temporal and the era and month code proposal count " +
			"it, and a year before its first is 0 or below: 250 CE is \"-34 Anno Martyrum\" " +
			"(test262's formatToParts/era)",
		Node: "ICU4C writes an era before it, which neither ICU nor CLDR names: \"35\" and " +
			"no era",
	},
	{
		Name: "ChineseAstronomy", Flag: ChineseAstronomy,
		Area: "DateTimeFormat",
		What: "the days of the Chinese calendar and Dangi",
		Standard: "a formatter's days are Temporal's, which are ICU4X's: its tables from 1912 to " +
			"2102 and its mean-motion approximation outside them: ISO 2030-03-03 is the 29th " +
			"of the first month (test262's compare-to-temporal-lunisolar)",
		Node: "V8 formats with ICU4C, whose calendar computes the astronomy, and whose day is " +
			"not Temporal's there: the 30th",
	},
	{
		Name: "SubdivisionHourCycles", Flag: SubdivisionHourCycles,
		Area: "Locale",
		What: "getHourCycles of a locale with a \"-u-sd-\" subdivision and no region subtag",
		Standard: "RegionPreference takes the subdivision's region before the likely one: " +
			"\"en-u-sd-gbeng\" has Britain's [\"h23\"] (test262's getHourCycles/region-priority " +
			"and subdivision-region)",
		Node: "ICU's pattern generator passes over the subdivision, and V8 answers America's: " +
			"[\"h12\"]",
	},
	{
		Name: "RegionZones", Flag: RegionZones,
		Area: "supportedValuesOf",
		What: "the time zones in no region",
		Standard: "Intl.supportedValuesOf(\"timeZone\") lists UTC and Etc/GMT+1 to Etc/GMT-14 " +
			"among the rest (test262's timeZones-include-non-continental)",
		Node: "V8 lists only ICU's canonical zones that are in a region, 418 without them",
	},
	{
		Name: "RoundingWindow", Flag: RoundingWindow,
		Area: "Temporal",
		What: "rounding a difference up to a calendar unit smaller than its largest, where it has " +
			"none of that unit",
		Standard: "ECMA-262's ComputeNudgeWindow starts the window at the origin only where the " +
			"duration it starts with is zero (the Stage 4 text's \"Fix condition for bounding window " +
			"with zero start\"): 2012-01-01 until 2012-02-01 in weeks, largest months, ceil, is " +
			"P1M (test262's exact-multiple-of-larger-unit)",
		Node: "temporal_rs 0.2.3 starts it at the origin wherever none of the smallest unit is in " +
			"it, so the month counts as progress into a week: P1M1W",
	},
	{
		Name: "RepeatedMidnight", Flag: RepeatedMidnight,
		Area: "Temporal",
		What: "ZonedDateTime's round to a day, of an instant after the next day began, where the " +
			"clock went back past midnight",
		Standard: "ECMA-262's Temporal takes the instant as the day's last nanosecond (the Stage 4 " +
			"text's \"Fix assertion in ZonedDateTime.round\"): 2010-03-04T23:10+08:00 in " +
			"Antarctica/Casey, after 02:00 on the 5th went back to 23:00 on the 4th, rounds down " +
			"to the 4th's midnight and up to the 5th's first (test262's same-date-starts-twice)",
		Node: "temporal_rs 0.2.3 asserted the instant was before the next day's start: a " +
			"RangeError, \"ZonedDateTime is outside the expected day bounds\"",
	},
	{
		Name: "PatternCalendar", Flag: PatternCalendar,
		Area: "DateTimeFormat",
		What: "the patterns of uz-AF and uz-Arab-AF, which reckon in the Persian calendar, " +
			"when no calendar is asked for",
		Standard: "CLDR's: a date is written with the patterns of the calendar it is reckoned " +
			"in, and uz-Arab-AF inherits uz-Arab's Persian ones, so {year, month, day} is " +
			"\"AP ۱۳۴۸-۱۰-۱۱\"",
		Node: "ICU's pattern generator and interval formatter look the calendar up with " +
			"ures_getFunctionalEquivalent, which passes over uz-Arab, where the default is, for " +
			"the root's Gregorian: Persian dates in Gregorian patterns, \"۱۳۴۸-۱۰-۱۱\", unless " +
			"-u-ca or the calendar option names one",
	},
	{
		Name: "PluralRulesDigits", Flag: PluralRulesDigits,
		Area: "PluralRules",
		What: "the digit options resolvedOptions reports where both kinds were settled on: a " +
			"roundingPriority given, or compact notation asking for no digits",
		Standard: "SetNumberFormatDigitOptions sets the decimals and the significant digits, and " +
			"resolvedOptions reports both, as NumberFormat's does: {roundingPriority: " +
			"\"morePrecision\"} reports 0 to 3 decimals and 1 to 21 significant digits",
		Node: "V8 reads them back from ICU's number skeleton, which holds the significant " +
			"digits, and reports only those (js-plural-rules.cc)",
	},
	{
		Name: "UnknownSubtags", Flag: UnknownSubtags,
		Area: "Locale",
		What: "maximize() of a locale that names a language, a script and a region where the " +
			"script is the unknown \"Zzzz\" or the region the unknown \"ZZ\"",
		Standard: "UTS #35's Add Likely Subtags removes \"Zzzz\" and \"ZZ\" before it looks " +
			"the rest up: \"en-Zzzz-US\" maximizes to \"en-Latn-US\"",
		Node: "ICU's addLikelySubtags keeps a locale with all three subtags as it is: " +
			"\"en-Zzzz-US\" is itself",
	},
	{
		Name: "AccountingNever", Flag: AccountingNever,
		Area: "NumberFormat",
		What: "an accounting currency amount with signDisplay \"never\"",
		Standard: "GetNumberFormatPattern takes the accounting patterns whenever currencySign " +
			"is \"accounting\", and the positive one for any sign: Norwegian -1 euro is " +
			"\"€ 1,00\"",
		Node: "V8 asks ICU for UNUM_SIGN_NEVER, which has no accounting form, so the plain " +
			"currency pattern is used: \"1,00 €\"",
	},
	{
		Name: "RelativeEpsilon", Flag: RelativeEpsilon,
		Area: "RelativeTimeFormat",
		What: "which offsets numeric \"auto\" writes in words",
		Standard: "PartitionRelativeTimePattern uses a word only for the offset ToString " +
			"writes as its key, a whole number: 0.0001 days are \"in 0 days\"",
		Node: "ICU's formatRelativeImpl rounds a hundred times the offset to a whole number " +
			"and takes a word for any offset within 0.005 of -2, -1, 0, 1 or 2: 0.0001 days " +
			"are \"today\" and 1.004 days \"tomorrow\"",
	},
	{
		Name: "ApproximateIncrement", Flag: ApproximateIncrement,
		Area: "NumberFormat",
		What: "a double rounded to an increment other than 1 or 5",
		Standard: "the number is the double's own decimal, as for any rounding: " +
			"3.9967620239602476e27 to the nearest 0.02 is " +
			"\"3,996,762,023,960,247,600,000,000,000.00\"",
		Node: "ICU's DecimalQuantity reads a double by scaling it to an integer in floating " +
			"point, which is right to about sixteen digits, and roundToIncrement divides " +
			"those digits by the increment as decimals without correcting them, where its " +
			"other roundings do: \"3,996,762,023,960,248,000,000,000,000.00\". A range's " +
			"ends are the standard's: ICU's range formatter reads each through a Formattable, " +
			"which corrects the reading",
	},
	{
		Name: "CurrencyFormats", Flag: CurrencyFormats,
		Area: "NumberFormat",
		What: "the currencies a locale gives a pattern and separators of their own",
		Standard: "UTS #35's currency pattern, decimal and group override the locale's for " +
			"that currency, and its pattern is the standard currency pattern: Cape Verdean " +
			"escudos in Kabuverdianu are \"-1\u00a0234$50\u00a0\u200b\", while German " +
			"English writes dollars as it writes any currency, \"-1.234,50\u00a0US$\"",
		Node: "ICU's DecimalFormatSymbols sets the currency of the locale's region before the " +
			"one asked for and keeps its format where the other has none, so German English " +
			"writes every currency as English does euros in Europe, \"-US$1,234.50\"; and it " +
			"takes the pattern for the accounting form and the name as well, writing the " +
			"symbol and then the name, \"-US$1,234.50 US dollars\". Where a space comes " +
			"between such a symbol and the number, ICU then puts the name one character " +
			"early, \"-CVE\u00a01,234.5 Cape Verdean escudos0\", which is not reproduced",
	},
	{
		Name: "EmptyListItems", Flag: EmptyListItems,
		Area: "ListFormat",
		What: "an empty string among the items of formatToParts",
		Standard: "CreatePartsFromList writes each item as an element, an empty one too: " +
			"[\"a\", \"\"] is an element \"a\", a literal \" and \" and an element \"\"",
		Node: "ICU's FormattedList has no field for an empty span, so the item is left out " +
			"and the literals either side of it are one: [\"a\", \"\", \"b\"] is \"a\", " +
			"\", , and \", \"b\", and [\"\"] has no parts",
	},
	{
		Name: "HourCycleStyles", Flag: HourCycleStyles,
		Area: "DateTimeFormat",
		What: "a time style for a locale with a -u-hc keyword that hour12 or hourCycle overrode",
		Standard: "the keyword is not in the locale the formatter resolves, so the locale's own " +
			"time style is written in the hour cycle asked for: \"en-US-u-hc-h23\" with hour12 " +
			"is \"2:12:47 PM\", as test262's timedatestyle-en.js asks",
		Node: "V8 gives ICU the locale with its keyword, and SimpleDateFormat::construct makes " +
			"the time style afresh in the keyword's cycle, which V8 then turns to twelve hours " +
			"keeping its two digits: \"02:12:47 PM\"",
	},
	{
		Name: "IdenticalPrefix", Flag: IdenticalPrefix,
		Area: "Collator",
		What: "two strings that share a prefix",
		Standard: "UCA's comparison of the whole strings: with numeric, \"١٥\" (15) " +
			"sorts before \"١٠٠\" (100) in Arabic and \"1é\" before \"1𝟏\" " +
			"(1 before 11), French Canada's backward accents are read from the end of each whole " +
			"string, and with ignorePunctuation an ignorable after a shifted character is ignored: " +
			"\"-\" and \"-ं\" are equal",
		Node: "ICU's doCompare compares from after the shared prefix, backed up only before a " +
			"character in its unsafe-backward set or, with numeric, a digit. Its digit test reads " +
			"only the collation's own table, which in Arabic holds no Arabic-Indic digit, and one " +
			"UTF-16 unit, which is never a supplementary digit: 15 against 100 compares 5 with 00, " +
			"and \"1é\" sorts after \"1𝟏\". The backward accents are those after the " +
			"prefix, and the ignorable after a shifted \"-\" is weighed: \"-\" sorts first",
	},
	{
		Name: "HostAbbreviations", Flag: HostAbbreviations,
		Area: "TimeZone",
		What: "the host's zone where nothing names it: outside Windows, neither TZ, nor the zone " +
			"/etc/localtime links to, nor a zone file it is a copy of; on Windows, no zone Windows " +
			"names that CLDR maps",
		Standard: "the host's offset, an offset time zone identifier, as ECMA-262's " +
			"SystemTimeZoneIdentifier has it: \"+01:00\"",
		Node: "ICU's uprv_tzname guesses from the C library's abbreviations: whether daylight " +
			"saving was in force at the solstices of 2007, the two abbreviations and the standard " +
			"offset, looked up in putil.cpp's OFFSET_ZONE_MAPPINGS, \"EST\" and \"EDT\" at five " +
			"hours west being US/Eastern; else the standard abbreviation, a zone where ICU has " +
			"one at that offset and a fixed zone of that name where not. On Windows, Etc/Unknown",
	},
	{
		Name: "DoubleRangeIdentity", Flag: DoubleRangeIdentity,
		Area: "NumberFormat",
		What: "a range of two ends that are the same double but not the same number, a BigInt " +
			"or a numeric string among them",
		Standard: "FormatNumericRange writes one number marked approximate only where the two " +
			"ends are written alike: \"0.1\" and \"0.10000000000000000001\" to twenty places " +
			"are \"0.1–0.10000000000000000001\"",
		Node: "V8 hands ICU a BigInt or a numeric string as its decimal, and ICU's " +
			"Formattable holds one that does not fit in an int64 as the double nearest it as " +
			"well; formatFormattableRange asks Formattable's operator==, which compares those " +
			"doubles, whether the ends were equal before rounding, and where they were writes " +
			"the first end marked approximate: \"~0.1\", and 10n ** 30n against 10n ** 30n + 1n " +
			"\"~1,000,000,000,000,000,000,000,000,000,000\". A double never equals an integer, " +
			"nor two ends whose signs, exponents or affixes differ",
	},
	{
		Name: "TrueKeywords", Flag: TrueKeywords,
		Area: "Locale",
		What: "getCalendars, getCollations, getHourCycles and getNumberingSystems, and the " +
			"getters Intl.Locale has for them, for a Unicode extension keyword with no value",
		Standard: "the Intl Locale Info API's CalendarsOfLocale and the like answer the locale's " +
			"[[Calendar]], which a keyword with no value -- or \"true\", which canonicalizes away " +
			"-- leaves the empty string: \"en-u-co\" and \"en-u-co-true\" have collation \"\" and " +
			"getCollations() [\"\"] (test262's preferred-from-unicode-extension-true-empty)",
		Node: "V8 answers the getters \"true\", and the lists ICU's own spelling of it, " +
			"[\"yes\"] (with YesValues; [\"true\"] without)",
	},
	{
		Name: "LeftToRightDirection", Flag: LeftToRightDirection,
		Area: "Locale",
		What: "getTextInfo's direction where the script's direction is not known: Common, " +
			"Inherited, Braille, Unknown, a private use or unregistered script, or a locale whose " +
			"likely script cannot be found",
		Standard: "the Intl Locale Info API's TextDirectionOfLocale answers undefined: " +
			"\"und-Zyyy\", \"en-Brai\", \"und-Qaaq\" and \"tlh\" have no direction, and " +
			"\"ar-Zxxx\" none either (test262's script-metadata-rtl-is-unknown and its fellows)",
		Node: "ICU's uloc_isRightToLeft answers false for any script it does not mark right to " +
			"left, and V8 writes that as \"ltr\": all of these are \"ltr\"",
	},
}

// A Divergence is one named difference between the profiles.
type Divergence struct {
	// Name is the flag's name, and Flag the flag.
	Name string
	Flag Compat
	// Area is the service it belongs to, "DateTimeFormat".
	Area string
	// What is the behavior, in one line.
	What string
	// Standard and Node say what each side does.
	Standard string
	Node     string
}
