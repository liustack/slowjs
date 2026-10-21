package intl

import (
	"fmt"
	"math"
	"strings"
)

// The Islamic calendars that are not pure arithmetic, as ICU 78 reckons them
// (islamcal.cpp).
//
// The astronomical one, "islamic" and "islamic-rgsa" alike, starts a month
// on the first day whose midnight, in UTC, finds the moon past new by
// CalendarAstronomer's reckoning. Umm al-Qura is Saudi Arabia's published
// calendar: ICU's table of which months of 1300 to 1600 AH have thirty
// days, and the civil calendar's arithmetic outside it.

// hijraMillis is 16 July 622 (Julian), midnight UTC, the astronomical
// calendar's epoch.
const hijraMillis = -42521587200000.0

// islamicMoonAge is IslamicCalendar's moonAge: the moon's age at a moment,
// in degrees from -180 to 180.
func islamicMoonAge(ms float64) float64 {
	pi := float64(astroPI)
	age := newAstronomer(ms).moonAge() * 180 / pi
	if age > 180 {
		age = age - 360
	}
	return age
}

// islamicTrueMonthStart is trueMonthStart: the day, from the epoch, month
// number n (from zero at the epoch) of the astronomical calendar starts on.
func islamicTrueMonthStart(month int) int {
	origin := hijraMillis + float64(math.Floor(float64(month)*astroSynodicMonth)*astroDayMS)
	if islamicMoonAge(origin) >= 0 {
		// The month has already started.
		for {
			origin -= astroDayMS
			if islamicMoonAge(origin) < 0 {
				break
			}
		}
	} else {
		// The month before has not yet ended.
		for {
			origin += astroDayMS
			if islamicMoonAge(origin) >= 0 {
				break
			}
		}
	}
	return int(floorDiv64(int64(origin-hijraMillis), 86400000) + 1)
}

// islamicAstroDate is IslamicCalendar::handleComputeFields. It takes the
// moment as well as its day: ICU guesses the month from the moon's age at
// the moment itself, and the guess, when it is a month short, stands.
func islamicAstroDate(jd int, ms float64) (year, month, day, dayOfYear int) {
	days := jd - islamicCivilEpoch
	m := int(math.Floor(float64(days) / astroSynodicMonth))
	startDate := int(math.Floor(float64(m) * astroSynodicMonth))
	if days-startDate >= 25 && islamicMoonAge(ms) > 0 {
		// Near the end of a month, guess the next and search back.
		m++
	}
	for islamicTrueMonthStart(m) > days {
		m--
	}
	if m >= 0 {
		year = m/12 + 1
	} else {
		year = (m + 1) / 12
	}
	m = (m%12 + 12) % 12
	day = days - islamicTrueMonthStart(12*(year-1)+m) + 1
	dayOfYear = days - islamicTrueMonthStart(12*(year-1)) + 1
	return year, m + 1, day, dayOfYear
}

// islamicAstroYearLength is IslamicCalendar::handleGetYearLength.
func islamicAstroYearLength(year int) int {
	return islamicTrueMonthStart(12*year) - islamicTrueMonthStart(12*(year-1))
}

// civilLeapYear is islamcal.cpp's, whose remainder keeps the sign of the
// dividend, so that every year before 0 is a leap year.
func civilLeapYear(year int) bool {
	return (14+11*year)%30 < 11
}

// civilYearStart is IslamicCivilCalendar::yearStart.
func civilYearStart(year int) int {
	return 354*(year-1) + floorDiv(3+11*year, 30)
}

// civilMonthLength is IslamicCivilCalendar::handleGetMonthLength, the month
// from zero.
func civilMonthLength(year, month int) int {
	length := 29 + (month+1)%2
	if month == 11 && civilLeapYear(year) {
		length++
	}
	return length
}

// ummAlQura is the Umm al-Qura calendar's table, as ICU keeps it.
type ummAlQura struct {
	first  int
	months []int // a mask a year, bit 1<<(11-m) for a thirty-day month m
	fixes  []int // the correction to each year's estimated start
}

// loadUmmAlQura reads the table.
func loadUmmAlQura(src Source) (*ummAlQura, error) {
	b, err := src.Open(MarkerUmmAlQura, DataLocale{})
	if err != nil {
		return nil, fmt.Errorf("intl: the Umm al-Qura calendar: %w", err)
	}
	u := &ummAlQura{}
	for _, line := range strings.Split(string(b), "\n") {
		var year, mask, fix int
		if n, _ := fmt.Sscan(line, &year, &mask, &fix); n != 3 {
			continue
		}
		if len(u.months) == 0 {
			u.first = year
		} else if year != u.first+len(u.months) {
			return nil, fmt.Errorf("intl: the Umm al-Qura calendar: year %d out of order", year)
		}
		u.months = append(u.months, mask)
		u.fixes = append(u.fixes, fix)
	}
	if len(u.months) == 0 {
		return nil, fmt.Errorf("intl: the Umm al-Qura calendar is empty")
	}
	return u, nil
}

func (u *ummAlQura) covers(year int) bool {
	return year >= u.first && year < u.first+len(u.months)
}

// yearStart is IslamicUmalquraCalendar::yearStart: a least-squares fit to
// the table, corrected year by year.
func (u *ummAlQura) yearStart(year int) int {
	if !u.covers(year) {
		return civilYearStart(year)
	}
	i := year - u.first
	base := 460322.05
	return int(float64(354.36720*float64(i))+base+0.5) + u.fixes[i]
}

// monthLength is handleGetMonthLength, the month from zero.
func (u *ummAlQura) monthLength(year, month int) int {
	if !u.covers(year) {
		return civilMonthLength(year, month)
	}
	if u.months[year-u.first]&(1<<(11-month)) != 0 {
		return 30
	}
	return 29
}

// monthStart is monthStart: the year's start and the months before.
func (u *ummAlQura) monthStart(year, month int) int {
	ms := u.yearStart(year)
	for i := 0; i < month; i++ {
		ms += u.monthLength(year, i)
	}
	return ms
}

// yearLength is IslamicUmalquraCalendar::yearLength.
func (u *ummAlQura) yearLength(year int) int {
	if !u.covers(year) {
		return 354 + b2i(civilLeapYear(year))
	}
	length := 0
	for i := 0; i < 12; i++ {
		length += u.monthLength(year, i)
	}
	return length
}

// date is IslamicUmalquraCalendar::handleComputeFields.
func (u *ummAlQura) date(jd int) (year, month, day, dayOfYear int) {
	days := jd - islamicCivilEpoch
	if days < u.yearStart(u.first) {
		// Before the table, the civil calendar's arithmetic, though with
		// this calendar's own year and month starts, as ICU's virtual
		// calls give it.
		year = floorDiv(30*days+10646, 10631)
		m := int(math.Ceil(float64(days-29-u.yearStart(year)) / 29.5))
		if m > 11 {
			m = 11
		}
		return year, m + 1, days - u.monthStart(year, m) + 1, days - u.monthStart(year, 0) + 1
	}
	// An estimate of the year that is not past it, from the inverse of
	// the fit, then forward a year at a time.
	base := 460322.05
	year = int((float64(days)-(base+0.5))/354.36720 + float64(u.first) - 1)
	m := 0
	for d := 1; d > 0; {
		year++
		d = days - u.yearStart(year) + 1
		length := u.yearLength(year)
		if d == length {
			m = 11
			break
		}
		if d < length {
			for m = 0; d > u.monthLength(year, m); m++ {
				d -= u.monthLength(year, m)
			}
			break
		}
	}
	return year, m + 1, days - u.monthStart(year, m) + 1, days - u.monthStart(year, 0) + 1
}

// floorDiv64 is floorDiv for the milliseconds that outgrow a 32-bit int.
func floorDiv64(a, b int64) int64 {
	q := a / b
	if a%b != 0 && (a < 0) != (b < 0) {
		q--
	}
	return q
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}
