package temporal

import intl "github.com/liustack/quickjs/internal/gointl"

// Rounding and totalling durations relative to a date, as temporal_rs
// 0.2.3's normalized duration module does it.

type nudgeRecord struct {
	d        internalDuration
	nudgedNs int128
	expanded bool
}

type nudgeWindow struct {
	r1, r2           int128
	startNs, endNs   int128
	startDur, endDur DateDuration
}

func int128ToInt64(v int128) (int64, error) {
	if !v.fitsInt64() {
		return 0, rangeError("")
	}
	return v.int64(), nil
}

// computeNudgeWindow is ComputeNudgeWindow.
func (d internalDuration) computeNudgeWindow(sign int, origin int128, dt PlainDateTime, tz *TimeZone, o resolvedRounding, additionalShift bool) (nudgeWindow, error) {
	var w nudgeWindow
	inc := i128(int64(o.increment.get()))
	incSign := inc.mul64(int64(sign))
	date := d.date
	var err error
	switch o.smallest {
	case Year:
		years := roundIncrement(i128(date.Years), inc, Trunc)
		w.r1 = years
		if additionalShift {
			w.r1 = years.add(incSign)
		}
		w.r2 = w.r1.add(incSign)
		y1, err := int128ToInt64(w.r1)
		if err != nil {
			return w, err
		}
		y2, err := int128ToInt64(w.r2)
		if err != nil {
			return w, err
		}
		if w.startDur, err = newDateDuration(y1, 0, 0, 0); err != nil {
			return w, err
		}
		if w.endDur, err = newDateDuration(y2, 0, 0, 0); err != nil {
			return w, err
		}
	case Month:
		months := roundIncrement(i128(date.Months), inc, Trunc)
		w.r1 = months
		if additionalShift {
			w.r1 = months.add(incSign)
		}
		w.r2 = w.r1.add(incSign)
		m1, err := int128ToInt64(w.r1)
		if err != nil {
			return w, err
		}
		m2, err := int128ToInt64(w.r2)
		if err != nil {
			return w, err
		}
		w.startDur = date.adjust(0, nil, &m1)
		w.endDur = date.adjust(0, nil, &m2)
	case Week:
		one, err := tryBalanceISODate(int32(dt.iso.Date.Year)+int32(date.Years), int32(dt.iso.Date.Month)+int32(date.Months),
			int64(dt.iso.Date.Day))
		if err != nil {
			return w, err
		}
		two, err := tryBalanceISODate(int32(dt.iso.Date.Year)+int32(date.Years), int32(dt.iso.Date.Month)+int32(date.Months),
			int64(dt.iso.Date.Day)+date.Days)
		if err != nil {
			return w, err
		}
		start, err := newPlainDate(one.Year, one.Month, one.Day, dt.cal, Reject)
		if err != nil {
			return w, err
		}
		end, err := newPlainDate(two.Year, two.Month, two.Day, dt.cal, Reject)
		if err != nil {
			return w, err
		}
		until, err := start.diffDate(end, Week)
		if err != nil {
			return w, err
		}
		weeks := roundIncrement(i128(date.Weeks+until.Weeks), inc, Trunc)
		w.r1 = weeks
		w.r2 = weeks.add(incSign)
		w1, err := int128ToInt64(w.r1)
		if err != nil {
			return w, err
		}
		w2, err := int128ToInt64(w.r2)
		if err != nil {
			return w, err
		}
		if w.startDur, err = newDateDuration(date.Years, date.Months, w1, 0); err != nil {
			return w, err
		}
		if w.endDur, err = newDateDuration(date.Years, date.Months, w2, 0); err != nil {
			return w, err
		}
	case Day:
		days := roundIncrement(i128(date.Days), inc, Trunc)
		w.r1 = days
		w.r2 = days.add(incSign)
		d1, err := int128ToInt64(w.r1)
		if err != nil {
			return w, err
		}
		d2, err := int128ToInt64(w.r2)
		if err != nil {
			return w, err
		}
		if w.startDur, err = newDateDuration(date.Years, date.Months, date.Weeks, d1); err != nil {
			return w, err
		}
		if w.endDur, err = newDateDuration(date.Years, date.Months, date.Weeks, d2); err != nil {
			return w, err
		}
	default:
		return w, internalError("NudgeCalendarUnit invoked with unexpected unit")
	}
	zero := int128{}
	if !(sign >= 0 && w.r1.cmp(zero) >= 0 && w.r1.cmp(w.r2) < 0 || sign < 0 && w.r1.cmp(zero) <= 0 && w.r1.cmp(w.r2) > 0) {
		return w, assertError()
	}
	epochOf := func(dd DateDuration) (int128, error) {
		pd, err := dt.cal.dateAdd(dt.iso.Date, dd, Constrain)
		if err != nil {
			return int128{}, err
		}
		edt := ISODateTime{pd.iso, dt.iso.Time}
		if tz != nil {
			e, err := tz.epochNanosecondsFor(edt, Compatible)
			return e.ns, err
		}
		return edt.epochNanoseconds(), nil
	}
	// The window starts at the origin where the duration it starts with is
	// zero: ECMA-262's Temporal asks DateDurationSign of it. temporal_rs
	// 0.2.3 asks whether none of the smallest unit is in it, which starts
	// P1M's window for weeks a month early (intl.RoundingWindow).
	zeroStart := w.startDur.sign() == 0
	if o.compat.Has(intl.RoundingWindow) {
		zeroStart = w.r1.isZero()
	}
	if zeroStart {
		w.startNs = origin
	} else if w.startNs, err = epochOf(w.startDur); err != nil {
		return w, err
	}
	if w.endNs, err = epochOf(w.endDur); err != nil {
		return w, err
	}
	return w, nil
}

// computeAndAdjustNudgeWindow is ComputeAndAdjustNudgeWindow.
func (d internalDuration) computeAndAdjustNudgeWindow(sign int, origin, dest int128, dt PlainDateTime, tz *TimeZone, o resolvedRounding) (nudgeWindow, bool, error) {
	w, err := d.computeNudgeWindow(sign, origin, dt, tz, o, false)
	if err != nil {
		return w, false, err
	}
	inside := func(w nudgeWindow) bool {
		if sign >= 0 {
			return w.startNs.cmp(dest) <= 0 && dest.cmp(w.endNs) <= 0
		}
		return w.endNs.cmp(dest) <= 0 && dest.cmp(w.startNs) <= 0
	}
	if inside(w) {
		return w, false, nil
	}
	if w, err = d.computeNudgeWindow(sign, origin, dt, tz, o, true); err != nil {
		return w, false, err
	}
	if !inside(w) {
		return w, false, assertError()
	}
	return w, true, nil
}

// nudgeCalendarUnitTotal is NudgeToCalendarUnit's total.
func (d internalDuration) nudgeCalendarUnitTotal(sign int, origin, dest int128, dt PlainDateTime, tz *TimeZone, o resolvedRounding) (float64, error) {
	w, _, err := d.computeAndAdjustNudgeWindow(sign, origin, dest, dt, tz, o)
	if err != nil {
		return 0, err
	}
	if w.startNs == w.endNs {
		return 0, assertError()
	}
	progress := dest.sub(w.startNs)
	den := w.endNs.sub(w.startNs)
	num := w.r1.mul(den).add(progress.mul64(int64(o.increment.get())).mul64(int64(sign)))
	return fractionToFloat64(num, den.float64()), nil
}

// nudgeCalendarUnit is NudgeToCalendarUnit.
func (d internalDuration) nudgeCalendarUnit(sign int, origin, dest int128, dt PlainDateTime, tz *TimeZone, o resolvedRounding) (nudgeRecord, error) {
	w, expanded, err := d.computeAndAdjustNudgeWindow(sign, origin, dest, dt, tz, o)
	if err != nil {
		return nudgeRecord{}, err
	}
	if w.startNs == w.endNs {
		return nudgeRecord{}, assertError()
	}
	dividend := dest.sub(w.startNs)
	divisor := w.endNs.sub(w.startNs)
	total := w.r1.mul(divisor).add(dividend.mul64(int64(o.increment.get())).mul64(int64(sign)))
	um := o.mode.unsigned(sign >= 0)
	var rounded int128
	if total.divEuclid(divisor) == w.r2 && total.remEuclid(divisor).isZero() {
		rounded = w.r2.abs()
	} else {
		rounded = applyUnsignedRoundingRatio(um, total.abs(), divisor.abs(), w.r1.abs(), w.r2.abs())
	}
	if rounded == w.r2.abs() {
		id, err := newInternalDuration(w.endDur, timeDuration{})
		return nudgeRecord{id, w.endNs, true}, err
	}
	id, err := newInternalDuration(w.startDur, timeDuration{})
	return nudgeRecord{id, w.startNs, expanded}, err
}

// nudgeToZonedTime is NudgeToZonedTime.
func (d internalDuration) nudgeToZonedTime(sign int, dt PlainDateTime, tz TimeZone, o resolvedRounding) (nudgeRecord, error) {
	start, err := dt.cal.dateAdd(dt.iso.Date, d.date, Constrain)
	if err != nil {
		return nudgeRecord{}, err
	}
	startDT := ISODateTime{start.iso, dt.iso.Time}
	endDate := balanceISODate(int32(start.iso.Year), int32(start.iso.Month), int32(start.iso.Day)+int32(sign))
	endDT := ISODateTime{endDate, dt.iso.Time}
	s, err := tz.epochNanosecondsFor(startDT, Compatible)
	if err != nil {
		return nudgeRecord{}, err
	}
	e, err := tz.epochNanosecondsFor(endDT, Compatible)
	if err != nil {
		return nudgeRecord{}, err
	}
	span, err := timeDurationFromDifference(e.ns, s.ns)
	if err != nil {
		return nudgeRecord{}, err
	}
	inc := i128(o.smallest.nanoseconds()).mul64(int64(o.increment.get()))
	rounded, err := d.time.roundInner(inc, o.mode)
	if err != nil {
		return nudgeRecord{}, err
	}
	beyond, err := rounded.plus(timeDuration{span.neg()})
	if err != nil {
		return nudgeRecord{}, err
	}
	var expanded bool
	var dayDelta int64
	var nudged timeDuration
	if beyond.sign() != -sign {
		expanded, dayDelta = true, int64(sign)
		if rounded, err = beyond.roundInner(inc, o.mode); err != nil {
			return nudgeRecord{}, err
		}
		if nudged, err = rounded.plus(timeDuration{e.ns}); err != nil {
			return nudgeRecord{}, err
		}
	} else if nudged, err = rounded.plus(timeDuration{s.ns}); err != nil {
		return nudgeRecord{}, err
	}
	dd, err := newDateDuration(d.date.Years, d.date.Months, d.date.Weeks, d.date.Days+dayDelta)
	if err != nil {
		return nudgeRecord{}, err
	}
	id, err := newInternalDuration(dd, rounded)
	return nudgeRecord{id, nudged.int128, expanded}, err
}

// nudgeToDayOrTime is NudgeToDayOrTime.
func (d internalDuration) nudgeToDayOrTime(dest int128, o resolvedRounding) (nudgeRecord, error) {
	t, err := d.time.addDays(d.date.Days)
	if err != nil {
		return nudgeRecord{}, err
	}
	inc := i128(o.smallest.nanoseconds()).mul64(int64(o.increment.get()))
	rounded, err := t.roundInner(inc, o.mode)
	if err != nil {
		return nudgeRecord{}, err
	}
	diff, err := rounded.minus(t)
	if err != nil {
		return nudgeRecord{}, err
	}
	wholeDays := t.quo(i128(nsPerDay)).int64()
	roundedWholeDays := rounded.quo(i128(nsPerDay)).int64()
	delta := roundedWholeDays - wholeDays
	expanded := sign64(delta) == t.sign()
	nudged := diff.add(dest)
	days := int64(0)
	remainder := rounded
	if o.largest.isDateUnit() {
		days = roundedWholeDays
		if remainder, err = rounded.plus(timeDurationFromComponents(-roundedWholeDays*24, 0, 0, 0, int128{}, int128{})); err != nil {
			return nudgeRecord{}, err
		}
	}
	return nudgeRecord{internalDuration{d.date.adjust(days, nil, nil), remainder}, nudged, expanded}, nil
}

// bubbleRelativeDuration is BubbleRelativeDuration.
func (d internalDuration) bubbleRelativeDuration(sign int, nudged int128, dt ISODateTime, tz *TimeZone, cal *Calendar, largest, smallest Unit) (internalDuration, error) {
	duration := d
	if smallest == largest {
		return duration, nil
	}
	li, err := largest.tableIndex()
	if err != nil {
		return duration, err
	}
	si, err := smallest.tableIndex()
	if err != nil {
		return duration, err
	}
	upper := si
	if li > upper {
		upper = li
	}
	units := unitTable[li:upper]
	for k := len(units) - 1; k >= 0; k-- {
		u := units[k]
		if u == Week && largest != Week {
			continue
		}
		var end DateDuration
		switch u {
		case Year:
			years := d.date.Years + int64(sign)
			if end, err = newDateDuration(years, 0, 0, 0); err != nil {
				return duration, err
			}
		case Month:
			months := d.date.Months + int64(sign)
			zero := int64(0)
			end = duration.date.adjust(0, &zero, &months)
		default:
			weeks := d.date.Weeks + int64(sign)
			end = duration.date.adjust(0, &weeks, nil)
		}
		pd, err := cal.dateAdd(dt.Date, end, Constrain)
		if err != nil {
			return duration, err
		}
		endDT := ISODateTime{pd.iso, dt.Time}
		var endNs int128
		if tz == nil {
			endNs = endDT.epochNanoseconds()
		} else {
			e, err := tz.epochNanosecondsFor(endDT, Compatible)
			if err != nil {
				return duration, err
			}
			endNs = e.ns
		}
		beyond := nudged.sub(endNs).sign()
		if beyond != -sign {
			if duration, err = newInternalDuration(end, timeDuration{}); err != nil {
				return duration, err
			}
		} else {
			break
		}
	}
	return duration, nil
}

// roundRelative is RoundRelativeDuration.
func (d internalDuration) roundRelative(origin, dest int128, dt PlainDateTime, tz *TimeZone, o resolvedRounding) (internalDuration, error) {
	irregular := o.smallest.isCalendarUnit() || tz != nil && o.smallest == Day
	sign := d.sign()
	if sign == 0 {
		sign = 1
	}
	var n nudgeRecord
	var err error
	switch {
	case irregular:
		n, err = d.nudgeCalendarUnit(sign, origin, dest, dt, tz, o)
	case tz != nil:
		n, err = d.nudgeToZonedTime(sign, dt, *tz, o)
	default:
		n, err = d.nudgeToDayOrTime(dest, o)
	}
	if err != nil {
		return internalDuration{}, err
	}
	out := n.d
	if n.expanded && o.smallest != Week {
		start, err := largerUnit(o.smallest, Day)
		if err != nil {
			return internalDuration{}, err
		}
		if out, err = out.bubbleRelativeDuration(sign, n.nudgedNs, dt.iso, tz, dt.cal, o.largest, start); err != nil {
			return internalDuration{}, err
		}
	}
	return out, nil
}

// totalRelative is TotalRelativeDuration.
func (d internalDuration) totalRelative(origin, dest int128, dt PlainDateTime, tz *TimeZone, u Unit) (float64, error) {
	if u.isCalendarUnit() || tz != nil && u == Day {
		sign := d.sign()
		if sign == 0 {
			sign = 1
		}
		return d.nudgeCalendarUnitTotal(sign, origin, dest, dt, tz,
			resolvedRounding{largest: u, smallest: u, increment: 1, mode: Trunc})
	}
	t, err := d.time.addDays(d.date.Days)
	if err != nil {
		return 0, err
	}
	return t.total(u), nil
}

// A RelativeTo is a duration's relativeTo option: a PlainDate or a
// ZonedDateTime, or neither.
type RelativeTo struct {
	Date  *PlainDate
	Zoned *ZonedDateTime
}

// daysRelative is DateDurationDays.
func (dd DateDuration) daysRelative(rel PlainDate) (int64, error) {
	ymw := dd.adjust(0, nil, nil)
	if ymw.sign() == 0 {
		return dd.Days, nil
	}
	later, err := rel.cal.dateAdd(rel.iso, ymw, Constrain)
	if err != nil {
		return 0, err
	}
	e1 := isoDateToEpochDays(int32(rel.iso.Year), int32(rel.iso.Month), int32(rel.iso.Day))
	e2 := isoDateToEpochDays(int32(later.iso.Year), int32(later.iso.Month), int32(later.iso.Day))
	return dd.Days + e2 - e1, nil
}

// Compare is Temporal.Duration.compare.
func (d Duration) Compare(o Duration, rel RelativeTo) (int, error) {
	if d == o {
		return 0, nil
	}
	l1, l2 := d.defaultLargestUnit(), o.defaultLargestUnit()
	d1, d2 := d.internal(), o.internal()
	if rel.Zoned != nil && (l1.isDateUnit() || l2.isDateUnit()) {
		a1, err := rel.Zoned.addZoned(d1, Constrain)
		if err != nil {
			return 0, err
		}
		a2, err := rel.Zoned.addZoned(d2, Constrain)
		if err != nil {
			return 0, err
		}
		return a1.Compare(a2), nil
	}
	days1, days2 := d.date().Days, o.date().Days
	if l1.isCalendarUnit() || l2.isCalendarUnit() {
		if rel.Date == nil {
			return 0, rangeError("")
		}
		var err error
		if days1, err = d.date().daysRelative(*rel.Date); err != nil {
			return 0, err
		}
		if days2, err = o.date().daysRelative(*rel.Date); err != nil {
			return 0, err
		}
	}
	t1, err := d.timePart().addDays(days1)
	if err != nil {
		return 0, err
	}
	t2, err := o.timePart().addDays(days2)
	if err != nil {
		return 0, err
	}
	return t1.cmp(t2.int128), nil
}

// Round is Temporal.Duration.prototype.round.
func (d Duration) Round(o RoundingOptions, rel RelativeTo) (Duration, error) {
	mode := o.RoundingMode.or(HalfExpand)
	if err := groupDateTime.validate(o.SmallestUnit, NoUnit); err != nil {
		return Duration{}, err
	}
	if err := groupDateTime.validate(o.LargestUnit, UnitAuto); err != nil {
		return Duration{}, err
	}
	smallest := o.SmallestUnit
	if smallest == NoUnit {
		smallest = Nanosecond
	}
	existing := d.defaultLargestUnit()
	defLargest, err := largerUnit(existing, smallest)
	if err != nil {
		return Duration{}, err
	}
	largest := o.LargestUnit
	if largest == NoUnit || largest == UnitAuto {
		largest = defLargest
	}
	if o.LargestUnit == NoUnit && o.SmallestUnit == NoUnit {
		return Duration{}, rangeError("smallestUnit and largestUnit cannot both be None.")
	}
	if l, err := largerUnit(largest, smallest); err != nil {
		return Duration{}, err
	} else if l != largest {
		return Duration{}, rangeError("smallestUnit is larger than largestUnit.")
	}
	if max := smallest.maximumIncrement(); max != 0 {
		if err := o.Increment.validate(max, false); err != nil {
			return Duration{}, err
		}
	}
	if o.Increment.get() > 1 && largest != smallest && smallest.isDateUnit() {
		return Duration{}, rangeError("roundingIncrement > 1 and largest_unit is not smallest_unit and smallest_unit is date")
	}
	r := resolvedRounding{largest: largest, smallest: smallest, increment: o.Increment, mode: mode, compat: o.Compat}
	switch {
	case rel.Zoned != nil:
		z := *rel.Zoned
		target, err := z.addZoned(d.internal(), Constrain)
		if err != nil {
			return Duration{}, err
		}
		id, err := z.diffWithRounding(target, r)
		if err != nil {
			return Duration{}, err
		}
		l := r.largest
		if l.isDateUnit() {
			l = Hour
		}
		return durationFromInternal(id, l)
	case rel.Date != nil:
		p := *rel.Date
		id, err := d.internalWith24HourDays()
		if err != nil {
			return Duration{}, err
		}
		days, t := PlainTime{}.addTimeDuration(id.time)
		dd := id.date.adjust(days, nil, nil)
		target, err := p.cal.dateAdd(p.iso, dd, Constrain)
		if err != nil {
			return Duration{}, err
		}
		from := PlainDateTime{ISODateTime{Date: p.iso}, p.cal}
		to := PlainDateTime{ISODateTime{target.iso, t.iso}, p.cal}
		out, err := from.diffWithRounding(to, r)
		if err != nil {
			return Duration{}, err
		}
		return durationFromInternal(out, r.largest)
	}
	if existing.isCalendarUnit() || r.largest.isCalendarUnit() {
		return Duration{}, rangeError("largestUnit when rounding Duration was not the largest provided unit")
	}
	if r.smallest.isCalendarUnit() {
		return Duration{}, assertError()
	}
	id, err := d.internalWith24HourDays()
	if err != nil {
		return Duration{}, err
	}
	if r.smallest == Day {
		days := id.time.roundToFractionalDays(r.increment, r.mode)
		dd, err := newDateDuration(0, 0, 0, days)
		if err != nil {
			return Duration{}, err
		}
		if id, err = newInternalDuration(dd, timeDuration{}); err != nil {
			return Duration{}, err
		}
	} else {
		t, err := id.time.round(r)
		if err != nil {
			return Duration{}, err
		}
		if id, err = newInternalDuration(DateDuration{}, t); err != nil {
			return Duration{}, err
		}
	}
	return durationFromInternal(id, r.largest)
}

// Total is Temporal.Duration.prototype.total.
func (d Duration) Total(u Unit, rel RelativeTo) (float64, error) {
	// The unit is required and is a unit: "auto" is refused as temporal_rs
	// refuses it, where the total had come out infinite.
	switch {
	case u == NoUnit:
		return 0, rangeError("Unit is required")
	case u == UnitAuto:
		return 0, rangeError("Auto unit not allowed here")
	case u < UnitAuto || u > Year:
		return 0, rangeError("Unit was not a valid unit.")
	}
	switch {
	case rel.Zoned != nil:
		z := *rel.Zoned
		target, err := z.addZoned(d.internal(), Constrain)
		if err != nil {
			return 0, err
		}
		return z.diffWithTotal(target, u)
	case rel.Date != nil:
		p := *rel.Date
		days, t := PlainTime{}.addTimeDuration(d.timePart())
		sum := d.Days() + days
		if (days > 0 && sum < d.Days()) || (days < 0 && sum > d.Days()) {
			return 0, rangeError("")
		}
		dd, err := newDateDuration(d.Years(), d.Months(), d.Weeks(), sum)
		if err != nil {
			return 0, err
		}
		target, err := p.cal.dateAdd(p.iso, dd, Constrain)
		if err != nil {
			return 0, err
		}
		from := PlainDateTime{ISODateTime{Date: p.iso}, p.cal}
		to := PlainDateTime{ISODateTime{target.iso, t.iso}, p.cal}
		return from.diffWithTotal(to, u)
	}
	if d.defaultLargestUnit().isCalendarUnit() || u.isCalendarUnit() {
		return 0, rangeError("")
	}
	id, err := d.internalWith24HourDays()
	if err != nil {
		return 0, err
	}
	return id.time.total(u), nil
}

// String is Temporal.Duration.prototype.toString.
func (d Duration) String(o ToStringRoundingOptions) (string, error) {
	if o.SmallestUnit == Hour || o.SmallestUnit == Minute {
		return "", rangeError("string rounding options cannot have hour or minute smallest unit.")
	}
	r, err := o.resolve()
	if err != nil {
		return "", err
	}
	if r.smallest == Nanosecond && r.increment.get() == 1 {
		return d.formattable(r.precision).String(), nil
	}
	largest := d.defaultLargestUnit()
	id := d.internal()
	t, err := id.time.round(fromToStringOptions(r))
	if err != nil {
		return "", err
	}
	rounded, err := durationFromInternal(internalDuration{id.date, t}, maxUnit(largest, Second))
	if err != nil {
		return "", err
	}
	return rounded.formattable(r.precision).String(), nil
}
