// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package intl // import "github.com/liustack/quickjs/intl"

import (
	"strings"
	"time"
)

// calendarPrefixes are the calendars there is data for and the key prefix of
// that data. The buddhist and persian ones are there only for the locales
// they are the default calendar of (th, fa): see the generator.
var calendarPrefixes = map[string]string{
	"gregory":  "g",
	"buddhist": "b",
	"persian":  "p",
}

// calendarData is the date data of one locale in one calendar.
type calendarData struct {
	data   *cldrData
	name   string // the BCP 47 name: gregory, buddhist, persian
	prefix string
}

// calendarFor returns the data of calendar ca in d, or of the gregorian
// calendar when there is none for ca.
func calendarFor(d *cldrData, ca string) *calendarData {
	if p, ok := calendarPrefixes[ca]; ok && ca != "gregory" {
		if _, ok := d.get(p + "/months/format/wide/1"); ok {
			return &calendarData{d, ca, p}
		}
	}
	return &calendarData{d, "gregory", "g"}
}

// defaultCalendar returns the calendar of region, as CLDR's
// calendarPreferenceData has it.
func defaultCalendar(region string) string {
	switch c, _ := cldr().supplemental().get("ca/" + region); c {
	case "":
		return "gregory"
	case "gregorian":
		return "gregory"
	default:
		return c
	}
}

func (c *calendarData) key() string { return c.data.name + "/" + c.prefix }

// get returns a value of the calendar, falling back to the gregorian one as
// ICU does for data a calendar does not have.
func (c *calendarData) get(key string) (string, bool) {
	if v, ok := c.data.get(c.prefix + "/" + key); ok || c.prefix == "g" {
		return v, ok
	}

	return c.data.get("g/" + key)
}

func (c *calendarData) str(key string) string {
	s, _ := c.get(key)
	return s
}

// eachAvailableFormat calls f with the skeleton and pattern of every
// availableFormats entry of the calendar, in key order.
func (c *calendarData) eachAvailableFormat(f func(skel, pattern string)) {
	prefix := c.prefix + "/dateTimeFormats/availableFormats/"
	c.data.eachNearestFirst(prefix, func(k, v string) { f(strings.TrimPrefix(k, prefix), v) })
}

// dateFields are the calendar fields of an instant in a time zone.
type dateFields struct {
	era, year     int // year within the era
	month, day    int // month 0-11
	weekday       int // 0 is Sunday
	hour, minute  int
	second, milli int
	offset        int // seconds east of UTC
	dst           bool
	t             time.Time // the instant in its zone
}

// The julian day numbers of 1970-01-01 and of the gregorian reform,
// 1582-10-15, before which ICU's gregorian calendar is the julian one.
const (
	epochJulianDay   = 2440588
	cutoverJulianDay = 2299161
)

// fields computes the fields of t, which is in the formatter's zone, in
// calendar ca.
func fields(t time.Time, ca string) dateFields {
	_, offset := t.Zone()
	f := dateFields{
		weekday: int(t.Weekday()),
		hour:    t.Hour(), minute: t.Minute(), second: t.Second(), milli: t.Nanosecond() / 1e6,
		offset: offset, dst: t.IsDST(), t: t,
	}
	y, m, d := t.Date()
	switch ca {
	case "buddhist":
		// ICU's buddhist calendar is its gregorian one, julian before the
		// reform, counted from 543 BC.
		jd := julianDay(t)
		if jd < cutoverJulianDay {
			y, m, d = julianDate(jd)
		}
		f.era, f.year, f.month, f.day = 0, y+543, int(m)-1, d
	case "persian":
		f.era = 0
		f.year, f.month, f.day = persianDate(julianDay(t))
	default:
		// Browsers make the gregorian calendar proleptic, without the julian
		// part ICU has by default.
		f.era, f.year = 1, y
		if y <= 0 {
			f.era, f.year = 0, 1-y
		}
		f.month, f.day = int(m)-1, d
	}
	return f
}

// julianDay returns the julian day number of t's local date.
func julianDay(t time.Time) int64 {
	y, m, d := t.Date()
	days := time.Date(y, m, d, 0, 0, 0, 0, time.UTC).Unix() / 86400
	return days + epochJulianDay
}

// julianDate converts a julian day number to a date of the julian calendar.
func julianDate(jd int64) (int, time.Month, int) {
	c := jd + 32082
	d := floorDiv(4*c+3, 1461)
	e := c - floorDiv(1461*d, 4)
	m := floorDiv(5*e+2, 153)
	day := e - floorDiv(153*m+2, 5) + 1
	month := m + 3 - 12*floorDiv(m, 10)
	year := d - 4800 + floorDiv(m, 10)
	return int(year), time.Month(month), int(day)
}

func floorDiv(a, b int64) int64 {
	q := a / b
	if a%b != 0 && (a < 0) != (b < 0) {
		q--
	}
	return q
}

// The persian calendar as ICU computes it (persncal.cpp): the 33 year
// arithmetic cycle, with the years where the astronomical calendar differs
// from it listed.
const persianEpoch = 1948320

var persianMonthStart = [12]int64{0, 31, 62, 93, 124, 155, 186, 216, 246, 276, 306, 336}

var persianNonLeap = map[int64]bool{}

func init() {
	for _, y := range []int64{
		1502, 1601, 1634, 1667, 1700, 1733, 1766, 1799, 1832, 1865, 1898, 1931, 1964, 1997, 2030, 2059,
		2063, 2096, 2129, 2158, 2162, 2191, 2195, 2224, 2228, 2257, 2261, 2290, 2294, 2323, 2327, 2356,
		2360, 2389, 2393, 2422, 2426, 2455, 2459, 2488, 2492, 2521, 2525, 2554, 2558, 2587, 2591, 2620,
		2624, 2653, 2657, 2686, 2690, 2719, 2723, 2748, 2752, 2756, 2781, 2785, 2789, 2818, 2822, 2847,
		2851, 2855, 2880, 2884, 2888, 2913, 2917, 2921, 2946, 2950, 2954, 2979, 2983, 2987,
	} {
		persianNonLeap[y] = true
	}
}

const persianMinCorrection = 1502

func persianFirstDay(year int64) int64 {
	d := 365*(year-1) + floorDiv(8*year+21, 33)
	if year > persianMinCorrection && persianNonLeap[year-1] {
		d--
	}
	return d
}

func persianDate(jd int64) (year, month, day int) {
	days := jd - persianEpoch
	y := floorDiv(33*days+3, 12053) + 1
	doy := days - persianFirstDay(y)
	if doy == 365 && y >= persianMinCorrection && persianNonLeap[y] {
		y++
		doy = 0
	}
	m := int64(0)
	if doy < 216 {
		m = doy / 31
	} else {
		m = (doy - 6) / 30
	}
	return int(y), int(m), int(doy + 1 - persianMonthStart[m])
}
