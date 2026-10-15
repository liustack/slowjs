// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package intl // import "github.com/liustack/quickjs/intl"

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

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

// currencySymbols maps a currency code to its en-US symbol and narrow symbol,
// as extracted from ICU. A code that is not here formats as the code itself,
// which is what CLDR does for every currency it has no symbol for - as does the
// symbol column of half of these, where only the narrow form is a symbol.
var currencySymbols = map[string][2]string{
	"ARS": {"ARS", "$"},
	"AUD": {"A$", "$"},
	"BDT": {"BDT", "৳"},
	"BND": {"BND", "$"},
	"BRL": {"R$", "R$"},
	"CAD": {"CA$", "$"},
	"CLP": {"CLP", "$"},
	"CNY": {"CN¥", "¥"},
	"COP": {"COP", "$"},
	"CZK": {"CZK", "Kč"},
	"DKK": {"DKK", "kr"},
	"EGP": {"EGP", "E£"},
	"EUR": {"€", "€"},
	"FJD": {"FJD", "$"},
	"GBP": {"£", "£"},
	"HKD": {"HK$", "$"},
	"HRK": {"HRK", "kn"},
	"HUF": {"HUF", "Ft"},
	"IDR": {"IDR", "Rp"},
	"ILS": {"₪", "₪"},
	"INR": {"₹", "₹"},
	"ISK": {"ISK", "kr"},
	"JPY": {"¥", "¥"},
	"KHR": {"KHR", "៛"},
	"KRW": {"₩", "₩"},
	"LAK": {"LAK", "₭"},
	"LKR": {"LKR", "Rs"},
	"MMK": {"MMK", "K"},
	"MXN": {"MX$", "$"},
	"MYR": {"MYR", "RM"},
	"NGN": {"NGN", "₦"},
	"NOK": {"NOK", "kr"},
	"NPR": {"NPR", "Rs"},
	"NZD": {"NZ$", "$"},
	"PHP": {"₱", "₱"},
	"PKR": {"PKR", "Rs"},
	"PLN": {"PLN", "zł"},
	"RON": {"RON", "lei"},
	"RUB": {"RUB", "₽"},
	"SEK": {"SEK", "kr"},
	"SGD": {"SGD", "$"},
	"THB": {"THB", "฿"},
	"TRY": {"TRY", "₺"},
	"TWD": {"NT$", "$"},
	"UAH": {"UAH", "₴"},
	"USD": {"$", "$"},
	"UYU": {"UYU", "$"},
	"VND": {"₫", "₫"},
	"XAF": {"FCFA", "FCFA"},
	"XCD": {"EC$", "$"},
	"XOF": {"F CFA", "F CFA"},
	"XPF": {"CFPF", "CFPF"},
	"ZAR": {"ZAR", "R"},
}

// currencyMinorUnits returns the number of fraction digits a currency is
// written with. The two lists are every code CLDR does not write with two,
// which is not the same set ISO 4217 names: CLDR writes IQD with none where
// ISO gives it three, and a browser follows CLDR.
var currencyMinorUnits = func() func(string) int {
	m := map[string]int{}
	for c := range strings.FieldsSeq(
		"ADP AFN ALL BIF BYR CLP DJF ESP GNF IQD IRR ISK ITL JPY KMF KPW KRW LAK LBP LUF MGA MGF " +
			"MMK MRO PYG RSD RWF SLL SOS STD SYP TMM TRL UGX UYI VND VUV XAF XOF XPF YER ZMK ZWD") {
		m[c] = 0
	}
	for c := range strings.FieldsSeq("BHD JOD KWD LYD OMR TND") {
		m[c] = 3
	}
	return func(code string) int {
		if n, ok := m[code]; ok {
			return n
		}

		return 2
	}
}()

// currencyLabel returns the text standing in for the currency and whether it
// follows the amount rather than preceding it.
func currencyLabel(code, display string) (string, bool) {
	s, ok := currencySymbols[code]
	switch {
	case display == "name":
		// CLDR spells this one out and puts it after the amount, as in
		// "1.00 US dollars". Without the names the code stands in, but it keeps
		// the place the caller asked for.
		return code, true
	case !ok, display == "code":
		return code, false
	case display == "narrowSymbol":
		return s[1], false
	}
	return s[0], false
}

// currencySpace reports whether CLDR's currency spacing rule puts a no-break
// space between the currency label and the amount: it does when the label ends
// with a letter and the amount starts with a digit, so "USD 1.00" but "$1.00"
// and "USD" straight against a NaN.
func currencySpace(label, amount string) bool {
	if label == "" || amount == "" {
		return false
	}

	last, _ := utf8.DecodeLastRuneInString(label)
	first, _ := utf8.DecodeRuneInString(amount)
	return unicode.IsLetter(last) && unicode.IsDigit(first)
}

// unitForms holds the en names of a unit: the short singular and plural, then
// whether the short form is separated from the amount by a space, then the
// narrow form and the long singular and plural. A narrow unit is never
// separated and a long one always is.
type unitForms struct {
	shortOne, shortOther string
	shortSpace           bool
	narrow               string
	longOne, longOther   string
}

// unitNames is CLDR's en unit data, extracted from ICU rather than written by
// hand: the spellings are irregular enough ("mth"/"mths" but "hr"/"hr", a
// narrow celsius that keeps its C while a narrow fahrenheit drops its F) that
// nothing here is guessable.
var unitNames = map[string]unitForms{
	"acre":              {"ac", "ac", true, "ac", "acre", "acres"},
	"bit":               {"bit", "bit", true, "bit", "bit", "bits"},
	"byte":              {"byte", "byte", true, "B", "byte", "bytes"},
	"celsius":           {"°C", "°C", false, "°C", "degree Celsius", "degrees Celsius"},
	"centimeter":        {"cm", "cm", true, "cm", "centimeter", "centimeters"},
	"day":               {"day", "days", true, "d", "day", "days"},
	"degree":            {"deg", "deg", true, "°", "degree", "degrees"},
	"fahrenheit":        {"°F", "°F", false, "°", "degree Fahrenheit", "degrees Fahrenheit"},
	"fluid-ounce":       {"fl oz", "fl oz", true, "fl oz", "fluid ounce", "fluid ounces"},
	"foot":              {"ft", "ft", true, "′", "foot", "feet"},
	"gallon":            {"gal", "gal", true, "gal", "gallon", "gallons"},
	"gigabit":           {"Gb", "Gb", true, "Gb", "gigabit", "gigabits"},
	"gigabyte":          {"GB", "GB", true, "GB", "gigabyte", "gigabytes"},
	"gram":              {"g", "g", true, "g", "gram", "grams"},
	"hectare":           {"ha", "ha", true, "ha", "hectare", "hectares"},
	"hour":              {"hr", "hr", true, "h", "hour", "hours"},
	"inch":              {"in", "in", true, "″", "inch", "inches"},
	"kilobit":           {"kb", "kb", true, "kb", "kilobit", "kilobits"},
	"kilobyte":          {"kB", "kB", true, "kB", "kilobyte", "kilobytes"},
	"kilogram":          {"kg", "kg", true, "kg", "kilogram", "kilograms"},
	"kilometer":         {"km", "km", true, "km", "kilometer", "kilometers"},
	"liter":             {"L", "L", true, "L", "liter", "liters"},
	"megabit":           {"Mb", "Mb", true, "Mb", "megabit", "megabits"},
	"megabyte":          {"MB", "MB", true, "MB", "megabyte", "megabytes"},
	"meter":             {"m", "m", true, "m", "meter", "meters"},
	"microsecond":       {"μs", "μs", true, "μs", "microsecond", "microseconds"},
	"mile":              {"mi", "mi", true, "mi", "mile", "miles"},
	"mile-scandinavian": {"smi", "smi", true, "smi", "mile-scandinavian", "miles-scandinavian"},
	"milliliter":        {"mL", "mL", true, "mL", "milliliter", "milliliters"},
	"millimeter":        {"mm", "mm", true, "mm", "millimeter", "millimeters"},
	"millisecond":       {"ms", "ms", true, "ms", "millisecond", "milliseconds"},
	"minute":            {"min", "min", true, "m", "minute", "minutes"},
	"month":             {"mth", "mths", true, "m", "month", "months"},
	"nanosecond":        {"ns", "ns", true, "ns", "nanosecond", "nanoseconds"},
	"ounce":             {"oz", "oz", true, "oz", "ounce", "ounces"},
	"percent":           {"%", "%", false, "%", "percent", "percent"},
	"petabyte":          {"PB", "PB", true, "PB", "petabyte", "petabytes"},
	"pound":             {"lb", "lb", true, "#", "pound", "pounds"},
	"second":            {"sec", "sec", true, "s", "second", "seconds"},
	"stone":             {"st", "st", true, "st", "stone", "stones"},
	"terabit":           {"Tb", "Tb", true, "Tb", "terabit", "terabits"},
	"terabyte":          {"TB", "TB", true, "TB", "terabyte", "terabytes"},
	"week":              {"wk", "wks", true, "w", "week", "weeks"},
	"yard":              {"yd", "yd", true, "yd", "yard", "yards"},
	"year":              {"yr", "yrs", true, "y", "year", "years"},
}

// unitLabel returns the text following the amount for the unit option and
// whether a space separates them. A unit this package has no name for stands in
// for itself, so an exotic identifier formats oddly rather than throwing.
func unitLabel(unit, display string, one bool) (string, bool) {
	if num, den, ok := strings.Cut(unit, "-per-"); ok {
		// The denominator of a compound unit takes its narrowest form, which is
		// what makes kilometer-per-hour "km/h" rather than "km/hr".
		d, space := unitLabel(num, display, one)
		if display == "long" {
			n, _ := unitLabel(den, display, true)
			return d + " per " + n, space
		}

		n, _ := unitLabel(den, "narrow", true)
		return d + "/" + n, space
	}

	f, ok := unitNames[unit]
	switch {
	case !ok:
		return unit, true
	case display == "narrow":
		return f.narrow, false
	case display == "long" && one:
		return f.longOne, true
	case display == "long":
		return f.longOther, true
	case one:
		return f.shortOne, f.shortSpace
	}
	return f.shortOther, f.shortSpace
}
