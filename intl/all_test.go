// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package intl // import "github.com/liustack/quickjs/intl"

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/liustack/quickjs"

	_ "time/tzdata" // The test expectations name IANA zones.
)

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

// newVM returns a VM with Intl registered. The default time zone is pinned so
// that the expected values do not depend on the host's.
func newVM(tb testing.TB, opts ...Option) *quickjs.VM {
	tb.Helper()
	vm, err := quickjs.NewVM()
	if err != nil {
		tb.Fatal(err)
	}

	tb.Cleanup(func() { vm.Close() })
	if len(opts) == 0 {
		opts = []Option{TimeZone("UTC")}
	}
	if err := Register(vm, opts...); err != nil {
		tb.Fatal(err)
	}

	return vm
}

func eval(tb testing.TB, vm *quickjs.VM, src string) any {
	tb.Helper()
	v, err := vm.Eval(src, quickjs.EvalGlobal)
	if err != nil {
		tb.Fatalf("%s: %v", src, err)
	}

	return v
}

// evalTests runs src/want pairs, each in the same VM.
func evalTests(t *testing.T, vm *quickjs.VM, tests [][2]string) {
	t.Helper()
	for _, test := range tests {
		src, want := test[0], test[1]
		got, err := vm.Eval(src, quickjs.EvalGlobal)
		switch {
		case err != nil:
			t.Errorf("%s\n\tgot error %v\n\twant %q", src, err, want)
		case fmt.Sprint(got) != want:
			t.Errorf("%s\n\tgot  %q\n\twant %q", src, fmt.Sprint(got), want)
		}
	}
}

func TestIntlExists(t *testing.T) {
	vm := newVM(t)
	evalTests(t, vm, [][2]string{
		{`typeof Intl`, "object"},
		{`typeof Intl.DateTimeFormat`, "function"},
		{`typeof Intl.NumberFormat`, "function"},
		{`typeof Intl.getCanonicalLocales`, "function"},
		{`Object.prototype.toString.call(Intl)`, "[object Intl]"},
		{`typeof new Intl.DateTimeFormat().format`, "function"},
		// Callable without new, as ECMA-402 requires for these two.
		{`typeof Intl.DateTimeFormat()`, "object"},
		{`typeof Intl.NumberFormat()`, "object"},
	})
}

// TestSvelteKitStart is the constructor from the client start of a SvelteKit
// app, the shape that made this package necessary: on a VM without Intl it
// throws a ReferenceError, and the framework's fallback for a failed start is
// a hard navigation to the same URL, so the throw repeats forever.
//
// It is not by itself an acceptance test for any page. The one this came from
// needed a second, unrelated fix on the embedder's side before it settled, and
// either blocker alone was enough to keep it looping.
func TestSvelteKitStart(t *testing.T) {
	vm := newVM(t)
	evalTests(t, vm, [][2]string{
		{
			"new Intl.DateTimeFormat(`en-US`, {hourCycle:`h23`, timeZone:`America/New_York`, " +
				"year:`numeric`, month:`2-digit`, day:`2-digit`, hour:`2-digit`, minute:`2-digit`, second:`2-digit`})" +
				".format(new Date(0))",
			"12/31/1969, 19:00:00",
		},
	})
}

func TestDateTimeFormat(t *testing.T) {
	vm := newVM(t)
	evalTests(t, vm, [][2]string{
		{`new Intl.DateTimeFormat('en-US').format(new Date(0))`, "1/1/1970"},
		{`new Intl.DateTimeFormat('en-US', {timeZone: 'America/New_York'}).format(new Date(0))`, "12/31/1969"},
		{`new Intl.DateTimeFormat('en-US', {dateStyle: 'full'}).format(new Date(0))`, "Thursday, January 1, 1970"},
		{`new Intl.DateTimeFormat('en-US', {dateStyle: 'long'}).format(new Date(0))`, "January 1, 1970"},
		{`new Intl.DateTimeFormat('en-US', {dateStyle: 'medium'}).format(new Date(0))`, "Jan 1, 1970"},
		{`new Intl.DateTimeFormat('en-US', {dateStyle: 'short'}).format(new Date(0))`, "1/1/70"},
		{`new Intl.DateTimeFormat('en-US', {timeStyle: 'short'}).format(new Date(0))`, "12:00 AM"},
		{`new Intl.DateTimeFormat('en-US', {timeStyle: 'medium'}).format(new Date(0))`, "12:00:00 AM"},
		{`new Intl.DateTimeFormat('en-US', {timeStyle: 'long'}).format(new Date(0))`, "12:00:00 AM UTC"},
		{`new Intl.DateTimeFormat('en-US', {timeStyle: 'full'}).format(new Date(0))`, "12:00:00 AM Coordinated Universal Time"},
		{`new Intl.DateTimeFormat('en-US', {dateStyle: 'full', timeStyle: 'short'}).format(new Date(0))`, "Thursday, January 1, 1970 at 12:00 AM"},
		{`new Intl.DateTimeFormat('en-US', {dateStyle: 'medium', timeStyle: 'short'}).format(new Date(0))`, "Jan 1, 1970, 12:00 AM"},
		{`new Intl.DateTimeFormat('en-US', {weekday: 'short', month: 'numeric', day: 'numeric'}).format(new Date(0))`, "Thu, 1/1"},
		{`new Intl.DateTimeFormat('en-US', {month: 'long', day: 'numeric', year: 'numeric'}).format(new Date(0))`, "January 1, 1970"},
		{`new Intl.DateTimeFormat('en-US', {month: 'long'}).format(new Date(0))`, "January"},
		{`new Intl.DateTimeFormat('en-US', {month: 'narrow'}).format(new Date(0))`, "J"},
		{`new Intl.DateTimeFormat('en-US', {weekday: 'long'}).format(new Date(0))`, "Thursday"},
		{`new Intl.DateTimeFormat('en-US', {year: 'numeric', month: 'numeric'}).format(new Date(0))`, "1/1970"},
		{`new Intl.DateTimeFormat('en-US', {year: 'numeric', era: 'short'}).format(new Date(0))`, "1970 AD"},
		{`new Intl.DateTimeFormat('en-US', {hour: 'numeric'}).format(new Date(0))`, "12 AM"},
		{`new Intl.DateTimeFormat('en-US', {hour: 'numeric', minute: 'numeric'}).format(new Date(0))`, "12:00 AM"},
		{`new Intl.DateTimeFormat('en-US', {hour: 'numeric', minute: 'numeric', hour12: false}).format(new Date(0))`, "00:00"},
		{`new Intl.DateTimeFormat('en-US', {hour: '2-digit', minute: '2-digit', hourCycle: 'h23'}).format(new Date(0))`, "00:00"},
		{`new Intl.DateTimeFormat('en-US', {hour: 'numeric', hourCycle: 'h24'}).format(new Date(0))`, "24"},
		{`new Intl.DateTimeFormat('en-US', {timeZone: 'UTC', timeZoneName: 'short'}).format(new Date(0))`, "1/1/1970, UTC"},
		{`new Intl.DateTimeFormat('en-US', {timeZone: 'America/New_York', timeZoneName: 'short'}).format(new Date(0))`, "12/31/1969, EST"},
		{`new Intl.DateTimeFormat('en-US', {timeZone: 'America/New_York', timeZoneName: 'shortOffset'}).format(new Date(0))`, "12/31/1969, GMT-5"},
		{`new Intl.DateTimeFormat('en-US', {timeZone: 'Asia/Kolkata', timeZoneName: 'longOffset'}).format(new Date(0))`, "1/1/1970, GMT+05:30"},
		{`new Intl.DateTimeFormat('en-US', {timeZone: '+05:30'}).format(new Date(0))`, "1/1/1970"},
		{`new Intl.DateTimeFormat('en-US', {hour: 'numeric', minute: 'numeric', second: 'numeric', fractionalSecondDigits: 3}).format(new Date(1500))`, "12:00:01.500 AM"},
		// en has no midnight: 00:00 falls in the night, and only noon is named.
		{`new Intl.DateTimeFormat('en-US', {dayPeriod: 'short'}).format(new Date(0))`, "at night"},
		{`new Intl.DateTimeFormat('en-US', {dayPeriod: 'short'}).format(new Date(9 * 3600 * 1000))`, "in the morning"},
		// A date before AD 1 names its era even though none was asked for.
		{`new Intl.DateTimeFormat('en-US', {timeZone: 'UTC'}).format(new Date(Date.UTC(-1, 0, 1)))`, "1/1/2 BC"},
		// The bound format function survives being passed around.
		{`[0, 86400000].map(new Intl.DateTimeFormat('en-US').format).join(' ')`, "1/1/1970 1/2/1970"},
	})
}

func TestDateTimeFormatToParts(t *testing.T) {
	vm := newVM(t)
	evalTests(t, vm, [][2]string{
		{
			`JSON.stringify(new Intl.DateTimeFormat('en-US').formatToParts(new Date(0)))`,
			`[{"type":"month","value":"1"},{"type":"literal","value":"/"},{"type":"day","value":"1"},` +
				`{"type":"literal","value":"/"},{"type":"year","value":"1970"}]`,
		},
		{
			`new Intl.DateTimeFormat('en-US', {timeStyle: 'medium'}).formatToParts(new Date(0))` +
				`.map(p => p.type).join(',')`,
			"hour,literal,minute,literal,second,literal,dayPeriod",
		},
	})
}

func TestToLocaleStringOverrides(t *testing.T) {
	vm := newVM(t)
	evalTests(t, vm, [][2]string{
		// The whole point: before Register these answer in the host zone,
		// silently ignoring the one asked for.
		{`new Date(0).toLocaleString('en-US', {timeZone: 'America/New_York'})`, "12/31/1969, 7:00:00 PM"},
		{`new Date(0).toLocaleDateString('en-US', {timeZone: 'America/New_York'})`, "12/31/1969"},
		{`new Date(0).toLocaleTimeString('en-US', {timeZone: 'America/New_York'})`, "7:00:00 PM"},
		{`new Date(0).toLocaleString('en-US')`, "1/1/1970, 12:00:00 AM"},
		{`new Date(0).toLocaleDateString('en-US')`, "1/1/1970"},
		{`new Date(0).toLocaleTimeString('en-US')`, "12:00:00 AM"},
		{`new Date(0).toLocaleDateString('en-US', {dateStyle: 'full'})`, "Thursday, January 1, 1970"},
		{`new Date(0).toLocaleString()`, "1/1/1970, 12:00:00 AM"},
		{`new Date(NaN).toLocaleString('en-US')`, "Invalid Date"},
		{`[new Date(0)].toLocaleString('en-US')`, "1/1/1970, 12:00:00 AM"},
		{`(1234567.891).toLocaleString('en-US')`, "1,234,567.891"},
		{`(0.42).toLocaleString('en-US', {style: 'percent'})`, "42%"},
		{`(1234.5).toLocaleString('en-US', {style: 'currency', currency: 'USD'})`, "$1,234.50"},
	})
}

func TestNumberFormat(t *testing.T) {
	vm := newVM(t)
	evalTests(t, vm, [][2]string{
		{`new Intl.NumberFormat('en-US').format(1234567.891)`, "1,234,567.891"},
		{`new Intl.NumberFormat('en-US').format(0)`, "0"},
		{`new Intl.NumberFormat('en-US').format(-0)`, "-0"},
		{`new Intl.NumberFormat('en-US').format(NaN)`, "NaN"},
		{`new Intl.NumberFormat('en-US').format(Infinity)`, "∞"},
		{`new Intl.NumberFormat('en-US').format(-Infinity)`, "-∞"},
		{`new Intl.NumberFormat('en-US').format(1e21)`, "1,000,000,000,000,000,000,000"},
		{`new Intl.NumberFormat('en-US').format(1000)`, "1,000"},
		{`new Intl.NumberFormat('en-US', {useGrouping: false}).format(1234567.891)`, "1234567.891"},
		{`new Intl.NumberFormat('en-US', {useGrouping: 'min2'}).format(1000)`, "1000"},
		{`new Intl.NumberFormat('en-US', {useGrouping: 'min2'}).format(10000)`, "10,000"},
		{`new Intl.NumberFormat('en-US', {minimumFractionDigits: 2}).format(0)`, "0.00"},
		{`new Intl.NumberFormat('en-US', {maximumFractionDigits: 2}).format(1.005)`, "1.01"},
		{`new Intl.NumberFormat('en-US', {maximumFractionDigits: 0}).format(2.5)`, "3"},
		{`new Intl.NumberFormat('en-US', {maximumFractionDigits: 0}).format(-2.5)`, "-3"},
		{`new Intl.NumberFormat('en-US', {minimumIntegerDigits: 2}).format(7)`, "07"},
		{`new Intl.NumberFormat('en-US', {maximumSignificantDigits: 2}).format(123456)`, "120,000"},
		{`new Intl.NumberFormat('en-US', {minimumSignificantDigits: 3}).format(1.5)`, "1.50"},
		{`new Intl.NumberFormat('en-US', {style: 'percent'}).format(0.4256)`, "43%"},
		{`new Intl.NumberFormat('en-US', {style: 'percent', maximumFractionDigits: 2}).format(0.4256)`, "42.56%"},
		{`new Intl.NumberFormat('en-US', {style: 'currency', currency: 'USD'}).format(-1234.5)`, "-$1,234.50"},
		{`new Intl.NumberFormat('en-US', {style: 'currency', currency: 'USD', currencySign: 'accounting'}).format(-1234.5)`, "($1,234.50)"},
		{`new Intl.NumberFormat('en-US', {style: 'currency', currency: 'JPY'}).format(1234.5)`, "¥1,235"},
		{`new Intl.NumberFormat('en-US', {style: 'currency', currency: 'EUR'}).format(1)`, "€1.00"},
		{`new Intl.NumberFormat('en-US', {style: 'currency', currency: 'CAD'}).format(1)`, "CA$1.00"},
		{`new Intl.NumberFormat('en-US', {style: 'currency', currency: 'CAD', currencyDisplay: 'narrowSymbol'}).format(1)`, "$1.00"},
		{`new Intl.NumberFormat('en-US', {style: 'unit', unit: 'liter'}).format(16)`, "16 L"},
		{`new Intl.NumberFormat('en-US', {style: 'unit', unit: 'kilometer-per-hour'}).format(50)`, "50 km/h"},
		{`new Intl.NumberFormat('en-US', {style: 'unit', unit: 'day', unitDisplay: 'long'}).format(1)`, "1 day"},
		{`new Intl.NumberFormat('en-US', {style: 'unit', unit: 'day', unitDisplay: 'long'}).format(3)`, "3 days"},
		{`new Intl.NumberFormat('en-US', {notation: 'compact'}).format(1000)`, "1K"},
		{`new Intl.NumberFormat('en-US', {notation: 'compact'}).format(1234)`, "1.2K"},
		{`new Intl.NumberFormat('en-US', {notation: 'compact'}).format(12345)`, "12K"},
		{`new Intl.NumberFormat('en-US', {notation: 'compact'}).format(123456)`, "123K"},
		{`new Intl.NumberFormat('en-US', {notation: 'compact'}).format(999999)`, "1M"},
		{`new Intl.NumberFormat('en-US', {notation: 'compact', compactDisplay: 'long'}).format(1000000)`, "1 million"},
		{`new Intl.NumberFormat('en-US', {notation: 'scientific'}).format(12345)`, "1.235E4"},
		{`new Intl.NumberFormat('en-US', {notation: 'scientific'}).format(0.000001234)`, "1.234E-6"},
		{`new Intl.NumberFormat('en-US', {notation: 'engineering'}).format(12345)`, "12.345E3"},
		{`new Intl.NumberFormat('en-US', {signDisplay: 'always'}).format(42)`, "+42"},
		{`new Intl.NumberFormat('en-US', {signDisplay: 'never'}).format(-42)`, "42"},
		{`new Intl.NumberFormat('en-US', {signDisplay: 'exceptZero'}).format(0)`, "0"},
		{`new Intl.NumberFormat('en-US').format(123456789012345678901234567890n)`, "123,456,789,012,345,678,901,234,567,890"},
		{`[1000, 2000].map(new Intl.NumberFormat('en-US').format).join(' ')`, "1,000 2,000"},
		{
			`JSON.stringify(new Intl.NumberFormat('en-US').formatToParts(-1234.5))`,
			`[{"type":"minusSign","value":"-"},{"type":"integer","value":"1"},{"type":"group","value":","},` +
				`{"type":"integer","value":"234"},{"type":"decimal","value":"."},{"type":"fraction","value":"5"}]`,
		},
	})
}

// TestICURules pins the en-US rules this package was checked against a full
// ICU (node's Intl) for. They are the ones no reading of the spec gives you:
// which field widens to two digits next to which other one, where compact
// notation stops rounding, how a currency label attaches to its amount.
func TestICURules(t *testing.T) {
	vm := newVM(t)
	evalTests(t, vm, [][2]string{
		// A minute or second beside another time field is written, and
		// reported, with two digits. Alone it is not.
		{`new Intl.DateTimeFormat('en-US', {minute: 'numeric', second: 'numeric'}).format(new Date(1500))`, "00:01"},
		{`new Intl.DateTimeFormat('en-US', {minute: 'numeric'}).format(new Date(1500))`, "0"},
		{`new Intl.DateTimeFormat('en-US', {hour: 'numeric', minute: 'numeric'}).resolvedOptions().minute`, "2-digit"},
		{`new Intl.DateTimeFormat('en-US', {minute: 'numeric'}).resolvedOptions().minute`, "numeric"},
		// The 24 hour clock is written with two digits, the 12 hour one is not.
		{`new Intl.DateTimeFormat('en-US', {hour: 'numeric', hourCycle: 'h23'}).resolvedOptions().hour`, "2-digit"},
		{`new Intl.DateTimeFormat('en-US', {hour: 'numeric'}).resolvedOptions().hour`, "numeric"},
		// A bare weekday joins the time with a space, a date with a comma, and
		// a spelled out date with " at ".
		{`new Date(0).toLocaleTimeString('en-US', {weekday: 'long'})`, "Thursday 12:00:00 AM"},
		{`new Date(0).toLocaleString('en-US', {month: 'long', day: 'numeric', hour: 'numeric'})`, "January 1 at 12 AM"},
		// Compact rounds to an integer but never below two significant digits,
		// so a value under one keeps its digits instead of collapsing to zero.
		{`new Intl.NumberFormat('en-US', {notation: 'compact'}).format(0.0001)`, "0.0001"},
		{`new Intl.NumberFormat('en-US', {notation: 'compact'}).format(0.55555)`, "0.56"},
		{`new Intl.NumberFormat('en-US', {notation: 'compact'}).format(12.345)`, "12"},
		// Rounding can carry the mantissa out of its notation's range.
		{`new Intl.NumberFormat('en-US', {notation: 'scientific'}).format(999999)`, "1E6"},
		{`new Intl.NumberFormat('en-US', {notation: 'engineering'}).format(999999999)`, "1E9"},
		{`new Intl.NumberFormat('en-US', {notation: 'engineering'}).format(0.5)`, "500E-3"},
		// The sign follows the rounded value, so a number that rounds to zero
		// is signless, but an infinity is not.
		{`new Intl.NumberFormat('en-US', {signDisplay: 'exceptZero'}).format(-1e-7)`, "0"},
		{`new Intl.NumberFormat('en-US', {signDisplay: 'exceptZero'}).format(-Infinity)`, "-∞"},
		{`new Intl.NumberFormat('en-US', {signDisplay: 'exceptZero'}).format(NaN)`, "NaN"},
		{`new Intl.NumberFormat('en-US', {signDisplay: 'always'}).format(NaN)`, "+NaN"},
		// A currency label ending in a letter is held off the amount by a
		// no-break space - but only when an amount, not a NaN, follows it.
		{`new Intl.NumberFormat('en-US', {style: 'currency', currency: 'USD', currencyDisplay: 'code'}).format(1)`, "USD\u00a01.00"},
		{`new Intl.NumberFormat('en-US', {style: 'currency', currency: 'USD', currencyDisplay: 'code'}).format(NaN)`, "USDNaN"},
		{`new Intl.NumberFormat('en-US', {style: 'currency', currency: 'SEK'}).format(1)`, "SEK\u00a01.00"},
		{`new Intl.NumberFormat('en-US', {style: 'currency', currency: 'SEK', currencyDisplay: 'narrowSymbol'}).format(1)`, "kr\u00a01.00"},
		{`new Intl.NumberFormat('en-US', {style: 'currency', currency: 'IQD'}).format(1)`, "IQD\u00a01"},
		// A narrow unit is written against the amount, a short one is not.
		{`new Intl.NumberFormat('en-US', {style: 'unit', unit: 'celsius', unitDisplay: 'narrow'}).format(20)`, "20°C"},
		{`new Intl.NumberFormat('en-US', {style: 'unit', unit: 'celsius'}).format(20)`, "20°C"},
		{`new Intl.NumberFormat('en-US', {style: 'unit', unit: 'byte'}).format(20)`, "20 byte"},
		// Zero counts as one significant digit.
		{`new Intl.NumberFormat('en-US', {minimumSignificantDigits: 3}).format(0)`, "0.00"},
		// Only the North American zones have letter names in en.
		{`new Intl.DateTimeFormat('en-US', {timeZone: 'Asia/Shanghai', timeZoneName: 'short'}).format(0)`, "1/1/1970, GMT+8"},
		{`new Intl.DateTimeFormat('en-US', {timeZone: 'America/Chicago', timeZoneName: 'short'}).format(0)`, "12/31/1969, CST"},
		// Rounding modes round the shortest decimal representation, not the
		// binary value: (1.005).toFixed(2) is 1.00, this is 1.01.
		{`new Intl.NumberFormat('en-US', {maximumFractionDigits: 2, roundingMode: 'halfEven'}).format(1.005)`, "1"},
		{`new Intl.NumberFormat('en-US', {maximumFractionDigits: 0, roundingMode: 'ceil'}).format(-2.5)`, "-2"},
		{`new Intl.NumberFormat('en-US', {maximumFractionDigits: 0, roundingMode: 'floor'}).format(2.5)`, "2"},
		{`new Intl.NumberFormat('en-US', {maximumFractionDigits: 0, roundingMode: 'halfEven'}).format(2.5)`, "2"},
		{`new Intl.NumberFormat('en-US', {maximumFractionDigits: 0, roundingMode: 'halfEven'}).format(3.5)`, "4"},
		{`new Intl.NumberFormat('en-US', {minimumFractionDigits: 2, trailingZeroDisplay: 'stripIfInteger'}).format(3)`, "3"},
		// Rounding away from zero has to survive dropping every digit there is.
		{`new Intl.NumberFormat('en-US', {maximumFractionDigits: 0, roundingMode: 'ceil'}).format(0.0001)`, "1"},
		{`new Intl.NumberFormat('en-US', {maximumFractionDigits: 2, roundingMode: 'expand'}).format(1e-7)`, "0.01"},
		{`new Intl.NumberFormat('en-US', {maximumFractionDigits: 0, roundingMode: 'ceil'}).format(-0.0001)`, "-0"},
	})
}

func TestResolvedOptions(t *testing.T) {
	vm := newVM(t)
	evalTests(t, vm, [][2]string{
		// The locale is reported as what was used, not as what was asked for.
		{`new Intl.DateTimeFormat('de-DE').resolvedOptions().locale`, "en-US"},
		{`new Intl.NumberFormat(['fr', 'de']).resolvedOptions().locale`, "en-US"},
		{`new Intl.DateTimeFormat().resolvedOptions().timeZone`, "UTC"},
		{`new Intl.DateTimeFormat('en', {timeZone: 'america/new_york'}).resolvedOptions().timeZone`, "America/New_York"},
		{`new Intl.DateTimeFormat('en', {timeZone: 'GMT'}).resolvedOptions().timeZone`, "UTC"},
		{`new Intl.DateTimeFormat('en').resolvedOptions().calendar`, "gregory"},
		{
			`JSON.stringify(new Intl.DateTimeFormat('en-US').resolvedOptions())`,
			`{"locale":"en-US","calendar":"gregory","numberingSystem":"latn","timeZone":"UTC",` +
				`"year":"numeric","month":"numeric","day":"numeric"}`,
		},
		{`new Intl.DateTimeFormat('en', {hour: 'numeric'}).resolvedOptions().hourCycle`, "h12"},
		{`new Intl.DateTimeFormat('en', {hour: 'numeric', hour12: false}).resolvedOptions().hourCycle`, "h23"},
		{`new Intl.DateTimeFormat('en', {hour: 'numeric', hour12: false}).resolvedOptions().hour12`, "false"},
		{`new Intl.DateTimeFormat('en', {dateStyle: 'full'}).resolvedOptions().dateStyle`, "full"},
		{`new Intl.DateTimeFormat('en', {dateStyle: 'full'}).resolvedOptions().weekday`, "undefined"},
		{
			`JSON.stringify(new Intl.NumberFormat('en-US').resolvedOptions())`,
			`{"locale":"en-US","numberingSystem":"latn","style":"decimal","minimumIntegerDigits":1,` +
				`"minimumFractionDigits":0,"maximumFractionDigits":3,"useGrouping":"auto",` +
				`"notation":"standard","signDisplay":"auto","roundingMode":"halfExpand",` +
				`"roundingIncrement":1,"trailingZeroDisplay":"auto","roundingPriority":"auto"}`,
		},
		// No internal state leaks into the reported options.
		{`Object.keys(new Intl.NumberFormat('en', {notation: 'compact'}).resolvedOptions()).filter(k => k[0] === '_').length`, "0"},
		{`Intl.getCanonicalLocales(['EN-us', 'de-de']).join(' ')`, "en-US de-DE"},
		{`Intl.NumberFormat.supportedLocalesOf(['de', 'en-US']).join(' ')`, "en-US"},
		{`Intl.DateTimeFormat.supportedLocalesOf('en-GB').join(' ')`, "en-GB"},
	})
}

func TestErrors(t *testing.T) {
	vm := newVM(t)
	catch := func(src string) string {
		return fmt.Sprintf("(() => {try {%s; return 'no error'} catch (e) {return e.constructor.name + ': ' + e.message}})()", src)
	}
	for _, test := range []struct{ src, want string }{
		{`new Intl.DateTimeFormat('en', {timeZone: 'Mars/Olympus'})`, "RangeError:"},
		{`new Intl.DateTimeFormat('en', {timeZone: 'Local'})`, "RangeError:"},
		{`new Intl.DateTimeFormat('en', {weekday: 'wide'})`, "RangeError:"},
		{`new Intl.DateTimeFormat('en', {dateStyle: 'full', year: 'numeric'})`, "TypeError:"},
		{`new Date(0).toLocaleDateString('en', {timeStyle: 'short'})`, "TypeError:"},
		{`new Intl.DateTimeFormat('en').format(new Date(NaN))`, "RangeError:"},
		{`new Intl.DateTimeFormat('123')`, "RangeError:"},
		{`new Intl.NumberFormat('en', {style: 'currency'})`, "TypeError:"},
		{`new Intl.NumberFormat('en', {style: 'currency', currency: 'US'})`, "RangeError:"},
		{`new Intl.NumberFormat('en', {minimumFractionDigits: 3, maximumFractionDigits: 1})`, "RangeError:"},
		{`Intl.DateTimeFormat.prototype.resolvedOptions.call({})`, "TypeError:"},
	} {
		got := fmt.Sprint(eval(t, vm, catch(test.src)))
		if !strings.HasPrefix(got, test.want) {
			t.Errorf("%s\n\tgot  %q\n\twant a %s", test.src, got, test.want)
		}
	}
}

// TestDefaultTimeZone checks that a formatter with no timeZone option uses the
// host zone, the one resolvedOptions() reports.
func TestDefaultTimeZone(t *testing.T) {
	vm := newVM(t, TimeZone("America/New_York"))
	evalTests(t, vm, [][2]string{
		{`new Intl.DateTimeFormat().resolvedOptions().timeZone`, "America/New_York"},
		{`new Date(0).toLocaleString('en-US')`, "12/31/1969, 7:00:00 PM"},
		{`new Date(0).toLocaleString('en-US', {timeZone: 'UTC'})`, "1/1/1970, 12:00:00 AM"},
	})
}

func TestLocalTimeZone(t *testing.T) {
	tz := localTimeZone()
	if _, _, err := timeZone(tz); err != nil {
		t.Errorf("localTimeZone returned %q, which does not resolve: %v", tz, err)
	}

	t.Logf("localTimeZone() = %q", tz)
}

// TestRegisterPerRealm covers what an embedder giving every document its own
// realm runs into: a VM swapped in for another does not inherit Intl, and
// registering again on a VM that already has it has to stay harmless.
func TestRegisterPerRealm(t *testing.T) {
	vm := newVM(t)
	if got := fmt.Sprint(eval(t, vm, `typeof Intl`)); got != "object" {
		t.Fatalf("got %q, want %q", got, "object")
	}

	// The replacement realm starts without it.
	next, err := quickjs.NewVM()
	if err != nil {
		t.Fatal(err)
	}

	defer next.Close()

	if got := fmt.Sprint(eval(t, next, `typeof Intl`)); got != "undefined" {
		t.Fatalf("a fresh VM already has Intl: got %q", got)
	}

	if err := Register(next, TimeZone("UTC")); err != nil {
		t.Fatal(err)
	}

	// Registering again says so rather than dropping the second call's options
	// or failing on a duplicate host function name.
	err = Register(next, TimeZone("Asia/Tokyo"))
	if err == nil || !strings.Contains(err.Error(), "already has Intl") {
		t.Errorf("second Register: got %v, want an already-registered error", err)
	}

	evalTests(t, next, [][2]string{
		{`new Intl.DateTimeFormat('en-US').format(new Date(0))`, "1/1/1970"},
		{`new Date(0).toLocaleString('en-US', {timeZone: 'America/New_York'})`, "12/31/1969, 7:00:00 PM"},
		{`(1234.5).toLocaleString('en-US')`, "1,234.5"},
	})
}

func TestRegisterOnManyVMs(t *testing.T) {
	// Each VM gets its own host functions and its own default zone.
	for _, tz := range []string{"UTC", "Europe/Prague", "Australia/Sydney"} {
		vm := newVM(t, TimeZone(tz))
		if got, want := fmt.Sprint(eval(t, vm, `new Intl.DateTimeFormat().resolvedOptions().timeZone`)), tz; got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	}
}

// benchmarkLoop times n calls of a javascript loop body, compiling the source
// once so that what is measured is the formatting and its json round trip to
// the host, not QuickJS parsing the benchmark itself.
func benchmarkLoop(b *testing.B, setup, body string) {
	vm := newVM(b)
	eval(b, vm, setup)
	eval(b, vm, "function run(count) {for (let i = 0; i < count; i++) {"+body+"}}")
	b.ResetTimer()
	eval(b, vm, fmt.Sprintf("run(%d)", b.N))
}

func BenchmarkFormatDate(b *testing.B) {
	benchmarkLoop(b, `const f = new Intl.DateTimeFormat('en-US', {dateStyle: 'medium', timeStyle: 'short'})`, `f.format(i * 1e7)`)
}

func BenchmarkFormatNumber(b *testing.B) {
	benchmarkLoop(b, `const n = new Intl.NumberFormat('en-US')`, `n.format(i * 1.5)`)
}

func BenchmarkToLocaleString(b *testing.B) {
	benchmarkLoop(b, `const d = new Date(0)`, `d.toLocaleString('en-US')`)
}

func BenchmarkNewDateTimeFormat(b *testing.B) {
	benchmarkLoop(b, ``, `new Intl.DateTimeFormat('en-US', {dateStyle: 'medium'})`)
}

func ExampleRegister() {
	vm, err := quickjs.NewVM()
	if err != nil {
		panic(err)
	}

	defer vm.Close()

	if err := Register(vm, TimeZone("Europe/Prague")); err != nil {
		panic(err)
	}

	for _, src := range []string{
		`new Intl.DateTimeFormat('en-US', {dateStyle: 'long', timeStyle: 'short', timeZone: 'America/New_York'}).format(new Date(0))`,
		`new Date(0).toLocaleString('en-US', {timeZone: 'Asia/Tokyo'})`,
		`Intl.DateTimeFormat().resolvedOptions().timeZone`,
		`new Intl.NumberFormat('en-US', {style: 'currency', currency: 'USD'}).format(1234.5)`,
	} {
		v, err := vm.Eval(src, quickjs.EvalGlobal)
		if err != nil {
			panic(err)
		}

		fmt.Println(v)
	}
	// Output:
	// December 31, 1969 at 7:00 PM
	// 1/1/1970, 9:00:00 AM
	// Europe/Prague
	// $1,234.50
}
