// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package intl // import "github.com/liustack/quickjs/intl"

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// maxTimeValue is the largest magnitude of a javascript time value, ie. the
// range Date can represent.
const maxTimeValue = 8.64e15

// dateTimeFormat is a resolved Intl.DateTimeFormat. As with numberFormat its
// json form is both the resolvedOptions() result and the state the host gets
// back on every format call, so the fields are in the order ECMA-402 reports
// them.
type dateTimeFormat struct {
	Locale                 string `json:"locale"`
	Calendar               string `json:"calendar"`
	NumberingSystem        string `json:"numberingSystem"`
	TimeZone               string `json:"timeZone"`
	HourCycle              string `json:"hourCycle,omitempty"`
	Hour12                 *bool  `json:"hour12,omitempty"`
	Weekday                string `json:"weekday,omitempty"`
	Era                    string `json:"era,omitempty"`
	Year                   string `json:"year,omitempty"`
	Month                  string `json:"month,omitempty"`
	Day                    string `json:"day,omitempty"`
	DayPeriod              string `json:"dayPeriod,omitempty"`
	Hour                   string `json:"hour,omitempty"`
	Minute                 string `json:"minute,omitempty"`
	Second                 string `json:"second,omitempty"`
	FractionalSecondDigits int    `json:"fractionalSecondDigits,omitempty"`
	TimeZoneName           string `json:"timeZoneName,omitempty"`
	DateStyle              string `json:"dateStyle,omitempty"`
	TimeStyle              string `json:"timeStyle,omitempty"`
}

var (
	numericWidths = []string{"numeric", "2-digit"}
	textWidths    = []string{"narrow", "short", "long"}
	styleWidths   = []string{"full", "long", "medium", "short"}
)

func resolveDateTimeFormat(req request, defaultZone string) (*dateTimeFormat, error) {
	locale, err := resolveLocale(req.Locales)
	if err != nil {
		return nil, err
	}

	o := req.Options
	if o == nil {
		o = map[string]any{}
	}
	if _, err := optString(o, "localeMatcher", "lookup", "best fit"); err != nil {
		return nil, err
	}

	if _, err := optString(o, "formatMatcher", "basic", "best fit"); err != nil {
		return nil, err
	}

	// Only the gregorian calendar and latin digits are available. ECMA-402
	// resolves an unavailable request to the locale's default and reports what
	// it actually used, which is what happens here.
	f := &dateTimeFormat{Locale: locale, Calendar: "gregory", NumberingSystem: "latn"}
	if f.TimeZone, err = timeZoneOption(o, defaultZone); err != nil {
		return nil, err
	}

	for _, v := range []struct {
		p       *string
		key     string
		allowed []string
	}{
		{&f.Weekday, "weekday", textWidths},
		{&f.Era, "era", textWidths},
		{&f.Year, "year", numericWidths},
		{&f.Month, "month", append([]string{"narrow", "short", "long"}, numericWidths...)},
		{&f.Day, "day", numericWidths},
		{&f.DayPeriod, "dayPeriod", textWidths},
		{&f.Hour, "hour", numericWidths},
		{&f.Minute, "minute", numericWidths},
		{&f.Second, "second", numericWidths},
		{&f.TimeZoneName, "timeZoneName", []string{"short", "long", "shortOffset", "longOffset", "shortGeneric", "longGeneric"}},
		{&f.DateStyle, "dateStyle", styleWidths},
		{&f.TimeStyle, "timeStyle", styleWidths},
	} {
		if *v.p, err = optString(o, v.key, v.allowed...); err != nil {
			return nil, err
		}
	}
	if f.FractionalSecondDigits, _, err = optInt(o, "fractionalSecondDigits", 1, 3); err != nil {
		return nil, err
	}

	hourCycle, err := optString(o, "hourCycle", "h11", "h12", "h23", "h24")
	if err != nil {
		return nil, err
	}

	hour12, hasHour12 := optBool(o, "hour12")
	if hasHour12 {
		// hour12 wins over hourCycle. en-US spells the 24 hour clock h23.
		if hourCycle = "h23"; hour12 {
			hourCycle = "h12"
		}
	}

	hasComponents := f.Weekday != "" || f.Era != "" || f.Year != "" || f.Month != "" || f.Day != "" ||
		f.DayPeriod != "" || f.Hour != "" || f.Minute != "" || f.Second != "" || f.FractionalSecondDigits != 0 ||
		f.TimeZoneName != ""
	switch {
	case f.DateStyle != "" || f.TimeStyle != "":
		if hasComponents {
			return nil, typeError("can't set option %s when %s is used", "dateStyle or timeStyle", "a component option")
		}
	case !hasComponents:
		f.Year, f.Month, f.Day = "numeric", "numeric", "numeric"
	}

	// en-US writes a minute or a second sharing a pattern with another time
	// field with two digits, and resolvedOptions reports it that way.
	if f.Minute != "" && (f.Hour != "" || f.Second != "") {
		f.Minute = "2-digit"
	}
	if f.Second != "" && (f.Hour != "" || f.Minute != "") {
		f.Second = "2-digit"
	}
	if f.Hour != "" || f.TimeStyle != "" {
		if hourCycle == "" {
			hourCycle = "h12"
		}
		h12 := hourCycle == "h11" || hourCycle == "h12"
		f.HourCycle, f.Hour12 = hourCycle, &h12
		if !h12 && f.Hour != "" {
			// en-US writes the 24 hour clock with two digits, "00:00" where the
			// 12 hour one has "12:00 AM", and resolves the option to match.
			f.Hour = "2-digit"
		}
	}
	return f, nil
}

// timeZoneOption resolves the timeZone option, defaulting to defaultZone.
func timeZoneOption(o map[string]any, defaultZone string) (string, error) {
	v, ok := o["timeZone"]
	if !ok {
		return defaultZone, nil
	}

	s, ok := v.(string)
	if !ok {
		return "", rangeError("invalid time zone specified: %v", v)
	}

	name, _, err := timeZone(s)
	return name, err
}

// components holds the fields a format actually renders, with dateStyle and
// timeStyle expanded into them.
type components struct {
	weekday, era, year, month, day  string
	dayPeriod, hour, minute, second string
	fractionalSecondDigits          int
	timeZoneName, hourCycle         string
	wide                            bool // Join date and time with " at ".
	hasDate, hasTime                bool
}

// components expands f into the fields to render. The en-US date and time
// styles are the CLDR patterns for the locale: a full date names the weekday
// and spells the month out, a short one is all digits, and the wide two glue
// date to time with " at " rather than a comma.
func (f *dateTimeFormat) components() (c components) {
	c = components{
		weekday: f.Weekday, era: f.Era, year: f.Year, month: f.Month, day: f.Day,
		dayPeriod: f.DayPeriod, hour: f.Hour, minute: f.Minute, second: f.Second,
		fractionalSecondDigits: f.FractionalSecondDigits,
		timeZoneName:           f.TimeZoneName,
		hourCycle:              f.HourCycle,
	}
	switch f.DateStyle {
	case "full":
		c.weekday, c.year, c.month, c.day = "long", "numeric", "long", "numeric"
	case "long":
		c.year, c.month, c.day = "numeric", "long", "numeric"
	case "medium":
		c.year, c.month, c.day = "numeric", "short", "numeric"
	case "short":
		c.year, c.month, c.day = "2-digit", "numeric", "numeric"
	}
	switch f.TimeStyle {
	case "full", "long":
		c.hour, c.minute, c.second = "numeric", "2-digit", "2-digit"
		if c.timeZoneName = "short"; f.TimeStyle == "full" {
			c.timeZoneName = "long"
		}
	case "medium":
		c.hour, c.minute, c.second = "numeric", "2-digit", "2-digit"
	case "short":
		c.hour, c.minute = "numeric", "2-digit"
	}
	c.wide = c.month == "long" || f.DateStyle == "full" || f.DateStyle == "long"
	c.hasDate = c.weekday != "" || c.era != "" || c.year != "" || c.month != "" || c.day != ""
	c.hasTime = c.dayPeriod != "" || c.hour != "" || c.minute != "" || c.second != "" ||
		c.fractionalSecondDigits != 0 || c.timeZoneName != ""
	return c
}

// format renders the time value ms as ECMA-402 parts.
func (f *dateTimeFormat) format(ms float64) ([]part, error) {
	if math.IsNaN(ms) || math.IsInf(ms, 0) || math.Abs(ms) > maxTimeValue {
		return nil, rangeError("invalid time value")
	}

	_, loc, err := timeZone(f.TimeZone)
	if err != nil {
		return nil, err
	}

	t := time.UnixMilli(int64(ms)).In(loc)
	c := f.components()
	date, tim := f.dateParts(c, t), f.timeParts(c, t)
	switch {
	case len(date) == 0:
		return tim, nil
	case len(tim) == 0:
		return date, nil
	}

	sep := ", "
	switch {
	case c.wide:
		sep = " at "
	case len(date) == 1 && date[0].Type == "weekday":
		sep = " "
	}
	return append(append(date, part{"literal", sep}), tim...), nil
}

func (f *dateTimeFormat) dateParts(c components, t time.Time) (r []part) {
	if !c.hasDate {
		return nil
	}

	year, era := t.Year(), 1
	if year < 1 { // Proleptic year 0 is 1 BC.
		year, era = 1-year, 0
	}
	yearPart := part{"year", strconv.Itoa(year)}
	if c.year == "2-digit" {
		yearPart.Value = fmt.Sprintf("%02d", year%100)
	}
	monthPart := part{"month", monthName(c.month, t.Month())}
	dayPart := part{"day", pad(t.Day(), c.day == "2-digit")}
	weekdayPart := part{"weekday", weekdayNames[width(c.weekday)][int(t.Weekday())]}
	lit := func(s string) part { return part{"literal", s} }

	numericMonth := c.month == "numeric" || c.month == "2-digit"
	switch {
	case c.month != "" && numericMonth:
		if c.weekday != "" {
			r = append(r, weekdayPart, lit(", "))
		}
		r = append(r, monthPart)
		if c.day != "" {
			r = append(r, lit("/"), dayPart)
		}
		if c.year != "" {
			r = append(r, lit("/"), yearPart)
		}
	case c.month != "":
		if c.weekday != "" {
			r = append(r, weekdayPart, lit(", "))
		}
		r = append(r, monthPart)
		if c.day != "" {
			r = append(r, lit(" "), dayPart)
		}
		if c.year != "" {
			if c.day != "" {
				r = append(r, lit(", "))
			} else {
				r = append(r, lit(" "))
			}
			r = append(r, yearPart)
		}
	default:
		var seq []part
		if c.weekday != "" {
			seq = append(seq, weekdayPart)
		}
		if c.day != "" {
			seq = append(seq, dayPart)
		}
		if c.year != "" {
			seq = append(seq, yearPart)
		}
		for i, p := range seq {
			if i != 0 {
				r = append(r, lit(" "))
			}
			r = append(r, p)
		}
	}
	// A date before AD 1 names its era whether or not the option asked for it:
	// "1/1/2" with no era would be a different year altogether.
	if c.era != "" || era == 0 {
		if len(r) != 0 {
			r = append(r, lit(" "))
		}
		r = append(r, part{"era", eraNames[widthOr(c.era, "short")][era]})
	}
	return r
}

func (f *dateTimeFormat) timeParts(c components, t time.Time) (r []part) {
	if !c.hasTime {
		return nil
	}

	lit := func(s string) part { return part{"literal", s} }
	if c.hour != "" {
		hour, period := clockHour(t.Hour(), c.hourCycle)
		r = append(r, part{"hour", pad(hour, c.hour == "2-digit")})
		if c.minute != "" {
			r = append(r, lit(":"), part{"minute", pad(t.Minute(), c.minute == "2-digit")})
		}
		if c.second != "" {
			r = append(r, lit(":"), part{"second", pad(t.Second(), c.second == "2-digit")})
		}
		r = appendFractionalSecond(r, c, t)
		switch {
		case c.dayPeriod != "":
			r = append(r, lit(" "), part{"dayPeriod", dayPeriodName(t.Hour())})
		case period != "":
			r = append(r, lit(" "), part{"dayPeriod", period})
		}
	} else {
		if c.minute != "" {
			r = append(r, part{"minute", pad(t.Minute(), c.minute == "2-digit")})
		}
		if c.second != "" {
			if c.minute != "" {
				r = append(r, lit(":"))
			}
			r = append(r, part{"second", pad(t.Second(), c.second == "2-digit")})
		}
		r = appendFractionalSecond(r, c, t)
		if c.dayPeriod != "" {
			if len(r) != 0 {
				r = append(r, lit(" "))
			}
			r = append(r, part{"dayPeriod", dayPeriodName(t.Hour())})
		}
	}
	if c.timeZoneName != "" {
		if len(r) != 0 {
			r = append(r, lit(" "))
		}
		r = append(r, part{"timeZoneName", timeZoneNameText(t, c.timeZoneName, f.TimeZone)})
	}
	return r
}

func appendFractionalSecond(r []part, c components, t time.Time) []part {
	if c.fractionalSecondDigits == 0 {
		return r
	}

	s := fmt.Sprintf("%03d", t.Nanosecond()/int(time.Millisecond))[:c.fractionalSecondDigits]
	if len(r) != 0 {
		r = append(r, part{"literal", "."})
	}
	return append(r, part{"fractionalSecond", s})
}

// clockHour maps a 0-23 hour to the hour cycle's numbering and reports the day
// period to show, if any.
func clockHour(hour int, hourCycle string) (int, string) {
	switch hourCycle {
	case "h23":
		return hour, ""
	case "h24":
		if hour == 0 {
			return 24, ""
		}

		return hour, ""
	}
	period := "AM"
	if hour >= 12 {
		period = "PM"
	}
	h := hour % 12
	if h == 0 && hourCycle != "h11" {
		h = 12
	}
	return h, period
}

// timeZoneNameText renders the timeZoneName option. Only the abbreviation Go
// reads from the zone database and the GMT offset are available: CLDR's
// spelled out names ("Eastern Standard Time") are not shipped with Go, so a
// long name falls back to the offset, which CLDR itself does for a zone it has
// no name for.
func timeZoneNameText(t time.Time, style, name string) string {
	abbr, offset := t.Zone()
	switch style {
	case "short":
		if name == "UTC" {
			return "UTC"
		}

		if !hasShortName(name, abbr) {
			return gmtOffset(offset, true)
		}

		return abbr
	case "long":
		if name == "UTC" {
			return "Coordinated Universal Time"
		}

		return gmtOffset(offset, false)
	case "shortOffset", "shortGeneric":
		return gmtOffset(offset, true)
	}
	return gmtOffset(offset, false)
}

// northAmericanZones are the identifier prefixes whose zones CLDR gives en
// short names to.
var northAmericanZones = []string{"America/", "US/", "Canada/", "Mexico/", "Pacific/Honolulu", "Atlantic/Bermuda"}

// hasShortName reports whether en writes this zone's name as letters rather
// than as a GMT offset. CLDR only does so for North America, so "EST" is a time
// zone name in en-US while Europe/Prague is "GMT+1" and never "CET" - and the
// distinction matters because the zone database's abbreviations collide:
// Asia/Shanghai is CST there too, where en calls it GMT+8.
func hasShortName(name, abbr string) bool {
	switch {
	case abbr == "", abbr[0] == '+', abbr[0] == '-':
		return false
	}
	switch abbr {
	case "EST", "EDT", "CST", "CDT", "MST", "MDT", "PST", "PDT", "AKST", "AKDT", "HST", "HDT", "AST", "ADT":
		for _, prefix := range northAmericanZones {
			if strings.HasPrefix(name, prefix) {
				return true
			}
		}
	}
	return false
}

func monthName(width string, m time.Month) string {
	switch width {
	case "numeric":
		return strconv.Itoa(int(m))
	case "2-digit":
		return fmt.Sprintf("%02d", int(m))
	}
	return monthNames[widthOr(width, "long")][int(m)-1]
}

func pad(n int, two bool) string {
	if two {
		return fmt.Sprintf("%02d", n)
	}

	return strconv.Itoa(n)
}

func width(s string) string { return widthOr(s, "long") }

func widthOr(s, def string) string {
	switch s {
	case "narrow", "short", "long":
		return s
	}
	return def
}
