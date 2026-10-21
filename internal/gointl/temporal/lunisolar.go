package temporal

import "github.com/liustack/quickjs/internal/gointl/internal/eastasian"

// The lunisolar calendars, whose years have a thirteenth month: the Hebrew,
// and the Chinese and Korean, as icu_calendar 2.2.1 reckons them.

// The Hebrew calendar is arithmetic, and ICU4X's keviyah tables give the
// same years as Calendrical Calculations' rules, which these are.

// hebrewEpoch is the Hebrew calendar's epoch, 1 Tishri AM 1.
var hebrewEpoch = julianFixed(-3760, 10, 7)

func hebrewLeap(year int) bool { return floorMod(7*int64(year)+1, 19) < 7 }

// hebrewElapsedDays is hebrew_calendar_elapsed_days: the days from the
// epoch to the molad of Tishri, deferred for the day of the week.
func hebrewElapsedDays(year int) int64 {
	months := floorDiv(235*int64(year)-234, 19)
	parts := 12084 + 13753*months
	days := 29*months + floorDiv(parts, 25920)
	if floorMod(3*(days+1), 7) < 3 {
		days++
	}
	return days
}

// hebrewNewYear is the day Tishri 1 falls on, with the corrections for
// years of impossible length.
func hebrewNewYear(year int) int64 {
	ny0, ny1, ny2 := hebrewElapsedDays(year-1), hebrewElapsedDays(year), hebrewElapsedDays(year+1)
	correction := int64(0)
	switch {
	case ny2-ny1 == 356:
		correction = 2
	case ny1-ny0 == 382:
		correction = 1
	}
	return hebrewEpoch + ny1 + correction
}

type hebrewRules struct{}

func (hebrewRules) year(extended int) calYear {
	start := hebrewNewYear(extended)
	length := hebrewNewYear(extended+1) - start
	y := calYear{extended: extended, start: start, count: 12, leapYear: hebrewLeap(extended)}
	months := []uint8{30, 29, 30, 29, 30, 29, 30, 29, 30, 29, 30, 29}
	if length == 355 || length == 385 {
		// A long Heshvan.
		months[1] = 30
	}
	if length == 353 || length == 383 {
		// A short Kislev.
		months[2] = 29
	}
	if y.leapYear {
		// Adar I, of thirty days, before Adar.
		months = append(months[:5], append([]uint8{30}, months[5:]...)...)
		y.count = 13
		y.leap = 6
	}
	copy(y.lengths[:], months)
	return y
}

func (r hebrewRules) yearOfRD(rd int64) calYear {
	year := int(floorDiv((rd-hebrewEpoch)*98496, 35975351)) + 1
	for hebrewNewYear(year) > rd {
		year--
	}
	for hebrewNewYear(year+1) <= rd {
		year++
	}
	return r.year(year)
}

// ordinalFromMonth is Hebrew's: Adar I is M05L, which a common year
// constrains to Adar.
func (hebrewRules) ordinalFromMonth(y calYear, m month, constrain bool) (int, errMonth) {
	switch {
	case !m.leap && m.number >= 1 && m.number <= 12:
		if m.number >= 6 && y.leapYear {
			return m.number + 1, monthOK
		}
		return m.number, monthOK
	case m.leap && m.number == 5:
		if y.leapYear || constrain {
			return 6, monthOK
		}
		return 0, monthNotInYear
	}
	return 0, monthNotInCalendar
}

func (hebrewRules) monthFromOrdinal(y calYear, ordinal int) month {
	if y.leapYear && ordinal >= 6 {
		return month{number: ordinal - 1, leap: ordinal == 6}
	}
	return month{number: ordinal}
}

func (hebrewRules) eraYear(y calYear, month, day int) (string, int, bool) {
	return "am", y.extended, true
}

func (hebrewRules) extendedFromEra(era string, year int) (int, bool) { return year, era == "am" }

func (hebrewRules) referenceYear(m month, day int) (int, errReference) {
	switch {
	case !m.leap && m.number == 1:
		return 5733, referenceOK
	case !m.leap && (m.number == 2 || m.number == 3):
		if day <= 29 {
			return 5733, referenceOK
		}
		return 5732, referenceOK
	case !m.leap && m.number == 4:
		if day <= 26 {
			return 5733, referenceOK
		}
		return 5732, referenceOK
	case !m.leap && m.number >= 5 && m.number <= 12:
		return 5732, referenceOK
	case m.leap && m.number == 5:
		return 5730, referenceOK
	}
	return 0, referenceNotInCalendar
}

func (hebrewRules) minMonthsFrom(y calYear, years int) int { return 235 * years / 19 }

// The Chinese and Korean calendars, as internal/eastasian reckons them,
// which DateTimeFormat's are too.

const msPerDay = 86400000

// eastAsianRules are ICU4X's China or Korea rules.
type eastAsianRules struct {
	rules  eastasian.Rules
	korean bool
}

func (r eastAsianRules) year(related int) calYear { return calYearOf(r.rules.Year(related)) }

// yearOfRD is Rules::year_containing_rd.
func (r eastAsianRules) yearOfRD(rd int64) calYear { return calYearOf(r.rules.YearOfRD(rd)) }

func calYearOf(y eastasian.Year) calYear {
	return calYear{extended: y.Related, start: y.Start, lengths: y.Lengths, count: y.Count,
		leap: y.Leap, leapYear: y.Leap != 0}
}

func (eastAsianRules) ordinalFromMonth(y calYear, m month, constrain bool) (int, errMonth) {
	if m.number < 1 || m.number > 12 {
		return 0, monthNotInCalendar
	}
	sentinel := 14
	if y.leap != 0 {
		sentinel = y.leap
	}
	if m.leap && m.number == sentinel-1 {
		return sentinel, monthOK
	}
	if m.leap && !constrain {
		return 0, monthNotInYear
	}
	if m.number >= sentinel {
		return m.number + 1, monthOK
	}
	return m.number, monthOK
}

func (eastAsianRules) monthFromOrdinal(y calYear, ordinal int) month {
	leap := 14
	if y.leap != 0 {
		leap = y.leap
	}
	if ordinal >= leap {
		return month{number: ordinal - 1, leap: ordinal == leap}
	}
	return month{number: ordinal}
}

// The Chinese and Korean calendars have no eras in Temporal.
func (eastAsianRules) eraYear(y calYear, month, day int) (string, int, bool) { return "", 0, false }
func (eastAsianRules) extendedFromEra(era string, year int) (int, bool)      { return 0, false }

func (eastAsianRules) minMonthsFrom(y calYear, years int) int { return 12*years + years/3 }

// referenceYear is ecma_reference_year_common.
func (r eastAsianRules) referenceYear(m month, day int) (int, errReference) {
	long := day > 29
	regular := func() (int, errReference) { return 0, referenceUseRegularIfConstrain }
	switch {
	case m.number < 1 || m.number > 12:
		return 0, referenceNotInCalendar
	case !m.leap:
		switch m.number {
		case 1:
			if long {
				return 1970, referenceOK
			}
			return 1972, referenceOK
		case 2, 5, 7, 9, 10:
			return 1972, referenceOK
		case 3:
			if !long {
				return 1972, referenceOK
			}
			if r.korean {
				return 1968, referenceOK
			}
			return 1966, referenceOK
		case 4:
			if long {
				return 1970, referenceOK
			}
			return 1972, referenceOK
		case 6, 8:
			if long {
				return 1971, referenceOK
			}
			return 1972, referenceOK
		case 11:
			switch {
			case long:
				return 1969, referenceOK
			case day > 26:
				return 1971, referenceOK
			}
			return 1972, referenceOK
		}
		return 1971, referenceOK
	}
	pick := func(short, longYear int) (int, errReference) {
		if long {
			if longYear == 0 {
				return regular()
			}
			return longYear, referenceOK
		}
		if short == 0 {
			return regular()
		}
		return short, referenceOK
	}
	switch m.number {
	case 1:
		return regular()
	case 2:
		return pick(1947, 0)
	case 3:
		return pick(1966, 1955)
	case 4:
		return pick(1963, 1944)
	case 5:
		return pick(1971, 1952)
	case 6:
		return pick(1960, 1941)
	case 7:
		return pick(1968, 1938)
	case 8:
		return pick(1957, 0)
	case 9:
		return pick(2014, 0)
	case 10:
		return pick(1984, 0)
	case 11:
		return pick(2033, 0)
	}
	return regular()
}
