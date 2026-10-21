package intl

import (
	"math"
	"time"
)

// The Chinese calendar and the Korean one, Dangi, as ICU 78 reckons them
// (chnsecal.cpp, dangical.cpp): lunar months that start on the day of the
// new moon, the eleventh containing the winter solstice, and a leap month,
// numbered as the month before it, in a year of thirteen wherever a month
// contains no major solar term. The sun and moon are CalendarAstronomer's
// (astro.go), and the days are counted in China's time, UTC+8, or Korea's.
//
// ICU caches the solstices and new years it finds; go-intl's formatters are
// immutable and keep no caches, so each date reckons its own, some tens of
// microseconds' work.

// synodicGap is SYNODIC_GAP: days to add to a new moon to come close to the
// next without passing it.
const synodicGap = 25

// chineseReckoner is ChineseCalendar with its Setting: the offset from UTC,
// in milliseconds at a UTC moment, of the zone the astronomy is done in.
type chineseReckoner struct {
	offset func(ms float64) float64
}

// chinaOffset is CHINA_OFFSET, UTC+8 at every moment.
func chinaOffset(float64) float64 { return 8 * 3600000 }

// Dangi's zone, dangical.cpp's KOREA_ZONE: UTC+8, but UTC+7 through 1897,
// an ad hoc fix to the astronomy there, and UTC+9 from 1912. Its rules give
// their starts as 365-day years from 1970 in the standard time of the rule
// before, which RuleBasedTimeZone takes back to UTC.
const (
	korea1897 = (1897-1970)*365*86400000.0 - 8*3600000
	korea1898 = (1898-1970)*365*86400000.0 - 7*3600000
	korea1912 = (1912-1970)*365*86400000.0 - 8*3600000
)

func koreaOffset(ms float64) float64 {
	switch {
	case ms < korea1897:
		return 8 * 3600000
	case ms < korea1898:
		return 7 * 3600000
	case ms < korea1912:
		return 8 * 3600000
	}
	return 9 * 3600000
}

// daysToMillis is the UTC moment the zone's day starts, the day counted
// from 1970.
func (c chineseReckoner) daysToMillis(days float64) float64 {
	// Rounded before the difference, as ICU's two statements are, where an
	// arm64 build would fuse them.
	millis := float64(days * astroDayMS)
	return millis - c.offset(millis)
}

// millisToDays is the zone's day a UTC moment falls on.
func (c chineseReckoner) millisToDays(ms float64) float64 {
	return math.Floor((ms + c.offset(ms)) / astroDayMS)
}

// winterSolstice is the day of the winter solstice in a Gregorian year,
// searched for from the first of December.
func (c chineseReckoner) winterSolstice(gyear int) int {
	dec1 := time.Date(gyear, time.December, 1, 0, 0, 0, 0, time.UTC).Unix() / 86400
	ms := c.daysToMillis(float64(dec1))
	pi := float64(astroPI)
	return int(c.millisToDays(newAstronomer(ms).sunTime(pi*3/2, true)))
}

// newMoonNear is the day of the new moon after, or before, a day.
func (c chineseReckoner) newMoonNear(days float64, after bool) int {
	ms := c.daysToMillis(days)
	return int(c.millisToDays(newAstronomer(ms).moonTime(0, after)))
}

// synodicMonthsBetween is the number of new moons from one day to another,
// rounded.
func synodicMonthsBetween(day1, day2 int) int {
	roundme := float64(day2-day1) / astroSynodicMonth
	if roundme >= 0 {
		return int(roundme + .5)
	}
	return int(roundme - .5)
}

// majorSolarTerm is the major solar term, 1 to 12, a day is in.
func (c chineseReckoner) majorSolarTerm(days int) int {
	ms := c.daysToMillis(float64(days))
	pi := float64(astroPI)
	term := (int(6*newAstronomer(ms).sunLongitude()/pi) + 2) % 12
	if term < 1 {
		term += 12
	}
	return term
}

// hasNoMajorSolarTerm reports whether the month starting at a new moon
// contains no major solar term.
func (c chineseReckoner) hasNoMajorSolarTerm(newMoon int) bool {
	return c.majorSolarTerm(newMoon) ==
		c.majorSolarTerm(c.newMoonNear(float64(newMoon+synodicGap), true))
}

// isLeapMonthBetween reports whether a leap month starts on or between two
// new moons.
func (c chineseReckoner) isLeapMonthBetween(newMoon1, newMoon2 int) bool {
	for newMoon2 >= newMoon1 {
		if c.hasNoMajorSolarTerm(newMoon2) {
			return true
		}
		newMoon2 = c.newMoonNear(float64(newMoon2-synodicGap), false)
	}
	return false
}

// newYear is the day of the Chinese new year in a Gregorian year: the
// second new moon after the winter solstice before it, or the third when
// a leap month comes between.
func (c chineseReckoner) newYear(gyear int) int {
	solsticeBefore := c.winterSolstice(gyear - 1)
	solsticeAfter := c.winterSolstice(gyear)
	newMoon1 := c.newMoonNear(float64(solsticeBefore+1), true)
	newMoon2 := c.newMoonNear(float64(newMoon1+synodicGap), true)
	newMoon11 := c.newMoonNear(float64(solsticeAfter+1), false)
	if synodicMonthsBetween(newMoon1, newMoon11) == 12 &&
		(c.hasNoMajorSolarTerm(newMoon1) || c.hasNoMajorSolarTerm(newMoon2)) {
		return c.newMoonNear(float64(newMoon2+synodicGap), true)
	}
	return newMoon2
}

// chineseDate is what ChineseCalendar::handleComputeFields sets.
type chineseDate struct {
	month       int // from one; a leap month has the number of the one before
	leap        bool
	cycle       int // the sixty-year cycle, from one: ICU's era
	yearOfCycle int // from one to sixty: ICU's year
	extYear     int // the Gregorian year the Chinese year starts in
	day         int
	dayOfYear   int
	// failed says ICU refuses the date: its solstices do not fall either
	// side of it.
	failed bool
}

// date reckons a local date, given as its Gregorian year and month and its
// day from 1970.
func (c chineseReckoner) date(gyear int, gmonth time.Month, days int) chineseDate {
	// computeMonthInfo: the winter solstices either side of the date
	// place month 11.
	solsticeBefore, solsticeAfter, ok := c.solsticesAround(gyear, days)
	failed := !ok
	firstMoon := c.newMoonNear(float64(solsticeBefore+1), true)
	lastMoon := c.newMoonNear(float64(solsticeAfter+1), false)
	thisMoon := c.newMoonNear(float64(days+1), false)
	hasLeap := synodicMonthsBetween(firstMoon, lastMoon) == 12
	month := synodicMonthsBetween(firstMoon, thisMoon)
	if hasLeap && c.isLeapMonthBetween(firstMoon, thisMoon) {
		month--
	}
	if month < 1 {
		month += 12
	}
	leap := hasLeap && c.hasNoMajorSolarTerm(thisMoon) &&
		!c.isLeapMonthBetween(firstMoon, c.newMoonNear(float64(thisMoon-synodicGap), false))

	eyear := gyear - 1
	cycleYear := gyear + 2636
	if month < 11 || gmonth >= time.July {
		eyear++
		cycleYear++
	}
	cycle := floorDiv(cycleYear-1, 60)
	yearOfCycle := cycleYear - 1 - cycle*60

	newYear := c.newYear(gyear)
	if days < newYear {
		newYear = c.newYear(gyear - 1)
	}
	return chineseDate{
		month: month, leap: leap,
		cycle: cycle + 1, yearOfCycle: yearOfCycle + 1,
		extYear: eyear,
		day:     days - thisMoon + 1, dayOfYear: days - newYear + 1,
		failed: failed,
	}
}

// solsticesAround is the start of computeMonthInfo: the winter solstices
// ICU finds before and after a day, and whether they are either side of
// it, which ICU fails where they are not; its astronomy comes to that some
// seventy thousand years from now and a hundred thousand ago.
func (c chineseReckoner) solsticesAround(gyear, days int) (before, after int, ok bool) {
	after = c.winterSolstice(gyear)
	if days < after {
		before = c.winterSolstice(gyear - 1)
	} else {
		before = after
		after = c.winterSolstice(gyear + 1)
	}
	return before, after, before <= days && days < after
}

// yearStartFails reports whether ICU fails to find the start of a Chinese
// year, by its extended year, as handleComputeMonthStartWithLeap does for
// its first month: computeMonthInfo for the new moon the year starts at.
func (c chineseReckoner) yearStartFails(eyear int) bool {
	newMoon := c.newMoonNear(float64(c.newYear(eyear)), true)
	gyear := time.Unix(int64(newMoon)*86400, 0).UTC().Year()
	_, _, ok := c.solsticesAround(gyear, newMoon)
	return !ok
}

// yearLength is the days from the new year of a Chinese year, by its
// extended year, to the next.
func (c chineseReckoner) yearLength(eyear int) int {
	return c.newYear(eyear+1) - c.newYear(eyear)
}
