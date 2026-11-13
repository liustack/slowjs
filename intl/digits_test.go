// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package intl // import "github.com/liustack/quickjs/intl"

import (
	"errors"
	"runtime"
	"testing"
	"unicode/utf16"

	"github.com/liustack/quickjs"
	"github.com/liustack/quickjs/internal/intlhook"
	"github.com/liustack/quickjs/internal/vmhook"
)

// A numeric string of a few characters can ask for a written form of a
// thousand million digits, which Intl.NumberFormat refuses with a
// RangeError before it writes any, when it would have more than
// intlhook.MaxDigits digits.

// TestWrittenDigitsRefused gives short strings whose written form is too
// long: the call throws a RangeError, and Go allocates no more than a few
// KiB for it.
func TestWrittenDigitsRefused(t *testing.T) {
	for _, c := range []struct{ opts, call string }{
		{`{maximumSignificantDigits: 3}`, `format('1e-1073741000')`},
		{`{notation: 'compact'}`, `format('1e-1073741000')`},
		{`{minimumSignificantDigits: 21}`, `formatToParts('-1e-600000')`},
		{`{maximumFractionDigits: 100, roundingPriority: 'morePrecision'}`, `format('1e-600000')`},
		{`{style: 'percent', maximumSignificantDigits: 1}`, `format('7e-600000')`},
	} {
		t.Run(c.opts+"."+c.call, func(t *testing.T) {
			vm := newVM(t)
			eval(t, vm, `var f = new Intl.NumberFormat('en', `+c.opts+`); f.format(1); f.formatToParts(1); 0`)
			runtime.GC()
			var before, after runtime.MemStats
			runtime.ReadMemStats(&before)
			_, err := vm.Eval(`f.`+c.call, quickjs.EvalGlobal)
			runtime.ReadMemStats(&after)
			var e *quickjs.Error
			if !errors.As(err, &e) || e.Uncatchable || e.Name != "RangeError" || e.Message != intlhook.KindDigits.Message() {
				t.Fatalf("got %v, want the RangeError %q", err, intlhook.KindDigits.Message())
			}
			if n := after.TotalAlloc - before.TotalAlloc; n > 64<<10 {
				t.Errorf("Go allocated %d bytes for the call", n)
			}
		})
	}
}

// Under the cap a short string is written in full, and so is the longest
// BigInt, of 1 Mi bits less its sign: its 315,653 digits are fewer than a
// numeric string may have, and no BigInt is refused.
func TestWrittenDigitsUnderCap(t *testing.T) {
	vm := newVM(t)
	for _, c := range []struct {
		js   string
		want int
	}{
		// "0." and 499,999 zeros and then the three digits kept.
		{`new Intl.NumberFormat('en', {maximumSignificantDigits: 3}).format('1.23456e-500000').length`, 2 + 499_999 + 3},
		// lessPrecision takes the decimals' result, zero, without the
		// significant digits', which would write every zero.
		{`new Intl.NumberFormat('en', {maximumFractionDigits: 100, roundingPriority: 'lessPrecision'}).format('1e-1073741000').length`, 1},
		// A digit and a separator after each three.
		{`(2n ** 1048574n).toLocaleString('en').length`, 315_653 + 315_652/3},
	} {
		if got := eval(t, vm, c.js); got != c.want {
			t.Errorf("%s = %v, want %d", c.js, got, c.want)
		}
	}
	if _, err := vm.Eval(`2n ** 1048575n`, quickjs.EvalGlobal); err == nil {
		t.Errorf("2n ** 1048575n is a BigInt: a longer one may be refused")
	}
}

// TestDigitsRoomCoversGo checks that what the door counts for a call that
// writes many digits covers what Go takes for it: with the room one byte
// short of what Go took, the door refuses the call. The calls are those
// that make Go take the most for each digit: the parts of formatToParts,
// a separator after every two digits as Hindi groups them, digits of four
// bytes in UTF-8, and the name of a unit or a currency, which copies the
// parts once more.
func TestDigitsRoomCoversGo(t *testing.T) {
	for _, c := range []struct{ locale, opts, call string }{
		{`en`, `{}`, `formatToParts(10n ** 60000n)`},
		{`hi`, `{}`, `formatToParts(10n ** 60000n)`},
		{`hi`, `{numberingSystem: 'adlm'}`, `formatToParts(10n ** 60000n)`},
		{`hi`, `{numberingSystem: 'adlm'}`, `format(10n ** 60000n)`},
		{`hi`, `{numberingSystem: 'adlm', style: 'unit', unit: 'meter', unitDisplay: 'long'}`, `format(10n ** 60000n)`},
		{`hi`, `{numberingSystem: 'adlm', style: 'unit', unit: 'meter', unitDisplay: 'long'}`, `formatToParts(10n ** 60000n)`},
		{`hi`, `{numberingSystem: 'adlm', style: 'currency', currency: 'EUR', currencyDisplay: 'name'}`, `format(10n ** 60000n)`},
		{`hi`, `{numberingSystem: 'adlm', notation: 'compact', compactDisplay: 'long'}`, `format(10n ** 60000n)`},
		{`en-IN`, `{style: 'percent', minimumFractionDigits: 20}`, `formatToParts(10n ** 60000n)`},
		{`en`, `{numberingSystem: 'adlm', minimumSignificantDigits: 21}`, `formatToParts('1e-523000')`},
		{`en`, `{notation: 'compact'}`, `format('1e-523000')`},
		{`en`, `{minimumFractionDigits: 100}`, `formatToParts('0.' + '1'.repeat(500000))`},
	} {
		t.Run(c.locale+c.opts+"."+c.call, func(t *testing.T) {
			vm := newVM(t)
			eval(t, vm, `var f = new Intl.NumberFormat('`+c.locale+`', `+c.opts+`); f.format(1); f.formatToParts(1); 0`)
			src := `(function () { f.` + c.call + `; return 0; })()`
			runtime.GC()
			var before, after runtime.MemStats
			runtime.ReadMemStats(&before)
			eval(t, vm, src)
			runtime.ReadMemStats(&after)
			took := int64(after.TotalAlloc - before.TotalAlloc)

			room := vmhook.Room
			vmhook.Room = func(any) int64 { return took - 1 }
			defer func() { vmhook.Room = room }()
			_, err := vm.Eval(src, quickjs.EvalGlobal)
			var e *quickjs.Error
			if !errors.As(err, &e) || !e.OutOfMemory {
				t.Errorf("Go took %d bytes, and with one less of room the call gave %v, want the out of memory error", took, err)
			}
		})
	}
}

// Every other output of Intl whose length the value sets, and not the
// length of a string a script gave, is bounded whatever the value, so
// that the door needs no count for it. The bounds:
//
//   - a Number, formatted: at most 309 integer digits and 345 decimals,
//     those of 5e-324 to 21 significant digits, two more for a percent,
//     each of two code units in a numbering system past the BMP, with a
//     separator after every two and the affixes: numberLength.
//   - Intl.RelativeTimeFormat: a Number, formatted, in a pattern.
//   - Intl.PluralRules: a category, "other" the longest.
//   - Intl.DurationFormat: ten fields of at most 2**53 each, or the
//     duration is not valid, with at most nine decimals of a second: 16
//     digits a field, the last of 20 as NodeICU writes it past 2**63
//     nanoseconds, and its unit's name.
//   - Intl.DateTimeFormat: a year of at most six digits, in a pattern of
//     CLDR, and a time zone of the table or an offset.
const (
	numberLength   = 2*(311+345) + (311+345)/2 + 256
	durationLength = 10 * (20 + 64)
	dateLength     = 512
)

func TestValueDrivenOutputBounded(t *testing.T) {
	vm := newVM(t)
	const max, tiny = `1.7976931348623157e308`, `5e-324`
	for _, c := range []struct {
		js    string
		bound int
	}{
		{`new Intl.NumberFormat('hi', {numberingSystem: 'adlm', minimumFractionDigits: 100, minimumIntegerDigits: 21, style: 'percent'}).format(-` + max + `)`, numberLength},
		{`new Intl.NumberFormat('hi', {numberingSystem: 'adlm', maximumSignificantDigits: 21, minimumSignificantDigits: 21, style: 'percent'}).format(-` + tiny + `)`, numberLength},
		{`new Intl.NumberFormat('en', {style: 'unit', unit: 'kilometer-per-hour', unitDisplay: 'long', minimumFractionDigits: 100}).format(-` + max + `)`, numberLength},
		{`new Intl.NumberFormat('en', {style: 'currency', currency: 'XDR', currencyDisplay: 'name', maximumSignificantDigits: 21, minimumSignificantDigits: 21}).format(-` + tiny + `)`, numberLength},
		{`new Intl.NumberFormat('en', {notation: 'compact', compactDisplay: 'long', minimumFractionDigits: 100}).format(` + max + `)`, numberLength},
		{`new Intl.NumberFormat('en', {notation: 'engineering', minimumFractionDigits: 100, minimumIntegerDigits: 21}).format(-` + tiny + `)`, numberLength},
		{`new Intl.NumberFormat('en', {maximumFractionDigits: 0, roundingIncrement: 5000, roundingMode: 'ceil'}).format(` + max + `)`, numberLength},
		{`(-` + max + `).toLocaleString('hi-u-nu-adlm', {minimumFractionDigits: 100})`, numberLength},
		{`new Intl.RelativeTimeFormat('hi-u-nu-adlm', {style: 'long'}).format(-` + max + `, 'quarters')`, numberLength + 256},
		{`new Intl.RelativeTimeFormat('en', {numeric: 'auto'}).format(` + tiny + `, 'day')`, numberLength + 256},
		{`new Intl.PluralRules('en', {minimumFractionDigits: 100, type: 'ordinal'}).select(` + max + `)`, 5},
		{`new Intl.PluralRules('en').selectRange(-` + max + `, ` + max + `)`, 5},
		{`new Intl.DurationFormat('hi-u-nu-adlm', {style: 'long'}).format({years: 4294967295, months: 4294967295, weeks: 4294967295, days: 104249991, hours: 1, minutes: 1, seconds: 1, milliseconds: 999, microseconds: 999, nanoseconds: 999})`, durationLength},
		{`new Intl.DurationFormat('en', {style: 'digital', fractionalDigits: 9}).format({hours: 1, milliseconds: 9007199254740991, nanoseconds: 9007199254740991})`, durationLength},
		{`new Intl.DurationFormat('en', {style: 'long'}).format({seconds: -9007199254740991, nanoseconds: -999999999})`, durationLength},
		{`new Date(-8.64e15).toLocaleString('en', {fractionalSecondDigits: 3, era: 'long', timeZone: 'America/Argentina/ComodRivadavia', timeZoneName: 'longGeneric', weekday: 'long', month: 'long', year: 'numeric', day: 'numeric', hour: 'numeric', minute: 'numeric', second: 'numeric', dayPeriod: 'long'})`, dateLength},
		{`new Intl.DateTimeFormat('hi-u-nu-adlm-ca-ethiopic', {dateStyle: 'full', timeStyle: 'full', timeZone: '-23:59'}).format(8.64e15)`, dateLength},
	} {
		got, err := vm.Eval(c.js, quickjs.EvalGlobal)
		if err != nil {
			t.Errorf("%s: %v", c.js, err)
			continue
		}
		t.Logf("%d of %d: %s", len(utf16.Encode([]rune(got.(string)))), c.bound, c.js)
		if s, _ := got.(string); s == "" || len(utf16.Encode([]rune(s))) > c.bound {
			t.Errorf("%s = %q, longer than %d", c.js, got, c.bound)
		}
	}
}
