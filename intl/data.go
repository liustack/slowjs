// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package intl // import "github.com/liustack/quickjs/intl"

// The en-US names ECMA-402 formatters need. They are the whole of this
// package's locale data: Go's standard library ships the zoneinfo database but
// no CLDR, so anything not spelled out here - month names in another language,
// a currency's spelled out name, a time zone's display name - has to fall back
// to something the code can derive, never to a guess.

var (
	monthNames = map[string][12]string{
		"long":   {"January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"},
		"short":  {"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"},
		"narrow": {"J", "F", "M", "A", "M", "J", "J", "A", "S", "O", "N", "D"},
	}

	weekdayNames = map[string][7]string{
		"long":   {"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"},
		"short":  {"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"},
		"narrow": {"S", "M", "T", "W", "T", "F", "S"},
	}

	eraNames = map[string][2]string{ // BC, AD
		"long":   {"Before Christ", "Anno Domini"},
		"short":  {"BC", "AD"},
		"narrow": {"B", "A"},
	}
)

// dayPeriodName returns the en day period CLDR's rules put a time in, the strings
// the dayPeriod option writes. English has no distinct narrow or short forms
// worth a table, so the width is ignored. The rules are CLDR's: the morning
// runs from midnight to noon. Midnight has a name in the data, but ICU does
// not use it, and noon is named only when the minute and second, if they are
// shown, are zero.
func dayPeriodName(hour, minute, second int, hasMinute, hasSecond bool) string {
	switch {
	case hour == 12 && (!hasMinute || minute == 0) && (!hasSecond || second == 0):
		return "noon"
	case hour < 12:
		return "in the morning"
	case hour < 18:
		return "in the afternoon"
	case hour < 21:
		return "in the evening"
	}
	return "at night"
}
