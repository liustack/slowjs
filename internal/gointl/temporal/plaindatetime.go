package temporal

// A PlainDateTime is Temporal.PlainDateTime's value: an ISO date and time,
// and a calendar.
type PlainDateTime struct {
	iso ISODateTime
	cal *Calendar
}

// ISO is the date and time in the ISO calendar.
func (dt PlainDateTime) ISO() ISODateTime { return dt.iso }

// Calendar is the calendar.
func (dt PlainDateTime) Calendar() *Calendar { return dt.cal }

// NewPlainDateTime is PlainDateTime::new_with_overflow.
func NewPlainDateTime(date ISODate, t ISOTime, cal *Calendar, overflow Overflow) (PlainDateTime, error) {
	d, err := newISODateWithOverflow(date.Year, date.Month, date.Day, overflow)
	if err != nil {
		return PlainDateTime{}, err
	}
	tt, err := newISOTime(t.Hour, t.Minute, t.Second, t.Millisecond, t.Microsecond, t.Nanosecond, overflow)
	if err != nil {
		return PlainDateTime{}, err
	}
	iso, err := newISODateTime(d, tt)
	return PlainDateTime{iso, cal}, err
}

// PlainDateTimeFromDateAndTime is PlainDateTime::from_date_and_time.
func PlainDateTimeFromDateAndTime(d PlainDate, t PlainTime) (PlainDateTime, error) {
	iso, err := newISODateTime(d.iso, t.iso)
	return PlainDateTime{iso, d.cal}, err
}

// PlainDateTimeFromFields is PlainDateTime::from_partial.
func PlainDateTimeFromFields(f CalendarFields, t PartialTime, cal *Calendar, overflow Overflow) (PlainDateTime, error) {
	if f.isEmpty() && t.isEmpty() {
		return PlainDateTime{}, typeError("PartialDateTime cannot be empty.")
	}
	d, err := cal.dateFromFields(f, overflow)
	if err != nil {
		return PlainDateTime{}, err
	}
	tt, err := ISOTime{}.with(t, overflow)
	if err != nil {
		return PlainDateTime{}, err
	}
	iso, err := newISODateTime(d.iso, tt)
	return PlainDateTime{iso, cal}, err
}

// With is Temporal.PlainDateTime.prototype.with.
func (dt PlainDateTime) With(f CalendarFields, t PartialTime, overflow Overflow) (PlainDateTime, error) {
	if f.isEmpty() && t.isEmpty() {
		return PlainDateTime{}, typeError("fields cannot be empty")
	}
	d, err := dt.cal.dateFromFields(f.withFallback(dt.cal, dt.iso.Date, true), overflow)
	if err != nil {
		return PlainDateTime{}, err
	}
	tt, err := dt.iso.Time.with(t, overflow)
	if err != nil {
		return PlainDateTime{}, err
	}
	iso, err := newISODateTime(d.iso, tt)
	return PlainDateTime{iso, dt.cal}, err
}

// WithPlainTime is Temporal.PlainDateTime.prototype.withPlainTime; a nil
// time is midnight.
func (dt PlainDateTime) WithPlainTime(t *PlainTime) (PlainDateTime, error) {
	var tt ISOTime
	if t != nil {
		tt = t.iso
	}
	return NewPlainDateTime(dt.iso.Date, tt, dt.cal, Reject)
}

// WithCalendar is Temporal.PlainDateTime.prototype.withCalendar.
func (dt PlainDateTime) WithCalendar(c *Calendar) PlainDateTime { return PlainDateTime{dt.iso, c} }

// Compare is Temporal.PlainDateTime.compare.
func (dt PlainDateTime) Compare(o PlainDateTime) int { return dt.iso.compare(o.iso) }

// Equals is Temporal.PlainDateTime.prototype.equals.
func (dt PlainDateTime) Equals(o PlainDateTime) bool { return dt.iso == o.iso && dt.cal.Equal(o.cal) }

// Add is Temporal.PlainDateTime.prototype.add.
func (dt PlainDateTime) Add(d Duration, overflow Overflow) (PlainDateTime, error) {
	i, err := d.internalWith24HourDays()
	if err != nil {
		return PlainDateTime{}, err
	}
	days, t := dt.iso.Time.add(i.time)
	added, err := dt.cal.dateAdd(dt.iso.Date, i.date.adjust(days, nil, nil), overflow)
	if err != nil {
		return PlainDateTime{}, err
	}
	iso, err := newISODateTime(added.iso, t)
	return PlainDateTime{iso, dt.cal}, err
}

// Subtract is Temporal.PlainDateTime.prototype.subtract.
func (dt PlainDateTime) Subtract(d Duration, overflow Overflow) (PlainDateTime, error) {
	return dt.Add(d.Negated(), overflow)
}

// Until is Temporal.PlainDateTime.prototype.until.
func (dt PlainDateTime) Until(o PlainDateTime, s DifferenceSettings) (Duration, error) {
	return dt.diff(opUntil, o, s)
}

// Since is Temporal.PlainDateTime.prototype.since.
func (dt PlainDateTime) Since(o PlainDateTime, s DifferenceSettings) (Duration, error) {
	return dt.diff(opSince, o, s)
}

func (dt PlainDateTime) diff(op differenceOp, o PlainDateTime, s DifferenceSettings) (Duration, error) {
	if !dt.cal.Equal(o.cal) {
		return Duration{}, rangeError("Calendar must be the same when diffing two PlainDateTimes")
	}
	r, err := fromDiffSettings(s, op, groupDateTime, Day, Nanosecond)
	if err != nil {
		return Duration{}, err
	}
	if dt.iso == o.iso {
		return Duration{}, nil
	}
	i, err := dt.diffWithRounding(o, r)
	if err != nil {
		return Duration{}, err
	}
	d, err := durationFromInternal(i, r.largest)
	if err != nil {
		return Duration{}, err
	}
	if op == opSince {
		d = d.Negated()
	}
	return d, nil
}

// diffWithRounding is DifferencePlainDateTimeWithRounding.
func (dt PlainDateTime) diffWithRounding(o PlainDateTime, r resolvedRounding) (internalDuration, error) {
	if dt.iso.compare(o.iso) == 0 {
		return internalDuration{}, nil
	}
	if err := dt.iso.checkWithinLimits(); err != nil {
		return internalDuration{}, err
	}
	if err := o.iso.checkWithinLimits(); err != nil {
		return internalDuration{}, err
	}
	diff, err := dt.iso.diff(o.iso, dt.cal, r.largest)
	if err != nil {
		return internalDuration{}, err
	}
	if r.smallest == Nanosecond && r.increment.get() == 1 {
		return diff, nil
	}
	return diff.roundRelative(dt.iso.epochNanoseconds(), o.iso.epochNanoseconds(), dt, nil, r)
}

// diffWithTotal is DifferencePlainDateTimeWithTotal.
func (dt PlainDateTime) diffWithTotal(o PlainDateTime, u Unit) (float64, error) {
	if dt.iso.compare(o.iso) == 0 {
		return 0, nil
	}
	if !dt.iso.withinLimits() || !o.iso.withinLimits() {
		return 0, rangeError("DateTime is not within valid limits.")
	}
	diff, err := dt.iso.diff(o.iso, dt.cal, u)
	if err != nil {
		return 0, err
	}
	if u == Nanosecond {
		return diff.time.float64(), nil
	}
	return diff.totalRelative(dt.iso.epochNanoseconds(), o.iso.epochNanoseconds(), dt, nil, u)
}

// Round is Temporal.PlainDateTime.prototype.round.
func (dt PlainDateTime) Round(o RoundingOptions) (PlainDateTime, error) {
	r, err := fromDateTimeOptions(o)
	if err != nil {
		return PlainDateTime{}, err
	}
	if r.isNoop() {
		return dt, nil
	}
	iso, err := dt.iso.round(r)
	return PlainDateTime{iso, dt.cal}, err
}

// ToPlainDate is Temporal.PlainDateTime.prototype.toPlainDate.
func (dt PlainDateTime) ToPlainDate() PlainDate { return PlainDate{dt.iso.Date, dt.cal} }

// ToPlainTime is Temporal.PlainDateTime.prototype.toPlainTime.
func (dt PlainDateTime) ToPlainTime() PlainTime { return PlainTime{dt.iso.Time} }

// String is Temporal.PlainDateTime.prototype.toString.
func (dt PlainDateTime) String(o ToStringRoundingOptions, show DisplayCalendar) (string, error) {
	r, err := o.resolve()
	if err != nil {
		return "", err
	}
	iso, err := dt.iso.round(fromToStringOptions(r))
	if err != nil {
		return "", err
	}
	if !iso.withinLimits() {
		return "", rangeError("DateTime is not within valid limits.")
	}
	var b ixdtfBuilder
	b.date(iso.Date)
	b.time(iso.Time, r.precision)
	b.calendar(dt.cal.id, show)
	return b.String(), nil
}

// EpochNanosecondsForUTC is the date and time read as UTC, as
// Intl.DateTimeFormat formats a PlainDateTime, as a high and low word as
// Instant.EpochNanoseconds gives them.
func (dt PlainDateTime) EpochNanosecondsForUTC() (hi int64, lo uint64) {
	ns := dt.iso.epochNanoseconds()
	return ns.hi, ns.lo
}

// The date's fields as its calendar reckons them.
func (dt PlainDateTime) date() PlainDate { return PlainDate{dt.iso.Date, dt.cal} }

// ToZonedDateTime is Temporal.PlainDateTime.prototype.toZonedDateTime.
func (dt PlainDateTime) ToZonedDateTime(tz TimeZone, d Disambiguation) (ZonedDateTime, error) {
	e, err := tz.epochNanosecondsFor(dt.iso, d)
	if err != nil {
		return ZonedDateTime{}, err
	}
	return ZonedDateTime{Instant{e.ns}, tz, dt.cal, e.offset * 1_000_000_000}, nil
}
