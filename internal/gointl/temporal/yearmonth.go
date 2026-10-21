package temporal

// A PlainYearMonth is Temporal.PlainYearMonth's value: an ISO date, the
// month's reference day, and a calendar.
type PlainYearMonth struct {
	iso ISODate
	cal *Calendar
}

// ISO is the reference date in the ISO calendar.
func (ym PlainYearMonth) ISO() ISODate { return ym.iso }

// Calendar is the calendar.
func (ym PlainYearMonth) Calendar() *Calendar { return ym.cal }

// NewPlainYearMonth is PlainYearMonth::new_with_overflow: an ISO year and
// month, the reference day 1 where none is given.
func NewPlainYearMonth(year, month int, referenceDay *int, cal *Calendar, overflow Overflow) (PlainYearMonth, error) {
	day := 1
	if referenceDay != nil {
		day = *referenceDay
	}
	iso, err := regulateISODateRS(year, month, day, overflow)
	if err != nil {
		return PlainYearMonth{}, err
	}
	if !yearMonthWithinLimits(iso.Year, iso.Month) {
		return PlainYearMonth{}, rangeError("Exceeded valid range.")
	}
	return PlainYearMonth{iso, cal}, nil
}

// yearMonthFromFields is Calendar::year_month_from_fields.
func (c *Calendar) yearMonthFromFields(f CalendarFields, overflow Overflow) (PlainYearMonth, error) {
	iso, err := c.YearMonthFromFields(f, overflow)
	if err != nil {
		return PlainYearMonth{}, err
	}
	return PlainYearMonth{iso, c}, nil
}

// PlainYearMonthFromFields is PlainYearMonth::from_partial.
func PlainYearMonthFromFields(f CalendarFields, cal *Calendar, overflow Overflow) (PlainYearMonth, error) {
	f.Day = nil
	return cal.yearMonthFromFields(f, overflow)
}

// fields is YearMonthCalendarFields::try_from_year_month: the era and the
// year in it where the calendar has eras, else the year.
func (ym PlainYearMonth) fields() CalendarFields {
	f := CalendarFields{Month: Int(ym.Month()), MonthCode: String(ym.MonthCode())}
	if e, ok := ym.Era(); ok {
		f.Era = &e
		if y, ok := ym.EraYear(); ok {
			f.EraYear = &y
		}
	} else {
		f.Year = Int(ym.Year())
	}
	return f
}

// With is Temporal.PlainYearMonth.prototype.with.
func (ym PlainYearMonth) With(f CalendarFields, overflow Overflow) (PlainYearMonth, error) {
	f.Day = nil
	if f.isEmpty() {
		return PlainYearMonth{}, typeError("fields cannot be empty")
	}
	return ym.cal.yearMonthFromFields(f.withFallback(ym.cal, ym.iso, false), overflow)
}

// Compare is Temporal.PlainYearMonth.compare.
func (ym PlainYearMonth) Compare(o PlainYearMonth) int { return ym.iso.compare(o.iso) }

// Equals is Temporal.PlainYearMonth.prototype.equals.
func (ym PlainYearMonth) Equals(o PlainYearMonth) bool { return ym.iso == o.iso && ym.cal.Equal(o.cal) }

// Add is Temporal.PlainYearMonth.prototype.add.
func (ym PlainYearMonth) Add(d Duration, overflow Overflow) (PlainYearMonth, error) {
	i := d.internal()
	if i.date.Weeks != 0 || i.date.Days != 0 || !i.time.isZero() {
		return PlainYearMonth{}, rangeError("Can only add years or months to PlainYearMonth.")
	}
	f := ym.fields()
	f.Day = Int(1)
	date, err := ym.cal.dateFromFields(f, Constrain)
	if err != nil {
		return PlainYearMonth{}, err
	}
	added, err := ym.cal.dateAdd(date.iso, i.date, overflow)
	if err != nil {
		return PlainYearMonth{}, err
	}
	return ym.cal.yearMonthFromFields(CalendarFields{Year: Int(added.Year()), MonthCode: String(added.MonthCode())}, overflow)
}

// Subtract is Temporal.PlainYearMonth.prototype.subtract.
func (ym PlainYearMonth) Subtract(d Duration, overflow Overflow) (PlainYearMonth, error) {
	return ym.Add(d.Negated(), overflow)
}

// Until is Temporal.PlainYearMonth.prototype.until.
func (ym PlainYearMonth) Until(o PlainYearMonth, s DifferenceSettings) (Duration, error) {
	return ym.diff(opUntil, o, s)
}

// Since is Temporal.PlainYearMonth.prototype.since.
func (ym PlainYearMonth) Since(o PlainYearMonth, s DifferenceSettings) (Duration, error) {
	return ym.diff(opSince, o, s)
}

func (ym PlainYearMonth) diff(op differenceOp, o PlainYearMonth, s DifferenceSettings) (Duration, error) {
	if ym.cal.id != o.cal.id {
		return Duration{}, rangeError("Calendars for difference operation are not the same.")
	}
	if s.LargestUnit == Week || s.LargestUnit == Day || s.SmallestUnit == Week || s.SmallestUnit == Day {
		return Duration{}, rangeError("Weeks and days are not allowed in this operation.")
	}
	r, err := fromDiffSettings(s, op, groupDate, Year, Month)
	if err != nil {
		return Duration{}, err
	}
	if ym.iso == o.iso {
		return Duration{}, nil
	}
	tf := ym.fields()
	tf.Day = Int(1)
	thisDate, err := ym.cal.dateFromFields(tf, Constrain)
	if err != nil {
		return Duration{}, err
	}
	of := o.fields()
	of.Day = Int(1)
	otherDate, err := ym.cal.dateFromFields(of, Constrain)
	if err != nil {
		return Duration{}, err
	}
	dd, err := ym.cal.dateUntil(thisDate.iso, otherDate.iso, r.largest)
	if err != nil {
		return Duration{}, err
	}
	zero := int64(0)
	id := internalDuration{date: dd.adjust(0, &zero, nil)}
	if r.smallest != Month || r.increment.get() != 1 {
		dt := ISODateTime{Date: thisDate.iso}
		dest := epochNanoseconds(otherDate.iso, ISOTime{})
		if id, err = id.roundRelative(dt.epochNanoseconds(), dest, PlainDateTime{dt, ym.cal}, nil, r); err != nil {
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

// The year and month as the calendar reckons them.
func (ym PlainYearMonth) Year() int            { return ym.cal.year(ym.iso) }
func (ym PlainYearMonth) Month() int           { return ym.cal.month(ym.iso) }
func (ym PlainYearMonth) MonthCode() string    { return ym.cal.monthCode(ym.iso) }
func (ym PlainYearMonth) ReferenceDay() int    { return ym.cal.day(ym.iso) }
func (ym PlainYearMonth) Era() (string, bool)  { return ym.cal.era(ym.iso) }
func (ym PlainYearMonth) EraYear() (int, bool) { return ym.cal.eraYear(ym.iso) }
func (ym PlainYearMonth) DaysInMonth() int     { return ym.cal.date(ym.iso).DaysInMonth }
func (ym PlainYearMonth) DaysInYear() int      { return ym.cal.date(ym.iso).DaysInYear }
func (ym PlainYearMonth) InLeapYear() bool     { return ym.cal.date(ym.iso).InLeapYear }
func (ym PlainYearMonth) MonthsInYear() int {
	if ym.cal.isISO() {
		return 12
	}
	return ym.cal.date(ym.iso).MonthsInYear
}

// ToPlainDate is Temporal.PlainYearMonth.prototype.toPlainDate, with the
// day V8 has read, which it must have.
func (ym PlainYearMonth) ToPlainDate(day *int) (PlainDate, error) {
	if day == nil {
		return PlainDate{}, typeError("CalendarFields must contain a day field")
	}
	return ym.cal.dateFromFields(CalendarFields{Year: Int(ym.Year()), MonthCode: String(ym.MonthCode()), Day: day}, Constrain)
}

// String is Temporal.PlainYearMonth.prototype.toString.
func (ym PlainYearMonth) String(show DisplayCalendar) string {
	var b ixdtfBuilder
	writeYear(&b.Builder, int32(ym.iso.Year))
	b.WriteByte('-')
	writePadded2(&b.Builder, ym.iso.Month)
	if show == CalendarAlways || show == CalendarCritical || ym.cal.id != "iso8601" {
		b.WriteByte('-')
		writePadded2(&b.Builder, ym.iso.Day)
	}
	b.calendar(ym.cal.id, show)
	return b.String()
}

// EpochNanosecondsForUTC is the reference date's noon read as UTC, as a
// high and low word as Instant.EpochNanoseconds gives them.
func (ym PlainYearMonth) EpochNanosecondsForUTC() (hi int64, lo uint64) {
	ns := epochNanoseconds(ym.iso, noon)
	return ns.hi, ns.lo
}

// A PlainMonthDay is Temporal.PlainMonthDay's value: an ISO date in the
// reference year, and a calendar.
type PlainMonthDay struct {
	iso ISODate
	cal *Calendar
}

// ISO is the date in the reference year.
func (md PlainMonthDay) ISO() ISODate { return md.iso }

// Calendar is the calendar.
func (md PlainMonthDay) Calendar() *Calendar { return md.cal }

// NewPlainMonthDay is PlainMonthDay::new_with_overflow: an ISO month and
// day, in 1972 where no reference year is given.
func NewPlainMonthDay(month, day int, cal *Calendar, overflow Overflow, referenceYear *int) (PlainMonthDay, error) {
	year := 1972
	if referenceYear != nil {
		year = *referenceYear
	}
	iso, err := newISODateWithOverflow(year, month, day, overflow)
	if err != nil {
		return PlainMonthDay{}, err
	}
	return PlainMonthDay{iso, cal}, nil
}

// monthDayFromFields is Calendar::month_day_from_fields.
func (c *Calendar) monthDayFromFields(f CalendarFields, overflow Overflow) (PlainMonthDay, error) {
	iso, err := c.MonthDayFromFields(f, overflow)
	if err != nil {
		return PlainMonthDay{}, err
	}
	return PlainMonthDay{iso, c}, nil
}

// PlainMonthDayFromFields is PlainMonthDay::from_partial.
func PlainMonthDayFromFields(f CalendarFields, cal *Calendar, overflow Overflow) (PlainMonthDay, error) {
	return cal.monthDayFromFields(f, overflow)
}

// With is Temporal.PlainMonthDay.prototype.with.
func (md PlainMonthDay) With(f CalendarFields, overflow Overflow) (PlainMonthDay, error) {
	if f.isEmpty() {
		return PlainMonthDay{}, typeError("fields cannot be empty")
	}
	if f.Day == nil {
		f.Day = Int(md.Day())
	}
	if f.Month == nil && f.MonthCode == nil {
		f.MonthCode = String(md.MonthCode())
	}
	return md.cal.monthDayFromFields(f, overflow)
}

// Equals is Temporal.PlainMonthDay.prototype.equals.
func (md PlainMonthDay) Equals(o PlainMonthDay) bool { return md.iso == o.iso && md.cal.Equal(o.cal) }

// The month and day as the calendar reckons them.
func (md PlainMonthDay) MonthCode() string  { return md.cal.monthCode(md.iso) }
func (md PlainMonthDay) Day() int           { return md.cal.day(md.iso) }
func (md PlainMonthDay) ReferenceYear() int { return md.cal.year(md.iso) }

// ToPlainDate is Temporal.PlainMonthDay.prototype.toPlainDate, with the
// year fields V8 has read: the year, else the era and the year in it.
func (md PlainMonthDay) ToPlainDate(year *int, era *string, eraYear *int) (PlainDate, error) {
	f := CalendarFields{MonthCode: String(md.MonthCode()), Day: Int(md.Day())}
	switch {
	case year != nil:
		f.Year = year
	case era != nil && eraYear != nil:
		f.Era, f.EraYear = era, eraYear
	default:
		return PlainDate{}, typeError("PartialDate must contain a year or era/era_year fields")
	}
	return md.cal.dateFromFields(f, Constrain)
}

// String is Temporal.PlainMonthDay.prototype.toString.
func (md PlainMonthDay) String(show DisplayCalendar) string {
	var b ixdtfBuilder
	if show == CalendarAlways || show == CalendarCritical || md.cal.id != "iso8601" {
		writeYear(&b.Builder, int32(md.iso.Year))
		b.WriteByte('-')
	}
	writePadded2(&b.Builder, md.iso.Month)
	b.WriteByte('-')
	writePadded2(&b.Builder, md.iso.Day)
	b.calendar(md.cal.id, show)
	return b.String()
}

// EpochNanosecondsForUTC is the reference date's noon read as UTC, as a
// high and low word as Instant.EpochNanoseconds gives them.
func (md PlainMonthDay) EpochNanosecondsForUTC() (hi int64, lo uint64) {
	ns := epochNanoseconds(md.iso, noon)
	return ns.hi, ns.lo
}
