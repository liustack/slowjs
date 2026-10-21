// Package eastasian reckons the Chinese and Korean calendars as ICU4X's
// icu_calendar 2.2.1 does: its tables where they reach, and its mean-motion
// approximation, anchored in the Gregorian calendar, outside them. Temporal
// reckons with it, and so does DateTimeFormat where it answers as the
// standard, which asks the two to agree.
//
// It also reads the file of ICU4X's tables, which holds the Umm al-Qura
// calendar's years as well.
package eastasian

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// A TableYear is a year of ICU4X's tables: which months are long, the
// ordinal of the leap month, the count of months, and the first day, a
// Rata Die.
type TableYear struct {
	Year, Leap, Count int
	Long              uint16
	Start             int64
}

// ParseTables reads the tables temporalgen writes: each line a table's
// name, the year, its months' lengths as 'l' and 's', the leap month's
// ordinal, and the first day. Only the tables named are kept, all of them
// where none is.
func ParseTables(b []byte, names ...string) (map[string][]TableYear, error) {
	out := map[string][]TableYear{}
	for text := string(b); text != ""; {
		var line string
		line, text, _ = strings.Cut(text, "\n")
		if line == "" || line[0] == '#' {
			continue
		}
		name, rest, _ := strings.Cut(line, " ")
		if len(names) > 0 && !contains(names, name) {
			continue
		}
		t, err := parseYear(rest)
		if err != nil {
			return nil, fmt.Errorf("the calendar tables: %q: %w", line, err)
		}
		if prev := out[name]; len(prev) > 0 && prev[len(prev)-1].Year != t.Year-1 {
			return nil, fmt.Errorf("the calendar tables: %s %d out of order", name, t.Year)
		}
		out[name] = append(out[name], t)
	}
	for _, name := range names {
		if len(out[name]) == 0 {
			return nil, fmt.Errorf("the calendar tables have no %s", name)
		}
	}
	return out, nil
}

func contains(names []string, name string) bool {
	for _, n := range names {
		if n == name {
			return true
		}
	}
	return false
}

var errLine = errors.New("not a year of a table")

// parseYear reads "1912 lslsslssllsls 0 1912-02-18": twelve or thirteen
// months, a leap month's ordinal of 0 or, in a year of thirteen, from 2 to
// 13, as it follows its base month, and a Gregorian date that exists. A
// Source other than the embedded data can hand it anything, and month 13
// had indexed past GregorianFixed's table.
func parseYear(s string) (TableYear, error) {
	fields := strings.Fields(s)
	if len(fields) != 4 || len(fields[1]) != 12 && len(fields[1]) != 13 {
		return TableYear{}, errLine
	}
	year, err1 := strconv.Atoi(fields[0])
	leap, err2 := strconv.Atoi(fields[2])
	date := strings.Split(fields[3], "-")
	if err1 != nil || err2 != nil || len(date) != 3 {
		return TableYear{}, errLine
	}
	if leap != 0 && (len(fields[1]) != 13 || leap < 2 || leap > 13) {
		return TableYear{}, errLine
	}
	var ymd [3]int
	for i, d := range date {
		n, err := strconv.Atoi(d)
		if err != nil {
			return TableYear{}, errLine
		}
		ymd[i] = n
	}
	if ymd[1] < 1 || ymd[1] > 12 || ymd[2] < 1 || ymd[2] > gregorianMonthDays(ymd[0], ymd[1]) {
		return TableYear{}, errLine
	}
	t := TableYear{Year: year, Leap: leap, Count: len(fields[1]), Start: GregorianFixed(ymd[0], ymd[1], ymd[2])}
	for i, c := range fields[1] {
		switch c {
		case 'l':
			t.Long |= 1 << i
		case 's':
		default:
			return TableYear{}, errLine
		}
	}
	return t, nil
}

// Lookup is the table's year, false where the table does not reach.
func Lookup(table []TableYear, year int) (TableYear, bool) {
	if len(table) == 0 {
		return TableYear{}, false
	}
	i := year - table[0].Year
	if i < 0 || i >= len(table) {
		return TableYear{}, false
	}
	return table[i], true
}

// Durations in milliseconds, as simple.rs computes them in 128 bits and
// truncates.
const (
	msPerDay = 86400000

	// UTCPlus8 and UTCPlus9 are China's offset and Korea's.
	UTCPlus8 = msPerDay * 8 / 24
	UTCPlus9 = msPerDay * 9 / 24
	// The reference time of the Qing calendar, UTC+(1397/180) hours.
	beijingOffset = msPerDay * 1397 / 180 / 24

	meanGregorianYear      = msPerDay * 146097 / 400
	meanGregorianSolarTerm = msPerDay * 146097 / 400 / 12
	// meanSynodicMonth is 86400000 × 29.5305888531 days, truncated.
	meanSynodicMonth = 2551442876
)

// A localMoment is a day and the milliseconds into it.
type localMoment struct {
	rd     int64
	millis int64
}

func (m localMoment) add(ms int64) localMoment {
	t := m.millis + ms
	return localMoment{rd: m.rd + floorDiv(t, msPerDay), millis: floorMod(t, msPerDay)}
}

// The solstice of 1999-12-22T07:44 and the new moon of 2000-01-06T18:14,
// in UTC, that the approximation counts from.
var (
	utcSolstice = localMoment{rd: GregorianFixed(1999, 12, 22), millis: (7*60 + 44) * 60 * 1000}
	utcNewMoon  = localMoment{rd: GregorianFixed(2000, 1, 6), millis: (18*60 + 14) * 60 * 1000}
)

// periodicOnOrBefore is the last moment base + n × period that falls on or
// before a day.
func periodicOnOrBefore(rd int64, base localMoment, period int64) localMoment {
	diff := (rd-base.rd)*msPerDay - base.millis
	n := floorDiv(diff+msPerDay-1, period)
	millis := base.rd*msPerDay + base.millis + n*period
	return localMoment{rd: floorDiv(millis, msPerDay), millis: floorMod(millis, msPerDay)}
}

// simpleYear is EastAsianTraditionalYear::simple: the píngqì rule with
// mean solar terms and mean new moons, at an offset from UTC.
func simpleYear(offset int64, related int) (lengths [13]bool, leap int, newYear int64) {
	majorTerm := periodicOnOrBefore(gregorianDayBeforeYear(related), utcSolstice.add(offset), meanGregorianYear)
	newMoon := periodicOnOrBefore(majorTerm.rd, utcNewMoon.add(offset), meanSynodicMonth)
	nextNewMoon := newMoon.add(meanSynodicMonth)
	term := -2
	hadLeap := false
	// The months before the year: the eleventh, the twelfth, and a leap
	// month after either.
	for term < 0 || nextNewMoon.rd <= majorTerm.rd && !hadLeap {
		if nextNewMoon.rd <= majorTerm.rd && !hadLeap {
			hadLeap = true
		} else {
			term++
			majorTerm = majorTerm.add(meanGregorianSolarTerm)
		}
		newMoon, nextNewMoon = nextNewMoon, nextNewMoon.add(meanSynodicMonth)
	}
	newYear = newMoon.rd
	for term < 12 || nextNewMoon.rd <= majorTerm.rd && !hadLeap {
		i := term
		if leap != 0 {
			i++
		}
		if i < len(lengths) {
			lengths[i] = nextNewMoon.rd-newMoon.rd == 30
		}
		if nextNewMoon.rd <= majorTerm.rd && !hadLeap {
			hadLeap = true
			leap = term + 1
		} else {
			term++
			majorTerm = majorTerm.add(meanGregorianSolarTerm)
		}
		newMoon, nextNewMoon = nextNewMoon, nextNewMoon.add(meanSynodicMonth)
	}
	return lengths, leap, newYear
}

// Rules are ICU4X's China or Korea rules: the country's table, the Qing
// table before it, and the approximation at the country's offset after the
// table and at Beijing's before the Qing.
type Rules struct {
	Table, Qing []TableYear
	Offset      int64
}

// China and Korea are the rules of the Chinese calendar and of Dangi, from
// tables ParseTables read.
func China(tables map[string][]TableYear) Rules {
	return Rules{Table: tables["china"], Qing: tables["qing"], Offset: UTCPlus8}
}

func Korea(tables map[string][]TableYear) Rules {
	return Rules{Table: tables["korea"], Qing: tables["qing"], Offset: UTCPlus9}
}

// A Year is a year of the calendar: the Gregorian year it starts in, its
// first day, a Rata Die, its months' lengths and count, and the ordinal of
// its leap month, 0 for none.
type Year struct {
	Related int
	Start   int64
	Lengths [13]uint8
	Count   int
	Leap    int
}

// Year is the year that starts in a Gregorian year.
func (r Rules) Year(related int) Year {
	var lengths [13]bool
	var leap int
	var newYear int64
	if t, ok := Lookup(r.Table, related); ok {
		lengths, leap, newYear = t.lengthsLeap(), t.Leap, t.Start
	} else if related > r.Table[0].Year {
		lengths, leap, newYear = simpleYear(r.Offset, related)
	} else if t, ok := Lookup(r.Qing, related); ok {
		lengths, leap, newYear = t.lengthsLeap(), t.Leap, t.Start
	} else {
		lengths, leap, newYear = simpleYear(beijingOffset, related)
	}
	return pack(related, lengths, leap, newYear)
}

func (t TableYear) lengthsLeap() (lengths [13]bool) {
	for i := range lengths {
		lengths[i] = t.Long&(1<<i) != 0
	}
	return lengths
}

// pack is PackedEastAsianTraditionalYearData: the new year kept as six bits
// of days from 19 January, and a month length for the thirteenth month only
// in a leap year.
func pack(related int, lengths [13]bool, leap int, newYear int64) Year {
	earliest := GregorianFixed(related, 1, 19)
	y := Year{Related: related, Start: earliest + (newYear-earliest)&0x3F, Count: 12, Leap: leap}
	if leap != 0 {
		y.Count = 13
	}
	for i := 0; i < 13; i++ {
		y.Lengths[i] = 29
		if lengths[i] {
			y.Lengths[i] = 30
		}
	}
	if leap == 0 {
		y.Lengths[12] = 0
	}
	return y
}

// YearOfRD is Rules::year_containing_rd: the year of the day's Gregorian
// year, or the one before where the day comes before its new year.
func (r Rules) YearOfRD(rd int64) Year {
	related := gregorianYearFromFixed(rd)
	y := r.Year(related)
	if rd < y.Start {
		y = r.Year(related - 1)
	}
	return y
}

func floorDiv(a, b int64) int64 {
	q := a / b
	if a%b != 0 && (a < 0) != (b < 0) {
		q--
	}
	return q
}

func floorMod(a, b int64) int64 { return a - floorDiv(a, b)*b }

// gregorianDayBeforeYear is day_before_year: the Rata Die of the last day
// of the year before.
func gregorianDayBeforeYear(year int) int64 {
	p := int64(year) - 1
	return 365*p + floorDiv(p, 4) - floorDiv(p, 100) + floorDiv(p, 400)
}

// GregorianFixed is the Rata Die of a Gregorian date.
func GregorianFixed(year, month, day int) int64 {
	days := [...]int{0, 31, 59, 90, 120, 151, 181, 212, 243, 273, 304, 334}[month-1]
	if month > 2 && year%4 == 0 && (year%100 != 0 || year%400 == 0) {
		days++
	}
	return gregorianDayBeforeYear(year) + int64(days) + int64(day)
}

// gregorianMonthDays is the days in a Gregorian month.
func gregorianMonthDays(year, month int) int {
	if month == 2 && year%4 == 0 && (year%100 != 0 || year%400 == 0) {
		return 29
	}
	return [...]int{31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31}[month-1]
}

// gregorianYearFromFixed is year_from_fixed.
func gregorianYearFromFixed(rd int64) int {
	d := rd - 1
	n400, d := floorDiv(d, 146097), floorMod(d, 146097)
	n100, d := d/36524, d%36524
	n4, d := d/1461, d%1461
	n1 := d / 365
	year := 400*n400 + 100*n100 + 4*n4 + n1
	if n100 != 4 && n1 != 4 {
		year++
	}
	return int(year)
}
