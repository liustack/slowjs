package intl

import (
	"strings"
	"time"

	"github.com/liustack/slowjs/internal/gointl/internal/datedata"
)

// UTS #35 date patterns.
//
// A pattern is a run of field letters and literal text: "EEEE, MMMM d, y" is
// the weekday wide, a comma, the month wide, the day, a comma and the year.
// How many times a letter repeats says how wide the field is written, and the
// meaning of that differs by letter: MMM is an abbreviated month name while dd
// is a day padded to two digits.
//
// Text outside the fields is quoted with apostrophes, so a pattern can write a
// letter that would otherwise be a field.

// a dateField is one run of the same pattern letter.
type dateField struct {
	// letter is the field, and count how many times it repeated. A literal
	// run has a zero letter and its text in literal.
	letter  byte
	count   int
	literal string
}

// parseDatePattern takes a pattern apart into its fields and literals.
func parseDatePattern(pattern string) []dateField {
	var out []dateField
	var literal strings.Builder
	flush := func() {
		if literal.Len() > 0 {
			out = append(out, dateField{literal: literal.String()})
			literal.Reset()
		}
	}
	for i := 0; i < len(pattern); {
		c := pattern[i]
		switch {
		case c == '\'':
			// An apostrophe quotes what follows, and two of them are one
			// apostrophe.
			if i+1 < len(pattern) && pattern[i+1] == '\'' {
				literal.WriteByte('\'')
				i += 2
				continue
			}
			i++
			for i < len(pattern) && pattern[i] != '\'' {
				literal.WriteByte(pattern[i])
				i++
			}
			i++
		case isPatternLetter(c):
			flush()
			n := 0
			for i < len(pattern) && pattern[i] == c {
				n++
				i++
			}
			out = append(out, dateField{letter: c, count: n})
		default:
			literal.WriteByte(c)
			i++
		}
	}
	flush()
	return out
}

func isPatternLetter(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

// a dateParts is what a formatter needs to write one instant: the fields of
// the date in its calendar and zone, and the zone's own names.
type dateParts struct {
	year, month, day int
	weekday          int // Sunday is zero
	hour, minute     int
	second, millis   int
	era              int // zero before the epoch, one after
	// leapMonth marks a leap month of the Chinese calendars, which has the
	// number of the month before it.
	leapMonth bool
	// instant is the moment in the formatter's zone, which the zone's name
	// depends on, and zoneOffset its distance from UTC in seconds.
	instant    time.Time
	zoneOffset int
	// hasMinute and hasSecond say whether the pattern being written has
	// them, which decides whether a time is written as exactly noon, and
	// overrides are its numbering overrides, by letter.
	hasMinute, hasSecond bool
	overrides            map[byte]string
	// extYear is the calendar's extended year, which counts on through the
	// eras and is what the week-based year is reckoned from: the Gregorian
	// year in the Buddhist calendar, whose years only relabel it. dayOfYear
	// counts from one, and relatedYear is the Gregorian year the calendar's
	// year mostly falls in.
	extYear, dayOfYear, relatedYear int
	// yearLength is the length in days of a year of the calendar, by
	// extended year.
	yearLength func(int) int
	// hour12 and dayPeriod are worked out once rather than per field.
	hour12    int
	afternoon bool
	// failed says the calendar cannot reckon the date, as ICU's Chinese
	// astronomy cannot far enough from now, and yearStartFails whether it
	// can find where a year starts, by its extended year.
	failed         bool
	yearStartFails func(eyear int) bool
}

func partsOf(t time.Time) dateParts {
	year, month, day := t.Date()
	era := 1
	if year <= 0 {
		// The year before 1 is 1 BC, and there is no year zero.
		era, year = 0, 1-year
	}
	p := dateParts{
		year: year, month: int(month), day: day,
		weekday: int(t.Weekday()),
		hour:    t.Hour(), minute: t.Minute(),
		second: t.Second(), millis: t.Nanosecond() / 1e6,
		era: era,
	}
	p.afternoon = p.hour >= 12
	p.hour12 = p.hour % 12
	if p.hour12 == 0 {
		p.hour12 = 12
	}
	return p
}

// pad writes a number to at least a given width.
func pad(v, width int) string {
	s := itoa(v)
	for len(s) < width {
		s = "0" + s
	}
	return s
}

// datePartKind maps a pattern letter to the piece it produces, so that
// formatToParts can name what it wrote.
func datePartKind(letter byte) PartKind {
	switch letter {
	case 'G':
		return PartEra
	case 'y', 'Y', 'u':
		return PartYear
	case 'U':
		return PartYearName
	case 'r':
		return PartRelatedYear
	case 'M', 'L':
		return PartMonth
	case 'd':
		return PartDay
	case 'E', 'e', 'c':
		return PartWeekday
	case 'a', 'b', 'B':
		return PartDayPeriod
	case 'h', 'H', 'K', 'k':
		return PartHour
	case 'm':
		return PartMinute
	case 's':
		return PartSecond
	case 'S':
		return PartFractionalSecond
	case 'z', 'Z', 'O', 'v', 'V', 'X', 'x':
		return PartTimeZoneName
	}
	return PartLiteral
}

// writeField writes one field of a pattern.
//
// The width rules differ by letter and are UTS #35's: a month written once or
// twice is a number, three times an abbreviation, four times the whole word
// and five times a single letter. A year is padded to the count except that
// two of them means the last two digits.
func (f *DateTimeFormat) writeField(fd dateField, p *dateParts) string {
	cal := f.calendar
	switch fd.letter {
	case 'G':
		if f.system == Chinese || f.system == Dangi {
			// SimpleDateFormat writes the Chinese calendars' era, the
			// sixty-year cycle, as a number.
			return f.number(p, fd.letter, p.era, 1)
		}
		width := datedata.Abbreviated
		switch {
		case fd.count >= 5:
			width = datedata.Narrow
		case fd.count == 4:
			width = datedata.Wide
		}
		return cal.Era(width, p.era)

	case 'y':
		if fd.count == 2 {
			return f.twoDigitYear(p, fd.letter, p.year)
		}
		// A calendar with one era counts back through zero before it:
		// the Islamic year -435.
		return f.signedNumber(p, fd.letter, p.year, fd.count)
	case 'Y':
		year := f.weekYear(p)
		if fd.count == 2 {
			return f.twoDigitYear(p, fd.letter, year)
		}
		return f.signedNumber(p, fd.letter, year, fd.count)
	case 'u':
		return f.signedNumber(p, fd.letter, p.extYear, fd.count)
	case 'U':
		// The year's name in the sixty-year cycle, where the calendar has
		// names; otherwise the year, as "y" writes it.
		if p.year >= 1 && p.year <= len(cal.CyclicYears) {
			return cal.CyclicYears[p.year-1]
		}
		if fd.count == 2 {
			return f.twoDigitYear(p, fd.letter, p.year)
		}
		return f.signedNumber(p, fd.letter, p.year, fd.count)
	case 'r':
		return f.signedNumber(p, fd.letter, p.relatedYear, fd.count)

	case 'M', 'L':
		context := datedata.Format
		if fd.letter == 'L' {
			context = datedata.StandAlone
		}
		month := p.month
		if f.system == Hebrew {
			// SimpleDateFormat: in a leap year Adar is called Adar II, the
			// fourteenth name; in a common year, which has no Adar I, the
			// months after it are numbered one lower.
			leap := hebrewLeap(p.year)
			switch {
			case leap && month == 7 && fd.count >= 3:
				month = 14
			case !leap && month >= 7 && fd.count < 3:
				month--
			}
		}
		var text string
		leap := datedata.LeapNumeric
		switch {
		case fd.count <= 2:
			text = f.number(p, fd.letter, month, fd.count)
		case fd.count == 3:
			text = cal.Month(context, datedata.Abbreviated, month)
			leap = datedata.LeapFormatAbbreviated
		case fd.count == 4:
			text = cal.Month(context, datedata.Wide, month)
			leap = datedata.LeapFormatWide
		default:
			text = cal.Month(context, datedata.Narrow, month)
			leap = datedata.LeapFormatNarrow
		}
		if p.leapMonth {
			// A leap month is its month's name or number in the locale's
			// leap-month pattern, "{0}bis".
			if context == datedata.StandAlone && leap != datedata.LeapNumeric {
				leap += datedata.LeapStandAloneWide - datedata.LeapFormatWide
			}
			if pattern := cal.LeapMonthPatterns[leap]; pattern != "" {
				text = strings.Replace(pattern, "{0}", text, 1)
			}
		}
		return text

	case 'd':
		return f.number(p, 'd', p.day, fd.count)

	case 'E', 'e', 'c':
		context := datedata.Format
		if fd.letter == 'c' {
			context = datedata.StandAlone
		}
		// e and c written once or twice are the day's number in the week,
		// which nothing here asks for; every other count is a name.
		width := datedata.Abbreviated
		switch {
		case fd.count == 4:
			width = datedata.Wide
		case fd.count == 5:
			width = datedata.Narrow
		case fd.count >= 6:
			width = datedata.Short
		}
		return cal.Day(context, width, p.weekday)

	case 'a', 'b', 'B':
		width := datedata.Abbreviated
		switch {
		case fd.count == 4 || fd.count > 5:
			width = datedata.Wide
		case fd.count == 5:
			width = datedata.Narrow
		}
		// b and B are ICU's subFormat: noon when the time as written is
		// exactly noon -- its minutes and seconds zero, where the pattern
		// writes them -- and B otherwise the part of the day the hour is
		// in, the small hours being 凌晨 in Chinese rather than morning or
		// afternoon. Midnight is never written, for ICU finds it ambiguous.
		// What has no name falls back: noon to the part of the day, the
		// part of the day to which half it is.
		if fd.letter != 'a' {
			noon := p.hour == 12 && (!p.hasMinute || p.minute == 0) && (!p.hasSecond || p.second == 0)
			if noon && (fd.letter == 'b' || f.data.HasPoint("noon")) {
				if name := cal.PeriodName(width, "noon"); name != "" {
					return name
				}
			}
			if fd.letter == 'B' {
				if id := f.data.Period(p.hour * 60); id != "" && id != "am" && id != "pm" {
					if name := cal.PeriodName(width, id); name != "" {
						return name
					}
				}
			}
		}
		if p.afternoon {
			if s := cal.PM[width]; s != "" {
				return s
			}
			return cal.PM[datedata.Abbreviated]
		}
		if s := cal.AM[width]; s != "" {
			return s
		}
		return cal.AM[datedata.Abbreviated]

	case 'h':
		return f.digits(pad(p.hour12, fd.count))
	case 'H':
		return f.digits(pad(p.hour, fd.count))
	case 'K':
		return f.digits(pad(p.hour%12, fd.count))
	case 'k':
		hour := p.hour
		if hour == 0 {
			hour = 24
		}
		return f.digits(pad(hour, fd.count))

	case 'm':
		return f.digits(pad(p.minute, fd.count))
	case 's':
		return f.digits(pad(p.second, fd.count))
	case 'S':
		// The fraction is truncated or padded to the count asked for.
		s := pad(p.millis, 3)
		for len(s) < fd.count {
			s += "0"
		}
		return f.digits(s[:fd.count])

	case 'z':
		// TimeZoneFormat::format: a name, else the offset, long or short
		// as the name would have been.
		long := fd.count >= 4
		if name := f.zones.specific(&f.zone, p.instant, long); name != "" {
			return name
		}
		return f.digits(f.offsetText(p.zoneOffset, !long))
	case 'v':
		long := fd.count >= 4
		if name := f.zones.generic(&f.zone, p.instant, long); name != "" {
			return name
		}
		return f.digits(f.offsetText(p.zoneOffset, !long))
	case 'O':
		return f.digits(f.offsetText(p.zoneOffset, fd.count < 4))
	case 'Z', 'X', 'x', 'V':
		return f.digits(f.offsetText(p.zoneOffset, false))
	}
	return ""
}

// offsetText writes a zone as its distance from UTC, localized GMT format.
// Zero is written like any other offset, "GMT+0": ICU keeps the locale's
// "GMT" for reading, not for writing.
func (f *DateTimeFormat) offsetText(seconds int, short bool) string {
	return gmtOffset(seconds, f.zones.locale.GMTFormat, f.zones.locale.HourFormat, short)
}

// gmtOffset writes a zone as its distance from UTC, as ICU's
// formatOffsetLocalizedGMT does: the locale's pattern, "GMT{0}", around one of
// three offset patterns made from its hour format.
//
// With seconds the hour format is extended by them; without minutes, in the
// short form, it is cut off after the hour, and whatever followed the hour
// goes -- Hebrew's hour format ends in a left-to-right mark, which the short
// form therefore does not write. The hour is one digit in the short form and
// two in the long whatever the format says, and minutes and seconds are
// always two: Makhuwa's format says "H:mm" and ICU writes "GMT-05:00".
func gmtOffset(seconds int, pattern, hourFormat string, short bool) string {
	positive, negative, ok := strings.Cut(hourFormat, ";")
	if !ok {
		positive, negative = "+H:mm", "-H:mm"
	}
	form := positive
	if seconds < 0 {
		form = negative
		seconds = -seconds
	}
	hours, minutes, secs := seconds/3600, seconds%3600/60, seconds%60
	switch {
	case secs != 0:
		form = expandOffsetPattern(form)
	case minutes == 0 && short:
		form = truncateOffsetPattern(form)
	}

	var b strings.Builder
	for _, fd := range parseDatePattern(form) {
		switch fd.letter {
		case 'H':
			width := 2
			if short {
				width = 1
			}
			b.WriteString(pad(hours, width))
		case 'm':
			b.WriteString(pad(minutes, 2))
		case 's':
			b.WriteString(pad(secs, 2))
		default:
			b.WriteString(fd.literal)
		}
	}
	if pattern == "" {
		pattern = "GMT{0}"
	}
	return strings.ReplaceAll(pattern, "{0}", b.String())
}

// expandOffsetPattern adds seconds after the minutes, with the separator
// that stands between the hour and the minutes.
func expandOffsetPattern(hm string) string {
	mm := strings.Index(hm, "mm")
	if mm < 0 {
		return hm
	}
	sep := ""
	if h := strings.LastIndexByte(hm[:mm], 'H'); h >= 0 {
		sep = hm[h+1 : mm]
	}
	return hm[:mm+2] + sep + "ss" + hm[mm+2:]
}

// truncateOffsetPattern cuts an offset pattern off after its hour.
func truncateOffsetPattern(hm string) string {
	mm := strings.Index(hm, "mm")
	if mm < 0 {
		return hm
	}
	if hh := strings.LastIndex(hm[:mm], "HH"); hh >= 0 {
		return hm[:hh+2]
	}
	if h := strings.LastIndexByte(hm[:mm], 'H'); h >= 0 {
		return hm[:h+1]
	}
	return hm
}

// A datePattern is a pattern taken apart, with what writing it needs to know
// about the whole of it.
type datePattern struct {
	fields []dateField
	// hasMinute and hasSecond say whether the pattern writes them.
	hasMinute, hasSecond bool
	// overrides are the numbering systems some fields are written in, by
	// letter; nil for most.
	overrides map[byte]string
}

func compileDatePattern(pattern string, overrides map[byte]string) datePattern {
	dp := datePattern{fields: parseDatePattern(pattern), overrides: overrides}
	for _, fd := range dp.fields {
		dp.hasMinute = dp.hasMinute || fd.letter == 'm'
		dp.hasSecond = dp.hasSecond || fd.letter == 's'
	}
	return dp
}

// A dateSeg is one piece of written text: a field, by its pattern letter, or
// a literal, whose letter is zero.
type dateSeg struct {
	letter byte
	value  string
}

// instant reckons a moment in the formatter's zone and calendar.
func (f *DateTimeFormat) instant(t time.Time) dateParts {
	offset := f.tz.offsetAt(t.UnixMilli()).total()
	// Calendar arithmetic needs wall-clock fields. Astronomical calendars
	// and zone names also need the original instant. Keeping them separate
	// avoids a Location allocation for uncached offsets, including historical ones.
	local := t.UTC().Add(time.Duration(offset) * time.Second)
	p := reckon(local, t, f.system, f.rules)
	if p.year < 1 && isIslamic(f.system) && !f.opts.Compat.Has(IslamicEras) {
		// CLDR's era before the Hijrah, the year counted back from 1 AH,
		// where ICU4C writes a year of the era after it (IslamicEras).
		p.era, p.year = 1, 1-p.year
	}
	if f.system == Coptic && p.era == 0 && !f.opts.Compat.Has(CopticEra) {
		// The coptic calendar's one era, its years before 1 counted as 0
		// and below, where ICU4C writes an era CLDR names nowhere
		// (CopticEra).
		p.era, p.year = 1, 1-p.year
	}
	p.zoneOffset = offset
	p.instant = t
	return p
}

// render writes a moment in a pattern, as pieces.
func (f *DateTimeFormat) render(dp *datePattern, p dateParts) []dateSeg {
	p.hasMinute, p.hasSecond, p.overrides = dp.hasMinute, dp.hasSecond, dp.overrides
	var out []dateSeg
	for _, fd := range dp.fields {
		if fd.letter == 0 {
			if fd.literal != "" {
				out = append(out, dateSeg{0, fd.literal})
			}
			continue
		}
		if value := f.writeField(fd, &p); value != "" {
			out = append(out, dateSeg{fd.letter, value})
		}
	}
	return out
}
