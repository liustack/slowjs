package temporal

import (
	"fmt"
	"math"
)

// ISOCalendar is the ISO 8601 calendar, which needs no data.
func ISOCalendar() *Calendar { return isoCalendar }

// isoCalendar is ISOCalendar's calendar, which, like every Calendar, never
// changes.
var isoCalendar = &Calendar{id: "iso8601", r: gregorianRules{eras: isoEras}}

func (c *Calendar) isISO() bool { return c.id == "iso8601" }

// hasEras is Calendar::calendar_has_eras.
func (c *Calendar) hasEras() bool { return c.id != "iso8601" && c.id != "chinese" && c.id != "dangi" }

// HasEras reports whether the calendar has eras, whose era and eraYear
// fields an engine reads from a property bag (PrepareCalendarFields).
func (c *Calendar) HasEras() bool { return c.hasEras() }

// Equal is whether two calendars are the same, by identifier.
func (c *Calendar) Equal(o *Calendar) bool { return c.id == o.id }

// The calendar's reading of an ISO date, as temporal_rs's Calendar gives
// each field.

func (c *Calendar) fieldsOf(d ISODate) CalendarDate { return c.date(d) }

func (c *Calendar) year(d ISODate) int {
	if c.isISO() {
		return d.Year
	}
	return c.date(d).Year
}

func (c *Calendar) month(d ISODate) int {
	if c.isISO() {
		return d.Month
	}
	return c.date(d).Month
}

func (c *Calendar) monthCode(d ISODate) string { return c.date(d).MonthCode }

func (c *Calendar) day(d ISODate) int {
	if c.isISO() {
		return d.Day
	}
	return c.date(d).Day
}

// era is the era's code, false for a calendar without eras.
func (c *Calendar) era(d ISODate) (string, bool) {
	if c.isISO() {
		return "", false
	}
	e := c.date(d).Era
	return e, e != ""
}

func (c *Calendar) eraYear(d ISODate) (int, bool) {
	if c.isISO() {
		return 0, false
	}
	f := c.date(d)
	return f.EraYear, f.Era != ""
}

// isoWeekday is the ISO weekday of a date, Monday 1 to Sunday 7.
func isoWeekday(d ISODate) int {
	return int(floorMod(d.rataDie()-1, 7)) + 1
}

// isoWeekOfYear is ICU4X's ISO week: the week number and the year it
// belongs to.
func isoWeekOfYear(d ISODate) (week, year int) {
	dayOfYear := int(d.rataDie() - gregorianFixed(d.Year, 1, 1) + 1)
	weekday := isoWeekday(d)
	w := (dayOfYear - weekday + 10) / 7
	year = d.Year
	switch {
	case w < 1:
		year--
		w = isoWeeksInYear(year)
	case w > isoWeeksInYear(year):
		year++
		w = 1
	}
	return w, year
}

// isoWeeksInYear is 53 for a year starting on a Thursday, or a leap year
// starting on a Wednesday, else 52.
func isoWeeksInYear(year int) int {
	jan1 := isoWeekday(ISODate{year, 1, 1})
	if jan1 == 4 || jan1 == 3 && gregorianLeap(year) {
		return 53
	}
	return 52
}

// A PlainDate is Temporal.PlainDate's value: an ISO date and a calendar.
type PlainDate struct {
	iso ISODate
	cal *Calendar
}

// ISO is the date in the ISO calendar.
func (d PlainDate) ISO() ISODate { return d.iso }

// Calendar is the date's calendar.
func (d PlainDate) Calendar() *Calendar { return d.cal }

// NewPlainDate is PlainDate::new_with_overflow: an ISO date, regulated,
// within Temporal's range.
func NewPlainDate(year, month, day int, cal *Calendar, overflow Overflow) (PlainDate, error) {
	return newPlainDate(year, month, day, cal, overflow)
}

func newPlainDate(year, month, day int, cal *Calendar, overflow Overflow) (PlainDate, error) {
	iso, err := newISODateWithOverflow(year, month, day, overflow)
	if err != nil {
		return PlainDate{}, err
	}
	return PlainDate{iso, cal}, nil
}

// newISODateWithOverflow is IsoDate::new_with_overflow, with its messages.
func newISODateWithOverflow(year, month, day int, overflow Overflow) (ISODate, error) {
	d, err := regulateISODateRS(year, month, day, overflow)
	if err != nil {
		return ISODate{}, err
	}
	if err := d.checkWithinLimits(); err != nil {
		return ISODate{}, err
	}
	return d, nil
}

// regulateISODateRS is IsoDate::regulate over u8 fields.
func regulateISODateRS(year, month, day int, overflow Overflow) (ISODate, error) {
	// temporal_rs holds a year in an i32, and a year outside it is no ISO
	// date at all, before the limits are asked: 4294969316 had wrapped to
	// 2020.
	if year < math.MinInt32 || year > math.MaxInt32 {
		return ISODate{}, rangeError("Invalid ISO date.")
	}
	if overflow == Constrain {
		month = clamp(month, 1, 12)
		return ISODate{year, month, clamp(day, 1, gregorianMonthLength(year, month))}, nil
	}
	if !validISODate(year, month, day) {
		return ISODate{}, rangeError("not a valid ISO date.")
	}
	return ISODate{year, month, day}, nil
}

// PlainDateFromFields is PlainDate::from_partial: a date from a
// calendar's fields.
func PlainDateFromFields(f CalendarFields, cal *Calendar, overflow Overflow) (PlainDate, error) {
	if f.Year == nil && (f.Era == nil || f.EraYear == nil) || f.Month == nil && f.MonthCode == nil || f.Day == nil {
		return PlainDate{}, typeError("Invalid PlainDate fields provided.")
	}
	return cal.dateFromFields(f, overflow)
}

// dateFromFields is Calendar::date_from_fields.
func (c *Calendar) dateFromFields(f CalendarFields, overflow Overflow) (PlainDate, error) {
	iso, err := c.DateFromFields(f, overflow)
	if err != nil {
		return PlainDate{}, err
	}
	return PlainDate{iso, c}, nil
}

// fieldKeysToIgnore is field_keys_to_ignore: which of the date's fields
// the given ones replace.
func (f CalendarFields) fieldKeysToIgnore(cal *Calendar, withDay bool) (era, arithYear, month bool) {
	month = f.Month != nil || f.MonthCode != nil
	if cal.hasEras() {
		if f.Year != nil || f.EraYear != nil || f.Era != nil {
			era, arithYear = true, true
		}
		if cal.id == "japanese" && (month || withDay && f.Day != nil) {
			era = true
		}
	}
	return
}

// withFallback is with_fallback_date: the fields given, and the date's
// own for the rest.
func (f CalendarFields) withFallback(cal *Calendar, iso ISODate, withDay bool) CalendarFields {
	ignoreEra, ignoreYear, ignoreMonth := f.fieldKeysToIgnore(cal, withDay)
	out := f
	if !ignoreEra {
		if out.Era == nil {
			if e, ok := cal.era(iso); ok {
				out.Era = &e
			}
		}
		if out.EraYear == nil {
			if y, ok := cal.eraYear(iso); ok {
				out.EraYear = &y
			}
		}
	}
	if !ignoreYear && out.Year == nil {
		out.Year = Int(cal.year(iso))
	}
	if f.Month == nil && f.MonthCode == nil && !ignoreMonth {
		out.MonthCode = String(cal.monthCode(iso))
	}
	if withDay && out.Day == nil {
		out.Day = Int(cal.day(iso))
	}
	if !withDay {
		out.Day = nil
	}
	return out
}

func (f CalendarFields) isEmpty() bool {
	return f.Era == nil && f.EraYear == nil && f.Year == nil && f.Month == nil && f.MonthCode == nil && f.Day == nil
}

// With is Temporal.PlainDate.prototype.with.
func (d PlainDate) With(f CalendarFields, overflow Overflow) (PlainDate, error) {
	if f.isEmpty() {
		return PlainDate{}, typeError("fields cannot be empty")
	}
	return d.cal.dateFromFields(f.withFallback(d.cal, d.iso, true), overflow)
}

// WithCalendar is Temporal.PlainDate.prototype.withCalendar.
func (d PlainDate) WithCalendar(c *Calendar) PlainDate { return PlainDate{d.iso, c} }

// Compare is Temporal.PlainDate.compare, by ISO date alone.
func (d PlainDate) Compare(o PlainDate) int { return d.iso.compare(o.iso) }

// Equals is Temporal.PlainDate.prototype.equals.
func (d PlainDate) Equals(o PlainDate) bool { return d.iso == o.iso && d.cal.Equal(o.cal) }

// Add is Temporal.PlainDate.prototype.add.
func (d PlainDate) Add(dur Duration, overflow Overflow) (PlainDate, error) {
	dd, err := dur.dateDurationWithoutTime()
	if err != nil {
		return PlainDate{}, err
	}
	return d.cal.dateAdd(d.iso, dd, overflow)
}

// Subtract is Temporal.PlainDate.prototype.subtract.
func (d PlainDate) Subtract(dur Duration, overflow Overflow) (PlainDate, error) {
	return d.Add(dur.Negated(), overflow)
}

// dateAdd is Calendar::date_add.
func (c *Calendar) dateAdd(iso ISODate, dd DateDuration, overflow Overflow) (PlainDate, error) {
	r, err := c.DateAdd(iso, dd, overflow)
	if err != nil {
		return PlainDate{}, err
	}
	return PlainDate{r, c}, nil
}

// dateUntil is Calendar::date_until.
func (c *Calendar) dateUntil(one, two ISODate, largest Unit) (DateDuration, error) {
	d, err := c.DateUntil(one, two, largest)
	if err != nil {
		return DateDuration{}, err
	}
	return newDateDuration(d.Years, d.Months, d.Weeks, d.Days)
}

// diffDate is internal_diff_date.
func (d PlainDate) diffDate(o PlainDate, largest Unit) (DateDuration, error) {
	if d.iso == o.iso {
		return DateDuration{}, nil
	}
	if largest == Day {
		return newDateDuration(0, 0, 0, int64(int32(o.iso.epochDays()-d.iso.epochDays())))
	}
	return d.cal.dateUntil(d.iso, o.iso, largest)
}

// Until is Temporal.PlainDate.prototype.until.
func (d PlainDate) Until(o PlainDate, s DifferenceSettings) (Duration, error) {
	return d.diff(opUntil, o, s)
}

// Since is Temporal.PlainDate.prototype.since.
func (d PlainDate) Since(o PlainDate, s DifferenceSettings) (Duration, error) {
	return d.diff(opSince, o, s)
}

func (d PlainDate) diff(op differenceOp, o PlainDate, s DifferenceSettings) (Duration, error) {
	if d.cal.id != o.cal.id {
		return Duration{}, rangeError("Calendars are for difference operation are not the same.")
	}
	r, err := fromDiffSettings(s, op, groupDate, Day, Day)
	if err != nil {
		return Duration{}, err
	}
	if d.iso == o.iso {
		return Duration{}, nil
	}
	dd, err := d.diffDate(o, r.largest)
	if err != nil {
		return Duration{}, err
	}
	id := internalDuration{date: dd}
	if !(r.smallest == Day && r.increment.get() == 1) {
		dt := ISODateTime{Date: d.iso}
		origin := dt.epochNanoseconds()
		dest := epochNanoseconds(o.iso, ISOTime{})
		if id, err = id.roundRelative(origin, dest, PlainDateTime{dt, d.cal}, nil, r); err != nil {
			return Duration{}, err
		}
	}
	out, err := durationFromInternal(id, Day)
	if err != nil {
		return Duration{}, err
	}
	if op == opSince {
		out = out.Negated()
	}
	return out, nil
}

// The date's fields as its calendar reckons them.
func (d PlainDate) Year() int         { return d.cal.year(d.iso) }
func (d PlainDate) Month() int        { return d.cal.month(d.iso) }
func (d PlainDate) MonthCode() string { return d.cal.monthCode(d.iso) }
func (d PlainDate) Day() int          { return d.cal.day(d.iso) }
func (d PlainDate) DayOfWeek() int    { return isoWeekday(d.iso) }
func (d PlainDate) DayOfYear() int    { return d.cal.date(d.iso).DayOfYear }
func (d PlainDate) DaysInWeek() int   { return 7 }
func (d PlainDate) DaysInMonth() int  { return d.cal.date(d.iso).DaysInMonth }
func (d PlainDate) DaysInYear() int   { return d.cal.date(d.iso).DaysInYear }
func (d PlainDate) MonthsInYear() int {
	if d.cal.isISO() {
		return 12
	}
	return d.cal.date(d.iso).MonthsInYear
}
func (d PlainDate) InLeapYear() bool { return d.cal.date(d.iso).InLeapYear }

// WeekOfYear is the ISO week number, false outside the ISO calendar.
func (d PlainDate) WeekOfYear() (int, bool) {
	if !d.cal.isISO() {
		return 0, false
	}
	w, _ := isoWeekOfYear(d.iso)
	return w, true
}

// YearOfWeek is the year the ISO week belongs to, false outside the ISO
// calendar.
func (d PlainDate) YearOfWeek() (int, bool) {
	if !d.cal.isISO() {
		return 0, false
	}
	_, y := isoWeekOfYear(d.iso)
	return y, true
}

// Era is the era's code, false for a calendar without eras.
func (d PlainDate) Era() (string, bool) { return d.cal.era(d.iso) }

// EraYear is the year in the era, false for a calendar without eras.
func (d PlainDate) EraYear() (int, bool) { return d.cal.eraYear(d.iso) }

// ToPlainDateTime is Temporal.PlainDate.prototype.toPlainDateTime; a nil
// time is midnight.
func (d PlainDate) ToPlainDateTime(t *PlainTime) (PlainDateTime, error) {
	var iso ISOTime
	if t != nil {
		iso = t.iso
	}
	dt, err := newISODateTime(d.iso, iso)
	if err != nil {
		return PlainDateTime{}, err
	}
	return PlainDateTime{dt, d.cal}, nil
}

// ToPlainYearMonth is Temporal.PlainDate.prototype.toPlainYearMonth.
func (d PlainDate) ToPlainYearMonth() (PlainYearMonth, error) {
	f := CalendarFields{Year: Int(d.Year()), Month: Int(d.Month()), MonthCode: String(d.MonthCode())}
	if e, ok := d.Era(); ok {
		f.Era = &e
	}
	if y, ok := d.EraYear(); ok {
		f.EraYear = &y
	}
	return d.cal.yearMonthFromFields(f, Constrain)
}

// ToPlainMonthDay is Temporal.PlainDate.prototype.toPlainMonthDay.
func (d PlainDate) ToPlainMonthDay() (PlainMonthDay, error) {
	return d.cal.monthDayFromFields(CalendarFields{}.withFallback(d.cal, d.iso, true), Constrain)
}

// String is Temporal.PlainDate.prototype.toString.
func (d PlainDate) String(show DisplayCalendar) string {
	var b ixdtfBuilder
	b.date(d.iso)
	b.calendar(d.cal.id, show)
	return b.String()
}

// EpochNanosecondsForUTC is the date's noon read as UTC, as
// Intl.DateTimeFormat formats a PlainDate, as a high and low word as
// Instant.EpochNanoseconds gives them.
func (d PlainDate) EpochNanosecondsForUTC() (hi int64, lo uint64) {
	ns := epochNanoseconds(d.iso, noon)
	return ns.hi, ns.lo
}

func (d PlainDate) GoString() string { return fmt.Sprintf("PlainDate(%s)", d.String(CalendarAlways)) }

// ToZonedDateTime is Temporal.PlainDate.prototype.toZonedDateTime: the date
// at a time in a zone, or at the start of its day in the zone for a nil
// time.
func (d PlainDate) ToZonedDateTime(tz TimeZone, t *PlainTime) (ZonedDateTime, error) {
	var e epochAndOffset
	var err error
	if t != nil {
		var dt ISODateTime
		if dt, err = newISODateTime(d.iso, t.iso); err != nil {
			return ZonedDateTime{}, err
		}
		e, err = tz.epochNanosecondsFor(dt, Compatible)
	} else {
		e, err = tz.startOfDay(d.iso)
	}
	if err != nil {
		return ZonedDateTime{}, err
	}
	return newZonedDateTimeCached(e.ns, tz, d.cal, e.offset)
}
