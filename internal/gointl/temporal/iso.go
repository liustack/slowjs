package temporal

// The ISO calendar's arithmetic and Temporal's range, as temporal_rs 0.2.3
// has them.

// maxEpochDays is the most days from 1970 Temporal's dates reach, either
// way, and one more.
const maxEpochDays = 100000001

// withinLimits is ISODateWithinLimits: the date's noon is within a day of
// Temporal's instants, which is -271821-04-19 to 275760-09-13.
func (d ISODate) withinLimits() bool {
	days := d.EpochDays()
	return days >= -maxEpochDays && days <= maxEpochDays-1
}

// yearMonthWithinLimits is ISOYearMonthWithinLimits.
func yearMonthWithinLimits(year, month int) bool {
	switch {
	case year < -271821 || year > 275760:
		return false
	case year == -271821 && month < 4:
		return false
	case year == 275760 && month > 9:
		return false
	}
	return true
}

// validISODate is IsValidISODate.
func validISODate(year, month, day int) bool {
	return month >= 1 && month <= 12 && day >= 1 && day <= gregorianMonthLength(year, month)
}

// regulateISODate is RegulateISODate.
func regulateISODate(year, month, day int, overflow Overflow) (ISODate, error) {
	if overflow == Constrain {
		month = clamp(month, 1, 12)
		return ISODate{year, month, clamp(day, 1, gregorianMonthLength(year, month))}, nil
	}
	if !validISODate(year, month, day) {
		return ISODate{}, rangeError("not a valid ISO date.")
	}
	return ISODate{year, month, day}, nil
}

// newISODate is IsoDate::new_with_overflow: the date regulated, and within
// Temporal's range.
func newISODate(year, month, day int, overflow Overflow) (ISODate, error) {
	d, err := regulateISODate(year, month, day, overflow)
	if err != nil {
		return ISODate{}, err
	}
	if !d.withinLimits() {
		return ISODate{}, rangeError("Date is not within ISO date time limits.")
	}
	return d, nil
}

// balanceISOYearMonth is BalanceISOYearMonth.
func balanceISOYearMonth(year, month int64) (int64, int) {
	return year + floorDiv(month-1, 12), int(floorMod(month-1, 12)) + 1
}

// compare is CompareISODate.
func (d ISODate) compare(o ISODate) int {
	switch {
	case d.Year != o.Year:
		return sign(d.Year - o.Year)
	case d.Month != o.Month:
		return sign(d.Month - o.Month)
	}
	return sign(d.Day - o.Day)
}

func sign(v int) int {
	switch {
	case v < 0:
		return -1
	case v > 0:
		return 1
	}
	return 0
}

// A DateDuration is a duration's date part, Temporal's Date Duration
// Record: years, months, weeks and days, all of one sign.
type DateDuration struct {
	Years, Months, Weeks, Days int64
}

// sign is the duration's sign.
func (d DateDuration) sign() int {
	for _, v := range []int64{d.Years, d.Months, d.Weeks, d.Days} {
		if v != 0 {
			return sign64(v)
		}
	}
	return 0
}

func sign64(v int64) int {
	switch {
	case v < 0:
		return -1
	case v > 0:
		return 1
	}
	return 0
}

// addISODate is AddISODate.
func addISODate(d ISODate, dur DateDuration, overflow Overflow) (ISODate, error) {
	year, month := balanceISOYearMonth(int64(d.Year)+dur.Years, int64(d.Month)+dur.Months)
	if year < -1<<31 || year > 1<<31-1 {
		year = clamp64(year, -1<<31, 1<<31-1)
	}
	intermediate, err := newISODate(int(year), month, d.Day, overflow)
	if err != nil {
		return ISODate{}, err
	}
	days := ISODate{intermediate.Year, intermediate.Month, 1}.EpochDays() + int64(intermediate.Day) + dur.Days + 7*dur.Weeks - 1
	if days > maxEpochDays || days < -maxEpochDays {
		return ISODate{}, rangeError("epoch days exceed maximum range.")
	}
	return ISODateFromEpochDays(days), nil
}

func clamp64(v, lo, hi int64) int64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// isoDateSurpasses is ISODateSurpasses.
func isoDateSurpasses(sgn int, year, month, day int, target ISODate) bool {
	if year != target.Year {
		return sgn*(year-target.Year) > 0
	}
	if month != target.Month {
		return sgn*(month-target.Month) > 0
	}
	if day != target.Day {
		return sgn*(day-target.Day) > 0
	}
	return false
}

// differenceISODate is DifferenceISODate.
func differenceISODate(one, two ISODate, largest Unit) (DateDuration, error) {
	sgn := -one.compare(two)
	if sgn == 0 {
		return DateDuration{}, nil
	}
	var years, months int
	if largest == Year || largest == Month {
		candidateYears := two.Year - one.Year
		if candidateYears != 0 {
			candidateYears -= sgn
		}
		for !isoDateSurpasses(sgn, one.Year+candidateYears, one.Month, one.Day, two) {
			years = candidateYears
			candidateYears += sgn
		}
		candidateMonths := sgn
		y, m := balanceISOYearMonth(int64(one.Year+years), int64(one.Month+candidateMonths))
		for !isoDateSurpasses(sgn, int(y), m, one.Day, two) {
			months = candidateMonths
			candidateMonths += sgn
			y, m = balanceISOYearMonth(y, int64(m+sgn))
		}
		if largest == Month {
			months += years * 12
			years = 0
		}
	}
	y, m := balanceISOYearMonth(int64(one.Year+years), int64(one.Month+months))
	constrained, err := newISODate(int(y), m, one.Day, Constrain)
	if err != nil {
		return DateDuration{}, err
	}
	days := two.EpochDays() - constrained.EpochDays()
	var weeks int64
	if largest == Week {
		weeks, days = days/7, days%7
	}
	return DateDuration{Years: int64(years), Months: int64(months), Weeks: weeks, Days: days}, nil
}

// A Unit is one of Temporal's units of time, from the smallest, after
// NoUnit and UnitAuto.
type Unit int

const (
	// NoUnit is a unit option not given: unset, which is not "auto". It is
	// the zero value, so that an option struct's zero value is the default.
	NoUnit Unit = iota
	UnitAuto
	Nanosecond
	Microsecond
	Millisecond
	Second
	Minute
	Hour
	Day
	Week
	Month
	Year
)
