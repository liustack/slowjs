// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package intl // import "github.com/liustack/quickjs/intl"

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/liustack/quickjs"

	_ "time/tzdata" // The test expectations name IANA zones.
)

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

// newVM returns a VM with Intl registered, in UTC unless opts say otherwise.
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
		// en's morning starts at midnight, and ICU does not name midnight
		// itself. Noon is named only when no minute or second says otherwise.
		{`new Intl.DateTimeFormat('en-US', {dayPeriod: 'short'}).format(new Date(0))`, "in the morning"},
		{`new Intl.DateTimeFormat('en-US', {dayPeriod: 'short'}).format(new Date(3600 * 1000))`, "in the morning"},
		{`new Intl.DateTimeFormat('en-US', {dayPeriod: 'short'}).format(new Date(9 * 3600 * 1000))`, "in the morning"},
		{`new Intl.DateTimeFormat('en-US', {dayPeriod: 'short'}).format(new Date(12.5 * 3600 * 1000))`, "noon"},
		{`new Intl.DateTimeFormat('en-US', {hour: 'numeric', minute: 'numeric', dayPeriod: 'short'}).format(new Date(12.5 * 3600 * 1000))`, "12:30 in the afternoon"},
		{`new Intl.DateTimeFormat('en-US', {dayPeriod: 'short'}).format(new Date(20 * 3600 * 1000))`, "in the evening"},
		{`new Intl.DateTimeFormat('en-US', {dayPeriod: 'short'}).format(new Date(22 * 3600 * 1000))`, "at night"},
		// A date before AD 1 does not name its era unless asked to, as in
		// Chromium.
		{`new Intl.DateTimeFormat('en-US', {timeZone: 'UTC'}).format(new Date(Date.UTC(-1, 0, 1)))`, "1/1/2"},
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
// TestNumberFormatLocales checks the CLDR data behind NumberFormat in a
// few locales. TestGolden compares many more with a browser.
func TestNumberFormatLocales(t *testing.T) {
	evalTests(t, newVM(t), [][2]string{
		{`new Intl.NumberFormat('zh-CN', {style: 'currency', currency: 'CNY'}).format(1234567.891)`, "¥1,234,567.89"},
		{`new Intl.NumberFormat('zh-CN', {notation: 'compact'}).format(123456789)`, "1.2亿"},
		{`new Intl.NumberFormat('ja', {notation: 'compact'}).format(12345)`, "1.2万"},
		{`new Intl.NumberFormat('de-DE', {style: 'currency', currency: 'EUR'}).format(-1234.5)`, "-1.234,50\u00a0€"},
		{`new Intl.NumberFormat('de', {notation: 'compact', compactDisplay: 'long'}).format(1234567)`, "1,2 Millionen"},
		{`new Intl.NumberFormat('fr', {style: 'unit', unit: 'kilometer-per-hour'}).format(1.5)`, "1,5\u202fkm/h"},
		{`new Intl.NumberFormat('es').format(1234)`, "1234"},
		{`new Intl.NumberFormat('es').format(12345)`, "12.345"},
		{`new Intl.NumberFormat('hi').format(1234567.891)`, "12,34,567.891"},
		{`new Intl.NumberFormat('ar-EG').format(-1234.5)`, "\u061c-١٬٢٣٤٫٥"},
		{`new Intl.NumberFormat('en-US', {style: 'currency', currency: 'EUR', currencyDisplay: 'name'}).format(1)`, "1.00 euros"},
		{`new Intl.NumberFormat('fr', {style: 'currency', currency: 'USD', currencySign: 'accounting'}).format(-1)`, "(1,00\u00a0$US)"},
		{`new Intl.NumberFormat('ar', {style: 'unit', unit: 'day', unitDisplay: 'long'}).format(2)`, "يومان"},
		{`JSON.stringify(new Intl.NumberFormat('ar', {style: 'percent'}).formatToParts(-1))`,
			strings.ReplaceAll(`[{"type":"literal","value":"\u200e"},{"type":"minusSign","value":"-"},{"type":"integer","value":"100"},`+
				`{"type":"literal","value":"\u200e"},{"type":"percentSign","value":"%"},{"type":"literal","value":"\u200e"}]`, `\u200e`, "\u200e")},
		{`(123456789n).toLocaleString('de')`, "123.456.789"},
	})
}

// TestBrowserOverrides covers the values Chromium formats with that differ
// from CLDR (see browserOverrides in internal/gen), and a locale that inherits
// one.
// TestChromiumFallback covers locales whose data Chromium does not ship: the
// dates and numbers of az and kk come from root, the currency and region
// names of ak from English, the currency symbols of bs-Cyrl from root.
func TestChromiumFallback(t *testing.T) {
	evalTests(t, newVM(t), [][2]string{
		{`new Intl.DateTimeFormat('az', {dateStyle: 'full', timeZone: 'UTC'}).format(Date.UTC(2026, 0, 5))`, "2026 M01 5, Mon"},
		{`new Intl.NumberFormat('az').format(1234567.891)`, "1,234,567.891"},
		{`new Intl.NumberFormat('kk').format(1234567.891)`, "1,234,567.891"},
		{`new Intl.NumberFormat('ak', {style: 'currency', currency: 'GHS', currencyDisplay: 'name'}).format(2)`, "2.00 Ghanaian cedis"},
		{`new Intl.NumberFormat('bs-Cyrl', {style: 'currency', currency: 'BAM'}).format(2)`, "2,00 BAM"},
		// oc-ES has no currency names in Chromium: they are English, plural
		// names too, which CLDR has none of for oc.
		{`new Intl.NumberFormat('oc-ES', {style: 'currency', currency: 'USD', currencyDisplay: 'name'}).format(2)`, "2.00 US dollars"},
		// fa has no plural names for the dollar in Chromium, so fa-AF writes
		// its own name for it.
		{`new Intl.NumberFormat('fa-AF', {style: 'currency', currency: 'USD', currencyDisplay: 'name'}).format(2)`, "۲٫۰۰ دالر امریکا"},
		{`new Intl.DateTimeFormat('bs-Cyrl', {dateStyle: 'full', timeZone: 'UTC'}).format(Date.UTC(2026, 0, 5))`, "понедјељак, 05. јануар 2026."},
		{`new Intl.DateTimeFormat('ckb-IR', {weekday: 'long', calendar: 'gregory', timeZone: 'UTC'}).format(0)`, "Thu"},
		{`new Intl.DateTimeFormat('ckb-IR', {dateStyle: 'medium', timeZone: 'UTC'}).format(0)`, "AP 1348 بەفرانبار 11"},
		// Root's Persian day names and patterns are the ones of the locale's
		// Gregorian and generic calendars, which ckb-IR takes from root.
		{`new Intl.DateTimeFormat('ckb-IR', {timeZone: 'UTC'}).format(Date.UTC(2026, 0, 5))`, "AP 1404-10-15"},
		{`new Intl.DateTimeFormat('ckb-IR', {weekday: 'short', timeZone: 'UTC'}).format(Date.UTC(2026, 0, 5))`, "Mon"},
		{`new Intl.DateTimeFormat('ckb-IR', {month: 'long', year: 'numeric', timeZone: 'UTC'}).format(Date.UTC(2026, 0, 5))`, "AP 1404 بەفرانبار"},
		{`new Intl.DateTimeFormat('ckb-IR').resolvedOptions().month`, "2-digit"},
		// om-KE has its own narrow stand-alone day names in Chromium, and
		// root's others: om has none.
		{`new Intl.DateTimeFormat('om-KE', {weekday: 'short', timeZone: 'UTC'}).format(Date.UTC(2026, 6, 5))`, "Sun"},
		{`new Intl.DateTimeFormat('om-KE', {weekday: 'long', timeZone: 'UTC'}).format(Date.UTC(2026, 6, 5))`, "Sun"},
		// kk-Arab has no unit names in Chromium, and root's long ones are
		// its short ones.
		{`new Intl.NumberFormat('kk-Arab', {style: 'unit', unit: 'kilometer', unitDisplay: 'long'}).format(2)`, "2 km"},
		{`new Intl.NumberFormat('kk-Arab', {style: 'unit', unit: 'meter-per-second', unitDisplay: 'long'}).format(2)`, "2 m/s"},
		// Root's accounting pattern and Buddhist months stand for the
		// locale's own.
		{`new Intl.NumberFormat('ar', {style: 'currency', currency: 'EGP', currencySign: 'accounting'}).format(-2)`, "(؜2.00 ج.م.‏)"},
		{`new Intl.DateTimeFormat('th-u-ca-buddhist', {dateStyle: 'long', timeZone: 'UTC'}).format(0)`, "1 มกราคม 2513"},
	})
}

// TestCurrenciesChromiumLeavesOut covers the currencies Chromium has no
// names for, most of them no longer in use: they are written as their codes.
func TestCurrenciesChromiumLeavesOut(t *testing.T) {
	evalTests(t, newVM(t), [][2]string{
		{`new Intl.NumberFormat('de', {style: 'currency', currency: 'DEM'}).format(1)`, "1,00\u00a0DEM"},
		{`new Intl.NumberFormat('de', {style: 'currency', currency: 'DEM', currencyDisplay: 'narrowSymbol'}).format(1)`, "1,00\u00a0DEM"},
		{`new Intl.NumberFormat('de', {style: 'currency', currency: 'DEM', currencyDisplay: 'name'}).format(1)`, "1,00 DEM"},
		{`new Intl.NumberFormat('de', {style: 'currency', currency: 'XXX', currencyDisplay: 'name'}).format(1)`, "1,00 XXX"},
		{`new Intl.NumberFormat('en', {style: 'currency', currency: 'VEF', currencyDisplay: 'name'}).format(1)`, "1.00 VEF"},
		{`new Intl.NumberFormat('en', {style: 'currency', currency: 'XAU', currencyDisplay: 'name'}).format(1)`, "1.00 XAU"},
		{`new Intl.NumberFormat('ca', {style: 'currency', currency: 'ESP'}).format(-1234567.891)`, "-1.234.568\u00a0ESP"},
		{`new Intl.NumberFormat('el', {style: 'currency', currency: 'GRD'}).format(-1234567.891)`, "-1.234.567,89\u00a0GRD"},
		{`new Intl.NumberFormat('pt-PT', {style: 'currency', currency: 'PTE'}).format(-1234567.891)`, "-1\u00a0234\u00a0567,89\u00a0PTE"},
		// Chromium still has the kuna.
		{`new Intl.NumberFormat('en', {style: 'currency', currency: 'HRK', currencyDisplay: 'name'}).format(1)`, "1.00 Croatian kunas"},
		{`new Intl.NumberFormat('hr', {style: 'currency', currency: 'HRK'}).format(1)`, "1,00\u00a0kn"},
	})
}

func TestBrowserOverrides(t *testing.T) {
	zoneName := func(zone, date, style string) string {
		return `new Intl.DateTimeFormat('ko', {timeZone: '` + zone + `', timeZoneName: '` + style + `'}).formatToParts(new Date('` + date +
			`')).find(p => p.type === 'timeZoneName').value`
	}
	evalTests(t, newVM(t), [][2]string{
		{`new Intl.NumberFormat('fr-CH').format(1234567.891)`, "1\u202f234\u202f567,891"},
		{`new Intl.NumberFormat('fr-CH', {style: 'currency', currency: 'CHF'}).format(-1234567.891)`, "-1\u202f234\u202f567.89\u00a0CHF"},
		{`new Intl.NumberFormat('uk', {style: 'currency', currency: 'UAH'}).format(-1234.5)`, "-1\u00a0234,50\u00a0грн"},
		{`new Intl.NumberFormat('uk', {style: 'currency', currency: 'UAH', currencyDisplay: 'narrowSymbol'}).format(-1234.5)`, "-1\u00a0234,50\u00a0₴"},
		{`new Intl.NumberFormat('ru', {style: 'currency', currency: 'UAH'}).format(-1234.5)`, "-1\u00a0234,50\u00a0грн."},
		{`new Intl.NumberFormat('ru-UA', {style: 'currency', currency: 'UAH'}).format(-1234.5)`, "-1234,50\u00a0грн."},
		{zoneName("America/New_York", "2026-01-15T12:00Z", "long"), "북미 동부 표준시"},
		{zoneName("America/Chicago", "2026-07-15T12:00Z", "long"), "북미 중부 하계 표준시"},
		{zoneName("America/Denver", "2026-01-15T12:00Z", "long"), "북미 로키 산맥 표준시"},
		{zoneName("America/Denver", "2026-07-15T12:00Z", "long"), "북미 로키 산맥 하계 표준시"},
		{zoneName("America/Phoenix", "2026-07-15T12:00Z", "longGeneric"), "북미 로키 산맥 표준시"},
		{zoneName("America/Los_Angeles", "2026-07-15T12:00Z", "longGeneric"), "북미 태평양 시간"},
	})
}

// TestCompactWithoutDigits covers compact patterns that spell the number out,
// like French "mille" for exactly one thousand.
func TestCompactWithoutDigits(t *testing.T) {
	evalTests(t, newVM(t), [][2]string{
		{`new Intl.NumberFormat('fr', {notation: 'compact', compactDisplay: 'long'}).format(1000)`, "mille"},
		{`new Intl.NumberFormat('fr', {notation: 'compact', compactDisplay: 'long'}).format(999.9)`, "mille"},
		{`new Intl.NumberFormat('fr', {notation: 'compact', compactDisplay: 'long', signDisplay: 'always'}).format(1000)`, "+mille"},
		{`JSON.stringify(new Intl.NumberFormat('fr', {notation: 'compact', compactDisplay: 'long'}).formatToParts(1000))`, `[{"type":"compact","value":"mille"}]`},
		{`new Intl.NumberFormat('it', {notation: 'compact', compactDisplay: 'long'}).format(1000)`, "mille"},
		// An exact pattern is for that number only, not for its negative.
		{`new Intl.NumberFormat('fr', {notation: 'compact', compactDisplay: 'long'}).format(-1000)`, "-1 millier"},
		{`new Intl.NumberFormat('fr', {notation: 'compact', compactDisplay: 'long'}).format(2000)`, "2 mille"},
		{`new Intl.NumberFormat('fr', {notation: 'compact', compactDisplay: 'long'}).format(1000000)`, "1 million"},
	})
}

// TestOtherNumberingSystems covers digits a locale does not use by default.
// ICU takes their symbols and patterns from the locale's data for them, then
// from root's, which has them for arab and arabext, and only then from the
// locale's latn data. cldr-json has none of these for de, fr, en or sv.
func TestOtherNumberingSystems(t *testing.T) {
	evalTests(t, newVM(t), [][2]string{
		{`new Intl.NumberFormat('de-u-nu-arab').format(-1234.5)`, "\u061c-\u0661\u066c\u0662\u0663\u0664\u066b\u0665"},
		{`new Intl.NumberFormat('fr', {numberingSystem: 'arab'}).format(-1234.5)`, "\u061c-\u0661\u066c\u0662\u0663\u0664\u066b\u0665"},
		{`new Intl.NumberFormat('en', {numberingSystem: 'arab', style: 'percent'}).format(0.5)`, "\u0665\u0660\u066a\u061c"},
		{`new Intl.NumberFormat('en', {numberingSystem: 'arab', notation: 'scientific'}).format(12345)`, "\u0661\u066b\u0662\u0663\u0665\u0627\u0633\u0664"},
		{`new Intl.NumberFormat('en', {numberingSystem: 'arab', signDisplay: 'always'}).format(5)`, "\u061c+\u0665"},
		{`new Intl.NumberFormat('en', {numberingSystem: 'arab', style: 'currency', currency: 'USD'}).format(-1234.5)`, "\u061c-\u0661\u066c\u0662\u0663\u0664\u066b\u0665\u0660\u00a0$"},
		{`new Intl.NumberFormat('en', {numberingSystem: 'arabext'}).format(-1234.5)`, "\u200e-\u200e\u06f1\u066c\u06f2\u06f3\u06f4\u066b\u06f5"},
		{`new Intl.NumberFormat('en', {numberingSystem: 'arabext', style: 'percent'}).format(0.5)`, "\u06f5\u06f0\u066a"},
		{`new Intl.NumberFormat('en', {numberingSystem: 'beng'}).format(-1234.5)`, "-\u09e7,\u09e8\u09e9\u09ea.\u09eb"},
		{`new Intl.NumberFormat('sv', {numberingSystem: 'arab'}).format(-1234.5)`, "\u061c\u2212\u0661\u00a0\u0662\u0663\u0664\u066b\u0665"},
		{`new Intl.NumberFormat('fa', {numberingSystem: 'arab'}).format(NaN)`, "\u0646\u0627\u0639\u062f\u062f"},
	})
}

// TestCurrencyPatterns covers the currencies a locale formats with a pattern
// and separators of their own, as ICU does: the euro in en-150 and the
// locales under it, the lira in tr, the escudo in pt-CV. The currency of the
// region a locale names sets them for every currency it formats, unless the
// one formatted has its own.
func TestCurrencyPatterns(t *testing.T) {
	evalTests(t, newVM(t), [][2]string{
		{`new Intl.NumberFormat('en-DE', {style: 'currency', currency: 'EUR'}).format(-1234567.891)`, "-€1,234,567.89"},
		{`new Intl.NumberFormat('en-DE', {style: 'currency', currency: 'USD'}).format(-1234567.891)`, "-US$1,234,567.89"},
		{`new Intl.NumberFormat('en-DE', {style: 'currency', currency: 'USD', currencyDisplay: 'name'}).format(2)`, "US$2.00 US dollars"},
		{`new Intl.NumberFormat('en-DE', {style: 'currency', currency: 'EUR', currencyDisplay: 'code'}).format(-1234.5)`, "-EUR\u00a01,234.50"},
		{`new Intl.NumberFormat('en-DE', {style: 'currency', currency: 'EUR', currencySign: 'accounting'}).format(-1234.5)`, "-€1,234.50"},
		{`new Intl.NumberFormat('en-DE', {style: 'currency', currency: 'EUR', notation: 'compact'}).format(500)`, "€500"},
		{`new Intl.NumberFormat('en-DE', {style: 'currency', currency: 'EUR', notation: 'compact'}).format(5000)`, "€5K"},
		{`new Intl.NumberFormat('en-DE', {style: 'currency', currency: 'EUR', notation: 'scientific'}).format(5432.1)`, "€5.432E3"},
		{`new Intl.NumberFormat('en-DE', {style: 'currency', currency: 'USD', currencySign: 'accounting'}).format(-2)`, "-US$2.00"},
		{`new Intl.NumberFormat('en-DE', {style: 'currency', currency: 'EUR'}).formatToParts(-1234.5).map(p => p.type + ':' + p.value).join(' ')`,
			"minusSign:- currency:€ integer:1 group:, integer:234 decimal:. fraction:50"},
		{`new Intl.NumberFormat('en-DE').format(1234.5)`, "1.234,5"},
		{`new Intl.NumberFormat('en-DE', {style: 'percent'}).format(12.5)`, "1.250\u00a0%"},
		{`new Intl.NumberFormat('en-150', {style: 'currency', currency: 'EUR'}).format(-1234567.891)`, "-€1,234,567.89"},
		{`new Intl.NumberFormat('en-150', {style: 'currency', currency: 'USD'}).format(-1234.5)`, "-1,234.50\u00a0US$"},
		{`new Intl.NumberFormat('en-150', {style: 'currency', currency: 'USD', currencyDisplay: 'name'}).format(2)`, "2.00 US dollars"},
		{`new Intl.NumberFormat('en-CH', {style: 'currency', currency: 'EUR'}).format(-1234567.891)`, "-€1,234,567.89"},
		{`new Intl.NumberFormat('en-CH', {style: 'currency', currency: 'CHF'}).format(-1234.5)`, "CHF-1'234.50"},
		{`new Intl.NumberFormat('en-AT', {style: 'currency', currency: 'USD'}).format(-1234.5)`, "-US$\u00a01.234,50"},
		{`new Intl.NumberFormat('en-AT', {style: 'currency', currency: 'USD', currencyDisplay: 'name'}).format(2)`, "US$\u00a02,00 US dollars"},
		{`new Intl.NumberFormat('tr', {style: 'currency', currency: 'TRY', currencySign: 'accounting'}).format(-1234.5)`, "-₺1.234,50"},
		{`new Intl.NumberFormat('tr', {style: 'currency', currency: 'TRY', currencyDisplay: 'name'}).format(-1234.5)`, "-₺1.234,50 Türk lirası"},
		{`new Intl.NumberFormat('tr', {style: 'currency', currency: 'USD', currencySign: 'accounting'}).format(-1234.5)`, "($1.234,50)"},
		{`new Intl.NumberFormat('tr-TR', {style: 'currency', currency: 'USD', currencyDisplay: 'name'}).format(2)`, "$2,00 ABD doları"},
		{`new Intl.NumberFormat('tr-TR', {style: 'currency', currency: 'USD', currencySign: 'accounting'}).format(-1234.5)`, "-$1.234,50"},
		{`new Intl.NumberFormat('tr', {style: 'currency', currency: 'USD', currencyDisplay: 'name'}).format(2)`, "2,00 ABD doları"},
		{`new Intl.NumberFormat('tr-CY', {style: 'currency', currency: 'USD', currencyDisplay: 'name'}).format(2)`, "2,00 ABD doları"},
		{`new Intl.NumberFormat('tr-CY', {style: 'currency', currency: 'TRY', currencySign: 'accounting'}).format(-1234.5)`, "-₺1.234,50"},
		{`new Intl.NumberFormat('pt-CV', {style: 'currency', currency: 'CVE'}).format(-1234567.891)`, "-1\u00a0234\u00a0567$89\u00a0\u200b"},
		{`new Intl.NumberFormat('pt-CV', {style: 'currency', currency: 'EUR', currencySign: 'accounting'}).format(-1234567.891)`, "-1\u00a0234\u00a0567$89\u00a0€"},
		{`new Intl.NumberFormat('pt-CV', {style: 'currency', currency: 'USD', currencyDisplay: 'name'}).format(2)`, "2$00\u00a0US$ dólares dos Estados Unidos"},
		{`new Intl.NumberFormat('pt-CV').format(-1234567.891)`, "-1\u00a0234\u00a0567,891"},
		{`new Intl.NumberFormat('pt', {style: 'currency', currency: 'CVE'}).format(-1234567.891)`, "-CVE\u00a01.234.567,89"},
	})
}

// TestCurrencyDigitsByNotation checks that only standard notation gives a
// currency its own number of fraction digits: the others default to 0 to 3,
// as ECMA-402 says.
func TestCurrencyDigitsByNotation(t *testing.T) {
	evalTests(t, newVM(t), [][2]string{
		{`new Intl.NumberFormat('en', {style: 'currency', currency: 'EUR', notation: 'scientific'}).format(5000)`, "€5E3"},
		{`new Intl.NumberFormat('en', {style: 'currency', currency: 'EUR', notation: 'scientific'}).format(5432.1)`, "€5.432E3"},
		{`new Intl.NumberFormat('en', {style: 'currency', currency: 'JPY', notation: 'scientific'}).format(5432.1)`, "¥5.432E3"},
		{`new Intl.NumberFormat('de', {style: 'currency', currency: 'EUR', notation: 'engineering'}).format(54321)`, "54,321E3\u00a0€"},
		{`new Intl.NumberFormat('en', {style: 'currency', currency: 'EUR', notation: 'compact', minimumFractionDigits: 1}).format(1234)`, "€1.234K"},
		{`new Intl.NumberFormat('en', {style: 'currency', currency: 'EUR', notation: 'compact', maximumFractionDigits: 1}).format(1234)`, "€1.2K"},
		{`new Intl.NumberFormat('en', {style: 'currency', currency: 'EUR', notation: 'scientific'}).resolvedOptions().minimumFractionDigits`, "0"},
		{`new Intl.NumberFormat('en', {style: 'currency', currency: 'EUR', notation: 'scientific'}).resolvedOptions().maximumFractionDigits`, "3"},
		{`new Intl.NumberFormat('en', {style: 'currency', currency: 'EUR'}).resolvedOptions().minimumFractionDigits`, "2"},
	})
}

// A maximum below a currency's default minimum lowers the minimum, as
// ECMA-402 has said since 2021, instead of throwing.
func TestMaximumFractionDigitsAlone(t *testing.T) {
	evalTests(t, newVM(t), [][2]string{
		{`new Intl.NumberFormat('zh-CN', {style: 'currency', currency: 'CNY', maximumFractionDigits: 0}).format(428.5)`, "¥429"},
		{`new Intl.NumberFormat('en-US', {style: 'currency', currency: 'USD', maximumFractionDigits: 1}).format(1.25)`, "$1.3"},
		{`new Intl.NumberFormat('en-US', {style: 'currency', currency: 'USD', maximumFractionDigits: 1}).resolvedOptions().minimumFractionDigits`, "1"},
		{`new Intl.NumberFormat('en-US', {style: 'currency', currency: 'JPY', maximumFractionDigits: 0}).format(5.5)`, "¥6"},
		{`(() => { try { new Intl.NumberFormat('en-US', {style: 'currency', currency: 'USD', minimumFractionDigits: 3, maximumFractionDigits: 1}) } catch (e) { return e.constructor.name } })()`, "RangeError"},
	})
}

// Significant digits leave the fraction digits unread, so a fraction digit
// out of range does not throw next to them.
func TestFractionDigitsUnreadNextToSignificant(t *testing.T) {
	evalTests(t, newVM(t), [][2]string{
		{`new Intl.NumberFormat('en', {maximumSignificantDigits: 3, maximumFractionDigits: 200}).format(1.2345)`, "1.23"},
		{`new Intl.NumberFormat('en', {notation: 'compact', minimumSignificantDigits: 2, minimumFractionDigits: -1}).format(1234)`, "1.234K"},
		{`new Intl.PluralRules('en', {maximumSignificantDigits: 3, maximumFractionDigits: 200}).select(1)`, "one"},
		{`(() => { try { new Intl.NumberFormat('en', {maximumFractionDigits: 200}) } catch (e) { return e.constructor.name } })()`, "RangeError"},
	})
}

func TestPluralRules(t *testing.T) {
	evalTests(t, newVM(t), [][2]string{
		{`new Intl.PluralRules('en').select(1)`, "one"},
		{`new Intl.PluralRules('en').select(-1)`, "one"},
		{`new Intl.PluralRules('en').select(1.5)`, "other"},
		{`new Intl.PluralRules('en', {minimumFractionDigits: 1}).select(1)`, "other"},
		{`new Intl.PluralRules('en', {type: 'ordinal'}).select(22)`, "two"},
		{`new Intl.PluralRules('ar').select(11)`, "many"},
		{`new Intl.PluralRules('ru').select(21)`, "one"},
		{`new Intl.PluralRules('fr').select(1000000)`, "many"},
		{`new Intl.PluralRules('fr', {notation: 'compact'}).select(1000)`, "one"},
		{`new Intl.PluralRules('fr').select(1000)`, "other"},
		{`new Intl.PluralRules('zh-CN').resolvedOptions().locale`, "zh"},
		{`new Intl.PluralRules('pl').resolvedOptions().pluralCategories.join(' ')`, "one few many other"},
		{`JSON.stringify(new Intl.PluralRules('en').resolvedOptions())`,
			`{"locale":"en","type":"cardinal","notation":"standard","minimumIntegerDigits":1,"minimumFractionDigits":0,` +
				`"maximumFractionDigits":3,"pluralCategories":["one","other"],"roundingIncrement":1,"roundingMode":"halfExpand",` +
				`"roundingPriority":"auto","trailingZeroDisplay":"auto"}`},
		{`new Intl.PluralRules('ru').selectRange(1, 5)`, "many"},
		{`new Intl.PluralRules('en').selectRange(0, 1)`, "other"},
		{`new Intl.PluralRules('en').select(NaN)`, "other"},
		{`Intl.PluralRules.supportedLocalesOf(['cy', 'xx']).join(' ')`, "cy"},
		{`(() => { try { Intl.PluralRules('en') } catch (e) { return e.constructor.name } })()`, "TypeError"},
		{`(() => { try { new Intl.PluralRules('en').selectRange(1) } catch (e) { return e.constructor.name } })()`, "TypeError"},
		{`(() => { try { new Intl.PluralRules('en').selectRange(NaN, 1) } catch (e) { return e.constructor.name } })()`, "RangeError"},
		{`(() => { try { new Intl.PluralRules('en', {type: 'x'}) } catch (e) { return e.constructor.name } })()`, "RangeError"},
		{`Object.prototype.toString.call(new Intl.PluralRules())`, "[object Intl.PluralRules]"},
	})
}

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

// TestDateTimeFormatLocales checks dates in other languages against what
// Chromium writes.
// TestStyleTimeGenerated covers locales whose ICU data is found under
// another region or language: ICU generates their timeStyle pattern for the
// hour cycle of the region instead of taking the data's. kok-IN finds its
// data in kok_Deva and zh-MY in zh_Hans, kok-Deva-IN in a bundle of its own.
func TestStyleTimeGenerated(t *testing.T) {
	evalTests(t, newVM(t), [][2]string{
		{`new Intl.DateTimeFormat('kok-IN', {timeStyle: 'short', timeZone: 'UTC'}).format(Date.UTC(2026, 6, 5, 3, 7))`, "3:07 AM"},
		{`new Intl.DateTimeFormat('kok-IN', {dateStyle: 'medium', timeStyle: 'short', timeZone: 'UTC'}).format(Date.UTC(2026, 6, 5, 3, 7))`, "2026 M07 5 3:07 AM"},
		{`new Intl.DateTimeFormat('kok-Deva-IN', {dateStyle: 'medium', timeStyle: 'short', timeZone: 'UTC'}).format(Date.UTC(2026, 6, 5, 3, 7))`, "2026 M07 5 03:07 AM"},
		{`new Intl.DateTimeFormat('kok', {timeStyle: 'short', timeZone: 'UTC'}).format(Date.UTC(2026, 6, 5, 3, 7))`, "03:07 AM"},
		{`new Intl.DateTimeFormat('zh-MY', {dateStyle: 'full', timeStyle: 'long', timeZone: 'America/New_York'}).format(Date.UTC(2026, 0, 5, 15, 7, 9))`, "2026年1月5日星期一 GMT-5上午10:07:09"},
		{`new Intl.DateTimeFormat('de', {timeStyle: 'short', hourCycle: 'h12', timeZone: 'UTC'}).format(Date.UTC(2026, 6, 5, 3, 7))`, "03:07 AM"},
		{`new Intl.DateTimeFormat('de-u-hc-h12', {timeStyle: 'short', timeZone: 'UTC'}).format(Date.UTC(2026, 6, 5, 3, 7))`, "3:07 AM"},
	})
}

// TestPatternTies covers skeletons two available formats match equally
// well. ICU keeps the one it added first, and it adds a locale's own formats
// before its parent's: en-AU's "EEEE d MMMM" wins over en-001's "E, d MMM".
func TestPatternTies(t *testing.T) {
	evalTests(t, newVM(t), [][2]string{
		{`new Intl.DateTimeFormat('en-AU', {weekday: 'short', month: 'long', day: 'numeric', timeZone: 'UTC'}).format(Date.UTC(2026, 6, 5))`, "Sun 5 July"},
		{`new Intl.DateTimeFormat('en-ZA', {month: 'numeric', day: '2-digit', timeZone: 'UTC'}).format(Date.UTC(2026, 6, 5))`, "07/05"},
		{`new Intl.DateTimeFormat('en-ZA', {month: '2-digit', day: 'numeric', timeZone: 'UTC'}).format(Date.UTC(2026, 6, 5))`, "07/05"},
		{`new Intl.DateTimeFormat('es-MX', {month: 'long', day: '2-digit', timeZone: 'UTC'}).format(Date.UTC(2026, 6, 5))`, "05-julio"},
		{`new Intl.DateTimeFormat('es-US', {month: 'long', day: '2-digit', timeZone: 'UTC'}).format(Date.UTC(2026, 6, 5))`, "05-julio"},
		{`new Intl.DateTimeFormat('es-CO', {month: 'long', day: '2-digit', timeZone: 'UTC'}).format(Date.UTC(2026, 6, 5))`, "5 de julio"},
	})
}

func TestDateTimeFormatLocales(t *testing.T) {
	vm := newVM(t)
	evalTests(t, vm, [][2]string{
		{`new Intl.DateTimeFormat('zh-CN', {dateStyle: 'full', timeStyle: 'long', timeZone: 'Asia/Shanghai'}).format(0)`, "1970年1月1日星期四 GMT+8 08:00:00"},
		{`new Intl.DateTimeFormat('zh-TW', {hour: 'numeric', minute: 'numeric', timeZone: 'Asia/Taipei'}).format(0)`, "上午8:00"},
		{`new Intl.DateTimeFormat('ja', {dateStyle: 'medium', timeStyle: 'short', timeZone: 'Asia/Tokyo'}).format(0)`, "1970/01/01 9:00"},
		{`new Intl.DateTimeFormat('ja-JP', {hour: 'numeric', hour12: true, timeZone: 'UTC'}).format(0)`, "午前0時"},
		{`new Intl.DateTimeFormat('de', {weekday: 'long', day: 'numeric', month: 'long', timeZone: 'Europe/Berlin'}).format(0)`, "Donnerstag, 1. Januar"},
		{`new Intl.DateTimeFormat('ar-EG', {dateStyle: 'short', timeZone: 'Africa/Cairo'}).format(0)`, "١\u200f/١\u200f/١٩٧٠"},
		{`new Intl.DateTimeFormat('zh-CN', {dayPeriod: 'long', hour: 'numeric', timeZone: 'UTC'}).format(0)`, "0时"},
		{`new Date(0).toLocaleString('zh-CN', {timeZone: 'Asia/Shanghai'})`, "1970/1/1 08:00:00"},
		{`new Date(0).toLocaleDateString('de-DE', {timeZone: 'UTC'})`, "1.1.1970"},
		{`new Date(0).toLocaleTimeString('fr', {timeZone: 'UTC'})`, "00:00:00"},
		// fa and th count years in their own calendars by default.
		{`new Intl.DateTimeFormat('fa', {dateStyle: 'long', timeZone: 'Asia/Tehran'}).format(0)`, "۱۱ دی ۱۳۴۸"},
		{`new Intl.DateTimeFormat('fa').resolvedOptions().calendar`, "persian"},
		{`new Intl.DateTimeFormat('th', {dateStyle: 'long', timeZone: 'Asia/Bangkok'}).format(0)`, "1 มกราคม 2513"},
		// A calendar there is no data for is dropped, and the date is
		// Gregorian. Chromium keeps the keyword and formats in it.
		{`new Intl.DateTimeFormat('ja-JP-u-ca-japanese').resolvedOptions().locale`, "ja-JP"},
		{`new Intl.DateTimeFormat('ja-JP-u-ca-japanese').resolvedOptions().calendar`, "gregory"},
		// The hour cycle keyword, and an hour12 option overriding it.
		{`new Intl.DateTimeFormat('en-US-u-hc-h23', {hour: 'numeric', minute: 'numeric', timeZone: 'UTC'}).format(0)`, "00:00"},
		{`new Intl.DateTimeFormat('en-US-u-hc-h23', {hour: 'numeric', hour12: true}).resolvedOptions().locale`, "en-US"},
		// Zone names: the locale's own, a GMT offset where it has none, and
		// the location for a generic name with no metazone name.
		{`new Intl.DateTimeFormat('ko', {timeZoneName: 'long', timeZone: 'Asia/Seoul'}).format(0)`, "1970. 1. 1. 한국 표준시"},
		{`new Intl.DateTimeFormat('en-IN', {timeZoneName: 'short', timeZone: 'America/New_York'}).format(0)`, "31/12/1969, GMT-5"},
		{`new Intl.DateTimeFormat('pt-BR', {timeZoneName: 'shortGeneric', timeZone: 'America/Sao_Paulo'}).format(0)`, "31/12/1969, BRT"},
		{`new Intl.DateTimeFormat('en', {timeZoneName: 'shortGeneric', timeZone: 'Asia/Hong_Kong'}).format(0)`, "1/1/1970, Hong Kong Time"},
		{`new Intl.DateTimeFormat('en-US', {timeZoneName: 'shortOffset', timeZone: 'UTC'}).format(0)`, "1/1/1970, GMT+0"},
		// Before 1970 no zone is in a metazone, and local mean time has
		// seconds in its offset.
		{`new Intl.DateTimeFormat('en', {timeZoneName: 'long', timeZone: 'Asia/Shanghai'}).format(-62198755200000)`, "1/1/2, GMT+08:05:43"},
		// Chromium does not call British winter time Greenwich Mean Time.
		{`new Intl.DateTimeFormat('en-GB', {timeZoneName: 'long', timeZone: 'Europe/London'}).format(Date.UTC(2025, 0, 15))`, "15/01/2025, GMT+00:00"},
	})
}

func TestResolvedOptions(t *testing.T) {
	vm := newVM(t)
	evalTests(t, vm, [][2]string{
		// The locale is reported as what was used, not as what was asked for.
		{`new Intl.DateTimeFormat('cy').resolvedOptions().locale`, "en-US"},
		{`new Intl.DateTimeFormat('de-DE').resolvedOptions().locale`, "de-DE"},
		{`new Intl.NumberFormat(['fr', 'de']).resolvedOptions().locale`, "fr"},
		{`new Intl.NumberFormat(['cy', 'de']).resolvedOptions().locale`, "de"},
		{`new Intl.NumberFormat('zh-Hans-CN').resolvedOptions().locale`, "zh-Hans-CN"},
		{`new Intl.NumberFormat('ar-EG-u-nu-latn').resolvedOptions().locale`, "ar-EG-u-nu-latn"},
		{`new Intl.NumberFormat('ar-EG').resolvedOptions().numberingSystem`, "arab"},
		{`new Intl.NumberFormat('en-US-u-hc-h23').resolvedOptions().locale`, "en-US"},
		{`new Intl.DateTimeFormat().resolvedOptions().timeZone`, "UTC"},
		{`new Intl.DateTimeFormat('en', {timeZone: 'america/new_york'}).resolvedOptions().timeZone`, "America/New_York"},
		// The case is IANA's, whatever the host's file system would accept,
		// and every name of a zone is reported as the one CLDR (and so a
		// browser) treats as canonical.
		{`new Intl.DateTimeFormat('en', {timeZone: 'ASIA/CALCUTTA'}).resolvedOptions().timeZone`, "Asia/Calcutta"},
		{`new Intl.DateTimeFormat('en', {timeZone: 'Asia/Kolkata'}).resolvedOptions().timeZone`, "Asia/Calcutta"},
		{`new Intl.DateTimeFormat('en', {timeZone: 'US/Pacific'}).resolvedOptions().timeZone`, "America/Los_Angeles"},
		{`new Intl.DateTimeFormat('en', {timeZone: 'america/port-au-prince'}).resolvedOptions().timeZone`, "America/Port-au-Prince"},
		{`new Intl.DateTimeFormat('en', {timeZone: 'etc/gmt+5'}).resolvedOptions().timeZone`, "Etc/GMT+5"},
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
				`"notation":"standard","signDisplay":"auto","roundingIncrement":1,` +
				`"roundingMode":"halfExpand","roundingPriority":"auto","trailingZeroDisplay":"auto"}`,
		},
		// ECMA-402 lists the fraction digits before the significant ones.
		{
			`JSON.stringify(new Intl.NumberFormat('en-US', {notation: 'compact'}).resolvedOptions())`,
			`{"locale":"en-US","numberingSystem":"latn","style":"decimal","minimumIntegerDigits":1,` +
				`"minimumFractionDigits":0,"maximumFractionDigits":0,"minimumSignificantDigits":1,` +
				`"maximumSignificantDigits":2,"useGrouping":"min2","notation":"compact","compactDisplay":"short",` +
				`"signDisplay":"auto","roundingIncrement":1,"roundingMode":"halfExpand",` +
				`"roundingPriority":"morePrecision","trailingZeroDisplay":"auto"}`,
		},
		// No internal state leaks into the reported options.
		{`Object.keys(new Intl.NumberFormat('en', {notation: 'compact'}).resolvedOptions()).filter(k => k[0] === '_').length`, "0"},
		{`Intl.getCanonicalLocales(['EN-us', 'de-de']).join(' ')`, "en-US de-DE"},
		{`Intl.NumberFormat.supportedLocalesOf(['de', 'en-US', 'cy']).join(' ')`, "de en-US"},
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
		{`new Intl.DateTimeFormat('en', {timeZone: 'Etc/Unknown'})`, "RangeError:"},
		{`new Intl.DateTimeFormat('en', {weekday: 'wide'})`, "RangeError:"},
		{`new Intl.DateTimeFormat('en', {dateStyle: 'full', year: 'numeric'})`, "TypeError:"},
		{`new Date(0).toLocaleDateString('en', {timeStyle: 'short'})`, "TypeError:"},
		{`new Intl.DateTimeFormat('en').format(new Date(NaN))`, "RangeError:"},
		{`new Intl.DateTimeFormat('123')`, "RangeError:"},
		{`new Intl.NumberFormat('en', {style: 'currency'})`, "TypeError:"},
		{`new Intl.NumberFormat('en', {style: 'currency', currency: 'US'})`, "RangeError:"},
		{`new Intl.NumberFormat('en', {minimumFractionDigits: 3, maximumFractionDigits: 1})`, "RangeError:"},
		{`Intl.DateTimeFormat.prototype.resolvedOptions.call({})`, "TypeError:"},
		// null is no list of locales (as Node 24 has it).
		{`new Intl.NumberFormat(null)`, "TypeError:"},
		{`new Intl.DateTimeFormat(null)`, "TypeError:"},
		{`new Intl.PluralRules(null)`, "TypeError:"},
		{`Intl.getCanonicalLocales(null)`, "TypeError:"},
		{`Intl.NumberFormat.supportedLocalesOf(null)`, "TypeError:"},
		{`(1).toLocaleString(null)`, "TypeError:"},
		{`new Date(0).toLocaleString(null)`, "TypeError:"},
		{`Intl.getCanonicalLocales(5).length`, "no error"},
	} {
		got := fmt.Sprint(eval(t, vm, catch(test.src)))
		if !strings.HasPrefix(got, test.want) {
			t.Errorf("%s\n\tgot  %q\n\twant a %s", test.src, got, test.want)
		}
	}
}

// TestDefaultTimeZone checks that a formatter with no timeZone option uses the
// zone the TimeZone option set, the one resolvedOptions() reports.
func TestDefaultTimeZone(t *testing.T) {
	vm := newVM(t, TimeZone("America/New_York"))
	evalTests(t, vm, [][2]string{
		{`new Intl.DateTimeFormat().resolvedOptions().timeZone`, "America/New_York"},
		{`new Date(0).toLocaleString('en-US')`, "12/31/1969, 7:00:00 PM"},
		{`new Date(0).toLocaleString('en-US', {timeZone: 'UTC'})`, "1/1/1970, 12:00:00 AM"},
	})
}

// TestUTCByDefault checks that without the TimeZone option the zone is UTC
// whatever the host's is: a server renders the same page wherever it runs.
func TestUTCByDefault(t *testing.T) {
	t.Setenv("TZ", "Asia/Tokyo")
	vm, err := quickjs.NewVM()
	if err != nil {
		t.Fatal(err)
	}

	defer vm.Close()

	if err := Register(vm); err != nil {
		t.Fatal(err)
	}

	evalTests(t, vm, [][2]string{
		{`new Intl.DateTimeFormat().resolvedOptions().timeZone`, "UTC"},
		{`new Date(0).toLocaleString('en-US')`, "1/1/1970, 12:00:00 AM"},
	})
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

// TestInstallOnUse checks that Register only puts stand-ins on the VM, and
// that the shim is installed the first time a page touches one of them.
func TestInstallOnUse(t *testing.T) {
	before := shimInstalls.Load()
	vm := newVM(t)
	evalTests(t, vm, [][2]string{
		{`typeof Object.getOwnPropertyDescriptor(globalThis, 'Intl').get`, "function"},
		{`Object.getOwnPropertyNames(globalThis).filter(n => n.startsWith('__quickjs_intl')).length`, "0"},
	})
	if n := shimInstalls.Load() - before; n != 0 {
		t.Fatalf("Register installed the shim %d times, want 0", n)
	}

	evalTests(t, vm, [][2]string{
		// A method taken before the shim is there still works.
		{`const f = Date.prototype.toLocaleDateString; f.call(new Date(0), 'en-US', {timeZone: 'UTC'})`, "1/1/1970"},
		{`'value' in Object.getOwnPropertyDescriptor(globalThis, 'Intl')`, "true"},
		{`Date.prototype.toLocaleDateString === f`, "false"},
		{`f.call(new Date(0), 'de', {timeZone: 'UTC'})`, "1.1.1970"},
		{`Object.getOwnPropertyNames(globalThis).filter(n => n.startsWith('__quickjs_intl')).length`, "0"},
	})
	if n := shimInstalls.Load() - before; n != 1 {
		t.Fatalf("the shim was installed %d times, want 1", n)
	}

	// A page that assigns its own Intl keeps it.
	vm2 := newVM(t)
	evalTests(t, vm2, [][2]string{
		{`Intl = 42; (1234.5).toLocaleString('en-US') + ' ' + Intl`, "1,234.5 42"},
	})
}

// TestConcurrentVMs formats on several VMs at once. The locale data, pattern
// generators and formatters they share are built by whichever gets there
// first, so the VMs start together and are checked against a lone one after.
// Run it with -race.
func TestConcurrentVMs(t *testing.T) {
	exprs := []string{
		`new Intl.DateTimeFormat('zh-CN', {dateStyle: 'full', timeStyle: 'long', timeZone: 'Asia/Shanghai'}).format(0)`,
		`new Intl.DateTimeFormat('ja', {hour: 'numeric', minute: 'numeric', hour12: true}).format(0)`,
		`new Intl.DateTimeFormat('fa', {dateStyle: 'medium', timeStyle: 'short'}).format(0)`,
		`new Intl.DateTimeFormat('de', {timeZoneName: 'long', timeZone: 'Europe/Berlin'}).format(0)`,
		`new Intl.NumberFormat('hi', {style: 'currency', currency: 'INR'}).format(1234567.891)`,
		`new Intl.NumberFormat('ar-EG', {notation: 'compact'}).format(1234567)`,
		`new Intl.PluralRules('ru').select(21)`,
		`new Date(0).toLocaleString('ko')`,
	}
	const workers = 4
	var results [workers][]string
	var wg sync.WaitGroup
	for w := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			vm, err := quickjs.NewVM()
			if err != nil {
				t.Error(err)
				return
			}

			defer vm.Close()

			if err := Register(vm, TimeZone("UTC")); err != nil {
				t.Error(err)
				return
			}

			for _, e := range exprs {
				v, err := vm.Eval(e, quickjs.EvalGlobal)
				if err != nil {
					t.Errorf("%s: %v", e, err)
				}
				results[w] = append(results[w], fmt.Sprint(v))
			}
		}()
	}
	wg.Wait()

	vm := newVM(t)
	for i, e := range exprs {
		want := fmt.Sprint(eval(t, vm, e))
		for w := range workers {
			if i < len(results[w]) && results[w][i] != want {
				t.Errorf("%s: VM %d got %q, want %q", e, w, results[w][i], want)
			}
		}
	}
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
