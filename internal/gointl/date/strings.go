package date

import (
	"math"
	"strconv"
)

// The strings Date writes, as V8's ToDateString writes them.
//
// Each is written into one buffer on the stack and made a string once: a
// program that formats dates in a loop formats a great many, and printf's
// way made a string for each piece and parsed its format each time.

var (
	shortWeekdays = [7]string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"}
	shortMonths   = [12]string{"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"}
)

// appendPadded appends a non-negative n in at least width digits, with
// leading zeros: printf's "%0*d".
func appendPadded(b []byte, n int64, width int) []byte {
	var digits [20]byte
	d := strconv.AppendInt(digits[:0], n, 10)
	for i := len(d); i < width; i++ {
		b = append(b, '0')
	}
	return append(b, d...)
}

// append2 appends n, from 0 to 99, as two digits.
func append2(b []byte, n int) []byte {
	return append(b, byte('0'+n/10), byte('0'+n%10))
}

// appendYear is V8's "%04d", or "%05d" for a year before 1: a sign and at
// least four digits.
func appendYear(b []byte, year int64) []byte {
	if year < 0 {
		return appendPadded(append(b, '-'), -year, 4)
	}
	return appendPadded(b, year, 4)
}

// appendClock appends hh:mm:ss.
func appendClock(b []byte, f Fields) []byte {
	b = append2(b, f.Hour)
	b = append2(append(b, ':'), f.Minute)
	return append2(append(b, ':'), f.Second)
}

// appendOffset appends a distance from UTC in minutes as GMT+hhmm.
func appendOffset(b []byte, minutes int64) []byte {
	sign := byte('+')
	if minutes < 0 {
		sign = '-'
		minutes = -minutes
	}
	b = append(b, 'G', 'M', 'T', sign)
	b = appendPadded(b, minutes/60, 2)
	return appendPadded(b, minutes%60, 2)
}

// appendDate appends "Fri Sep 25 2026".
func appendDate(b []byte, f Fields) []byte {
	b = append(b, shortWeekdays[f.Weekday]...)
	b = append(append(b, ' '), shortMonths[f.Month]...)
	b = append2(append(b, ' '), f.Day)
	return appendYear(append(b, ' '), f.Year)
}

// appendTime appends "08:00:00 GMT-0400 (Eastern Daylight Time)".
func appendTime(b []byte, f Fields, offset int64, name string) []byte {
	b = appendOffset(append(appendClock(b, f), ' '), offset)
	b = append(append(b, " ("...), name...)
	return append(b, ')')
}

// local is a time value's local fields, its offset from UTC in minutes,
// and the zone's name at it.
func (e *Environment) local(t float64) (Fields, int64, string) {
	ms := int64(t)
	local := e.toLocal(ms)
	offset := -e.timezoneOffset(ms)
	return BreakDown(float64(local)), offset, e.zoneName(ms)
}

// String is Date.prototype.toString: "Fri Sep 25 2026 08:00:00 GMT-0400
// (Eastern Daylight Time)", or "Invalid Date".
func (e *Environment) String(t float64) string {
	if math.IsNaN(t) {
		return "Invalid Date"
	}
	f, offset, name := e.local(t)
	var buf [96]byte
	b := appendDate(buf[:0], f)
	return string(appendTime(append(b, ' '), f, offset, name))
}

// DateString is Date.prototype.toDateString: "Fri Sep 25 2026".
func (e *Environment) DateString(t float64) string {
	if math.IsNaN(t) {
		return "Invalid Date"
	}
	f, _, _ := e.local(t)
	var buf [32]byte
	return string(appendDate(buf[:0], f))
}

// TimeString is Date.prototype.toTimeString: "08:00:00 GMT-0400 (Eastern
// Daylight Time)".
func (e *Environment) TimeString(t float64) string {
	if math.IsNaN(t) {
		return "Invalid Date"
	}
	f, offset, name := e.local(t)
	var buf [80]byte
	return string(appendTime(buf[:0], f, offset, name))
}

// UTCString is Date.prototype.toUTCString: "Fri, 25 Sep 2026 12:00:00 GMT".
func UTCString(t float64) string {
	if math.IsNaN(t) {
		return "Invalid Date"
	}
	f := BreakDown(t)
	var buf [40]byte
	b := append(buf[:0], shortWeekdays[f.Weekday]...)
	b = append2(append(b, ", "...), f.Day)
	b = append(append(b, ' '), shortMonths[f.Month]...)
	b = appendYear(append(b, ' '), f.Year)
	b = appendClock(append(b, ' '), f)
	return string(append(b, " GMT"...))
}

// ISOString is Date.prototype.toISOString: "2026-09-25T12:00:00.000Z",
// with six digits and a sign for a year outside 0 to 9999. It is false for
// NaN, where JavaScript throws a RangeError.
func ISOString(t float64) (string, bool) {
	if math.IsNaN(t) {
		return "", false
	}
	f := BreakDown(t)
	var buf [32]byte
	b := buf[:0]
	switch {
	case f.Year >= 0 && f.Year <= 9999:
		b = appendPadded(b, f.Year, 4)
	case f.Year < 0:
		b = appendPadded(append(b, '-'), -f.Year, 6)
	default:
		b = appendPadded(append(b, '+'), f.Year, 6)
	}
	b = append2(append(b, '-'), f.Month+1)
	b = append2(append(b, '-'), f.Day)
	b = appendClock(append(b, 'T'), f)
	b = appendPadded(append(b, '.'), int64(f.Millisecond), 3)
	return string(append(b, 'Z')), true
}
