package intl

import "math"

// The Hebrew calendar, as ICU 78 reckons it (hebrwcal.cpp): lunar months
// kept to the solar year by a thirteenth month, Adar I, in seven years of
// every nineteen, and a year that starts at the new moon of Tishri, put off
// a day or two by the rules that keep holidays off certain days.
//
// ICU numbers the months from zero with Adar I as the sixth, month 5, in
// every year; a year without it simply skips it. go-intl keeps ICU's
// numbering, from one.

// The parts of an hour (halakim), a day, and the mean lunar month, and the
// new moon of Tishri in year 1, counted from noon the day before.
const (
	hebrewHourParts  = 1080
	hebrewDayParts   = 24 * hebrewHourParts
	hebrewMonthFract = 12*hebrewHourParts + 793
	hebrewMonthParts = 29*hebrewDayParts + hebrewMonthFract
	hebrewBaharad    = 11*hebrewHourParts + 204
)

// hebrewMonthStart and hebrewLeapMonthStart are the days of the year each
// month ends on, for years of each of the three lengths: deficient, normal
// and complete.
var hebrewMonthStart = [14][3]int{
	{0, 0, 0}, {30, 30, 30}, {59, 59, 60}, {88, 89, 90}, {117, 118, 119}, {147, 148, 149},
	{147, 148, 149}, {176, 177, 178}, {206, 207, 208}, {235, 236, 237}, {265, 266, 267},
	{294, 295, 296}, {324, 325, 326}, {353, 354, 355},
}

var hebrewLeapMonthStart = [14][3]int{
	{0, 0, 0}, {30, 30, 30}, {59, 59, 60}, {88, 89, 90}, {117, 118, 119}, {147, 148, 149},
	{177, 178, 179}, {206, 207, 208}, {236, 237, 238}, {265, 266, 267}, {295, 296, 297},
	{324, 325, 326}, {354, 355, 356}, {383, 384, 385},
}

// hebrewLeap is HebrewCalendar::isLeapYear.
func hebrewLeap(year int) bool {
	x := (year*12 + 17) % 19
	if x < 0 {
		return x >= -7
	}
	return x >= 12
}

// hebrewYearStart is startOfYear: the day, from ICU's Hebrew epoch, Tishri 1
// of a year falls on. The parts of a day since the epoch outgrow 32 bits,
// and are counted in 64, as ICU counts them.
func hebrewYearStart(year int) int {
	months := int64(floorDiv(235*year-234, 19))
	frac := months*hebrewMonthFract + hebrewBaharad
	day := int(months*29 + frac/hebrewDayParts)
	frac %= hebrewDayParts
	wd := day % 7 // 0 is Monday
	switch {
	case wd == 2 || wd == 4 || wd == 6:
		// Not on a Sunday, Wednesday or Friday.
		day++
	case wd == 1 && frac > 15*hebrewHourParts+204 && !hebrewLeap(year):
		day += 2
	case wd == 0 && frac > 21*hebrewHourParts+589 && hebrewLeap(year-1):
		day++
	}
	return day
}

// hebrewYearLength is the days from one Tishri 1 to the next.
func hebrewYearLength(year int) int {
	return hebrewYearStart(year+1) - hebrewYearStart(year)
}

// hebrewYearType is 0, 1 or 2 for a deficient, normal or complete year.
func hebrewYearType(year int) int {
	length := hebrewYearLength(year)
	if length > 380 {
		length -= 30
	}
	switch length {
	case 353:
		return 0
	case 355:
		return 2
	}
	return 1
}

// hebrewDate is HebrewCalendar::handleComputeFields: the year, the month in
// ICU's numbering from one (Adar I is 6 in any year), the day and the day
// of the year.
func hebrewDate(jd int) (year, month, day, dayOfYear int) {
	// The estimate of the year is ICU's, in doubles.
	d := jd - 347997
	m := math.Floor(float64(d) * float64(hebrewDayParts) / float64(hebrewMonthParts))
	year = int(math.Floor((float64(19.*m)+234.)/235.) + 1.)
	dayOfYear = d - hebrewYearStart(year)
	for dayOfYear < 1 {
		year--
		dayOfYear = d - hebrewYearStart(year)
	}
	kind := hebrewYearType(year)
	starts := &hebrewMonthStart
	if hebrewLeap(year) {
		starts = &hebrewLeapMonthStart
	}
	month = 0
	for month < len(starts) && dayOfYear > starts[month][kind] {
		month++
	}
	month--
	return year, month + 1, dayOfYear - starts[month][kind], dayOfYear
}
