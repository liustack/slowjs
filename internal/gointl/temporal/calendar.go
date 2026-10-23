// Package temporal is Temporal's calendar and time arithmetic apart from any
// JavaScript engine, as Node's Temporal reckons it.
//
// Node builds Temporal from temporal_rs over ICU4X's icu_calendar, not from
// ICU4C, so its calendars are ICU4X's: the Chinese and Korean calendars read
// ICU4X's tables for 1900 to 2102 and a mean-motion model outside them, where
// Intl.DateTimeFormat's, which ICU4C writes, compute the astronomy. This
// package ports icu_calendar 2.2.1 and temporal_rs 0.2.3, the versions Node
// 26.10.0 pins. The intl package keeps ICU4C's calendars for formatting as
// Node does, but for the Chinese and Korean ones where it answers as the
// standard, which it reckons as this package does (internal/eastasian).
package temporal

import (
	"fmt"
	"slices"

	intl "github.com/liustack/slowjs/internal/gointl"
	"github.com/liustack/slowjs/internal/gointl/internal/eastasian"
)

// An ISODate is a date in the proleptic Gregorian calendar, as Temporal's
// ISO date records hold one.
type ISODate struct {
	Year, Month, Day int
}

// rdEpoch is the Rata Die of 1970-01-01.
const rdEpoch = 719163

// rataDie is the day ICU4X counts from, 1 being 0001-01-01 in the proleptic
// Gregorian calendar.
func (d ISODate) rataDie() int64 { return gregorianFixed(d.Year, d.Month, d.Day) }

// EpochDays is the day's number from 1970-01-01.
func (d ISODate) EpochDays() int64 { return d.rataDie() - rdEpoch }

// isoFromRataDie is the ISO date of a day.
func isoFromRataDie(rd int64) ISODate {
	y, m, d := gregorianFromFixed(rd)
	return ISODate{y, m, d}
}

// ISODateFromEpochDays is the ISO date of a day numbered from 1970-01-01.
func ISODateFromEpochDays(days int64) ISODate { return isoFromRataDie(days + rdEpoch) }

// A Calendar is one of the calendars Temporal reckons in.
type Calendar struct {
	id string
	r  rules
}

// Calendars are the calendars Temporal reckons in, as Node's Temporal takes
// them: every calendar Intl knows but the Islamic calendars that are not
// arithmetic, "islamic" and "islamic-rgsa". The list is the caller's own, a
// copy.
func Calendars() []string { return slices.Clone(calendarIDs) }

var calendarIDs = []string{"buddhist", "chinese", "coptic", "dangi", "ethioaa", "ethiopic", "gregory",
	"hebrew", "indian", "islamic-civil", "islamic-tbla", "islamic-umalqura", "iso8601", "japanese",
	"persian", "roc"}

// NewCalendar is the calendar with a Temporal identifier, reading its data
// from the intl package's embedded data.
func NewCalendar(id string) (*Calendar, error) { return NewCalendarFrom(intl.Embedded, id) }

// NewCalendarFrom is the calendar with a Temporal identifier, reading its
// data from src.
func NewCalendarFrom(src intl.Source, id string) (*Calendar, error) {
	c := &Calendar{id: id}
	switch id {
	case "iso8601":
		c.r = gregorianRules{eras: isoEras}
	case "gregory":
		c.r = gregorianRules{eras: ceBCE}
	case "buddhist":
		c.r = gregorianRules{offset: -543, eras: buddhistEras}
	case "roc":
		c.r = gregorianRules{offset: 1911, eras: rocEras}
	case "japanese":
		c.r = gregorianRules{eras: japaneseEras}
	case "coptic":
		c.r = copticRules{style: copticYears}
	case "ethiopic":
		c.r = copticRules{style: ameteMihret}
	case "ethioaa":
		c.r = copticRules{style: ameteAlem}
	case "indian":
		c.r = indianRules{}
	case "persian":
		c.r = persianRules{}
	case "hebrew":
		c.r = hebrewRules{}
	case "islamic-civil":
		c.r = hijriRules{epoch: islamicEpochFriday, reference: civilReference}
	case "islamic-tbla":
		c.r = hijriRules{epoch: islamicEpochThursday, reference: tblaReference}
	case "islamic-umalqura", "chinese", "dangi":
		tables, err := loadTables(src)
		if err != nil {
			return nil, err
		}
		switch id {
		case "islamic-umalqura":
			c.r = hijriRules{epoch: islamicEpochFriday, table: tableYearsOf(tables["ummalqura"]), reference: umalquraReference}
		case "chinese":
			c.r = eastAsianRules{rules: eastasian.China(tables)}
		default:
			c.r = eastAsianRules{rules: eastasian.Korea(tables), korean: true}
		}
	default:
		return nil, fmt.Errorf("temporal: %q is not a calendar", id)
	}
	return c, nil
}

// ID is the calendar's identifier.
func (c *Calendar) ID() string { return c.id }

// A CalendarDate is a day as a calendar reckons it, with the fields
// Temporal's date objects answer with.
type CalendarDate struct {
	// Era and EraYear are the era and the year in it, Era being empty for a
	// calendar Temporal gives no eras, the ISO calendar and the Chinese and
	// Korean ones.
	Era     string
	EraYear int
	// Year is the calendar's arithmetic year, Month the month's position in
	// the year, from 1, and MonthCode its code, "M05L" for a leap month.
	Year      int
	Month     int
	MonthCode string
	Day       int

	DayOfYear    int
	DaysInMonth  int
	DaysInYear   int
	MonthsInYear int
	InLeapYear   bool
}

// Date is the day d as the calendar reckons it. A date outside ISO's
// date-time limits is an error: the calendars' arithmetic is not made for
// it, and the Hebrew calendar's had looped for ever on the year 2^40.
func (c *Calendar) Date(d ISODate) (CalendarDate, error) {
	if !d.withinLimits() {
		return CalendarDate{}, rangeError("Date is not within ISO date time limits.")
	}
	return c.date(d), nil
}

// date is Date for a date known to be within the limits.
func (c *Calendar) date(d ISODate) CalendarDate {
	rd := d.rataDie()
	y := c.r.yearOfRD(rd)
	month, day := y.monthDay(rd)
	out := CalendarDate{
		Year:         y.extended,
		Month:        month,
		MonthCode:    c.r.monthFromOrdinal(y, month).code(),
		Day:          day,
		DayOfYear:    y.daysBefore(month) + day,
		DaysInMonth:  y.monthLength(month),
		DaysInYear:   y.daysInYear(),
		MonthsInYear: y.count,
		InLeapYear:   y.leapYear,
	}
	out.Era, out.EraYear, _ = c.r.eraYear(y, month, day)
	return out
}

// A calYear is a year as a calendar lays it out: where it starts and how
// long each month is. It is ICU4X's year info for every calendar at once.
type calYear struct {
	extended int
	// start is the Rata Die of the year's first day.
	start   int64
	lengths [13]uint8
	count   int
	// leap is the ordinal of the leap month, 0 for none.
	leap     int
	leapYear bool
}

// monthLength is the days in month m, counted from 1.
func (y *calYear) monthLength(m int) int {
	if m < 1 || m > len(y.lengths) {
		return 0
	}
	return int(y.lengths[m-1])
}

// daysBefore is the days in the year before month m.
func (y *calYear) daysBefore(m int) int {
	n := 0
	for i := 1; i < m && i <= y.count; i++ {
		n += int(y.lengths[i-1])
	}
	return n
}

func (y *calYear) daysInYear() int { return y.daysBefore(y.count + 1) }

// rataDie is the day of a month and day in the year.
func (y *calYear) rataDie(month, day int) int64 {
	return y.start + int64(y.daysBefore(month)) + int64(day-1)
}

// monthDay is the month and day of a day in the year.
func (y *calYear) monthDay(rd int64) (month, day int) {
	d := int(rd - y.start)
	month = 1
	for month < y.count && d >= y.monthLength(month) {
		d -= y.monthLength(month)
		month++
	}
	return month, d + 1
}

// A month is a month code: its number and whether it is the leap month
// after that number, which is how ICU4X's Month orders them.
type month struct {
	number int
	leap   bool
}

func (m month) code() string {
	s := fmt.Sprintf("M%02d", m.number)
	if m.leap {
		s += "L"
	}
	return s
}

// parseMonthCode is Month::try_from_utf8: "M" and two digits, and "L" for
// a leap month.
func parseMonthCode(s string) (month, bool) {
	if len(s) != 3 && len(s) != 4 || s[0] != 'M' || s[1] < '0' || s[1] > '9' || s[2] < '0' || s[2] > '9' {
		return month{}, false
	}
	m := month{number: int(s[1]-'0')*10 + int(s[2]-'0')}
	if len(s) == 4 {
		if s[3] != 'L' {
			return month{}, false
		}
		m.leap = true
	}
	return m, true
}

// less is Month's order: by number, the leap month after its base.
func (m month) less(o month) bool {
	return m.number < o.number || m.number == o.number && !m.leap && o.leap
}

// errMonth is why a month code has no month in a year: NotInCalendar or
// NotInYear, as ICU4X's MonthError has it.
type errMonth int

const (
	monthOK errMonth = iota
	monthNotInCalendar
	monthNotInYear
)

// errReference is why ICU4X has no reference year for a month and day.
type errReference int

const (
	referenceOK errReference = iota
	referenceNotInCalendar
	// referenceUseRegularIfConstrain is a leap month too rare to have a
	// reference year, which Temporal constrains to its base month.
	referenceUseRegularIfConstrain
)

// rules are ICU4X's DateFieldsResolver for one calendar, over calYear.
type rules interface {
	// year is the year with an extended year.
	year(extended int) calYear
	// yearOfRD is the year a day falls in.
	yearOfRD(rd int64) calYear
	ordinalFromMonth(y calYear, m month, constrain bool) (int, errMonth)
	monthFromOrdinal(y calYear, ordinal int) month
	// eraYear is the era and the year in it of a day, false for a calendar
	// Temporal gives no eras.
	eraYear(y calYear, month, day int) (string, int, bool)
	// extendedFromEra is the extended year of an era's year, false for an
	// era the calendar does not have.
	extendedFromEra(era string, eraYear int) (int, bool)
	// referenceYear is the extended year Temporal takes for a month and a
	// day given without one.
	referenceYear(m month, day int) (int, errReference)
	// minMonthsFrom is a lower bound for the months in a number of years
	// from the start of a year.
	minMonthsFrom(y calYear, years int) int
}

// solarMonths are the months of a calendar without leap months: the
// defaults of DateFieldsResolver.
type solarMonths struct{}

func (solarMonths) ordinalFromMonth(y calYear, m month, constrain bool) (int, errMonth) {
	if m.leap || m.number < 1 || m.number > y.count {
		return 0, monthNotInCalendar
	}
	return m.number, monthOK
}

func (solarMonths) monthFromOrdinal(y calYear, ordinal int) month { return month{number: ordinal} }

func (solarMonths) minMonthsFrom(y calYear, years int) int { return 12 * years }

// loadTables reads the tables temporalgen writes, by table.
func loadTables(src intl.Source) (map[string][]eastasian.TableYear, error) {
	b, err := src.Open(intl.MarkerTemporalCalendars, intl.DataLocale{})
	if err != nil {
		return nil, fmt.Errorf("temporal: the calendar tables: %w", err)
	}
	tables, err := eastasian.ParseTables(b, "china", "korea", "qing", "ummalqura")
	if err != nil {
		return nil, fmt.Errorf("temporal: %w", err)
	}
	return tables, nil
}

// tableYearsOf is a table as the solar calendars read it.
func tableYearsOf(table []eastasian.TableYear) []tableYear {
	out := make([]tableYear, len(table))
	for i, t := range table {
		out[i] = tableYear{year: t.Year, leap: t.Leap, count: t.Count, long: t.Long, start: t.Start}
	}
	return out
}

// A tableYear is a year of ICU4X's tables: which months are long, the
// ordinal of the leap month, and the first day.
type tableYear struct {
	year, leap, count int
	long              uint16
	start             int64
}

// lookup is the table's year, false where the table does not reach.
func lookup(table []tableYear, year int) (tableYear, bool) {
	if len(table) == 0 {
		return tableYear{}, false
	}
	i := year - table[0].year
	if i < 0 || i >= len(table) {
		return tableYear{}, false
	}
	return table[i], true
}
