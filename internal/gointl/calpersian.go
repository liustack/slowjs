package intl

import "time"

// The Persian calendar, as ICU 78 reckons it (persncal.cpp): twelve months
// of 31, 30 and 29 or 30 days from the spring equinox, with leap years by a
// 33-year arithmetic, corrected in the years listed below where the
// arithmetic and the equinox as observed in Tehran disagree.

// persianEpoch is the Julian day before the first of Farvardin, year 1.
const persianEpoch = 1948320

// persianMonthStart is the day of the year each month starts on, from zero.
var persianMonthStart = [12]int{0, 31, 62, 93, 124, 155, 186, 216, 246, 276, 306, 336}

// persianNonLeapYears are the years the 33-year arithmetic calls leap and
// the equinox does not; the year after each is a leap year instead.
var persianNonLeapYears = [...]int{
	1502, 1601, 1634, 1667, 1700, 1733, 1766, 1799, 1832, 1865, 1898, 1931, 1964, 1997, 2030, 2059,
	2063, 2096, 2129, 2158, 2162, 2191, 2195, 2224, 2228, 2257, 2261, 2290, 2294, 2323, 2327, 2356,
	2360, 2389, 2393, 2422, 2426, 2455, 2459, 2488, 2492, 2521, 2525, 2554, 2558, 2587, 2591, 2620,
	2624, 2653, 2657, 2686, 2690, 2719, 2723, 2748, 2752, 2756, 2781, 2785, 2789, 2818, 2822, 2847,
	2851, 2855, 2880, 2884, 2888, 2913, 2917, 2921, 2946, 2950, 2954, 2979, 2983, 2987,
}

func persianCorrected(year int) bool {
	for _, y := range persianNonLeapYears {
		if y == year {
			return true
		}
	}
	return false
}

// persianYearLength is 366 in a leap year, as PersianCalendar::isLeapYear
// decides, and 365 otherwise.
func persianYearLength(year int) int {
	if year >= persianNonLeapYears[0] && persianCorrected(year) {
		return 365
	}
	if year > persianNonLeapYears[0] && persianCorrected(year-1) {
		return 366
	}
	if mod(25*year+11, 33) < 8 {
		return 366
	}
	return 365
}

// persianFirstDay is firstJulianOfYear: the first day of a year, counted
// from the epoch.
func persianFirstDay(year int) int {
	day := 365*(year-1) + floorDiv(8*year+21, 33)
	if year > persianNonLeapYears[0] && persianCorrected(year-1) {
		day--
	}
	return day
}

// persianDate is PersianCalendar::handleComputeFields.
func persianDate(jd int) (year, month, day int) {
	since := jd - persianEpoch
	year = floorDiv(33*since+3, 12053) + 1
	dayOfYear := since - persianFirstDay(year)
	if dayOfYear == 365 && year >= persianNonLeapYears[0] && persianCorrected(year) {
		year++
		dayOfYear = 0
	}
	if dayOfYear < 216 {
		month = dayOfYear / 31
	} else {
		month = (dayOfYear - 6) / 30
	}
	return year, month + 1, dayOfYear + 1 - persianMonthStart[month]
}

// julianDay is the Julian day of an instant's date in its own zone, as ICU
// counts it: whole days of local time since the Julian epoch.
func julianDay(t time.Time) int {
	year, month, day := t.Date()
	return int(time.Date(year, month, day, 0, 0, 0, 0, time.UTC).Unix()/86400) + 2440588
}

// floorDiv divides rounding toward minus infinity.
func floorDiv(a, b int) int {
	q := a / b
	if a%b != 0 && (a < 0) != (b < 0) {
		q--
	}
	return q
}
