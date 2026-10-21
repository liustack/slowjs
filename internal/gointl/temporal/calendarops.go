package temporal

// The calendar operations of Temporal's abstract operations, as temporal_rs
// 0.2.3's Calendar implements them: the ISO calendar by the specification's
// ISO algorithms, and the others through ICU4X's.

// DateFromFields is CalendarDateFromFields: the ISO date of a calendar's
// year, month and day.
func (c *Calendar) DateFromFields(f CalendarFields, overflow Overflow) (ISODate, error) {
	if c.id == "iso8601" {
		y, m, d, err := resolveISOFields(&f, overflow, resolveDate)
		if err != nil {
			return ISODate{}, err
		}
		return newISODate(y, m, d, overflow)
	}
	d, err := c.fromFields(toICUFields(&f), overflow, missingReject)
	if err != nil {
		return ISODate{}, err
	}
	iso := isoFromRataDie(d.rataDie())
	return newISODate(iso.Year, iso.Month, iso.Day, overflow)
}

// YearMonthFromFields is CalendarYearMonthFromFields: the ISO date of the
// first day of a calendar's month. f.Day is not read.
func (c *Calendar) YearMonthFromFields(f CalendarFields, overflow Overflow) (ISODate, error) {
	f.Day = nil
	if c.id == "iso8601" {
		y, m, d, err := resolveISOFields(&f, overflow, resolveYearMonth)
		if err != nil {
			return ISODate{}, err
		}
		return newYearMonth(y, m, d, overflow)
	}
	if f.Year == nil && f.EraYear == nil {
		return ISODate{}, typeError("Must specify year for YearMonth")
	}
	d, err := c.fromFields(toICUFields(&f), overflow, missingECMA)
	if err != nil {
		return ISODate{}, err
	}
	iso := isoFromRataDie(d.rataDie())
	return newYearMonth(iso.Year, iso.Month, iso.Day, overflow)
}

// newYearMonth is PlainYearMonth::new_with_overflow.
func newYearMonth(year, month, day int, overflow Overflow) (ISODate, error) {
	d, err := regulateISODate(year, month, day, overflow)
	if err != nil {
		return ISODate{}, err
	}
	if !yearMonthWithinLimits(d.Year, d.Month) {
		return ISODate{}, rangeError("Exceeded valid range.")
	}
	return d, nil
}

// MonthDayFromFields is CalendarMonthDayFromFields: the ISO date, in the
// calendar's reference year, of a month and day. A year, if given, only
// resolves the month and day: {year: 2025, month: 2, day: 29} constrains to
// 02-28.
func (c *Calendar) MonthDayFromFields(f CalendarFields, overflow Overflow) (ISODate, error) {
	if f.Year != nil || f.Era != nil && f.EraYear != nil {
		if c.id == "iso8601" {
			_, m, d, err := resolveISOFields(&f, overflow, resolveMonthDayWithYear)
			if err != nil {
				return ISODate{}, err
			}
			f = CalendarFields{Year: Int(1972), Month: Int(m), Day: Int(d)}
		} else {
			// The year must be one some day of which is in Temporal's range:
			// either the earliest or the latest date the fields can make.
			lo, hi := f, f
			lo.Month, lo.MonthCode, lo.Day = Int(1), nil, Int(1)
			hi.Month, hi.MonthCode, hi.Day = Int(15), nil, Int(40)
			dlo, err := c.fromFields(toICUFields(&lo), Constrain, missingReject)
			if err != nil {
				return ISODate{}, err
			}
			dhi, err := c.fromFields(toICUFields(&hi), Constrain, missingReject)
			if err != nil {
				return ISODate{}, err
			}
			if !isoFromRataDie(dlo.rataDie()).withinLimits() && !isoFromRataDie(dhi.rataDie()).withinLimits() {
				return ISODate{}, rangeError("Date is not within ISO date time limits.")
			}
			d, err := c.fromFields(toICUFields(&f), overflow, missingReject)
			if err != nil {
				return ISODate{}, err
			}
			code := c.r.monthFromOrdinal(d.y, d.month).code()
			f = CalendarFields{MonthCode: &code, Day: Int(d.day)}
		}
	}
	if c.id == "iso8601" {
		_, m, d, err := resolveISOFields(&f, overflow, resolveMonthDay)
		if err != nil {
			return ISODate{}, err
		}
		return newISODate(1972, m, d, overflow)
	}
	if f.Day == nil {
		return ISODate{}, typeError("Must specify day for MonthDay")
	}
	fields := toICUFields(&f)
	d, err := c.fromFields(fields, overflow, missingECMA)
	if err != nil {
		return ISODate{}, err
	}
	// A year given resolves the date; the reference year is then the
	// month and day's own.
	if fields.eraYear != nil || fields.extendedYear != nil {
		code := c.r.monthFromOrdinal(d.y, d.month).code()
		if d, err = c.fromFields(icuFields{monthCode: &code, day: Int(d.day)}, overflow, missingECMA); err != nil {
			return ISODate{}, err
		}
	}
	iso := isoFromRataDie(d.rataDie())
	return newISODate(iso.Year, iso.Month, iso.Day, overflow)
}

// DateAdd is CalendarDateAdd: a date moved by a duration's date part.
func (c *Calendar) DateAdd(date ISODate, dur DateDuration, overflow Overflow) (ISODate, error) {
	if c.id == "iso8601" {
		d, err := addISODate(date, dur, overflow)
		if err != nil {
			return ISODate{}, err
		}
		if !d.withinLimits() {
			return ISODate{}, rangeError("Date is not within ISO date time limits.")
		}
		return d, nil
	}
	neg := dur.Years < 0 || dur.Months < 0 || dur.Weeks < 0 || dur.Days < 0
	d := icuDuration{negative: neg, years: abs64(dur.Years), months: abs64(dur.Months),
		weeks: abs64(dur.Weeks), days: abs64(dur.Days)}
	for _, v := range []int64{d.years, d.months, d.weeks, d.days} {
		// math.MinInt64 has no absolute value, and abs64 leaves it below
		// zero: it is as far out of range as anything above, where a loop
		// counting its months down had run for ever.
		if v < 0 || v > 1<<32-1 {
			return ISODate{}, rangeError("Duration was not valid.")
		}
	}
	// early_constrain_date_duration: nothing so large can stay in range.
	const years = 2 * (275760 + 271821)
	if d.years > years || d.months > years*13 || d.weeks > years*390/7 || d.days > years*390 {
		return ISODate{}, rangeError("Intermediate ISO datetime was not within a valid range.")
	}
	start := c.arithDate(date.rataDie())
	out, err := c.added(start, d, overflow)
	if err != nil {
		return ISODate{}, err
	}
	iso := isoFromRataDie(out.rataDie())
	return newISODate(iso.Year, iso.Month, iso.Day, overflow)
}

// DateUntil is CalendarDateUntil: the date duration from one date to
// another, in units no larger than largest, one of Year, Month, Week and
// Day.
func (c *Calendar) DateUntil(one, two ISODate, largest Unit) (DateDuration, error) {
	if largest != Year && largest != Month && largest != Week && largest != Day {
		return DateDuration{}, typeError("Found time unit when computing CalendarDateUntil.")
	}
	if c.id == "iso8601" {
		return differenceISODate(one, two, largest)
	}
	d := c.until(c.arithDate(one.rataDie()), c.arithDate(two.rataDie()), largest)
	out := DateDuration{Years: d.years, Months: d.months, Weeks: d.weeks, Days: d.days}
	if d.negative {
		out = DateDuration{-out.Years, -out.Months, -out.Weeks, -out.Days}
	}
	return out, nil
}

// arithDate is the calendar's date of a day.
func (c *Calendar) arithDate(rd int64) arithDate {
	y := c.r.yearOfRD(rd)
	month, day := y.monthDay(rd)
	return arithDate{y, month, day}
}
