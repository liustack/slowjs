package intl

import (
	"errors"
	"sort"
	"strings"
	"time"
)

// Temporal values.
//
// A DateTimeFormat writes a Temporal value with a pattern of its own for the
// value's kind, which V8 makes when it writes one (CallICUFormat): the
// skeleton of the formatter's pattern, cut down to the fields the kind has
// and given the kind's defaults where it asked for none of them
// (GetSkeletonForPatternKind, after the Temporal proposal's
// AdjustDateTimeStyleFormat and GetDateTimeFormat), and a pattern generated
// for it in the formatter's locale. A range is written by an interval
// formatter made from the same skeleton.
//
// V8 departs from the proposal in two ways (TemporalFormats): it counts an
// era among the fields asked for, so an era alone is written alone, and the
// pattern it generates keeps no hour cycle, so hour12 and hourCycle change
// nothing for a value asked for no hour. The standard side writes the kind's
// defaults beside an era, and makes a fields' pattern in the formatter's
// hour cycle.
//
// A plain value is written as the instant PlainInstant makes of its ISO
// date and time, and a ZonedDateTime's toLocaleString writes its instant in
// its own zone. Before it does, the engine asks CalendarMatches, since a
// value in another calendar is a RangeError, and V8 asks that first.

// A TemporalKind is a kind of Temporal value a DateTimeFormat writes.
type TemporalKind int

const (
	TemporalPlainDate TemporalKind = iota + 1
	TemporalPlainDateTime
	TemporalPlainTime
	TemporalPlainYearMonth
	TemporalPlainMonthDay
	TemporalInstant
)

// ErrTemporalFormat reports that a formatter has no pattern for a kind of
// Temporal value: a time style alone for a date, a date style alone for a
// time, or fields the kind has none of. ECMA-402 throws a TypeError.
var ErrTemporalFormat = errors.New("intl: the formatter cannot write this kind of Temporal value")

// CalendarMatches reports whether a Temporal value of a kind, in a
// calendar, may be written by the formatter, as V8's CalendarEquals decides
// it: the calendars are the same, "islamic" standing for each of the
// Islamic calendars. A plain date or date-time in the ISO calendar matches
// any; a time or an instant has no calendar and always does.
func (f *DateTimeFormat) CalendarMatches(kind TemporalKind, calendar string) bool {
	switch kind {
	case TemporalPlainTime, TemporalInstant:
		return true
	case TemporalPlainDate, TemporalPlainDateTime:
		if calendar == string(ISO8601) {
			return true
		}
	}
	switch CalendarSystem(calendar) {
	case IslamicCivil, IslamicTabular, IslamicUmmAlQura:
		if f.system == Islamic {
			return true
		}
	}
	return string(f.system) == calendar
}

// ForTemporal returns the formatter that writes a Temporal value of a kind
// for this one: the same locale, calendar, zone and numbering, with the
// pattern made for the kind.
func (f *DateTimeFormat) ForTemporal(kind TemporalKind) (*DateTimeFormat, error) {
	dateStyle, timeStyle := f.opts.DateStyle != LengthNone, f.opts.TimeStyle != LengthNone
	switch kind {
	case TemporalPlainDate, TemporalPlainYearMonth, TemporalPlainMonthDay:
		if !dateStyle && timeStyle {
			return nil, ErrTemporalFormat
		}
	case TemporalPlainTime:
		if dateStyle && !timeStyle {
			return nil, ErrTemporalFormat
		}
	case TemporalPlainDateTime, TemporalInstant:
	default:
		return nil, ErrTemporalFormat
	}
	node := f.opts.Compat.Has(TemporalFormats)
	skeleton := temporalSkeleton(staticSkeleton(f.patternText), f.explicit, kind,
		dateStyle || timeStyle, f.opts.ToLocaleStringTimeZone, node)
	if skeleton == "" {
		return nil, ErrTemporalFormat
	}

	d := *f
	if kind != TemporalInstant && !f.opts.Compat.Has(PlainValueZone) {
		// A plain value's fields are written as they are, which
		// PlainInstant makes an instant in UTC.
		d.tz, d.zoneID = fixedZone(0), ""
	}
	if !node && (dateStyle || timeStyle) && skeleton == staticSkeleton(f.patternText) {
		// AdjustDateTimeStyleFormat keeps a style's format where the kind
		// has each of its fields (test262's
		// datestyle-not-adjusted-when-no-conflicting-options); V8 makes
		// one again from its skeleton.
		return &d, nil
	}

	// DateFormat::createInstanceForSkeleton: the best pattern for the
	// skeleton, in the formatter's locale, whose hour cycle is its own.
	hourChar, allowed, err := allowedHourFormats(f.src, f.locale)
	if err != nil {
		return nil, err
	}
	g := newDTPG(f.patterns, f.calendar, f.data.FieldNames, f.decimal, hourChar, allowed)
	var pattern string
	if node || dateStyle || timeStyle || !strings.ContainsAny(skeleton, "hHkKj") {
		pattern = g.bestPattern(skeleton, 0)
	} else {
		// The proposal's format options keep the formatter's hour cycle,
		// which a fields' pattern is made with as the formatter's own is.
		skeleton = withHourLetter(skeleton, f.clock)
		pattern = replaceHourCycleInPattern(g.bestPattern(skeleton, matchHourFieldLength), f.clock)
		d.hourCycle = f.clock
	}
	// The jpanyear rules stay loaded: a Temporal value's pattern with a 年
	// writes 元年 as any other does.
	d.overrides, d.systems = nil, nil
	d.patternText = pattern
	d.pattern = compileDatePattern(pattern, d.patternOverrides(pattern, nil))
	if d.ranges, err = d.newRangeFormat(f.src, g, skeleton); err != nil {
		return nil, err
	}
	if err := d.loadPatternData(); err != nil {
		return nil, err
	}
	return &d, nil
}

// PlainInstant is the instant a plain Temporal value, its ISO date and
// wall-clock time, is written as by the formatter ForTemporal makes for its
// kind. The proposal writes the fields as they are, whatever the
// formatter's zone (test262's PlainDate/prototype/toLocaleString/
// ignore-timezone): the instant is the fields read as UTC, where the kind's
// formatter writes. V8 reads them as the instant they name in the
// formatter's zone, as Temporal's GetEpochNanosecondsFor reckons it with
// "compatible" -- a skipped time and a repeated one both with the offset
// before the transition -- and writes that instant there, so a date or
// time a transition skips moves (PlainValueZone).
func (f *DateTimeFormat) PlainInstant(year int, month time.Month, day, hour, min, sec, nsec int) time.Time {
	t := time.Date(year, month, day, hour, min, sec, nsec, time.UTC)
	if !f.opts.Compat.Has(PlainValueZone) {
		return t
	}
	offset := f.tz.offsetFromLocal(t.UnixMilli(), tzLocalFormer, tzLocalFormer)
	return t.Add(-time.Duration(offset.raw+offset.dst) * time.Second)
}

// temporalSkeleton is V8's GetSkeletonForPatternKind: the skeleton a kind
// of Temporal value is written with, from the skeleton of the formatter's
// pattern and the fields its options named; empty for none. Where node is
// false it is the proposal's GetDateTimeFormat, whose required fields have
// no era (TemporalFormats).
func temporalSkeleton(best, explicit string, kind TemporalKind, styled, zoned, node bool) string {
	if styled {
		// AdjustDateTimeStyleFormat: the style's fields the kind has.
		switch kind {
		case TemporalPlainDate:
			return keepLetters(best, "EcGyMLd")
		case TemporalPlainYearMonth:
			return keepLetters(best, "GyML")
		case TemporalPlainMonthDay:
			return keepLetters(best, "MLd")
		case TemporalPlainTime:
			return keepLetters(best, "hHkKjmsBbaS")
		case TemporalPlainDateTime:
			return keepLetters(best, "EcGyMLdhHkKjmsBbaS")
		}
		return best
	}

	// The fields the options named, in the order the pattern writes them.
	options := keepLetters(best, explicit)
	requiredDate, requiredYearMonth, requiredAny := "EcyMLd", "yML", "EcyMLdhHkKjmsBbaS"
	if node {
		requiredDate, requiredYearMonth, requiredAny = "EcGyMLd", "GyML", "EcGyMLdhHkKjmsBbaS"
	}
	const defaultsAll = "yMdjms"
	switch kind {
	case TemporalPlainDate:
		return temporalFormat(options, explicit, requiredDate, "yMd", false, true, false, false)
	case TemporalPlainYearMonth:
		return temporalFormat(options, explicit, requiredYearMonth, "yM", false, true, false, false)
	case TemporalPlainMonthDay:
		return temporalFormat(options, explicit, "MLd", "Md", false, false, false, false)
	case TemporalPlainTime:
		return temporalFormat(options, explicit, "hHkKjmsBbaS", "jms", false, false, true, false)
	case TemporalPlainDateTime:
		return temporalFormat(options, explicit, requiredAny, defaultsAll, false, true, true, false)
	}
	return temporalFormat(options, explicit, requiredAny, defaultsAll, true, true, true, zoned)
}

// withHourLetter is a skeleton with its hour, 'j' or any, written in a
// cycle's letter.
func withHourLetter(skeleton string, hc HourCycle) string {
	to, ok := hourLetters[hc]
	if !ok {
		return skeleton
	}
	b := []byte(skeleton)
	for i, c := range b {
		if strings.IndexByte("jhHkK", c) >= 0 {
			b[i] = to
		}
	}
	return string(b)
}

// keepLetters is the letters of a skeleton that are among allowed, or
// written the same way as one of them: 'L' as 'M', an hour letter as 'j',
// 'O' and 'v' as 'z' (AdjustDateTimeStyleFormat, and V8's
// OrigionalOptions).
func keepLetters(s, allowed string) string {
	keep := allowed
	for i := 0; i < len(allowed); i++ {
		if also := equivalentLetter(allowed[i]); also != 0 {
			keep += string(also)
		}
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if strings.IndexByte(keep, s[i]) >= 0 {
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

// equivalentLetter is V8's EqualventSkeletonchar.
func equivalentLetter(c byte) byte {
	switch c {
	case 'L':
		return 'M'
	case 'h', 'H', 'k', 'K':
		return 'j'
	case 'O', 'v':
		return 'z'
	}
	return 0
}

// temporalFormat is V8's GetDateTimeFormat, over skeletons: of options, the
// fields that are required, else the defaults; empty when some other field
// was asked for and inherit is relevant. inheritAll keeps every field the
// options named, and copyEra and copyHourCycle the era and the hour letter
// when only the relevant ones are kept. zoned adds a short zone name to
// the defaults, for a ZonedDateTime's toLocaleString.
func temporalFormat(options, explicit, required, defaults string,
	inheritAll, copyEra, copyHourCycle, zoned bool) string {
	var format []byte
	if inheritAll {
		format = []byte(options)
	} else {
		for i := 0; i < len(options); i++ {
			c := options[i]
			if copyEra && c == 'G' || copyHourCycle && strings.IndexByte("hHkK", c) >= 0 {
				format = append(format, c)
			}
		}
	}
	anyPresent := strings.ContainsAny(explicit, "EcyMLdBbHhKkmsS")

	toAdd := map[byte]bool{}
	for i := 0; i < len(defaults); i++ {
		toAdd[defaults[i]] = true
	}
	needDefaults := true
	var last byte
	for i := 0; i < len(options); i++ {
		c := options[i]
		if strings.IndexByte(required, c) >= 0 {
			delete(toAdd, c)
			if also := equivalentLetter(c); also != 0 {
				delete(toAdd, also)
			}
			if last != c {
				format = removeByte(format, c)
			}
			format = append(format, c)
			needDefaults = false
		}
		last = c
	}
	if needDefaults {
		if anyPresent && !inheritAll {
			return ""
		}
		// A std::set<char16_t>: in the order of the letters.
		letters := make([]byte, 0, len(toAdd))
		for c := range toAdd {
			letters = append(letters, c)
		}
		sort.Slice(letters, func(i, j int) bool { return letters[i] < letters[j] })
		format = append(format, letters...)
		if zoned && !strings.ContainsAny(string(format), "zOv") {
			format = append(format, 'z')
		}
	}
	return string(format)
}

func removeByte(b []byte, c byte) []byte {
	out := b[:0]
	for _, x := range b {
		if x != c {
			out = append(out, x)
		}
	}
	return out
}
