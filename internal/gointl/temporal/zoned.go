package temporal

import intl "github.com/liustack/quickjs/internal/gointl"

// A ZonedDateTime is Temporal.ZonedDateTime's value: an instant, a zone and
// a calendar, and the zone's offset at the instant as temporal_rs caches
// it.
type ZonedDateTime struct {
	instant Instant
	tz      TimeZone
	cal     *Calendar
	offset  int64 // nanoseconds
}

// newZonedDateTime is ZonedDateTime::try_new_with_provider.
func newZonedDateTime(ns int128, tz TimeZone, cal *Calendar) (ZonedDateTime, error) {
	i, err := newInstant(ns)
	if err != nil {
		return ZonedDateTime{}, err
	}
	return newZonedDateTimeWithOffset(i, tz, cal)
}

// NewZonedDateTimeFromInstant is ZonedDateTime::try_new_with_provider for
// an engine that holds the nanoseconds as an Instant, which it has checked
// the range of in making it.
func NewZonedDateTimeFromInstant(i Instant, tz TimeZone, cal *Calendar) (ZonedDateTime, error) {
	return newZonedDateTimeWithOffset(i, tz, cal)
}

// newZonedDateTimeWithOffset is new_unchecked_with_provider.
func newZonedDateTimeWithOffset(i Instant, tz TimeZone, cal *Calendar) (ZonedDateTime, error) {
	off, err := tz.offsetNanosFor(i.ns)
	if err != nil {
		return ZonedDateTime{}, err
	}
	return ZonedDateTime{i, tz, cal, off}, nil
}

// newZonedDateTimeCached is try_new_with_cached_offset.
func newZonedDateTimeCached(ns int128, tz TimeZone, cal *Calendar, offsetSeconds int64) (ZonedDateTime, error) {
	i, err := newInstant(ns)
	if err != nil {
		return ZonedDateTime{}, err
	}
	return ZonedDateTime{i, tz, cal, offsetSeconds * 1_000_000_000}, nil
}

// Instant is the zoned date-time's instant.
func (z ZonedDateTime) Instant() Instant { return z.instant }

// TimeZone is its zone.
func (z ZonedDateTime) TimeZone() TimeZone { return z.tz }

// Calendar is its calendar.
func (z ZonedDateTime) Calendar() *Calendar { return z.cal }

// OffsetNanoseconds is the offset cached with it.
func (z ZonedDateTime) OffsetNanoseconds() int64 { return z.offset }

// Offset is the offset as Temporal writes it.
func (z ZonedDateTime) Offset() string { return formatOffset(z.offset) }

// isoDateTime is get_iso_datetime: the wall time at the cached offset.
func (z ZonedDateTime) isoDateTime() ISODateTime {
	return isoDateTimeFromEpochNanoseconds(z.instant.ns, z.offset)
}

// ToPlainDateTime is Temporal.ZonedDateTime.prototype.toPlainDateTime.
func (z ZonedDateTime) ToPlainDateTime() PlainDateTime { return PlainDateTime{z.isoDateTime(), z.cal} }

// ToPlainDate is Temporal.ZonedDateTime.prototype.toPlainDate.
func (z ZonedDateTime) ToPlainDate() PlainDate { return PlainDate{z.isoDateTime().Date, z.cal} }

// ToPlainTime is Temporal.ZonedDateTime.prototype.toPlainTime.
func (z ZonedDateTime) ToPlainTime() PlainTime { return PlainTime{z.isoDateTime().Time} }

// A PartialZonedDateTime is a zoned date-time's fields, as from and with
// take them.
type PartialZonedDateTime struct {
	Date CalendarFields
	Time PartialTime
	// Offset is the offset field in nanoseconds, nil where not given.
	Offset *int64
}

// ZonedDateTimeFromFields is ZonedDateTime::from_partial_with_provider:
// the zone is UTC where tz is nil.
func (zs *Zones) ZonedDateTimeFromFields(p PartialZonedDateTime, tz *TimeZone, cal *Calendar, overflow Overflow,
	d Disambiguation, o OffsetDisambiguation) (ZonedDateTime, error) {
	date, err := cal.dateFromFields(p.Date, overflow)
	if err != nil {
		return ZonedDateTime{}, err
	}
	t, err := ISOTime{}.with(p.Time, overflow)
	if err != nil {
		return ZonedDateTime{}, err
	}
	zone := zs.UTC()
	if tz != nil {
		zone = *tz
	}
	e, err := interpretISODateTimeOffset(date.iso, &t, false, p.Offset, zone, d, o.or(OffsetReject), false)
	if err != nil {
		return ZonedDateTime{}, err
	}
	return ZonedDateTime{Instant{e.ns}, zone, cal, e.offset * 1_000_000_000}, nil
}

// With is Temporal.ZonedDateTime.prototype.with.
func (z ZonedDateTime) With(p PartialZonedDateTime, d Disambiguation, o OffsetDisambiguation, overflow Overflow) (ZonedDateTime, error) {
	if p.Date.isEmpty() && p.Time.isEmpty() && p.Offset == nil {
		return ZonedDateTime{}, typeError("fields cannot be empty")
	}
	iso := z.isoDateTime()
	date, err := z.cal.dateFromFields(p.Date.withFallback(z.cal, iso.Date, true), overflow)
	if err != nil {
		return ZonedDateTime{}, err
	}
	t, err := iso.Time.with(p.Time, overflow)
	if err != nil {
		return ZonedDateTime{}, err
	}
	offset := z.offset
	if p.Offset != nil {
		offset = *p.Offset
	}
	e, err := interpretISODateTimeOffset(date.iso, &t, true, &offset, z.tz, d, o.or(OffsetReject), false)
	if err != nil {
		return ZonedDateTime{}, err
	}
	return ZonedDateTime{Instant{e.ns}, z.tz, z.cal, e.offset * 1_000_000_000}, nil
}

// WithTimeZone is Temporal.ZonedDateTime.prototype.withTimeZone.
func (z ZonedDateTime) WithTimeZone(tz TimeZone) (ZonedDateTime, error) {
	return newZonedDateTime(z.instant.ns, tz, z.cal)
}

// WithCalendar is Temporal.ZonedDateTime.prototype.withCalendar.
func (z ZonedDateTime) WithCalendar(c *Calendar) ZonedDateTime {
	z.cal = c
	return z
}

// WithPlainTime is Temporal.ZonedDateTime.prototype.withPlainTime; a nil
// time is the start of the day.
func (z ZonedDateTime) WithPlainTime(t *PlainTime) (ZonedDateTime, error) {
	iso := z.isoDateTime()
	var e epochAndOffset
	var err error
	if t != nil {
		e, err = z.tz.epochNanosecondsFor(ISODateTime{iso.Date, t.iso}, Compatible)
	} else {
		e, err = z.tz.startOfDay(iso.Date)
	}
	if err != nil {
		return ZonedDateTime{}, err
	}
	return newZonedDateTimeCached(e.ns, z.tz, z.cal, e.offset)
}

// Compare is Temporal.ZonedDateTime.compare, by instant.
func (z ZonedDateTime) Compare(o ZonedDateTime) int { return z.instant.Compare(o.instant) }

// Equals is Temporal.ZonedDateTime.prototype.equals.
func (z ZonedDateTime) Equals(o ZonedDateTime) bool {
	return z.instant == o.instant && z.tz.Equals(o.tz) && z.cal.Equal(o.cal)
}

// addZoned is AddZonedDateTime.
func (z ZonedDateTime) addZoned(d internalDuration, overflow Overflow) (Instant, error) {
	if d.date.sign() == 0 {
		return z.instant.addTimeDuration(d.time)
	}
	iso := z.isoDateTime()
	added, err := z.cal.dateAdd(iso.Date, d.date, overflow)
	if err != nil {
		return Instant{}, err
	}
	intermediate := ISODateTime{added.iso, iso.Time}
	if !intermediate.withinLimits() {
		return Instant{}, rangeError("Intermediate ISO datetime was not within a valid range.")
	}
	e, err := z.tz.epochNanosecondsFor(intermediate, Compatible)
	if err != nil {
		return Instant{}, err
	}
	return Instant{e.ns}.addTimeDuration(d.time)
}

// Add is Temporal.ZonedDateTime.prototype.add.
func (z ZonedDateTime) Add(d Duration, overflow Overflow) (ZonedDateTime, error) {
	i, err := z.addZoned(d.internal(), overflow)
	if err != nil {
		return ZonedDateTime{}, err
	}
	return newZonedDateTimeWithOffset(i, z.tz, z.cal)
}

// Subtract is Temporal.ZonedDateTime.prototype.subtract.
func (z ZonedDateTime) Subtract(d Duration, overflow Overflow) (ZonedDateTime, error) {
	return z.Add(d.Negated(), overflow)
}

// diffWithRounding is DifferenceZonedDateTimeWithRounding.
func (z ZonedDateTime) diffWithRounding(o Instant, r resolvedRounding) (internalDuration, error) {
	if r.largest.isTimeUnit() {
		return z.instant.diffInternal(o, r)
	}
	diff, err := z.diffZoned(o, r.largest)
	if err != nil {
		return internalDuration{}, err
	}
	if r.smallest == Nanosecond && r.increment.get() == 1 {
		return diff, nil
	}
	return diff.roundRelative(z.instant.ns, o.ns, PlainDateTime{z.isoDateTime(), z.cal}, &z.tz, r)
}

// diffWithTotal is DifferenceZonedDateTimeWithTotal.
func (z ZonedDateTime) diffWithTotal(o Instant, u Unit) (float64, error) {
	if u.isTimeUnit() {
		d, err := timeDurationFromDifference(o.ns, z.instant.ns)
		if err != nil {
			return 0, err
		}
		return d.total(u), nil
	}
	diff, err := z.diffZoned(o, u)
	if err != nil {
		return 0, err
	}
	return diff.totalRelative(z.instant.ns, o.ns, PlainDateTime{z.isoDateTime(), z.cal}, &z.tz, u)
}

// diffZoned is DifferenceZonedDateTime.
func (z ZonedDateTime) diffZoned(o Instant, largest Unit) (internalDuration, error) {
	if z.instant == o {
		return internalDuration{}, nil
	}
	start := z.isoDateTime()
	end, err := z.tz.isoDateTimeFor(o.ns)
	if err != nil {
		return internalDuration{}, err
	}
	if start.Date == end.Date {
		t, err := timeDurationFromDifference(o.ns, z.instant.ns)
		if err != nil {
			return internalDuration{}, err
		}
		return newInternalDuration(DateDuration{}, t)
	}
	sign := o.ns.sub(z.instant.ns).sign()
	if sign == 0 {
		sign = 1
	}
	maxCorrection := 1
	if sign > 0 {
		maxCorrection = 2
	}
	correction := 0
	if start.Time.diff(end.Time).sign() == -sign {
		correction = 1
	}
	var intermediate ISODateTime
	var t timeDuration
	success := false
	for correction <= maxCorrection && !success {
		date := balanceISODate(int32(end.Date.Year), int32(end.Date.Month), int32(end.Date.Day)-int32(int8(correction*sign)))
		intermediate = ISODateTime{date, start.Time}
		e, err := z.tz.epochNanosecondsFor(intermediate, Compatible)
		if err != nil {
			return internalDuration{}, err
		}
		if t, err = timeDurationFromDifference(o.ns, e.ns); err != nil {
			return internalDuration{}, err
		}
		if sign != -t.sign() {
			success = true
		}
		correction++
	}
	dd, err := z.cal.dateUntil(start.Date, intermediate.Date, maxUnit(largest, Day))
	if err != nil {
		return internalDuration{}, err
	}
	return newInternalDuration(dd, t)
}

func (z ZonedDateTime) diff(op differenceOp, o ZonedDateTime, s DifferenceSettings) (Duration, error) {
	if !z.cal.Equal(o.cal) {
		return Duration{}, rangeError("Calendar must be the same for operations involving two calendared types.")
	}
	r, err := fromDiffSettings(s, op, groupDateTime, Hour, Nanosecond)
	if err != nil {
		return Duration{}, err
	}
	negate := func(d Duration, err error) (Duration, error) {
		if err == nil && op == opSince {
			d = d.Negated()
		}
		return d, err
	}
	if r.largest.isTimeUnit() {
		i, err := z.instant.diffInternal(o.instant, r)
		if err != nil {
			return Duration{}, err
		}
		return negate(durationFromInternal(i, r.largest))
	}
	if !z.tz.Equals(o.tz) {
		return Duration{}, rangeError("Timezones must be the same if unit is a day unit.")
	}
	if z.instant == o.instant {
		return Duration{}, nil
	}
	i, err := z.diffWithRounding(o.instant, r)
	if err != nil {
		return Duration{}, err
	}
	return negate(durationFromInternal(i, Hour))
}

// Until is Temporal.ZonedDateTime.prototype.until.
func (z ZonedDateTime) Until(o ZonedDateTime, s DifferenceSettings) (Duration, error) {
	return z.diff(opUntil, o, s)
}

// Since is Temporal.ZonedDateTime.prototype.since.
func (z ZonedDateTime) Since(o ZonedDateTime, s DifferenceSettings) (Duration, error) {
	return z.diff(opSince, o, s)
}

// StartOfDay is Temporal.ZonedDateTime.prototype.startOfDay.
func (z ZonedDateTime) StartOfDay() (ZonedDateTime, error) {
	e, err := z.tz.startOfDay(z.isoDateTime().Date)
	if err != nil {
		return ZonedDateTime{}, err
	}
	return newZonedDateTimeCached(e.ns, z.tz, z.cal, e.offset)
}

// TimeZoneTransition is getTimeZoneTransition: the next or previous change
// of offset, false for none or for an offset zone.
func (z ZonedDateTime) TimeZoneTransition(next bool) (ZonedDateTime, bool, error) {
	if !z.tz.named {
		return ZonedDateTime{}, false, nil
	}
	ns, ok, err := z.tz.transition(z.instant.ns, next)
	if err != nil || !ok {
		return ZonedDateTime{}, false, err
	}
	if !validEpochNanoseconds(ns) {
		return ZonedDateTime{}, false, nil
	}
	out, err := newZonedDateTime(ns, z.tz, z.cal)
	if err != nil {
		return ZonedDateTime{}, false, assertError()
	}
	return out, true, nil
}

// HoursInDay is Temporal.ZonedDateTime.prototype.hoursInDay.
func (z ZonedDateTime) HoursInDay() (float64, error) {
	today := z.isoDateTime().Date
	tomorrow := balanceISODate(int32(today.Year), int32(today.Month), int32(today.Day+1))
	t0, err := z.tz.startOfDay(today)
	if err != nil {
		return 0, err
	}
	t1, err := z.tz.startOfDay(tomorrow)
	if err != nil {
		return 0, err
	}
	d, err := timeDurationFromDifference(t1.ns, t0.ns)
	if err != nil {
		return 0, err
	}
	return d.float64() / 3_600_000_000_000, nil
}

// Round is Temporal.ZonedDateTime.prototype.round.
func (z ZonedDateTime) Round(o RoundingOptions) (ZonedDateTime, error) {
	r, err := fromDateTimeOptions(o)
	if err != nil {
		return ZonedDateTime{}, err
	}
	if r.smallest == Nanosecond && r.increment.get() == 1 {
		return z, nil
	}
	this := z.instant.ns
	if r.smallest == Day {
		start := z.isoDateTime()
		endDate := balanceISODate(int32(start.Date.Year), int32(start.Date.Month), int32(start.Date.Day+1))
		s, err := z.tz.startOfDay(start.Date)
		if err != nil {
			return ZonedDateTime{}, err
		}
		e, err := z.tz.startOfDay(endDate)
		if err != nil {
			return ZonedDateTime{}, err
		}
		if this.cmp(s.ns) < 0 {
			return ZonedDateTime{}, assertError()
		}
		// Where the clock goes back past midnight, the day's last hours come
		// after the next day's start: ECMA-262's Temporal takes them as the
		// last nanosecond of the day, where temporal_rs 0.2.3 asserted it
		// could not happen (intl.RepeatedMidnight).
		if this.cmp(e.ns) >= 0 {
			if o.Compat.Has(intl.RepeatedMidnight) {
				return ZonedDateTime{}, rangeError("ZonedDateTime is outside the expected day bounds")
			}
			this = e.ns.sub(i128(1))
		}
		dayLen, err := timeDurationFromDifference(e.ns, s.ns)
		if err != nil {
			return ZonedDateTime{}, err
		}
		progress, err := timeDurationFromDifference(this, s.ns)
		if err != nil {
			return ZonedDateTime{}, err
		}
		var rounded int128
		if !dayLen.isZero() {
			rounded = roundIncrement(progress.int128, dayLen.abs(), r.mode)
		}
		offset := s.offset
		if !rounded.isZero() {
			offset = e.offset
		}
		candidate := s.ns.add(rounded)
		if _, err := newInstant(candidate); err != nil {
			return ZonedDateTime{}, err
		}
		return newZonedDateTimeCached(candidate, z.tz, z.cal, offset)
	}
	dt, err := z.isoDateTime().round(r)
	if err != nil {
		return ZonedDateTime{}, err
	}
	off, err := z.tz.offsetNanosFor(this)
	if err != nil {
		return ZonedDateTime{}, err
	}
	e, err := interpretISODateTimeOffset(dt.Date, &dt.Time, false, &off, z.tz, Compatible, OffsetPrefer, false)
	if err != nil {
		return ZonedDateTime{}, err
	}
	return newZonedDateTimeCached(e.ns, z.tz, z.cal, e.offset)
}

// String is Temporal.ZonedDateTime.prototype.toString.
func (z ZonedDateTime) String(showOffset DisplayOffset, showZone DisplayTimeZone, showCal DisplayCalendar,
	o ToStringRoundingOptions) (string, error) {
	r, err := o.resolve()
	if err != nil {
		return "", err
	}
	ns, err := z.instant.round(fromToStringOptions(r))
	if err != nil {
		return "", err
	}
	rounded, err := newInstant(ns)
	if err != nil {
		return "", err
	}
	off, err := z.tz.offsetNanosFor(ns)
	if err != nil {
		return "", err
	}
	dt, err := z.tz.isoDateTimeFor(rounded.ns)
	if err != nil {
		return "", err
	}
	neg, h, m := offsetMinutes(i128(off))
	var b ixdtfBuilder
	b.date(dt.Date)
	b.time(dt.Time, r.precision)
	b.minuteOffset(neg, h, m, showOffset)
	b.timeZone(z.tz.Identifier(), showZone)
	b.calendar(z.cal.id, showCal)
	return b.String(), nil
}

// A ParsedZonedDateTime is ParsedZonedDateTime.
type ParsedZonedDateTime struct {
	date         ParsedDate
	time         *ISOTime
	utc          bool
	matchMinutes bool
	offset       *int64
	tz           TimeZone
}

// Calendar is the parsed calendar's identifier.
func (p ParsedZonedDateTime) Calendar() string { return p.date.cal }

// ParseZonedDateTime is ParsedZonedDateTime::from_utf8_with_provider.
func (zs *Zones) ParseZonedDateTime(s []byte) (ParsedZonedDateTime, error) {
	r, err := parseZonedDateTimeString(s)
	if err != nil {
		return ParsedZonedDateTime{}, err
	}
	tz, err := zs.fromTzRecord(*r.tz)
	if err != nil {
		return ParsedZonedDateTime{}, err
	}
	p := ParsedZonedDateTime{tz: tz, matchMinutes: true}
	switch {
	case r.z:
		p.utc = true
	case r.offset != nil:
		if r.offset.hasSecond {
			p.matchMinutes = false
		}
		o, err := offsetNanoseconds(*r.offset)
		if err != nil {
			return ParsedZonedDateTime{}, err
		}
		p.offset = &o
	}
	cal, err := parsedCalendar(r)
	if err != nil {
		return ParsedZonedDateTime{}, err
	}
	if r.date == nil {
		return ParsedZonedDateTime{}, rangeError("Could not find a valid DateRecord node during parsing.")
	}
	p.date = ParsedDate{*r.date, cal}
	if r.time != nil {
		t, err := timeFromRecord(*r.time)
		if err != nil {
			return ParsedZonedDateTime{}, err
		}
		p.time = &t
	}
	return p, nil
}

// ZonedDateTimeFromParsed is ZonedDateTime::from_parsed_with_provider.
func ZonedDateTimeFromParsed(p ParsedZonedDateTime, cal *Calendar, d Disambiguation, o OffsetDisambiguation) (ZonedDateTime, error) {
	date, err := newISODateWithOverflow(int(p.date.record.year), int(p.date.record.month), int(p.date.record.day), Reject)
	if err != nil {
		return ZonedDateTime{}, err
	}
	e, err := interpretISODateTimeOffset(date, p.time, p.utc, p.offset, p.tz, d, o, p.matchMinutes)
	if err != nil {
		return ZonedDateTime{}, err
	}
	return ZonedDateTime{Instant{e.ns}, p.tz, cal, e.offset * 1_000_000_000}, nil
}

// interpretISODateTimeOffset is InterpretISODateTimeOffset; a nil time is
// the start of the day.
func interpretISODateTimeOffset(date ISODate, t *ISOTime, exact bool, offset *int64, tz TimeZone,
	d Disambiguation, o OffsetDisambiguation, matchMinutes bool) (epochAndOffset, error) {
	if t == nil {
		if offset != nil {
			return epochAndOffset{}, assertError()
		}
		return tz.startOfDay(date)
	}
	dt := ISODateTime{date, *t}
	switch {
	case !exact && offset == nil || offset != nil && o == OffsetIgnore:
		return tz.epochNanosecondsFor(dt, d)
	case exact && offset == nil || offset != nil && o == OffsetUse:
		off := int64(0)
		if offset != nil {
			off = *offset
		}
		b := balanceISODateTime(int32(date.Year), int32(date.Month), int32(date.Day), int64(t.Hour), int64(t.Minute),
			int64(t.Second), int64(t.Millisecond), i128(int64(t.Microsecond)), i128(int64(t.Nanosecond)-off))
		if err := b.Date.checkDayRange(); err != nil {
			return epochAndOffset{}, err
		}
		ns := b.epochNanoseconds()
		if !validEpochNanoseconds(ns) {
			return epochAndOffset{}, rangeError("Instant nanoseconds are not within a valid epoch range.")
		}
		tzOff, err := tz.offsetNanosFor(ns)
		if err != nil {
			return epochAndOffset{}, err
		}
		return epochAndOffset{ns, tzOff / 1_000_000_000}, nil
	}
	if err := date.checkDayRange(); err != nil {
		return epochAndOffset{}, err
	}
	utc := dt.epochNanoseconds()
	c, err := tz.possibleEpochNanoseconds(dt)
	if err != nil {
		return epochAndOffset{}, err
	}
	for _, cand := range c.list[:c.n] {
		candOffset := utc.sub(cand.ns)
		if candOffset == i128(*offset) {
			return cand, nil
		}
		if matchMinutes {
			if roundIncrement(candOffset, i128(60_000_000_000), HalfExpand) == i128(*offset) {
				return cand, nil
			}
		}
	}
	if o == OffsetReject {
		return epochAndOffset{}, rangeError("Offsets could not be determined without disambiguation")
	}
	return tz.disambiguate(c, dt, d)
}

// The zoned date-time's fields, at its cached offset.
func (z ZonedDateTime) date() PlainDate { return z.ToPlainDate() }
