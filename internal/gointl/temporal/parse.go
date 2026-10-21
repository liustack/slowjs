package temporal

import (
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// Parsing Temporal's strings, as temporal_rs 0.2.3's parsers module and
// parsed intermediates do over ixdtf.

// JSStringBytes is the bytes V8 hands temporal_rs for a JavaScript string:
// a string of Latin-1 characters byte for byte, as V8 holds it, and any
// other converted to UTF-8, a lone surrogate being a RangeError.
func JSStringBytes(s []uint16) ([]byte, error) {
	latin1 := true
	for _, u := range s {
		if u > 0xFF {
			latin1 = false
			break
		}
	}
	if latin1 {
		b := make([]byte, len(s))
		for i, u := range s {
			b[i] = byte(u)
		}
		return b, nil
	}
	var b []byte
	for i := 0; i < len(s); i++ {
		r := rune(s[i])
		switch {
		case utf16.IsSurrogate(r):
			if r < 0xDC00 && i+1 < len(s) && s[i+1] >= 0xDC00 && s[i+1] <= 0xDFFF {
				r = utf16.DecodeRune(r, rune(s[i+1]))
				i++
			} else {
				return nil, rangeError("")
			}
		}
		b = utf8.AppendRune(b, r)
	}
	return b, nil
}

type parseVariant int

const (
	variantYearMonth parseVariant = iota
	variantMonthDay
	variantDateTime
	variantTime
)

// parseIXDTF is parse_ixdtf: the string parsed for a variant, with
// Temporal's checks on its calendar and date.
func parseIXDTF(src []byte, v parseVariant) (parseRecord, error) {
	var first *annotation
	duplicate := false
	handler := func(a annotation) bool {
		if string(a.key) == "u-ca" {
			if first == nil {
				first = &a
			} else if first.critical || a.critical {
				duplicate = true
			}
			return false
		}
		return true
	}
	c := &cursor{src: src}
	var r parseRecord
	var err error
	switch v {
	case variantYearMonth:
		r, err = parseAnnotatedYearMonth(c, handler)
	case variantMonthDay:
		r, err = parseAnnotatedMonthDay(c, handler)
	case variantDateTime:
		r, err = parseAnnotatedDateTime(c, handler)
	default:
		r, err = parseAnnotatedTime(c, handler)
	}
	if err != nil {
		return r, err.(ixdtfError).asTemporal()
	}
	if r.offset != nil && r.offset.fraction != nil {
		if _, ok := r.offset.fraction.nanoseconds(); !ok {
			return r, rangeError("Fractional time exceeds nine digits.")
		}
	}
	r.calendar, r.hasCal = nil, false
	if first != nil {
		r.calendar, r.hasCal = first.value, true
	}
	if (v == variantMonthDay || v == variantYearMonth) && r.hasCal && !strings.EqualFold(string(r.calendar), "iso8601") {
		return r, rangeError("YearMonth/MonthDay formats only allowed for ISO calendar.")
	}
	if duplicate {
		return r, rangeError("Duplicate calendar value with critical flag found.")
	}
	if v != variantTime && r.date == nil {
		return r, rangeError("DateTime strings must contain a Date value.")
	}
	if r.date != nil {
		year, day := int(r.date.year), int(r.date.day)
		if v == variantMonthDay {
			year = 1972
		}
		if v == variantYearMonth {
			day = 1
		}
		if !validISODate(year, int(r.date.month), day) {
			return r, rangeError("DateTime strings must contain a valid ISO date.")
		}
	}
	return r, nil
}

func parseDateTimeString(src []byte) (parseRecord, error) {
	r, err := parseIXDTF(src, variantDateTime)
	if err != nil {
		return r, err
	}
	if r.z {
		return r, rangeError("UTC designator is not valid for DateTime parsing.")
	}
	return r, nil
}

func parseZonedDateTimeString(src []byte) (parseRecord, error) {
	r, err := parseIXDTF(src, variantDateTime)
	if err != nil {
		return r, err
	}
	if r.tz == nil {
		return r, rangeError("Time zone annotation is required for parsing a zoned date time.")
	}
	return r, nil
}

func checkNoZ(r parseRecord) (parseRecord, error) {
	if r.z {
		return r, rangeError("UTC designator is not valid for plain date/time parsing.")
	}
	return r, nil
}

func parseYearMonthString(src []byte) (parseRecord, error) {
	r, err := parseIXDTF(src, variantYearMonth)
	if err == nil {
		return checkNoZ(r)
	}
	if dt, err2 := parseDateTimeString(src); err2 == nil {
		return checkNoZ(dt)
	}
	return r, err
}

func parseMonthDayString(src []byte) (parseRecord, error) {
	r, err := parseIXDTF(src, variantMonthDay)
	if err == nil {
		return checkNoZ(r)
	}
	if dt, err2 := parseDateTimeString(src); err2 == nil {
		return checkNoZ(dt)
	}
	return r, err
}

func checkTimeRecord(r parseRecord) (timeRecord, error) {
	r, err := checkNoZ(r)
	if err != nil {
		return timeRecord{}, err
	}
	if r.time == nil {
		return timeRecord{}, rangeError("PlainTime can only be parsed from strings with a time component.")
	}
	return *r.time, nil
}

func parseTimeString(src []byte) (timeRecord, error) {
	r, err := parseIXDTF(src, variantTime)
	if err == nil {
		return checkTimeRecord(r)
	}
	if dt, err2 := parseDateTimeString(src); err2 == nil {
		return checkTimeRecord(dt)
	}
	return timeRecord{}, err
}

// keepAll is ixdtf's default handler, Some for every annotation.
func keepAll(annotation) bool { return true }

// parseAllowedCalendarFormats is parse_allowed_calendar_formats: the
// calendar of a string that is any of Temporal's formats, and false for
// one that is none.
func parseAllowedCalendarFormats(s []byte) ([]byte, bool) {
	if r, err := parseIXDTF(s, variantDateTime); err == nil {
		return r.calendar, true
	}
	if r, err := parseAnnotatedTime(&cursor{src: s}, keepAll); err == nil {
		return r.calendar, true
	}
	if r, err := parseIXDTF(s, variantYearMonth); err == nil {
		return r.calendar, true
	}
	if r, err := parseIXDTF(s, variantMonthDay); err == nil {
		return r.calendar, true
	}
	return nil, false
}

// timeFromRecord is IsoTime::from_time_record: a leap second is 59.
func timeFromRecord(t timeRecord) (ISOTime, error) {
	second := int(t.second)
	if second > 59 {
		second = 59
	}
	var ns uint32
	if t.fraction != nil {
		v, ok := t.fraction.nanoseconds()
		if !ok {
			return ISOTime{}, rangeError("Fractional time exceeds nine digits.")
		}
		ns = v
	}
	return newISOTime(int(t.hour), int(t.minute), second, int(ns/1_000_000), int(ns%1_000_000/1000),
		int(ns%1000), Reject)
}

// A ParsedDate is ParsedDate: a date string's date and calendar, before
// its date is checked against Temporal's range.
type ParsedDate struct {
	record dateRecord
	cal    string
}

func parsedCalendar(r parseRecord) (string, error) {
	if !r.hasCal {
		return "iso8601", nil
	}
	return calendarKindFromBytes(r.calendar)
}

func checkRecordTime(r parseRecord) error {
	if r.time != nil {
		_, err := timeFromRecord(*r.time)
		return err
	}
	return nil
}

// ParseDate is ParsedDate::from_utf8.
func ParseDate(s []byte) (ParsedDate, error) {
	r, err := parseDateTimeString(s)
	if err != nil {
		return ParsedDate{}, err
	}
	cal, err := parsedCalendar(r)
	if err != nil {
		return ParsedDate{}, err
	}
	if err := checkRecordTime(r); err != nil {
		return ParsedDate{}, err
	}
	return ParsedDate{*r.date, cal}, nil
}

// ParseYearMonth is ParsedDate::year_month_from_utf8.
func ParseYearMonth(s []byte) (ParsedDate, error) {
	r, err := parseYearMonthString(s)
	if err != nil {
		return ParsedDate{}, err
	}
	cal, err := parsedCalendar(r)
	if err != nil {
		return ParsedDate{}, err
	}
	if err := checkRecordTime(r); err != nil {
		return ParsedDate{}, err
	}
	return ParsedDate{*r.date, cal}, nil
}

// ParseMonthDay is ParsedDate::month_day_from_utf8.
func ParseMonthDay(s []byte) (ParsedDate, error) {
	r, err := parseMonthDayString(s)
	if err != nil {
		return ParsedDate{}, err
	}
	cal, err := parsedCalendar(r)
	if err != nil {
		return ParsedDate{}, err
	}
	if err := checkRecordTime(r); err != nil {
		return ParsedDate{}, err
	}
	return ParsedDate{*r.date, cal}, nil
}

// Calendar is the parsed calendar's identifier.
func (p ParsedDate) Calendar() string { return p.cal }

// A ParsedDateTime is ParsedDateTime.
type ParsedDateTime struct {
	date ParsedDate
	time ISOTime
}

// ParseDateTime is ParsedDateTime::from_utf8.
func ParseDateTime(s []byte) (ParsedDateTime, error) {
	r, err := parseDateTimeString(s)
	if err != nil {
		return ParsedDateTime{}, err
	}
	cal, err := parsedCalendar(r)
	if err != nil {
		return ParsedDateTime{}, err
	}
	var t ISOTime
	if r.time != nil {
		if t, err = timeFromRecord(*r.time); err != nil {
			return ParsedDateTime{}, err
		}
	}
	return ParsedDateTime{ParsedDate{*r.date, cal}, t}, nil
}

// Calendar is the parsed calendar's identifier.
func (p ParsedDateTime) Calendar() string { return p.date.cal }

// ParsePlainTime is PlainTime::from_utf8.
func ParsePlainTime(s []byte) (PlainTime, error) {
	r, err := parseTimeString(s)
	if err != nil {
		return PlainTime{}, err
	}
	t, err := timeFromRecord(r)
	return PlainTime{t}, err
}

// PlainDateFromParsed is PlainDate::from_parsed.
func PlainDateFromParsed(p ParsedDate, cal *Calendar) (PlainDate, error) {
	return newPlainDate(int(p.record.year), int(p.record.month), int(p.record.day), cal, Reject)
}

// PlainDateTimeFromParsed is PlainDateTime::from_parsed.
func PlainDateTimeFromParsed(p ParsedDateTime, cal *Calendar) (PlainDateTime, error) {
	d, err := newISODateWithOverflow(int(p.date.record.year), int(p.date.record.month), int(p.date.record.day), Reject)
	if err != nil {
		return PlainDateTime{}, err
	}
	iso, err := newISODateTime(d, p.time)
	return PlainDateTime{iso, cal}, err
}

// PlainYearMonthFromParsed is PlainYearMonth::from_parsed.
func PlainYearMonthFromParsed(p ParsedDate, cal *Calendar) (PlainYearMonth, error) {
	iso := ISODate{int(p.record.year), int(p.record.month), int(p.record.day)}
	if !yearMonthWithinLimits(iso.Year, iso.Month) {
		return PlainYearMonth{}, rangeError("Exceeded valid range.")
	}
	return PlainYearMonth{iso, cal}.cal.yearMonthFromFields(PlainYearMonth{iso, cal}.fields(), Constrain)
}

// PlainMonthDayFromParsed is PlainMonthDay::from_parsed.
func PlainMonthDayFromParsed(p ParsedDate, cal *Calendar) (PlainMonthDay, error) {
	if cal.isISO() {
		return PlainMonthDay{ISODate{1972, int(p.record.month), int(p.record.day)}, cal}, nil
	}
	iso := ISODate{int(p.record.year), int(p.record.month), int(p.record.day)}
	if err := iso.checkWithinLimits(); err != nil {
		return PlainMonthDay{}, err
	}
	md := PlainMonthDay{iso, cal}
	return cal.monthDayFromFields(CalendarFields{MonthCode: String(md.MonthCode()), Day: Int(md.Day())}, Constrain)
}

// ParseDuration is Duration::from_utf8.
func ParseDuration(s []byte) (Duration, error) {
	r, err := parseDuration(&cursor{src: s})
	if err != nil {
		return Duration{}, err.(ixdtfError).asTemporal()
	}
	fracNS := func() (uint64, error) {
		if r.fraction == nil {
			return 0, nil
		}
		v, ok := r.fraction.nanoseconds()
		if !ok {
			return 0, rangeError("Fractional time exceeds nine digits.")
		}
		return uint64(v), nil
	}
	var hours, minutes, seconds, millis, micros, nanos uint64
	switch r.timeUnit {
	case 1:
		f, err := fracNS()
		if err != nil {
			return Duration{}, err
		}
		ns := f * 3600
		hours = r.hours
		minutes = ns / 60_000_000_000
		rem := ns % 60_000_000_000
		seconds = rem / 1_000_000_000
		sub := rem % 1_000_000_000
		millis, micros, nanos = sub/1_000_000, sub%1_000_000/1000, sub%1000
	case 2:
		f, err := fracNS()
		if err != nil {
			return Duration{}, err
		}
		ns := f * 60
		hours, minutes = r.hours, r.minutes
		seconds = ns / 1_000_000_000
		sub := ns % 1_000_000_000
		millis, micros, nanos = sub/1_000_000, sub%1_000_000/1000, sub%1000
	case 3:
		f, err := fracNS()
		if err != nil {
			return Duration{}, err
		}
		hours, minutes, seconds = r.hours, r.minutes, r.seconds
		millis, micros, nanos = f/1_000_000, f%1_000_000/1000, f%1000
	}
	sign := int64(1)
	if r.negative {
		sign = -1
	}
	return newDuration(int64(r.years)*sign, int64(r.months)*sign, int64(r.weeks)*sign, int64(r.days)*sign,
		int64(hours)*sign, int64(minutes)*sign, int64(seconds)*sign, int64(millis)*sign,
		i128(int64(micros)*sign), i128(int64(nanos)*sign))
}

// calendarAlgorithm is ICU4X's CalendarAlgorithm read from a Unicode
// extension value: the calendar's identifier, "islamic" for the Hijri
// calendar without a sub-type, false for none.
func calendarAlgorithm(b []byte) (string, bool) {
	var subtags []string
	if len(b) > 0 {
		for _, chunk := range strings.Split(string(b), "-") {
			if len(chunk) < 2 || len(chunk) > 8 {
				return "", false
			}
			for i := 0; i < len(chunk); i++ {
				if !isDigit(chunk[i]) && !isAlpha(chunk[i]) {
					return "", false
				}
			}
			if chunk = strings.ToLower(chunk); chunk != "true" {
				subtags = append(subtags, chunk)
			}
		}
	}
	switch strings.Join(subtags, "-") {
	case "islamicc":
		return "islamic-civil", true
	case "ethiopic-amete-alem":
		return "ethioaa", true
	}
	if len(subtags) == 0 {
		return "", false
	}
	switch subtags[0] {
	case "buddhist", "chinese", "coptic", "dangi", "ethioaa", "ethiopic", "gregory", "hebrew", "indian",
		"iso8601", "japanese", "persian", "roc":
		if len(subtags) > 1 {
			return "", false
		}
		return subtags[0], true
	case "islamic":
		switch {
		case len(subtags) > 2:
			return "", false
		case len(subtags) == 1:
			return "islamic", true
		}
		switch subtags[1] {
		case "umalqura", "tbla", "civil", "rgsa":
			return "islamic-" + subtags[1], true
		}
	}
	return "", false
}

// CalendarID is AnyCalendarKind::get_for_str, V8's CanonicalizeCalendar:
// the identifier of a calendar Temporal takes, in any case, with CLDR's
// aliases. False for "islamic" and "islamic-rgsa", which Temporal does not
// take.
func CalendarID(s string) (string, bool) {
	id, ok := calendarAlgorithm([]byte(s))
	if !ok || id == "islamic" || id == "islamic-rgsa" {
		return "", false
	}
	return id, true
}

// ParseCalendarString is ParseTemporalCalendarString, as temporal_capi's
// parse_temporal_calendar_string has it: a calendar identifier, or the
// calendar of a string in any of Temporal's formats, ISO where it names
// none.
func ParseCalendarString(s []byte) (string, bool) {
	if cal, ok := parseAllowedCalendarFormats(s); ok {
		if len(cal) == 0 {
			return "iso8601", true
		}
		return CalendarID(string(cal))
	}
	return CalendarID(string(s))
}

// calendarKindFromBytes is Calendar::try_kind_from_utf8: the calendar of
// a string's annotation, "islamic" read as the tabular calendar of the
// civil epoch.
func calendarKindFromBytes(b []byte) (string, error) {
	id, ok := calendarAlgorithm([]byte(strings.ToLower(string(b))))
	switch {
	case !ok || id == "islamic-rgsa":
		return "", rangeError("unknown calendar")
	case id == "islamic":
		return "islamic-civil", nil
	}
	return id, nil
}
