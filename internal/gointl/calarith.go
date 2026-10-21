package intl

import (
	"math"
	"time"
)

// The arithmetic calendars, as ICU 78 reckons them. Each is a few divisions
// on the Julian day; the comments name the ICU source each follows.

// ceDate is jdToCE (cecal.cpp), the Coptic and Ethiopic reckoning: twelve
// months of thirty days and a thirteenth of five or six, in a four-year
// cycle, counted from an epoch. It returns the extended year, the month from
// one, the day and the day of the year.
func ceDate(jd, epoch int) (year, month, day, dayOfYear int) {
	days := jd - epoch
	c4 := floorDiv(days, 1461)
	r4 := days - c4*1461
	year = 4*c4 + (r4/365 - r4/1460)
	doy := r4 % 365
	if r4 == 1460 {
		doy = 365
	}
	return year, doy/30 + 1, doy%30 + 1, doy + 1
}

// ceYearLength is 366 in the fourth year of each cycle.
func ceYearLength(year int) int {
	if mod(year, 4) == 3 {
		return 366
	}
	return 365
}

// The epochs of the Coptic and Ethiopic calendars (coptccal.cpp,
// ethpccal.cpp), and the years between the Ethiopic eras.
const (
	copticEpoch     = 1824665
	ethiopicEpoch   = 1723856
	ameteAlemEpoch  = -285019
	ameteMihretDiff = 5500
)

// The Islamic calendar's epochs: the civil one counts from 16 July 622
// (Julian), the tabular one from the day before (islamcal.cpp).
const (
	islamicCivilEpoch = 1948440
	islamicAstroEpoch = 1948439
)

// islamicMonthStart is IslamicCivilCalendar::monthStart: the day, from the
// epoch, a month of the tabular calendar starts on, the month from zero.
func islamicMonthStart(year, month int) int {
	return int(math.Ceil(29.5*float64(month))) + 354*(year-1) + floorDiv(11*year+3, 30)
}

// islamicTabularDate is IslamicCivilCalendar::handleComputeFields.
func islamicTabularDate(jd, epoch int) (year, month, day, dayOfYear int) {
	days := jd - epoch
	year = floorDiv(30*days+10646, 10631)
	m := int(math.Ceil(float64(days-29-islamicMonthStart(year, 0)) / 29.5))
	if m > 11 {
		m = 11
	}
	day = days - islamicMonthStart(year, m) + 1
	dayOfYear = days - islamicMonthStart(year, 0) + 1
	return year, m + 1, day, dayOfYear
}

// islamicTabularYearLength is 355 in a leap year of the 30-year cycle.
func islamicTabularYearLength(year int) int {
	return 354 + b2i(civilLeapYear(year))
}

// islamicRelatedYear is gregoYearFromIslamicStart, ICU's rough Gregorian
// year for an Islamic one.
func islamicRelatedYear(year int) int {
	var cycle, offset, shift int
	if year >= 1397 {
		cycle = (year - 1397) / 67
		offset = (year - 1397) % 67
		shift = 2 * cycle
		if offset >= 33 {
			shift++
		}
	} else {
		cycle = (year-1396)/67 - 1
		offset = -(year - 1396) % 67
		shift = 2 * cycle
		if offset <= 33 {
			shift++
		}
	}
	return year + 579 - shift
}

// The Indian national calendar starts its year on the 22nd of March, the
// 80th day of the Gregorian year, and counts its years from AD 78
// (indiancal.cpp).
const (
	indianEraStart  = 78
	indianYearStart = 80
)

// indianDate is IndianCalendar::handleComputeFields.
func indianDate(t time.Time) (year, month, day, dayOfYear int) {
	gregorian := t.Year()
	year = gregorian - indianEraStart
	yday := t.YearDay() - 1
	var leapMonth int
	if yday < indianYearStart {
		year--
		leapMonth = 30
		if gregorianYearLength(gregorian-1) == 366 {
			leapMonth = 31
		}
		yday += leapMonth + 31*5 + 30*3 + 10
	} else {
		leapMonth = 30
		if gregorianYearLength(gregorian) == 366 {
			leapMonth = 31
		}
		yday -= indianYearStart
	}
	switch {
	case yday < leapMonth:
		month, day = 0, yday+1
	case yday-leapMonth < 31*5:
		m := yday - leapMonth
		month, day = m/31+1, m%31+1
	default:
		m := yday - leapMonth - 31*5
		month, day = m/30+6, m%30+1
	}
	return year, month + 1, day, yday + 1
}

// indianYearLength follows the Gregorian year it mostly falls in.
func indianYearLength(year int) int {
	return gregorianYearLength(year + indianEraStart)
}
