// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package intl // import "github.com/liustack/quickjs/intl"

import (
	"strconv"
	"strings"
	"sync"
	"time"
)

// This file formats a date pattern the way ICU's SimpleDateFormat does
// (smpdtfmt.cpp), field by field, reporting the parts browsers return from
// formatToParts.

// patternItem is a field of a date pattern, or literal text when ch is 0.
type patternItem struct {
	ch  byte
	n   int
	lit string
}

// parsePattern splits a date pattern into fields and literals, removing the
// quoting.
func parsePattern(p string) (r []patternItem) {
	var lit strings.Builder
	flush := func() {
		if lit.Len() != 0 {
			r = append(r, patternItem{lit: plainSpaces.Replace(lit.String())})
			lit.Reset()
		}
	}
	inQuote := false
	for i := 0; i < len(p); i++ {
		c := p[i]
		switch {
		case c == '\'':
			if i+1 < len(p) && p[i+1] == '\'' {
				lit.WriteByte('\'')
				i++
				continue
			}

			inQuote = !inQuote
		case !inQuote && isASCIILetter(c):
			j := i
			for j+1 < len(p) && p[j+1] == c {
				j++
			}
			flush()
			r = append(r, patternItem{ch: c, n: j - i + 1})
			i = j
		default:
			lit.WriteByte(c)
		}
	}
	flush()
	return r
}

// partTypes maps the pattern letters to the formatToParts types.
func partType(c byte) string {
	switch c {
	case 'G':
		return "era"
	case 'y', 'Y', 'u':
		return "year"
	case 'U':
		return "yearName"
	case 'r':
		return "relatedYear"
	case 'M', 'L':
		return "month"
	case 'd':
		return "day"
	case 'h', 'H', 'k', 'K':
		return "hour"
	case 'm':
		return "minute"
	case 's':
		return "second"
	case 'E', 'e', 'c':
		return "weekday"
	case 'a', 'b', 'B':
		return "dayPeriod"
	case 'z', 'Z', 'v', 'V', 'O', 'X', 'x':
		return "timeZoneName"
	case 'S':
		return "fractionalSecond"
	}
	return "literal"
}

// dateFormatter formats instants with one pattern.
type dateFormatter struct {
	items                []patternItem
	cal                  *calendarData
	ns                   *numberSystem
	zones                *zoneNamer
	zoneID               string // the CLDR id names are looked up by, "" for an offset zone
	loc                  *time.Location
	dayPeriods           *dayPeriodRules
	hasMinute, hasSecond bool
}

func (f *dateFormatter) format(ms float64) []part {
	t := time.UnixMilli(int64(ms)).In(f.loc)
	d := fields(t, f.cal.name)
	var r []part
	for _, it := range f.items {
		if it.ch == 0 {
			r = appendLiteral(r, it.lit)
			continue
		}

		typ, s := f.field(it.ch, it.n, &d)
		s = plainSpaces.Replace(s)
		if typ == "literal" {
			r = appendLiteral(r, s)
			continue
		}

		r = append(r, part{typ, s})
	}
	return r
}

// plainSpaces turns ICU's narrow and thin spaces into plain ones, as browsers
// do (crbug.com/1414292). The literals of a pattern get it once, when the
// pattern is parsed, and field values on every format.
var plainSpaces = strings.NewReplacer("\u202f", " ", "\u2009", " ")

func appendLiteral(r []part, s string) []part {
	if s == "" {
		return r
	}

	if n := len(r); n > 0 && r[n-1].Type == "literal" {
		r[n-1].Value += s
		return r
	}

	return append(r, part{"literal", s})
}

// number writes n with at least min digits and at most max, keeping the low
// ones, in the formatter's digits.
func (f *dateFormatter) number(n, min, max int) string {
	neg := n < 0
	if neg {
		n = -n
	}
	s := strconv.Itoa(n)
	if len(s) > max {
		s = s[len(s)-max:]
	}
	if len(s) < min {
		s = strings.Repeat("0", min-len(s)) + s
	}
	s = f.ns.transliterate(s)
	if neg {
		s = f.ns.minus + s
	}
	return s
}

// symbol returns a calendar name, such as "months/format/wide/1".
func (f *dateFormatter) symbol(key string) string { return f.cal.str(key) }

var weekdayKeys = [7]string{"sun", "mon", "tue", "wed", "thu", "fri", "sat"}

func textWidth(n int) string {
	switch {
	case n == 4:
		return "wide"
	case n == 5:
		return "narrow"
	case n == 6:
		return "short"
	}
	return "abbreviated"
}

// field formats one pattern field, ICU's subFormat.
func (f *dateFormatter) field(c byte, n int, d *dateFields) (string, string) {
	typ := partType(c)
	const maxInt = 10
	switch c {
	case 'G':
		key := "eras/eraAbbr/"
		switch n {
		case 4:
			key = "eras/eraNames/"
		case 5:
			key = "eras/eraNarrow/"
		}
		return typ, f.symbol(key + strconv.Itoa(d.era))
	case 'y', 'Y', 'u':
		if n == 2 {
			return typ, f.number(d.year, 2, 2)
		}

		return typ, f.number(d.year, n, maxInt)
	case 'M', 'L':
		context := "format"
		if c == 'L' {
			context = "stand-alone"
		}
		if n >= 3 {
			w := textWidth(n)
			if n == 6 {
				w = "abbreviated"
			}
			return typ, f.symbol("months/" + context + "/" + w + "/" + strconv.Itoa(d.month+1))
		}

		return typ, f.number(d.month+1, n, maxInt)
	case 'd':
		return typ, f.number(d.day, n, maxInt)
	case 'E', 'e', 'c':
		if c != 'E' && n < 3 {
			return typ, f.number(d.weekday+1, n, maxInt)
		}

		context := "format"
		if c == 'c' {
			context = "stand-alone"
		}
		return typ, f.symbol("days/" + context + "/" + textWidth(n) + "/" + weekdayKeys[d.weekday])
	case 'a':
		return typ, f.amPm(n, d)
	case 'b':
		if d.hour == 12 && (!f.hasMinute || d.minute == 0) && (!f.hasSecond || d.second == 0) {
			if s, ok := f.cal.get("dayPeriods/format/" + periodWidth(n) + "/noon"); ok {
				return typ, s
			}
		}

		return typ, f.amPm(n, d)
	case 'B':
		return typ, f.flexibleDayPeriod(n, d)
	case 'h':
		h := d.hour % 12
		if h == 0 {
			h = 12
		}
		return typ, f.number(h, n, maxInt)
	case 'H':
		return typ, f.number(d.hour, n, maxInt)
	case 'K':
		return typ, f.number(d.hour%12, n, maxInt)
	case 'k':
		h := d.hour
		if h == 0 {
			h = 24
		}
		return typ, f.number(h, n, maxInt)
	case 'm':
		return typ, f.number(d.minute, n, maxInt)
	case 's':
		return typ, f.number(d.second, n, maxInt)
	case 'S':
		v := d.milli
		switch n {
		case 1:
			v /= 100
		case 2:
			v /= 10
		}
		s := f.number(v, min(n, 3), maxInt)
		if n > 3 {
			s += f.number(0, n-3, maxInt)
		}
		return typ, s
	case 'z':
		return typ, f.zones.specific(f.zoneID, d.t, n >= 4)
	case 'v':
		return typ, f.zones.generic(f.zoneID, d.t, n == 4)
	case 'O':
		return typ, f.zones.gmt(d.offset, n == 1)
	case 'Z':
		if n == 4 {
			return typ, f.zones.gmt(d.offset, false)
		}
	case 'V':
		if n == 4 {
			if l := f.zones.location(f.zoneID); l != "" && f.zoneID != "" {
				return typ, l
			}

			return typ, f.zones.gmt(d.offset, false)
		}
	}
	// Letters browsers never put in a pattern of their own.
	return "literal", ""
}

func periodWidth(n int) string {
	switch {
	case n == 4 || n > 5:
		return "wide"
	case n == 5:
		return "narrow"
	}
	return "abbreviated"
}

func (f *dateFormatter) amPm(n int, d *dateFields) string {
	key := "am"
	if d.hour >= 12 {
		key = "pm"
	}
	return f.symbol("dayPeriods/format/" + periodWidth(n) + "/" + key)
}

// flexibleDayPeriod is the B letter: "in the morning", from CLDR's day
// period rules, falling back to am and pm.
func (f *dateFormatter) flexibleDayPeriod(n int, d *dateFields) string {
	rules := f.dayPeriods
	if rules == nil {
		return f.amPm(n, d)
	}

	minute, second := 0, 0
	if f.hasMinute {
		minute = d.minute
	}
	if f.hasSecond {
		second = d.second
	}
	period := rules.hours[d.hour]
	switch {
	case d.hour == 0 && minute == 0 && second == 0 && rules.midnight:
		period = "midnight"
	case d.hour == 12 && minute == 0 && second == 0 && rules.noon:
		period = "noon"
	}
	width := periodWidth(n)
	var s string
	ok := false
	if period != "am" && period != "pm" && period != "midnight" {
		s, ok = f.cal.get("dayPeriods/format/" + width + "/" + period)
	}
	if !ok && (period == "midnight" || period == "noon") {
		period = rules.hours[d.hour]
		s, ok = f.cal.get("dayPeriods/format/" + width + "/" + period)
	}
	if period == "am" || period == "pm" || !ok {
		return f.amPm(n, d)
	}

	return s
}

// dayPeriodRules are CLDR's day periods of a language: which hours are
// "morning1", "afternoon1" and so on.
type dayPeriodRules struct {
	hours          [24]string
	midnight, noon bool
}

var dayPeriodCache sync.Map

// dayPeriodsFor returns the rules ICU uses for locale, looking it up and then
// its truncations, or nil.
func dayPeriodsFor(locale string) *dayPeriodRules {
	if r, ok := dayPeriodCache.Load(locale); ok {
		return r.(*dayPeriodRules)
	}

	var rules *dayPeriodRules
	s := cldr().supplemental()
	for l := locale; l != ""; l = truncate(l) {
		v, ok := s.get("dp/" + l)
		if !ok {
			continue
		}

		rules = &dayPeriodRules{}
		for _, r := range strings.Fields(v) {
			name, span, _ := strings.Cut(r, "=")
			from, before, isSpan := strings.Cut(span, "-")
			if !isSpan {
				switch name {
				case "midnight":
					rules.midnight = true
				case "noon":
					rules.noon = true
				}
				continue
			}

			h0, h1 := hourOf(from), hourOf(before)
			for h := h0; h != h1; h = (h + 1) % 24 {
				rules.hours[h] = name
			}
		}
		break
	}
	if rules != nil && rules.hours[0] == "" {
		rules = nil
	}
	v, _ := dayPeriodCache.LoadOrStore(locale, rules)
	return v.(*dayPeriodRules)
}

func hourOf(s string) int {
	h, _, _ := strings.Cut(s, ":")
	n, _ := strconv.Atoi(h)
	return n % 24
}
