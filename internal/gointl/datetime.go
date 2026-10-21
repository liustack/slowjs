package intl

import (
	"bytes"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/liustack/quickjs/internal/gointl/internal/datedata"
	"github.com/liustack/quickjs/internal/gointl/internal/numdata"
)

// Intl.DateTimeFormat.
//
// A date is asked for two ways and they do not mix. A caller may ask for a
// whole date or time at a length -- full, long, medium, short -- and get the
// locale's own pattern for it. Or a caller may name the fields it wants and
// let the locale decide how to arrange them, which is UTS #35's skeleton
// matching. ECMA-402 forbids mixing the two, and so does this.
//
// Only the Gregorian calendar is written so far.

// DateTimeLength is how much of a date or time is written.
type DateTimeLength int

const (
	// LengthNone means the caller did not ask for a whole date or time at a
	// length, and named the fields instead.
	LengthNone DateTimeLength = iota
	LengthFull
	LengthLong
	LengthMedium
	LengthShort
)

// FieldWidth is how one named field is written.
type FieldWidth int

const (
	// WidthNone means the field was not asked for.
	WidthNone FieldWidth = iota
	// WidthNumeric is "1", and Width2Digit "01".
	WidthNumeric
	Width2Digit
	// WidthLong is "January", Short "Jan" and Narrow "J".
	WidthLong
	WidthShort
	WidthNarrow
)

// ZoneStyle is how a time zone is named, ECMA-402's timeZoneName.
type ZoneStyle int

const (
	ZoneNone ZoneStyle = iota
	// ZoneShort and ZoneLong are the specific names, "EST" and "Eastern
	// Standard Time", which say which half of the year it is.
	ZoneShort
	ZoneLong
	// ZoneShortOffset and ZoneLongOffset are the distance from UTC,
	// "GMT-5" and "GMT-05:00".
	ZoneShortOffset
	ZoneLongOffset
	// ZoneShortGeneric and ZoneLongGeneric are the names whatever the
	// season, "ET" and "Eastern Time".
	ZoneShortGeneric
	ZoneLongGeneric
)

// HourCycle is how the hours are counted.
type HourCycle int

const (
	// HourCycleAuto takes the locale's own, and is the default.
	HourCycleAuto HourCycle = iota
	// H11 counts 0 to 11, H12 1 to 12, H23 0 to 23 and H24 1 to 24.
	H11
	H12
	H23
	H24
)

// DateTimeComponents names a group of fields, for ECMA-402's "required" and
// "defaults": which fields a formatter must be asked for before it stops
// supplying its own, and which it supplies.
type DateTimeComponents int

const (
	// ComponentsUnset takes Intl.DateTimeFormat's: any field is enough, and
	// the date is the default.
	ComponentsUnset DateTimeComponents = iota
	// ComponentsDate is the weekday, year, month and day.
	ComponentsDate
	// ComponentsTime is the day period, hour, minute, second and fraction.
	ComponentsTime
	// ComponentsAny is either, and is only something required.
	ComponentsAny
	// ComponentsAll is both, and is only a default.
	ComponentsAll
)

// DateTimeFormatOptions mirrors the option bag of Intl.DateTimeFormat. Its
// zero value asks for nothing, which ECMA-402 answers with the year, month and
// day.
type DateTimeFormatOptions struct {
	// DateStyle and TimeStyle ask for a whole date or time at a length. They
	// cannot be combined with the fields below.
	DateStyle DateTimeLength
	TimeStyle DateTimeLength

	Weekday      FieldWidth
	Era          FieldWidth
	Year         FieldWidth
	Month        FieldWidth
	Day          FieldWidth
	Hour         FieldWidth
	Minute       FieldWidth
	Second       FieldWidth
	TimeZoneName ZoneStyle

	// DayPeriod names the part of the day, "in the afternoon", at a width:
	// short, long or narrow.
	DayPeriod FieldWidth
	// FractionalSecondDigits writes one to three digits of the second's
	// fraction. Zero writes none.
	FractionalSecondDigits int

	// TimeZone is the zone to write the instant in. Empty means UTC.
	TimeZone string
	// HourCycle chooses how the hours are counted, and Hour12 overrides it
	// when set.
	HourCycle HourCycle
	Hour12    *bool

	// Calendar is the calendar to reckon in. Empty means the locale's own,
	// which so far is always the Gregorian one.
	Calendar string

	// NumberingSystem names the digits to write, as for NumberFormat.
	NumberingSystem string

	// Required and Defaults are what the legacy methods differ in, and are
	// the arguments ECMA-402 passes to CreateDateTimeFormat:
	//
	//	Intl.DateTimeFormat            any   date   (the zero values)
	//	Date.prototype.toLocaleString  any   all
	//	toLocaleDateString             date  date
	//	toLocaleTimeString             time  time
	Required DateTimeComponents
	Defaults DateTimeComponents

	// ToLocaleStringTimeZone says TimeZone is a Temporal.ZonedDateTime's,
	// which its toLocaleString formats in, rather than one asked for. An
	// instant written for it with no field asked for then has a zone name
	// (see ForTemporal).
	ToLocaleStringTimeZone bool

	Compat Compat
}

// Bool returns a pointer to v, for the options where not saying anything is
// different from saying false.
func Bool(v bool) *bool { return &v }

// A DateTimeFormat writes instants in one locale. It never changes after it is
// built and is safe for any number of goroutines to share.
type DateTimeFormat struct {
	// src and asked are where the formatter was built from and the locale
	// asked for, which ForTemporal builds from again.
	src    Source
	asked  Locale
	locale Locale
	// reported is the locale resolvedOptions reports: locale, less an hour
	// cycle an option overrode.
	reported Locale
	opts     DateTimeFormatOptions
	// explicit are the fields the options named, before any defaults, as
	// the skeleton letters that write them.
	explicit string
	// patternText is the pattern as chosen, before it was taken apart.
	patternText string

	data     *datedata.Locale
	calendar *datedata.Calendar
	// patterns is the calendar a pattern generator and the interval
	// formats are read from: calendar, but for PatternCalendar.
	patterns *datedata.Calendar
	system   CalendarSystem
	numbers  *numberDigits

	tz *timeZone
	// zoneName is the zone as resolvedOptions reports it, and zoneID the
	// identifier its names are found by, empty for a zone that has none.
	zoneName string
	zoneID   string

	// pattern is the pattern this formatter writes, already taken apart.
	pattern datePattern
	// ranges writes a range of two moments, as ICU's interval formatter.
	ranges *rangeFormat
	// week is the region's week conventions, read when the pattern has a
	// week-based year.
	week weekRules
	// calendarWeek are the week conventions of the formatter's calendar,
	// which ICU's Chinese astronomy reckons the week of the year with
	// (Check).
	calendarWeek weekRules
	// rules are the tables the calendar reckons with, read when it is the
	// one in use.
	rules calendarRules
	// hourCycle is the cycle resolvedOptions reports, unset unless an hour
	// or a time style was asked for, and clock the cycle the options and the
	// locale settled on whether or not an hour was asked for, which a
	// Temporal value's format keeps (see ForTemporal).
	hourCycle HourCycle
	clock     HourCycle
	decimal   string
	minus     string
	// overrides are the numbering systems some fields of a style pattern
	// are written in, by pattern letter; nil for most.
	overrides map[byte]string
	// systems are the numeric numbering systems, read when an override
	// names one.
	systems []numdata.NumberingSystem
	// rbnf are the rule-based numbering systems overrides name, by name.
	rbnf map[string]rbnfSystem
	// zones are the locale's zone names and zone the formatter's zone as
	// they see it.
	zones *zoneNames
	zone  zoneInfo
}

// numberDigits is the little a date formatter needs from the number data: the
// digits to write, which are not the ASCII ones in every locale.
type numberDigits struct {
	system string
	table  []rune
}

// NewDateTimeFormat builds a formatter from the data built into the package.
//
// It works in loc as it is given. Resolve a requested locale among the
// service's available locales first, with a LocaleMatcher, to answer as
// ECMA-402 and Node do: "az-Arab" resolves to "az".
func NewDateTimeFormat(loc Locale, opts DateTimeFormatOptions) (*DateTimeFormat, error) {
	return NewDateTimeFormatFrom(Embedded, loc, opts)
}

// NewDateTimeFormatFrom builds a formatter from a source of the caller's own.
func NewDateTimeFormatFrom(src Source, loc Locale, opts DateTimeFormatOptions) (*DateTimeFormat, error) {
	if err := opts.check(); err != nil {
		return nil, err
	}
	if (opts.DateStyle != LengthNone || opts.TimeStyle != LengthNone) && opts.hasFields() {
		return nil, fmt.Errorf("intl: a date style cannot be combined with named fields")
	}
	explicit := opts.explicitLetters()
	if err := opts.applyDefaults(); err != nil {
		return nil, err
	}
	data, err := loadDates(src, loc)
	if err != nil {
		return nil, err
	}
	system, keep, err := chooseCalendar(src, loc, opts.Calendar)
	if err != nil {
		return nil, err
	}
	cal, ok, err := qjsCalendar(src, data, string(system))
	if err != nil {
		return nil, fmt.Errorf("intl: the dates for %s: %w", loc, err)
	}
	if !ok {
		// A locale with no data for the calendar its region uses falls back to
		// the Gregorian one rather than to nothing.
		if cal, ok, err = qjsCalendar(src, data, string(Gregory)); err != nil {
			return nil, fmt.Errorf("intl: the dates for %s: %w", loc, err)
		}
		if !ok {
			return nil, fmt.Errorf("intl: %s has no calendar data: %w", loc, ErrNotFound)
		}
		system, keep = Gregory, ""
	}
	if (system == Islamic || system == IslamicRGSA) && !opts.Compat.Has(IslamicFallback) {
		// CreateDateTimeFormat settles a calendar ECMA-402 has deprecated
		// on one AvailableCalendars lists, the tabular civil one; the
		// locale keeps the keyword it was asked with.
		system = IslamicCivil
		if cal, ok, err = qjsCalendar(src, data, string(system)); err != nil || !ok {
			return nil, fmt.Errorf("intl: the dates for %s: no %s calendar: %w", loc, system, ErrNotFound)
		}
	}

	// Of the Unicode extension, DateTimeFormat uses the calendar, the hour
	// cycle and the numbering system.
	f := &DateTimeFormat{src: src, asked: loc, explicit: explicit,
		locale: loc.onlyKeywords("hc", "nu").withKeyword("ca", keep), opts: opts, data: data, calendar: cal,
		patterns: cal, system: system}
	if opts.Compat.Has(PatternCalendar) && !calendarAsked(loc, opts.Calendar) {
		if name := patternCalendarOf(src, loc); name != "" && name != string(system) {
			if pc, ok, err := qjsCalendar(src, data, name); err != nil {
				return nil, fmt.Errorf("intl: the dates for %s: %w", loc, err)
			} else if ok {
				f.patterns = pc
			}
		}
	}
	if system == Japanese {
		if f.rules.eras, err = loadJapaneseEras(src); err != nil {
			return nil, err
		}
	}
	if system == IslamicUmmAlQura {
		if f.rules.ummAlQura, err = loadUmmAlQura(src); err != nil {
			return nil, err
		}
	}
	if (system == Chinese || system == Dangi) && !opts.Compat.Has(ChineseAstronomy) {
		// The days Temporal reckons, which the standard asks a formatter
		// to agree with (ChineseAstronomy).
		if f.rules.eastAsian, err = loadEastAsian(src, system); err != nil {
			return nil, err
		}
	}
	if (system == Chinese || system == Dangi) && opts.Compat.Has(ChineseAstronomy) {
		f.calendarWeek = loadWeekRules(src, loc)
	}
	if f.tz, f.zoneName, f.zoneID, err = loadZone(src, opts.TimeZone, opts.Compat); err != nil {
		return nil, err
	}
	if numbers, err := loadNumbers(src, loc); err == nil {
		chosen, nu, err := selectNumberingSystem(src, numbers, loc, opts.NumberingSystem)
		if err != nil {
			return nil, err
		}
		f.numbers = &numberDigits{system: chosen.NumberingSystem, table: digitRunes(chosen.Digits)}
		f.locale = f.locale.withKeyword("nu", nu)
		f.decimal = chosen.Symbols.Decimal
		f.minus = chosen.Symbols.MinusSign
	}

	pattern, cycle, g, err := f.choosePattern(src, f.decimal)
	if err != nil {
		return nil, err
	}
	f.hourCycle = cycle
	f.reported = f.reportedLocale()
	f.patternText = pattern
	if f.gannen() {
		f.loadRules(src, "jpanyear")
	}
	f.pattern = compileDatePattern(pattern, f.patternOverrides(pattern, f.overrides))
	if f.ranges, err = f.newRangeFormat(src, g, staticSkeleton(pattern)); err != nil {
		return nil, err
	}
	if err := f.loadPatternData(); err != nil {
		return nil, err
	}
	return f, nil
}

// reportedLocale is the locale resolvedOptions reports. Its hour cycle
// keyword stays where it is one and no option overrode it, as ResolveLocale
// leaves it: hour12 overrides any, and hourCycle any other. V8 drops it
// where either option was given and the formatter's hour cycle, none where
// it writes no hour, is another (HourCycleKeyword).
func (f *DateTimeFormat) reportedLocale() Locale {
	kw, ok := f.locale.keywordValue("hc")
	if !ok {
		return f.locale
	}
	hc, valid := parseHourCycle(kw)
	o := &f.opts
	overridden := o.Hour12 != nil || o.HourCycle != HourCycleAuto && o.HourCycle != hc
	if o.Compat.Has(HourCycleKeyword) {
		overridden = (o.Hour12 != nil || o.HourCycle != HourCycleAuto) && f.hourCycle != hc
	}
	if !valid || overridden {
		return f.locale.withKeyword("hc", "")
	}
	return f.locale
}

// loadPatternData reads what the pattern's fields need beyond the calendar:
// the week conventions for a week-based year, and the zone names for a zone.
func (f *DateTimeFormat) loadPatternData() error {
	hasZone, hasWeek := false, false
	for _, fd := range f.pattern.fields {
		hasZone = hasZone || datePartKind(fd.letter) == PartTimeZoneName
		hasWeek = hasWeek || fd.letter == 'Y'
	}
	if hasWeek && f.week == (weekRules{}) {
		f.week = loadWeekRules(f.src, f.asked)
		if f.system == ISO8601 {
			// ISO8601Calendar's weeks start on Monday and need four days,
			// wherever the locale is.
			f.week = weekRules{firstDay: 1, minDays: 4}
		}
	}
	// Most patterns write no zone, and the zone names are much the largest
	// thing a formatter would otherwise read.
	if hasZone && f.zones == nil {
		var err error
		if f.zones, err = loadZoneNames(f.src, f.asked); err != nil {
			return err
		}
		f.zone = f.zones.zone(f.zoneID, f.tz)
	}
	return nil
}

// applyDefaults supplies the fields nobody asked for, as ECMA-402's
// CreateDateTimeFormat does with its required and defaults arguments.
func (o *DateTimeFormatOptions) applyDefaults() error {
	required, defaults := o.Required, o.Defaults
	if required == ComponentsUnset {
		required = ComponentsAny
	}
	if defaults == ComponentsUnset {
		defaults = ComponentsDate
	}
	if required == ComponentsAll || defaults == ComponentsAny {
		return fmt.Errorf("intl: %d cannot be required, nor %d a default", required, defaults)
	}
	// A method that writes only a date refuses a time style, and the other
	// way about.
	if required == ComponentsDate && o.TimeStyle != LengthNone {
		return fmt.Errorf("intl: a time style where only a date is written")
	}
	if required == ComponentsTime && o.DateStyle != LengthNone {
		return fmt.Errorf("intl: a date style where only a time is written")
	}

	need := o.DateStyle == LengthNone && o.TimeStyle == LengthNone
	if required == ComponentsDate || required == ComponentsAny {
		if o.Weekday != WidthNone || o.Year != WidthNone || o.Month != WidthNone || o.Day != WidthNone {
			need = false
		}
	}
	if required == ComponentsTime || required == ComponentsAny {
		if o.DayPeriod != WidthNone || o.Hour != WidthNone || o.Minute != WidthNone ||
			o.Second != WidthNone || o.FractionalSecondDigits != 0 {
			need = false
		}
	}
	if !need {
		return nil
	}
	if defaults == ComponentsDate || defaults == ComponentsAll {
		o.Year, o.Month, o.Day = WidthNumeric, WidthNumeric, WidthNumeric
	}
	if defaults == ComponentsTime || defaults == ComponentsAll {
		o.Hour, o.Minute, o.Second = WidthNumeric, WidthNumeric, WidthNumeric
	}
	return nil
}

// explicitLetters are the fields the options name, as V8's
// ExplicitComponentsSet spells them: every skeleton letter a field can be
// written with.
func (o *DateTimeFormatOptions) explicitLetters() string {
	var b strings.Builder
	for _, f := range []struct {
		set     bool
		letters string
	}{
		{o.Weekday != WidthNone, "Ec"},
		{o.Era != WidthNone, "G"},
		{o.Year != WidthNone, "y"},
		{o.Month != WidthNone, "ML"},
		{o.Day != WidthNone, "d"},
		{o.DayPeriod != WidthNone, "Bb"},
		{o.Hour != WidthNone, "HhKk"},
		{o.Minute != WidthNone, "m"},
		{o.Second != WidthNone, "s"},
		{o.TimeZoneName != ZoneNone, "zOv"},
		{o.FractionalSecondDigits != 0, "S"},
	} {
		if f.set {
			b.WriteString(f.letters)
		}
	}
	return b.String()
}

func (o *DateTimeFormatOptions) hasFields() bool {
	return o.Weekday != WidthNone || o.Era != WidthNone || o.Year != WidthNone ||
		o.Month != WidthNone || o.Day != WidthNone || o.Hour != WidthNone ||
		o.Minute != WidthNone || o.Second != WidthNone || o.TimeZoneName != ZoneNone ||
		o.DayPeriod != WidthNone || o.FractionalSecondDigits != 0
}

// calendarAsked reports whether a calendar was asked for, by the option or
// by the locale's -u-ca, in a form chooseCalendar takes: V8 then hands ICU a
// locale with the calendar keyword.
func calendarAsked(loc Locale, option string) bool {
	if option != "" && implemented[CalendarSystem(strings.ToLower(option))] {
		return true
	}
	keyword, _ := loc.Keyword("ca")
	return implemented[CalendarSystem(keyword)]
}

// patternCalendarOf is the calendar ICU's pattern generator reads the
// locale's patterns from when none is asked for, where that is not the one
// it reckons in (dategen's patternCalendars); "" for almost every locale.
func patternCalendarOf(src Source, loc Locale) string {
	qjsUseDates()
	b, err := src.Open(MarkerPatternCalendars, DataLocale{})
	if err != nil {
		return ""
	}
	tag := loc.Data().String()
	for len(b) > 0 {
		line := b
		if i := bytes.IndexByte(b, '\n'); i >= 0 {
			line, b = b[:i], b[i+1:]
		} else {
			b = nil
		}
		name, cal, ok := bytes.Cut(line, []byte(" "))
		if ok && string(name) == tag {
			return string(cal)
		}
	}
	return ""
}

func loadDates(src Source, loc Locale) (*datedata.Locale, error) {
	if c, ok := src.(*qjsCachedSource); ok {
		return c.dates.get(loc.Data(), func() (*datedata.Locale, error) { return loadDates(c.Source, loc) })
	}

	qjsUseDates()
	chain := loc.Fallback()
	if f, err := NewFallbacker(src); err == nil {
		chain = f.ChainIn(treeLocales, loc.Data())
		if _, err := src.Open(MarkerDates, loc.Data()); err == nil {
			chain = f.Chain(loc.Data())
		}
	}
	pool, err := openShared(src, MarkerDatesShared, datedata.Version)
	if err != nil {
		return nil, err
	}
	for _, d := range chain {
		b, err := src.Open(MarkerDates, d)
		if err != nil {
			continue
		}
		l, err := datedata.Decode(b, pool)
		if err != nil {
			return nil, fmt.Errorf("intl: the dates for %s: %w", d, err)
		}
		return l, nil
	}
	return nil, fmt.Errorf("intl: no date data for %s: %w", loc, ErrNotFound)
}

// ResolveTimeZone is the zone a DateTimeFormat asked for a timeZone settles
// on, as its resolvedOptions reports it, or an error for one it does not
// know, which ECMA-402 throws as a RangeError. An engine reads it where
// CreateDateTimeFormat reads the option, before the options after it.
//
// ECMA-402 takes the names of the IANA time zone database and reports one
// as given, in the database's case: "Asia/Calcutta" is itself, and "ACT",
// which only ICU knows, is no zone. V8 takes any name ICU knows and
// reports ICU's canonical one, "Asia/Calcutta" for "Asia/Kolkata"
// (ZoneIdentifiers). The empty name, which DateTimeFormatOptions takes as
// UTC, is here an option given as "", and no zone.
func ResolveTimeZone(src Source, name string, compat Compat) (string, error) {
	if name == "" {
		return "", fmt.Errorf("intl: %q is %w", name, errNoZone)
	}
	_, resolved, _, err := loadZone(src, name, compat)
	return resolved, err
}

// loadZone finds a time zone: a named one, or an offset from UTC. An empty
// name is UTC, which is what ECMA-402 falls back to when the host says
// nothing. It returns the zone, the name resolvedOptions reports, and the
// identifier the zone is named by.
func loadZone(src Source, name string, compat Compat) (*timeZone, string, string, error) {
	if name == "" {
		name = "UTC"
	}
	if seconds, resolved, ok := parseOffsetZone(name); ok {
		// V8 hands ICU an offset as a custom zone, "GMT+05:30", which has
		// no names and so is written as its offset. ICU spells a custom
		// zone of no offset "GMT", which is Etc/GMT and has the names of
		// Greenwich Mean Time.
		id := ""
		if seconds == 0 {
			id = "GMT"
		}
		return fixedZone(seconds), resolved, id, nil
	}
	given := ""
	if !compat.Has(ZoneIdentifiers) {
		var ok bool
		if given, ok = ianaZoneName(src, name); !ok {
			return nil, "", "", fmt.Errorf("intl: %q is %w", name, errNoZone)
		}
	}
	load := name
	if isUTCAlias(name) {
		load = "UTC"
	}
	z, err := loadTimeZone(src, load)
	if err != nil {
		return nil, "", "", err
	}
	if given != "" {
		return z, given, z.id, nil
	}
	return z, z.resolvedID(), z.id, nil
}

// ianaZoneName is a zone's name as the IANA time zone database spells it,
// read in any case; false for a name the database does not have. The names
// are Temporal's, which are the database's, backzone and links included, as
// of the version Temporal's provider pins, which may be older than ICU's
// zones: a name newer than it is refused.
func ianaZoneName(src Source, name string) (string, bool) {
	b, err := src.Open(MarkerTemporalZones, DataLocale{})
	if err != nil {
		return "", false
	}
	text := string(b)
	// The first line is the database's version.
	_, text, _ = strings.Cut(text, "\n")
	for text != "" {
		var line string
		line, text, _ = strings.Cut(text, "\n")
		zone, _, _ := strings.Cut(line, " ")
		if strings.EqualFold(zone, name) {
			return zone, true
		}
	}
	return "", false
}

// isUTCAlias reports whether a zone is one V8's CanonicalizeTimeZoneID
// makes UTC before ICU sees it, and so is written with UTC's names, as
// ECMA-262 makes UTC the primary identifier of GMT, Etc/GMT and Etc/UTC.
// The other links to Etc/GMT keep Greenwich Mean Time's.
func isUTCAlias(name string) bool {
	switch strings.ToUpper(name) {
	case "GMT", "ETC/UTC", "ETC/GMT", "ETC/UCT", "GMT0", "GMT+0", "GMT-0":
		return true
	}
	return false
}

// parseOffsetZone reads an offset time zone as ECMA-402 allows one: a sign
// and two digits of hours, then optionally two of minutes, with or without a
// colon -- "+05:30", "+0530", "-08". It returns the offset in seconds and
// the form resolvedOptions reports, "+05:30"; minus zero is "+00:00".
func parseOffsetZone(s string) (int, string, bool) {
	if len(s) < 3 || s[0] != '+' && s[0] != '-' {
		return 0, "", false
	}
	digits := s[1:]
	switch {
	case len(digits) == 2:
		digits += "00"
	case len(digits) == 5 && digits[2] == ':':
		digits = digits[:2] + digits[3:]
	case len(digits) != 4:
		return 0, "", false
	}
	for i := 0; i < 4; i++ {
		if digits[i] < '0' || digits[i] > '9' {
			return 0, "", false
		}
	}
	hours := int(digits[0]-'0')*10 + int(digits[1]-'0')
	minutes := int(digits[2]-'0')*10 + int(digits[3]-'0')
	if hours > 23 || minutes > 59 {
		return 0, "", false
	}
	seconds := hours*3600 + minutes*60
	sign := s[0]
	if seconds == 0 {
		sign = '+'
	} else if sign == '-' {
		seconds = -seconds
	}
	return seconds, string(sign) + digits[:2] + ":" + digits[2:], true
}

// quoteLiteral puts a literal back into a pattern, quoting the letters that
// would otherwise be read as fields.
func quoteLiteral(s string) string {
	if s == "" {
		return ""
	}
	needs := false
	for i := 0; i < len(s); i++ {
		if isPatternLetter(s[i]) || s[i] == '\'' {
			needs = true
			break
		}
	}
	if !needs {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

// digits writes ASCII digits in the locale's own, where they differ.
// number writes a numeric field, in the numbering system an override gives
// its letter or else the formatter's.
// signedNumber is number for a year that may be before the first: the
// extended year of 1 BC is 0, and of 2 BC -1.
func (f *DateTimeFormat) signedNumber(p *dateParts, letter byte, v, width int) string {
	if v < 0 {
		minus := f.minus
		if minus == "" {
			minus = "-"
		}
		return minus + f.number(p, letter, -v, width)
	}
	return f.number(p, letter, v, width)
}

// twoDigitYear is a year's last two digits, as ICU's
// zeroPaddingNumber(value, 2, 2) writes them: a number of at most two
// digits, so it keeps its sign. The Persian year -121 is "-21".
func (f *DateTimeFormat) twoDigitYear(p *dateParts, letter byte, v int) string {
	if v < 0 {
		minus := f.minus
		if minus == "" {
			minus = "-"
		}
		return minus + f.number(p, letter, -v%100, 2)
	}
	return f.number(p, letter, v%100, 2)
}

func (f *DateTimeFormat) number(p *dateParts, letter byte, v, width int) string {
	if system, ok := p.overrides[letter]; ok {
		if rules, ok := f.rbnf[system]; ok {
			// SimpleDateFormat writes a Hebrew year of this millennium
			// without its thousands: 5784 is תשפ״ד.
			if system == "hebr" && (letter == 'y' || letter == 'Y') && v > 5000 && v < 6000 {
				v -= 5000
			}
			return rules.rules.format(rules.set, int64(v))
		}
		for _, s := range f.systems {
			if s.Name == system {
				return mapDigits(pad(v, width), s.Digits)
			}
		}
	}
	return f.digits(pad(v, width))
}

// rbnfSystem is a rule-based numbering system and the rule set it starts at.
type rbnfSystem struct {
	rules *rbnfRules
	set   string
}

// The fields an override without a letter applies to, ICU's kDateFields and
// kTimeFields as pattern letters.
const (
	overrideDateLetters = "yMdDFwWYugLqQUr"
	overrideTimeLetters = "kHmsSKhA"
)

// parseNumberingOverride reads CLDR's override for a style pattern: either a
// system for every date or time field, or "letter=system" pairs separated by
// semicolons.
func parseNumberingOverride(into map[byte]string, s string, date bool) {
	if s == "" {
		return
	}
	for _, part := range strings.Split(s, ";") {
		letter, system, found := strings.Cut(part, "=")
		if !found {
			letters := overrideTimeLetters
			if date {
				letters = overrideDateLetters
			}
			for i := 0; i < len(letters); i++ {
				into[letters[i]] = part
			}
			continue
		}
		if letter != "" {
			into[letter[0]] = system
		}
	}
}

func (f *DateTimeFormat) digits(s string) string {
	if f.numbers == nil {
		return s
	}
	return mapDigitRunes(s, f.numbers.table)
}

// check refuses what InitializeDateTimeFormat refuses: a field written in a
// width its table does not give it, an hour with "long" or an era with
// "2-digit"; more than three fractional digits; and a value no option has.
func (o *DateTimeFormatOptions) check() error {
	words := []FieldWidth{WidthNone, WidthLong, WidthShort, WidthNarrow}
	numbers := []FieldWidth{WidthNone, WidthNumeric, Width2Digit}
	for _, f := range []struct {
		name   string
		width  FieldWidth
		allows []FieldWidth
	}{
		{"weekday", o.Weekday, words}, {"era", o.Era, words}, {"dayPeriod", o.DayPeriod, words},
		{"year", o.Year, numbers}, {"day", o.Day, numbers}, {"hour", o.Hour, numbers},
		{"minute", o.Minute, numbers}, {"second", o.Second, numbers},
		{"month", o.Month, append(append([]FieldWidth(nil), numbers...), words[1:]...)},
	} {
		if !slices.Contains(f.allows, f.width) {
			return fmt.Errorf("%w: %s is not a width of %s", ErrOption, f.width, f.name)
		}
	}
	if o.FractionalSecondDigits < 0 || o.FractionalSecondDigits > 3 {
		return fmt.Errorf("%w: %d fractional second digits, not 1 to 3", ErrOption, o.FractionalSecondDigits)
	}
	for _, v := range []fmt.Stringer{o.DateStyle, o.TimeStyle, o.TimeZoneName, o.HourCycle} {
		if !named(v) {
			return fmt.Errorf("%w: %s", ErrOption, v)
		}
	}
	if o.Required < ComponentsUnset || o.Required > ComponentsAll ||
		o.Defaults < ComponentsUnset || o.Defaults > ComponentsAll {
		return fmt.Errorf("%w: components %d and %d", ErrOption, o.Required, o.Defaults)
	}
	return nil
}

// ErrCalendarRange reports that a formatter's calendar cannot reckon an
// instant, where Node throws a TypeError.
var ErrCalendarRange = errors.New("intl: the calendar cannot reckon the date")

// Check reports whether the formatter can write an instant: with
// ChineseAstronomy, ICU's Chinese and Korean calendars cannot from about
// the year 70,400 or before about the year -99,997, where the winter
// solstices its astronomy finds no longer fall either side of a date it
// needs, and Node throws. What Format writes for such an instant is not to
// be relied on.
func (f *DateTimeFormat) Check(t time.Time) error {
	p := f.instant(t)
	if p.failed {
		return ErrCalendarRange
	}
	if p.yearStartFails == nil {
		return nil
	}
	// Calendar::computeWeekFields asks for the length of the year the date
	// is in, or of the one before where the date is in its last week, and
	// handleGetYearLength finds where both years start.
	firstDay := f.calendarWeek.firstDay + 1
	dayOfWeek := p.weekday + 1
	relDowJan1 := (dayOfWeek - p.dayOfYear + 7001 - firstDay) % 7
	woy := (p.dayOfYear - 1 + relDowJan1) / 7
	if 7-relDowJan1 >= f.calendarWeek.minDays {
		woy++
	}
	year := p.extYear
	if woy == 0 {
		year--
	}
	if p.yearStartFails(year+1) || p.yearStartFails(year) {
		return ErrCalendarRange
	}
	return nil
}

// Format writes an instant.
func (f *DateTimeFormat) Format(t time.Time) string {
	p := f.instant(t)
	dp := &f.pattern
	p.hasMinute, p.hasSecond, p.overrides = dp.hasMinute, dp.hasSecond, dp.overrides
	var b strings.Builder
	b.Grow(len(f.patternText) + 24)
	for _, fd := range dp.fields {
		if fd.letter == 0 {
			b.WriteString(fd.literal)
		} else {
			b.WriteString(f.writeField(fd, &p))
		}
	}
	s := b.String()
	if f.opts.Compat.Has(NarrowSpace) {
		s = strings.ReplaceAll(s, "\u202f", " ")
	}
	return s
}

// FormatToParts writes an instant as the pieces it is made of.
func (f *DateTimeFormat) FormatToParts(t time.Time) []Part {
	var out []Part
	for _, seg := range f.render(&f.pattern, f.instant(t)) {
		kind := PartLiteral
		if seg.letter != 0 {
			kind = datePartKind(seg.letter)
		}
		out = append(out, Part{kind, seg.value})
	}
	if f.opts.Compat.Has(NarrowSpace) {
		// V8 writes a plain space wherever ICU writes a narrow no-break
		// one, reverting ICU 72 for the web's sake (Replace202F).
		for i := range out {
			out[i].Value = strings.ReplaceAll(out[i].Value, "\u202f", " ")
		}
	}
	return out
}

// ResolvedDateTimeFormat is what a formatter settled on.
type ResolvedDateTimeFormat struct {
	Locale          string
	Calendar        string
	NumberingSystem string
	TimeZone        string
	// HourCycle is unset unless an hour or a time style was asked for, and
	// Hour12 is meaningful only where it is set: true for h11 and h12, as
	// ECMA-402 reports it beside the hour cycle.
	HourCycle HourCycle
	Hour12    bool

	// DateStyle and TimeStyle are the styles asked for. A formatter asked
	// for neither reports the fields below instead: those its pattern
	// writes, at the widths it writes them, which need not be the ones
	// asked for -- a Japanese short month is written "M月", and numeric.
	DateStyle, TimeStyle DateTimeLength

	Weekday                FieldWidth
	Era                    FieldWidth
	Year                   FieldWidth
	Month                  FieldWidth
	Day                    FieldWidth
	DayPeriod              FieldWidth
	Hour                   FieldWidth
	Minute                 FieldWidth
	Second                 FieldWidth
	FractionalSecondDigits int
	TimeZoneName           ZoneStyle
}

// ResolvedOptions returns what the formatter settled on.
func (f *DateTimeFormat) ResolvedOptions() ResolvedDateTimeFormat {
	system := "latn"
	if f.numbers != nil && f.numbers.system != "" {
		system = f.numbers.system
	}
	cycle := f.hourCycle
	r := ResolvedDateTimeFormat{
		Locale:          f.reported.String(),
		Calendar:        string(f.system),
		NumberingSystem: system,
		TimeZone:        f.zoneName,
		HourCycle:       cycle,
		Hour12:          cycle == H11 || cycle == H12,
		DateStyle:       f.opts.DateStyle,
		TimeStyle:       f.opts.TimeStyle,
	}
	if r.DateStyle == LengthNone && r.TimeStyle == LengthNone {
		f.resolvedFields(&r)
	}
	return r
}

// resolvedFieldRuns are the fields resolvedOptions reports, in its order,
// each with the letter runs that write it and the width each run is, as
// V8's pattern items list them: the first run a pattern holds decides.
var resolvedFieldRuns = [...]struct {
	into func(*ResolvedDateTimeFormat) *FieldWidth
	runs []string
	as   []FieldWidth
}{
	{func(r *ResolvedDateTimeFormat) *FieldWidth { return &r.Weekday },
		[]string{"EEEEE", "EEEE", "EEE", "ccccc", "cccc", "ccc"},
		[]FieldWidth{WidthNarrow, WidthLong, WidthShort, WidthNarrow, WidthLong, WidthShort}},
	{func(r *ResolvedDateTimeFormat) *FieldWidth { return &r.Era },
		[]string{"GGGGG", "GGGG", "GGG"},
		[]FieldWidth{WidthNarrow, WidthLong, WidthShort}},
	{func(r *ResolvedDateTimeFormat) *FieldWidth { return &r.Year },
		[]string{"yy", "y"},
		[]FieldWidth{Width2Digit, WidthNumeric}},
	{func(r *ResolvedDateTimeFormat) *FieldWidth { return &r.Month },
		[]string{"MMMMM", "MMMM", "MMM", "MM", "M", "LLLLL", "LLLL", "LLL", "LL", "L"},
		[]FieldWidth{WidthNarrow, WidthLong, WidthShort, Width2Digit, WidthNumeric,
			WidthNarrow, WidthLong, WidthShort, Width2Digit, WidthNumeric}},
	{func(r *ResolvedDateTimeFormat) *FieldWidth { return &r.Day },
		[]string{"dd", "d"},
		[]FieldWidth{Width2Digit, WidthNumeric}},
	{func(r *ResolvedDateTimeFormat) *FieldWidth { return &r.DayPeriod },
		[]string{"BBBBB", "bbbbb", "BBBB", "bbbb", "B", "b"},
		[]FieldWidth{WidthNarrow, WidthNarrow, WidthLong, WidthLong, WidthShort, WidthShort}},
	{func(r *ResolvedDateTimeFormat) *FieldWidth { return &r.Hour },
		[]string{"HH", "H", "hh", "h", "kk", "k", "KK", "K"},
		[]FieldWidth{Width2Digit, WidthNumeric, Width2Digit, WidthNumeric,
			Width2Digit, WidthNumeric, Width2Digit, WidthNumeric}},
	{func(r *ResolvedDateTimeFormat) *FieldWidth { return &r.Minute },
		[]string{"mm", "m"},
		[]FieldWidth{Width2Digit, WidthNumeric}},
	{func(r *ResolvedDateTimeFormat) *FieldWidth { return &r.Second },
		[]string{"ss", "s"},
		[]FieldWidth{Width2Digit, WidthNumeric}},
}

// resolvedZoneRuns are the time zone name's, as resolvedFieldRuns.
var resolvedZoneRuns = [...]struct {
	run string
	as  ZoneStyle
}{
	{"zzzz", ZoneLong}, {"z", ZoneShort}, {"OOOO", ZoneLongOffset},
	{"O", ZoneShortOffset}, {"vvvv", ZoneLongGeneric}, {"v", ZoneShortGeneric},
}

// resolvedFields reads the fields resolvedOptions reports from the pattern,
// as ECMA-402 reports the fields of the format it chose: each at the width
// of the first of its letter runs the pattern's fields hold. V8 looks for
// the runs in the pattern's text, its quoted literals as well, so the "de"
// of "MMMM 'de' y" is a day (LiteralFields).
func (f *DateTimeFormat) resolvedFields(r *ResolvedDateTimeFormat) {
	text := f.patternText
	if !f.opts.Compat.Has(LiteralFields) {
		// The fields alone, apart, so that no two run together.
		var b strings.Builder
		for _, fd := range parseDatePattern(f.patternText) {
			if fd.letter != 0 {
				b.WriteString(strings.Repeat(string(fd.letter), fd.count))
				b.WriteByte(' ')
			}
		}
		text = b.String()
	}
	for _, item := range resolvedFieldRuns {
		for i, run := range item.runs {
			if strings.Contains(text, run) {
				*item.into(r) = item.as[i]
				break
			}
		}
	}
	// V8's FractionalSecondDigitsFromPattern: every S, up to three.
	r.FractionalSecondDigits = min(strings.Count(text, "S"), 3)
	for _, z := range resolvedZoneRuns {
		if strings.Contains(text, z.run) {
			r.TimeZoneName = z.as
			break
		}
	}
}
