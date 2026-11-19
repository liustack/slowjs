// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

package intl // import "github.com/liustack/slowjs/intl"

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/liustack/slowjs"
)

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

// newVM is a VM with Intl, closed when the test ends. Without opts its
// default time zone is UTC.
func newVM(tb testing.TB, opts ...Option) *slowjs.VM {
	tb.Helper()
	vm, err := slowjs.NewVM()
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

// eval is what src gives on vm, evaluated as a global script.
func eval(tb testing.TB, vm *slowjs.VM, src string) any {
	tb.Helper()
	v, err := vm.Eval(src, slowjs.EvalGlobal)
	if err != nil {
		tb.Fatalf("%s: %v", src, err)
	}
	return v
}

// evalTests evaluates each source of tests on vm, in turn, and checks that
// it gives the text next to it.
func evalTests(t *testing.T, vm *slowjs.VM, tests [][2]string) {
	t.Helper()
	for _, tc := range tests {
		v, err := vm.Eval(tc[0], slowjs.EvalGlobal)
		if err != nil {
			t.Errorf("%s: %v", tc[0], err)
			continue
		}
		if got := fmt.Sprint(v); got != tc[1] {
			t.Errorf("%s\n got %q\nwant %q", tc[0], got, tc[1])
		}
	}
}

// benchmarkLoop times body, a statement that may use the loop counter i,
// after setup ran once. The loop is a function compiled once, called with
// the count of the benchmark.
func benchmarkLoop(b *testing.B, setup, body string) {
	vm := newVM(b)
	eval(b, vm, setup)
	eval(b, vm, "function run(count) {for (let i = 0; i < count; i++) {"+body+"}}")
	b.ResetTimer()
	eval(b, vm, "run("+strconv.Itoa(b.N)+")")
}

func TestIntlExists(t *testing.T) {
	evalTests(t, newVM(t), intlExistsTests)
}

// TestSvelteKitStart makes the formatter SvelteKit's client makes when it
// starts. Without Intl that throws a ReferenceError, which SvelteKit answers
// by loading the same page again, and again.
func TestSvelteKitStart(t *testing.T) {
	evalTests(t, newVM(t), svelteKitStartTests)
}

func TestDateTimeFormat(t *testing.T) {
	evalTests(t, newVM(t), dateTimeFormatTests)
}

func TestDateTimeFormatToParts(t *testing.T) {
	evalTests(t, newVM(t), dateTimeFormatToPartsTests)
}

// TestToLocaleStringOverrides checks that the toLocale* methods follow their
// locales and time zone, which QuickJS's own ignore.
func TestToLocaleStringOverrides(t *testing.T) {
	evalTests(t, newVM(t), toLocaleStringOverridesTests)
}

// TestStandInsKeepPageCode checks that what a page does to Intl or the
// toLocale* methods before it first uses them stays done, as in V8: each
// case on a VM of its own.
func TestStandInsKeepPageCode(t *testing.T) {
	for _, tc := range standInsKeepPageCodeTests {
		evalTests(t, newVM(t), [][2]string{tc})
	}
}

// TestLocaleListFunction: a function in a list of locales is an object, whose
// string is its source, not a tag.
func TestLocaleListFunction(t *testing.T) {
	evalTests(t, newVM(t), localeListFunctionTests)
}

func TestNumberFormat(t *testing.T) {
	evalTests(t, newVM(t), numberFormatTests)
}

func TestNumberFormatLocales(t *testing.T) {
	evalTests(t, newVM(t), numberFormatLocalesTests)
}

// TestChromiumFallback covers locales Chromium has no data of its own for.
func TestChromiumFallback(t *testing.T) {
	evalTests(t, newVM(t), chromiumFallbackTests)
}

// TestCurrenciesChromiumLeavesOut: a currency Chromium has no name for is
// written as its code.
func TestCurrenciesChromiumLeavesOut(t *testing.T) {
	evalTests(t, newVM(t), currenciesChromiumLeavesOutTests)
}

// koZoneName is the expression of the name of zone in Korean, in the style
// of timeZoneName, at the instant date.
func koZoneName(zone, date, style string) string {
	return "new Intl.DateTimeFormat('ko', {timeZone: '" + zone + "', timeZoneName: '" + style +
		"'}).formatToParts(new Date('" + date + "')).find(p => p.type === 'timeZoneName').value"
}

// TestBrowserOverrides covers values where Chromium differs from CLDR.
func TestBrowserOverrides(t *testing.T) {
	evalTests(t, newVM(t), browserOverridesTests)
}

// TestCompactWithoutDigits: a compact form that spells the number out holds
// for that number only, not for its negative.
func TestCompactWithoutDigits(t *testing.T) {
	evalTests(t, newVM(t), compactWithoutDigitsTests)
}

func TestOtherNumberingSystems(t *testing.T) {
	evalTests(t, newVM(t), otherNumberingSystemsTests)
}

func TestCurrencyPatterns(t *testing.T) {
	evalTests(t, newVM(t), currencyPatternsTests)
}

// TestCurrencyDigitsByNotation: only the standard notation takes the digits
// of the currency.
func TestCurrencyDigitsByNotation(t *testing.T) {
	evalTests(t, newVM(t), currencyDigitsByNotationTests)
}

// TestMaximumFractionDigitsAlone: a maximum below the currency's minimum
// lowers the minimum, unless a minimum is given too.
func TestMaximumFractionDigitsAlone(t *testing.T) {
	evalTests(t, newVM(t), maximumFractionDigitsAloneTests)
}

// TestFractionDigitsUnreadNextToSignificant: with significant digits given,
// the fraction digits are not read, so not checked.
func TestFractionDigitsUnreadNextToSignificant(t *testing.T) {
	evalTests(t, newVM(t), fractionDigitsUnreadNextToSignificantTests)
}

func TestPluralRules(t *testing.T) {
	evalTests(t, newVM(t), pluralRulesTests)
}

// TestICURules covers what ECMA-402 leaves to the implementation, as ICU
// does it in en-US.
func TestICURules(t *testing.T) {
	evalTests(t, newVM(t), icuRulesTests)
}

func TestStyleTimeGenerated(t *testing.T) {
	evalTests(t, newVM(t), styleTimeGeneratedTests)
}

// TestPatternTies: of two formats that match as well, the one added first
// wins.
func TestPatternTies(t *testing.T) {
	evalTests(t, newVM(t), patternTiesTests)
}

func TestDateTimeFormatLocales(t *testing.T) {
	evalTests(t, newVM(t), dateTimeFormatLocalesTests)
}

func TestResolvedOptions(t *testing.T) {
	evalTests(t, newVM(t), resolvedOptionsTests)
}

// TestSupportedLocalesOfOptions: supportedLocalesOf reads its options after
// the locales, as ECMA-402 says.
func TestSupportedLocalesOfOptions(t *testing.T) {
	vm := newVM(t)
	for _, c := range []string{"NumberFormat", "DateTimeFormat", "PluralRules"} {
		var tests [][2]string
		for _, tc := range supportedLocalesOfOptionsTests {
			tests = append(tests, [2]string{
				"(() => {try {return JSON.stringify(Intl." + c + ".supportedLocalesOf(['en', 'de'], " + tc[0] +
					"))} catch (e) {return e.constructor.name}})()",
				tc[1],
			})
		}
		tests = append(tests,
			// An invalid tag throws before the options are looked at.
			[2]string{
				"(() => {try {Intl." + c + ".supportedLocalesOf(['en-'], null)} catch (e) {return e.constructor.name}})()",
				"RangeError",
			},
			[2]string{
				`(() => {
					const seen = [];
					const locales = {get length() {seen.push('length'); return 1}, get 0() {seen.push('0'); return 'en'}};
					const options = {get localeMatcher() {seen.push('localeMatcher'); return 'lookup'}};
					try {Intl.` + c + `.supportedLocalesOf(locales, options)} catch (e) {seen.push(e.constructor.name)}
					return seen.join();
				})()`,
				"length,0,localeMatcher",
			})
		evalTests(t, vm, tests)
	}
}

// TestErrors checks the constructor of the error each case throws.
func TestErrors(t *testing.T) {
	vm := newVM(t)
	for _, tc := range errorsTests {
		src := "(() => {try {" + tc[0] + "; return 'no error'} catch (e) {return e.constructor.name + ': ' + e.message}})()"
		if got := fmt.Sprint(eval(t, vm, src)); !strings.HasPrefix(got, tc[1]) {
			t.Errorf("%s\n got %q\nwant %q...", tc[0], got, tc[1])
		}
	}
}

// TestFormatInUserCode checks that a format called from the getter or
// valueOf a format reads leaves what the outer format knows of its own
// value alone.
func TestFormatInUserCode(t *testing.T) {
	vm := newVM(t)
	eval(t, vm, `var df = new Intl.DurationFormat('en', {style: 'long'}), df2 = new Intl.DurationFormat('en'), nf = new Intl.NumberFormat('en')`)
	evalTests(t, vm, formatInUserCodeTests)
}

var formatInUserCodeTests = [][2]string{
	{"df.format({get days() {df.format('P1D'); return 1}})", "1 day"},
	{"df.formatToParts({get days() {df.format('P1D'); return 1}}).map(p => p.value).join('')", "1 day"},
	{"df.format({get days() {df2.formatToParts('PT1H'); return 2}, hours: 3})", "2 days, 3 hours"},
	{"df.format({get days() {df.format({hours: 1}); return 1}})", "1 day"},
	{"nf.format({valueOf() {nf.format(1n); return 1.5}})", "1.5"},
	{"nf.format({valueOf() {nf.format(1); return 12345678901234567891n}})", "12,345,678,901,234,567,891"},
	{"nf.formatToParts({valueOf() {nf.format(1); return 12345678901234567891n}}).map(p => p.value).join('')", "12,345,678,901,234,567,891"},
	{"nf.formatRange(12345678901234567891n, {valueOf() {nf.format(1); return 22345678901234567891n}})", "12,345,678,901,234,567,891–22,345,678,901,234,567,891"},
	{"nf.formatRange({valueOf() {nf.format(2n); return 1.5}}, 12345678901234567899n)", "1.5–12,345,678,901,234,567,899"},
}

func TestDefaultTimeZone(t *testing.T) {
	evalTests(t, newVM(t, TimeZone("America/New_York")), defaultTimeZoneTests)
}

// TestUTCByDefault: without TimeZone the zone is UTC, whatever TZ says, so
// a page renders the same wherever the server runs.
func TestUTCByDefault(t *testing.T) {
	t.Setenv("TZ", "Asia/Tokyo")
	vm, err := slowjs.NewVM()
	if err != nil {
		t.Fatal(err)
	}

	defer vm.Close()

	if err := Register(vm); err != nil {
		t.Fatal(err)
	}
	evalTests(t, vm, utcByDefaultTests)
}

// TestRegisterPerRealm: a VM that replaces the realm of another has no Intl
// until it is registered itself, and only once.
func TestRegisterPerRealm(t *testing.T) {
	vm := newVM(t)
	evalTests(t, vm, [][2]string{{"typeof Intl", "object"}})

	next, err := slowjs.NewVM()
	if err != nil {
		t.Fatal(err)
	}

	defer next.Close()

	evalTests(t, next, [][2]string{{"typeof Intl", "undefined"}})
	if err := Register(next, TimeZone("UTC")); err != nil {
		t.Fatal(err)
	}
	if err := Register(next, TimeZone("Asia/Tokyo")); err == nil || !strings.Contains(err.Error(), "already has Intl") {
		t.Fatalf("second Register: %v", err)
	}
	evalTests(t, next, registerPerRealmTests)
}

// TestInstallOnUse: Register only puts stand-ins in place, and the first use
// installs Intl, leaving no other name on the global object. A method taken
// before still works, and an Intl the page assigns stays.
func TestInstallOnUse(t *testing.T) {
	vm := newVM(t)
	before := shimInstalls.Load()
	evalTests(t, vm, [][2]string{
		{"typeof Object.getOwnPropertyDescriptor(globalThis, 'Intl').get", "function"},
		{hostFuncsSeen, "0"},
	})
	if n := shimInstalls.Load() - before; n != 0 {
		t.Fatalf("%d installs before the first use", n)
	}

	evalTests(t, vm, [][2]string{
		{"const f = Date.prototype.toLocaleDateString; f.call(new Date(0), 'en-US', {timeZone: 'UTC'})", "1/1/1970"},
		{"'value' in Object.getOwnPropertyDescriptor(globalThis, 'Intl')", "true"},
		{"Date.prototype.toLocaleDateString === f", "false"},
		{"f.call(new Date(0), 'de', {timeZone: 'UTC'})", "1.1.1970"},
		{hostFuncsSeen, "0"},
		// The count sees a name the page adds.
		{"globalThis.probe = 1; " + hostFuncsSeen, "1"},
		{"delete globalThis.probe; " + hostFuncsSeen, "0"},
	})
	if n := shimInstalls.Load() - before; n != 1 {
		t.Fatalf("%d installs, want 1", n)
	}

	evalTests(t, newVM(t), [][2]string{
		{"Intl = 42; (1234.5).toLocaleString('en-US') + ' ' + Intl", "1,234.5 42"},
	})
}

// concurrentExprs are what TestConcurrentVMs formats on each VM.
var concurrentExprs = []string{
	"new Intl.DateTimeFormat('zh-CN', {dateStyle: 'full', timeStyle: 'long', timeZone: 'Asia/Shanghai'}).format(0)",
	"new Intl.DateTimeFormat('ja', {hour: 'numeric', minute: 'numeric', hour12: true}).format(0)",
	"new Intl.DateTimeFormat('fa', {dateStyle: 'medium', timeStyle: 'short'}).format(0)",
	"new Intl.DateTimeFormat('de', {timeZoneName: 'long', timeZone: 'Europe/Berlin'}).format(0)",
	"new Intl.NumberFormat('hi', {style: 'currency', currency: 'INR'}).format(1234567.891)",
	"new Intl.NumberFormat('ar-EG', {notation: 'compact'}).format(1234567)",
	"new Intl.PluralRules('ru').select(21)",
	"new Date(0).toLocaleString('ko')",
}

// TestConcurrentVMs formats on VMs in several goroutines at once, which
// must answer as one VM alone does.
func TestConcurrentVMs(t *testing.T) {
	const goroutines = 4
	results := make([][]string, goroutines)
	errs := make([]error, goroutines)
	var wg sync.WaitGroup
	for g := range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			vm, err := slowjs.NewVM()
			if err != nil {
				errs[g] = err
				return
			}

			defer vm.Close()

			if err := Register(vm, TimeZone("UTC")); err != nil {
				errs[g] = err
				return
			}
			for _, src := range concurrentExprs {
				v, err := vm.Eval(src, slowjs.EvalGlobal)
				if err != nil {
					errs[g] = fmt.Errorf("%s: %w", src, err)
					return
				}
				results[g] = append(results[g], fmt.Sprint(v))
			}
		}()
	}
	wg.Wait()

	vm := newVM(t)
	for i, src := range concurrentExprs {
		want := fmt.Sprint(eval(t, vm, src))
		for g := range goroutines {
			if errs[g] != nil {
				t.Fatalf("goroutine %d: %v", g, errs[g])
			}
			if results[g][i] != want {
				t.Errorf("goroutine %d: %s\n got %q\nwant %q", g, src, results[g][i], want)
			}
		}
	}
}

// TestRegisterOnManyVMs: each VM has the default zone it was registered
// with.
func TestRegisterOnManyVMs(t *testing.T) {
	for _, tz := range []string{"UTC", "Europe/Prague", "Australia/Sydney"} {
		evalTests(t, newVM(t, TimeZone(tz)), [][2]string{
			{"new Intl.DateTimeFormat().resolvedOptions().timeZone", tz},
		})
	}
}

func BenchmarkFormatDate(b *testing.B) {
	benchmarkLoop(b, "const f = new Intl.DateTimeFormat('en-US', {dateStyle: 'medium', timeStyle: 'short'})", "f.format(i * 1e7)")
}

func BenchmarkFormatNumber(b *testing.B) {
	benchmarkLoop(b, "const n = new Intl.NumberFormat('en-US')", "n.format(i * 1.5)")
}

func BenchmarkToLocaleString(b *testing.B) {
	benchmarkLoop(b, "const d = new Date(0)", "d.toLocaleString('en-US')")
}

func BenchmarkNewDateTimeFormat(b *testing.B) {
	benchmarkLoop(b, "", "new Intl.DateTimeFormat('en-US', {dateStyle: 'medium'})")
}

func ExampleRegister() {
	vm, err := slowjs.NewVM()
	if err != nil {
		panic(err)
	}

	defer vm.Close()

	if err := Register(vm, TimeZone("Europe/Prague")); err != nil {
		panic(err)
	}

	for _, src := range []string{
		"new Intl.DateTimeFormat('en-US', {dateStyle: 'long', timeStyle: 'short', timeZone: 'America/New_York'}).format(new Date(0))",
		"new Date(0).toLocaleString('en-US', {timeZone: 'Asia/Tokyo'})",
		"Intl.DateTimeFormat().resolvedOptions().timeZone",
		"new Intl.NumberFormat('en-US', {style: 'currency', currency: 'USD'}).format(1234.5)",
	} {
		v, err := vm.Eval(src, slowjs.EvalGlobal)
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

// intlExistsTests are the cases of TestIntlExists.
var intlExistsTests = [][2]string{
	{"typeof Intl", "object"},
	{"typeof Intl.DateTimeFormat", "function"},
	{"typeof Intl.NumberFormat", "function"},
	{"typeof Intl.getCanonicalLocales", "function"},
	{"Object.prototype.toString.call(Intl)", "[object Intl]"},
	{"typeof new Intl.DateTimeFormat().format", "function"},
	{"typeof Intl.DateTimeFormat()", "object"},
	{"typeof Intl.NumberFormat()", "object"},
}

// svelteKitStartTests are the cases of TestSvelteKitStart.
var svelteKitStartTests = [][2]string{
	{"new Intl.DateTimeFormat(`en-US`, {hourCycle:`h23`, timeZone:`America/New_York`, year:`numeric`, month:`2-digit`, day:`2-digit`, hour:`2-digit`, minute:`2-digit`, second:`2-digit`}).format(new Date(0))", "12/31/1969, 19:00:00"},
}

// dateTimeFormatTests are the cases of TestDateTimeFormat.
var dateTimeFormatTests = [][2]string{
	{"new Intl.DateTimeFormat('en-US').format(new Date(0))", "1/1/1970"},
	{"new Intl.DateTimeFormat('en-US', {timeZone: 'America/New_York'}).format(new Date(0))", "12/31/1969"},
	{"new Intl.DateTimeFormat('en-US', {dateStyle: 'full'}).format(new Date(0))", "Thursday, January 1, 1970"},
	{"new Intl.DateTimeFormat('en-US', {dateStyle: 'long'}).format(new Date(0))", "January 1, 1970"},
	{"new Intl.DateTimeFormat('en-US', {dateStyle: 'medium'}).format(new Date(0))", "Jan 1, 1970"},
	{"new Intl.DateTimeFormat('en-US', {dateStyle: 'short'}).format(new Date(0))", "1/1/70"},
	{"new Intl.DateTimeFormat('en-US', {timeStyle: 'short'}).format(new Date(0))", "12:00 AM"},
	{"new Intl.DateTimeFormat('en-US', {timeStyle: 'medium'}).format(new Date(0))", "12:00:00 AM"},
	{"new Intl.DateTimeFormat('en-US', {timeStyle: 'long'}).format(new Date(0))", "12:00:00 AM UTC"},
	{"new Intl.DateTimeFormat('en-US', {timeStyle: 'full'}).format(new Date(0))", "12:00:00 AM Coordinated Universal Time"},
	{"new Intl.DateTimeFormat('en-US', {dateStyle: 'full', timeStyle: 'short'}).format(new Date(0))", "Thursday, January 1, 1970 at 12:00 AM"},
	{"new Intl.DateTimeFormat('en-US', {dateStyle: 'medium', timeStyle: 'short'}).format(new Date(0))", "Jan 1, 1970, 12:00 AM"},
	{"new Intl.DateTimeFormat('en-US', {weekday: 'short', month: 'numeric', day: 'numeric'}).format(new Date(0))", "Thu, 1/1"},
	{"new Intl.DateTimeFormat('en-US', {month: 'long', day: 'numeric', year: 'numeric'}).format(new Date(0))", "January 1, 1970"},
	{"new Intl.DateTimeFormat('en-US', {month: 'long'}).format(new Date(0))", "January"},
	{"new Intl.DateTimeFormat('en-US', {month: 'narrow'}).format(new Date(0))", "J"},
	{"new Intl.DateTimeFormat('en-US', {weekday: 'long'}).format(new Date(0))", "Thursday"},
	{"new Intl.DateTimeFormat('en-US', {year: 'numeric', month: 'numeric'}).format(new Date(0))", "1/1970"},
	{"new Intl.DateTimeFormat('en-US', {year: 'numeric', era: 'short'}).format(new Date(0))", "1970 AD"},
	{"new Intl.DateTimeFormat('en-US', {hour: 'numeric'}).format(new Date(0))", "12 AM"},
	{"new Intl.DateTimeFormat('en-US', {hour: 'numeric', minute: 'numeric'}).format(new Date(0))", "12:00 AM"},
	{"new Intl.DateTimeFormat('en-US', {hour: 'numeric', minute: 'numeric', hour12: false}).format(new Date(0))", "00:00"},
	{"new Intl.DateTimeFormat('en-US', {hour: '2-digit', minute: '2-digit', hourCycle: 'h23'}).format(new Date(0))", "00:00"},
	{"new Intl.DateTimeFormat('en-US', {hour: 'numeric', hourCycle: 'h24'}).format(new Date(0))", "24"},
	{"new Intl.DateTimeFormat('en-US', {timeZone: 'UTC', timeZoneName: 'short'}).format(new Date(0))", "1/1/1970, UTC"},
	{"new Intl.DateTimeFormat('en-US', {timeZone: 'America/New_York', timeZoneName: 'short'}).format(new Date(0))", "12/31/1969, EST"},
	{"new Intl.DateTimeFormat('en-US', {timeZone: 'America/New_York', timeZoneName: 'shortOffset'}).format(new Date(0))", "12/31/1969, GMT-5"},
	{"new Intl.DateTimeFormat('en-US', {timeZone: 'Asia/Kolkata', timeZoneName: 'longOffset'}).format(new Date(0))", "1/1/1970, GMT+05:30"},
	{"new Intl.DateTimeFormat('en-US', {timeZone: '+05:30'}).format(new Date(0))", "1/1/1970"},
	{"new Intl.DateTimeFormat('en-US', {hour: 'numeric', minute: 'numeric', second: 'numeric', fractionalSecondDigits: 3}).format(new Date(1500))", "12:00:01.500 AM"},
	{"new Intl.DateTimeFormat('en-US', {dayPeriod: 'short'}).format(new Date(0))", "in the morning"},
	{"new Intl.DateTimeFormat('en-US', {dayPeriod: 'short'}).format(new Date(3600 * 1000))", "in the morning"},
	{"new Intl.DateTimeFormat('en-US', {dayPeriod: 'short'}).format(new Date(9 * 3600 * 1000))", "in the morning"},
	{"new Intl.DateTimeFormat('en-US', {dayPeriod: 'short'}).format(new Date(12.5 * 3600 * 1000))", "noon"},
	{"new Intl.DateTimeFormat('en-US', {hour: 'numeric', minute: 'numeric', dayPeriod: 'short'}).format(new Date(12.5 * 3600 * 1000))", "12:30 in the afternoon"},
	{"new Intl.DateTimeFormat('en-US', {dayPeriod: 'short'}).format(new Date(20 * 3600 * 1000))", "in the evening"},
	{"new Intl.DateTimeFormat('en-US', {dayPeriod: 'short'}).format(new Date(22 * 3600 * 1000))", "at night"},
	{"new Intl.DateTimeFormat('en-US', {timeZone: 'UTC'}).format(new Date(Date.UTC(-1, 0, 1)))", "1/1/2"},
	{"[0, 86400000].map(new Intl.DateTimeFormat('en-US').format).join(' ')", "1/1/1970 1/2/1970"},
}

// dateTimeFormatToPartsTests are the cases of TestDateTimeFormatToParts.
var dateTimeFormatToPartsTests = [][2]string{
	{"JSON.stringify(new Intl.DateTimeFormat('en-US').formatToParts(new Date(0)))", "[{\"type\":\"month\",\"value\":\"1\"},{\"type\":\"literal\",\"value\":\"/\"},{\"type\":\"day\",\"value\":\"1\"},{\"type\":\"literal\",\"value\":\"/\"},{\"type\":\"year\",\"value\":\"1970\"}]"},
	{"new Intl.DateTimeFormat('en-US', {timeStyle: 'medium'}).formatToParts(new Date(0)).map(p => p.type).join(',')", "hour,literal,minute,literal,second,literal,dayPeriod"},
}

// toLocaleStringOverridesTests are the cases of TestToLocaleStringOverrides.
var toLocaleStringOverridesTests = [][2]string{
	{"new Date(0).toLocaleString('en-US', {timeZone: 'America/New_York'})", "12/31/1969, 7:00:00 PM"},
	{"new Date(0).toLocaleDateString('en-US', {timeZone: 'America/New_York'})", "12/31/1969"},
	{"new Date(0).toLocaleTimeString('en-US', {timeZone: 'America/New_York'})", "7:00:00 PM"},
	{"new Date(0).toLocaleString('en-US')", "1/1/1970, 12:00:00 AM"},
	{"new Date(0).toLocaleDateString('en-US')", "1/1/1970"},
	{"new Date(0).toLocaleTimeString('en-US')", "12:00:00 AM"},
	{"new Date(0).toLocaleDateString('en-US', {dateStyle: 'full'})", "Thursday, January 1, 1970"},
	{"new Date(0).toLocaleString()", "1/1/1970, 12:00:00 AM"},
	{"new Date(NaN).toLocaleString('en-US')", "Invalid Date"},
	{"[new Date(0)].toLocaleString('en-US')", "1/1/1970, 12:00:00 AM"},
	{"(1234567.891).toLocaleString('en-US')", "1,234,567.891"},
	{"(0.42).toLocaleString('en-US', {style: 'percent'})", "42%"},
	{"(1234.5).toLocaleString('en-US', {style: 'currency', currency: 'USD'})", "$1,234.50"},
}

// standInsKeepPageCodeTests are the cases of TestStandInsKeepPageCode.
var standInsKeepPageCodeTests = [][2]string{
	{"Date.prototype.toLocaleString = function () { return 'mine'; }; Intl; new Date(0).toLocaleString()", "mine"},
	{"Number.prototype.toLocaleString = function () { return 'n'; }; [new Date(0).toLocaleDateString('en-US'), (1).toLocaleString()].join()", "1/1/1970,n"},
	{"Object.freeze(Date.prototype); new Date(0).toLocaleString('en-US')", "1/1/1970, 12:00:00 AM"},
	{"Object.freeze(globalThis); [typeof Intl.NumberFormat, Intl === Intl, (1234.5).toLocaleString('de')].join()", "function,true,1.234,5"},
	{"Object.defineProperty(globalThis, 'Intl', {value: 42, writable: true, configurable: false}); (1234.5).toLocaleString('de') + ' ' + Intl", "1.234,5 42"},
	{"delete globalThis.Intl; (1234.5).toLocaleString('de') + ' ' + typeof Intl", "1.234,5 undefined"},
	{"Intl = 5; [Intl, (1234.5).toLocaleString('de')].join()", "5,1.234,5"},
	{"var f = Number.prototype.toLocaleString; Intl.NumberFormat; f.call(1234.5, 'de')", "1.234,5"},
}

// localeListFunctionTests are the cases of TestLocaleListFunction.
var localeListFunctionTests = [][2]string{
	{"try { new Intl.NumberFormat([function () {}]) } catch (e) { e.name }", "RangeError"},
	{"try { (1).toLocaleString([() => 'en']) } catch (e) { e.name }", "RangeError"},
	{"try { new Intl.NumberFormat([1]) } catch (e) { e.name }", "TypeError"},
	{"var f = function () {}; f.toString = () => 'de'; new Intl.NumberFormat([f]).resolvedOptions().locale", "de"},
}

// numberFormatTests are the cases of TestNumberFormat.
var numberFormatTests = [][2]string{
	{"new Intl.NumberFormat('en-US').format(1234567.891)", "1,234,567.891"},
	{"new Intl.NumberFormat('en-US').format(0)", "0"},
	{"new Intl.NumberFormat('en-US').format(-0)", "-0"},
	{"new Intl.NumberFormat('en-US').format(NaN)", "NaN"},
	{"new Intl.NumberFormat('en-US').format(Infinity)", "∞"},
	{"new Intl.NumberFormat('en-US').format(-Infinity)", "-∞"},
	{"new Intl.NumberFormat('en-US').format(1e21)", "1,000,000,000,000,000,000,000"},
	{"new Intl.NumberFormat('en-US').format(1000)", "1,000"},
	{"new Intl.NumberFormat('en-US', {useGrouping: false}).format(1234567.891)", "1234567.891"},
	{"new Intl.NumberFormat('en-US', {useGrouping: 'min2'}).format(1000)", "1000"},
	{"new Intl.NumberFormat('en-US', {useGrouping: 'min2'}).format(10000)", "10,000"},
	{"new Intl.NumberFormat('en-US', {minimumFractionDigits: 2}).format(0)", "0.00"},
	{"new Intl.NumberFormat('en-US', {maximumFractionDigits: 2}).format(1.005)", "1.01"},
	{"new Intl.NumberFormat('en-US', {maximumFractionDigits: 0}).format(2.5)", "3"},
	{"new Intl.NumberFormat('en-US', {maximumFractionDigits: 0}).format(-2.5)", "-3"},
	{"new Intl.NumberFormat('en-US', {minimumIntegerDigits: 2}).format(7)", "07"},
	{"new Intl.NumberFormat('en-US', {maximumSignificantDigits: 2}).format(123456)", "120,000"},
	{"new Intl.NumberFormat('en-US', {minimumSignificantDigits: 3}).format(1.5)", "1.50"},
	{"new Intl.NumberFormat('en-US', {style: 'percent'}).format(0.4256)", "43%"},
	{"new Intl.NumberFormat('en-US', {style: 'percent', maximumFractionDigits: 2}).format(0.4256)", "42.56%"},
	{"new Intl.NumberFormat('en-US', {style: 'currency', currency: 'USD'}).format(-1234.5)", "-$1,234.50"},
	{"new Intl.NumberFormat('en-US', {style: 'currency', currency: 'USD', currencySign: 'accounting'}).format(-1234.5)", "($1,234.50)"},
	{"new Intl.NumberFormat('en-US', {style: 'currency', currency: 'JPY'}).format(1234.5)", "¥1,235"},
	{"new Intl.NumberFormat('en-US', {style: 'currency', currency: 'EUR'}).format(1)", "€1.00"},
	{"new Intl.NumberFormat('en-US', {style: 'currency', currency: 'CAD'}).format(1)", "CA$1.00"},
	{"new Intl.NumberFormat('en-US', {style: 'currency', currency: 'CAD', currencyDisplay: 'narrowSymbol'}).format(1)", "$1.00"},
	{"new Intl.NumberFormat('en-US', {style: 'unit', unit: 'liter'}).format(16)", "16 L"},
	{"new Intl.NumberFormat('en-US', {style: 'unit', unit: 'kilometer-per-hour'}).format(50)", "50 km/h"},
	{"new Intl.NumberFormat('en-US', {style: 'unit', unit: 'day', unitDisplay: 'long'}).format(1)", "1 day"},
	{"new Intl.NumberFormat('en-US', {style: 'unit', unit: 'day', unitDisplay: 'long'}).format(3)", "3 days"},
	{"new Intl.NumberFormat('en-US', {notation: 'compact'}).format(1000)", "1K"},
	{"new Intl.NumberFormat('en-US', {notation: 'compact'}).format(1234)", "1.2K"},
	{"new Intl.NumberFormat('en-US', {notation: 'compact'}).format(12345)", "12K"},
	{"new Intl.NumberFormat('en-US', {notation: 'compact'}).format(123456)", "123K"},
	{"new Intl.NumberFormat('en-US', {notation: 'compact'}).format(999999)", "1M"},
	{"new Intl.NumberFormat('en-US', {notation: 'compact', compactDisplay: 'long'}).format(1000000)", "1 million"},
	{"new Intl.NumberFormat('en-US', {notation: 'scientific'}).format(12345)", "1.235E4"},
	{"new Intl.NumberFormat('en-US', {notation: 'scientific'}).format(0.000001234)", "1.234E-6"},
	{"new Intl.NumberFormat('en-US', {notation: 'engineering'}).format(12345)", "12.345E3"},
	{"new Intl.NumberFormat('en-US', {signDisplay: 'always'}).format(42)", "+42"},
	{"new Intl.NumberFormat('en-US', {signDisplay: 'never'}).format(-42)", "42"},
	{"new Intl.NumberFormat('en-US', {signDisplay: 'exceptZero'}).format(0)", "0"},
	{"new Intl.NumberFormat('en-US').format(123456789012345678901234567890n)", "123,456,789,012,345,678,901,234,567,890"},
	{"[1000, 2000].map(new Intl.NumberFormat('en-US').format).join(' ')", "1,000 2,000"},
	{"JSON.stringify(new Intl.NumberFormat('en-US').formatToParts(-1234.5))", "[{\"type\":\"minusSign\",\"value\":\"-\"},{\"type\":\"integer\",\"value\":\"1\"},{\"type\":\"group\",\"value\":\",\"},{\"type\":\"integer\",\"value\":\"234\"},{\"type\":\"decimal\",\"value\":\".\"},{\"type\":\"fraction\",\"value\":\"5\"}]"},
}

// numberFormatLocalesTests are the cases of TestNumberFormatLocales.
var numberFormatLocalesTests = [][2]string{
	{"new Intl.NumberFormat('zh-CN', {style: 'currency', currency: 'CNY'}).format(1234567.891)", "¥1,234,567.89"},
	{"new Intl.NumberFormat('zh-CN', {notation: 'compact'}).format(123456789)", "1.2亿"},
	{"new Intl.NumberFormat('ja', {notation: 'compact'}).format(12345)", "1.2万"},
	{"new Intl.NumberFormat('de-DE', {style: 'currency', currency: 'EUR'}).format(-1234.5)", "-1.234,50\u00a0€"},
	{"new Intl.NumberFormat('de', {notation: 'compact', compactDisplay: 'long'}).format(1234567)", "1,2 Millionen"},
	{"new Intl.NumberFormat('fr', {style: 'unit', unit: 'kilometer-per-hour'}).format(1.5)", "1,5\u202fkm/h"},
	{"new Intl.NumberFormat('es').format(1234)", "1234"},
	{"new Intl.NumberFormat('es').format(12345)", "12.345"},
	{"new Intl.NumberFormat('hi').format(1234567.891)", "12,34,567.891"},
	{"new Intl.NumberFormat('ar-EG').format(-1234.5)", "\u061c-١٬٢٣٤٫٥"},
	{"new Intl.NumberFormat('en-US', {style: 'currency', currency: 'EUR', currencyDisplay: 'name'}).format(1)", "1.00 euros"},
	{"new Intl.NumberFormat('fr', {style: 'currency', currency: 'USD', currencySign: 'accounting'}).format(-1)", "(1,00\u00a0$US)"},
	{"new Intl.NumberFormat('ar', {style: 'unit', unit: 'day', unitDisplay: 'long'}).format(2)", "يومان"},
	{"JSON.stringify(new Intl.NumberFormat('ar', {style: 'percent'}).formatToParts(-1))",
		"[{\"type\":\"literal\",\"value\":\"\u200e\"},{\"type\":\"minusSign\",\"value\":\"-\"},{\"type\":\"integer\",\"value\":\"100\"},{\"type\":\"literal\",\"value\":\"\u200e\"},{\"type\":\"percentSign\",\"value\":\"%\"},{\"type\":\"literal\",\"value\":\"\u200e\"}]"},
	{"(123456789n).toLocaleString('de')", "123.456.789"},
}

// chromiumFallbackTests are the cases of TestChromiumFallback.
var chromiumFallbackTests = [][2]string{
	{"new Intl.DateTimeFormat('az', {dateStyle: 'full', timeZone: 'UTC'}).format(Date.UTC(2026, 0, 5))", "2026 M01 5, Mon"},
	{"new Intl.NumberFormat('az').format(1234567.891)", "1,234,567.891"},
	{"new Intl.NumberFormat('kk').format(1234567.891)", "1,234,567.891"},
	{"new Intl.NumberFormat('ak', {style: 'currency', currency: 'GHS', currencyDisplay: 'name'}).format(2)", "2.00 Ghanaian cedis"},
	{"new Intl.NumberFormat('bs-Cyrl', {style: 'currency', currency: 'BAM'}).format(2)", "2,00\u00a0BAM"},
	{"new Intl.NumberFormat('oc-ES', {style: 'currency', currency: 'USD', currencyDisplay: 'name'}).format(2)", "2.00 US dollars"},
	{"new Intl.NumberFormat('fa-AF', {style: 'currency', currency: 'USD', currencyDisplay: 'name'}).format(2)", "۲٫۰۰ دالر امریکا"},
	{"new Intl.DateTimeFormat('bs-Cyrl', {dateStyle: 'full', timeZone: 'UTC'}).format(Date.UTC(2026, 0, 5))", "понедјељак, 05. јануар 2026."},
	{"new Intl.DateTimeFormat('ckb-IR', {weekday: 'long', calendar: 'gregory', timeZone: 'UTC'}).format(0)", "Thu"},
	{"new Intl.DateTimeFormat('ckb-IR', {dateStyle: 'medium', timeZone: 'UTC'}).format(0)", "AP 1348 بەفرانبار 11"},
	{"new Intl.DateTimeFormat('ckb-IR', {timeZone: 'UTC'}).format(Date.UTC(2026, 0, 5))", "AP 1404-10-15"},
	{"new Intl.DateTimeFormat('ckb-IR', {weekday: 'short', timeZone: 'UTC'}).format(Date.UTC(2026, 0, 5))", "Mon"},
	{"new Intl.DateTimeFormat('ckb-IR', {month: 'long', year: 'numeric', timeZone: 'UTC'}).format(Date.UTC(2026, 0, 5))", "AP 1404 بەفرانبار"},
	{"new Intl.DateTimeFormat('ckb-IR').resolvedOptions().month", "2-digit"},
	{"new Intl.DateTimeFormat('om-KE', {weekday: 'short', timeZone: 'UTC'}).format(Date.UTC(2026, 6, 5))", "Sun"},
	{"new Intl.DateTimeFormat('om-KE', {weekday: 'long', timeZone: 'UTC'}).format(Date.UTC(2026, 6, 5))", "Sun"},
	{"new Intl.NumberFormat('kk-Arab', {style: 'unit', unit: 'kilometer', unitDisplay: 'long'}).format(2)", "2 km"},
	{"new Intl.NumberFormat('kk-Arab', {style: 'unit', unit: 'meter-per-second', unitDisplay: 'long'}).format(2)", "2 m/s"},
	{"new Intl.NumberFormat('ar', {style: 'currency', currency: 'EGP', currencySign: 'accounting'}).format(-2)", "(\u061c2.00\u00a0ج.م.\u200f)"},
	{"new Intl.DateTimeFormat('th-u-ca-buddhist', {dateStyle: 'long', timeZone: 'UTC'}).format(0)", "1 มกราคม 2513"},
}

// currenciesChromiumLeavesOutTests are the cases of TestCurrenciesChromiumLeavesOut.
var currenciesChromiumLeavesOutTests = [][2]string{
	{"new Intl.NumberFormat('de', {style: 'currency', currency: 'DEM'}).format(1)", "1,00\u00a0DEM"},
	{"new Intl.NumberFormat('de', {style: 'currency', currency: 'DEM', currencyDisplay: 'narrowSymbol'}).format(1)", "1,00\u00a0DEM"},
	{"new Intl.NumberFormat('de', {style: 'currency', currency: 'DEM', currencyDisplay: 'name'}).format(1)", "1,00 DEM"},
	{"new Intl.NumberFormat('de', {style: 'currency', currency: 'XXX', currencyDisplay: 'name'}).format(1)", "1,00 XXX"},
	{"new Intl.NumberFormat('en', {style: 'currency', currency: 'VEF', currencyDisplay: 'name'}).format(1)", "1.00 VEF"},
	{"new Intl.NumberFormat('en', {style: 'currency', currency: 'XAU', currencyDisplay: 'name'}).format(1)", "1.00 XAU"},
	{"new Intl.NumberFormat('ca', {style: 'currency', currency: 'ESP'}).format(-1234567.891)", "-1.234.568\u00a0ESP"},
	{"new Intl.NumberFormat('el', {style: 'currency', currency: 'GRD'}).format(-1234567.891)", "-1.234.567,89\u00a0GRD"},
	{"new Intl.NumberFormat('pt-PT', {style: 'currency', currency: 'PTE'}).format(-1234567.891)", "-1\u00a0234\u00a0567,89\u00a0PTE"},
	{"new Intl.NumberFormat('en', {style: 'currency', currency: 'HRK', currencyDisplay: 'name'}).format(1)", "1.00 Croatian kunas"},
	{"new Intl.NumberFormat('hr', {style: 'currency', currency: 'HRK'}).format(1)", "1,00\u00a0kn"},
}

// browserOverridesTests are the cases of TestBrowserOverrides.
var browserOverridesTests = [][2]string{
	{"new Intl.NumberFormat('fr-CH').format(1234567.891)", "1\u202f234\u202f567,891"},
	{"new Intl.NumberFormat('fr-CH', {style: 'currency', currency: 'CHF'}).format(-1234567.891)", "-1\u202f234\u202f567.89\u00a0CHF"},
	{"new Intl.NumberFormat('uk', {style: 'currency', currency: 'UAH'}).format(-1234.5)", "-1\u00a0234,50\u00a0грн"},
	{"new Intl.NumberFormat('uk', {style: 'currency', currency: 'UAH', currencyDisplay: 'narrowSymbol'}).format(-1234.5)", "-1\u00a0234,50\u00a0₴"},
	{"new Intl.NumberFormat('ru', {style: 'currency', currency: 'UAH'}).format(-1234.5)", "-1\u00a0234,50\u00a0грн."},
	{"new Intl.NumberFormat('ru-UA', {style: 'currency', currency: 'UAH'}).format(-1234.5)", "-1234,50\u00a0грн."},
	{koZoneName("America/New_York", "2026-01-15T12:00Z", "long"), "북미 동부 표준시"},
	{koZoneName("America/Chicago", "2026-07-15T12:00Z", "long"), "북미 중부 하계 표준시"},
	{koZoneName("America/Denver", "2026-01-15T12:00Z", "long"), "북미 로키 산맥 표준시"},
	{koZoneName("America/Denver", "2026-07-15T12:00Z", "long"), "북미 로키 산맥 하계 표준시"},
	{koZoneName("America/Phoenix", "2026-07-15T12:00Z", "longGeneric"), "북미 로키 산맥 표준시"},
	{koZoneName("America/Los_Angeles", "2026-07-15T12:00Z", "longGeneric"), "북미 태평양 시간"},
}

// compactWithoutDigitsTests are the cases of TestCompactWithoutDigits.
var compactWithoutDigitsTests = [][2]string{
	{"new Intl.NumberFormat('fr', {notation: 'compact', compactDisplay: 'long'}).format(1000)", "mille"},
	{"new Intl.NumberFormat('fr', {notation: 'compact', compactDisplay: 'long'}).format(999.9)", "mille"},
	{"new Intl.NumberFormat('fr', {notation: 'compact', compactDisplay: 'long', signDisplay: 'always'}).format(1000)", "+mille"},
	{"JSON.stringify(new Intl.NumberFormat('fr', {notation: 'compact', compactDisplay: 'long'}).formatToParts(1000))", "[{\"type\":\"compact\",\"value\":\"mille\"}]"},
	{"new Intl.NumberFormat('it', {notation: 'compact', compactDisplay: 'long'}).format(1000)", "mille"},
	{"new Intl.NumberFormat('fr', {notation: 'compact', compactDisplay: 'long'}).format(-1000)", "-1 millier"},
	{"new Intl.NumberFormat('fr', {notation: 'compact', compactDisplay: 'long'}).format(2000)", "2 mille"},
	{"new Intl.NumberFormat('fr', {notation: 'compact', compactDisplay: 'long'}).format(1000000)", "1 million"},
}

// otherNumberingSystemsTests are the cases of TestOtherNumberingSystems.
var otherNumberingSystemsTests = [][2]string{
	{"new Intl.NumberFormat('de-u-nu-arab').format(-1234.5)", "\u061c-١٬٢٣٤٫٥"},
	{"new Intl.NumberFormat('fr', {numberingSystem: 'arab'}).format(-1234.5)", "\u061c-١٬٢٣٤٫٥"},
	{"new Intl.NumberFormat('en', {numberingSystem: 'arab', style: 'percent'}).format(0.5)", "٥٠٪\u061c"},
	{"new Intl.NumberFormat('en', {numberingSystem: 'arab', notation: 'scientific'}).format(12345)", "١٫٢٣٥اس٤"},
	{"new Intl.NumberFormat('en', {numberingSystem: 'arab', signDisplay: 'always'}).format(5)", "\u061c+٥"},
	{"new Intl.NumberFormat('en', {numberingSystem: 'arab', style: 'currency', currency: 'USD'}).format(-1234.5)", "\u061c-١٬٢٣٤٫٥٠\u00a0$"},
	{"new Intl.NumberFormat('en', {numberingSystem: 'arabext'}).format(-1234.5)", "\u200e-\u200e۱٬۲۳۴٫۵"},
	{"new Intl.NumberFormat('en', {numberingSystem: 'arabext', style: 'percent'}).format(0.5)", "۵۰٪"},
	{"new Intl.NumberFormat('en', {numberingSystem: 'beng'}).format(-1234.5)", "-১,২৩৪.৫"},
	{"new Intl.NumberFormat('sv', {numberingSystem: 'arab'}).format(-1234.5)", "\u061c−١\u00a0٢٣٤٫٥"},
	{"new Intl.NumberFormat('fa', {numberingSystem: 'arab'}).format(NaN)", "ناعدد"},
}

// currencyPatternsTests are the cases of TestCurrencyPatterns.
var currencyPatternsTests = [][2]string{
	{"new Intl.NumberFormat('en-DE', {style: 'currency', currency: 'EUR'}).format(-1234567.891)", "-€1,234,567.89"},
	{"new Intl.NumberFormat('en-DE', {style: 'currency', currency: 'USD'}).format(-1234567.891)", "-US$1,234,567.89"},
	{"new Intl.NumberFormat('en-DE', {style: 'currency', currency: 'USD', currencyDisplay: 'name'}).format(2)", "US$2.00 US dollars"},
	{"new Intl.NumberFormat('en-DE', {style: 'currency', currency: 'EUR', currencyDisplay: 'code'}).format(-1234.5)", "-EUR\u00a01,234.50"},
	{"new Intl.NumberFormat('en-DE', {style: 'currency', currency: 'EUR', currencySign: 'accounting'}).format(-1234.5)", "-€1,234.50"},
	{"new Intl.NumberFormat('en-DE', {style: 'currency', currency: 'EUR', notation: 'compact'}).format(500)", "€500"},
	{"new Intl.NumberFormat('en-DE', {style: 'currency', currency: 'EUR', notation: 'compact'}).format(5000)", "€5K"},
	{"new Intl.NumberFormat('en-DE', {style: 'currency', currency: 'EUR', notation: 'scientific'}).format(5432.1)", "€5.432E3"},
	{"new Intl.NumberFormat('en-DE', {style: 'currency', currency: 'USD', currencySign: 'accounting'}).format(-2)", "-US$2.00"},
	{"new Intl.NumberFormat('en-DE', {style: 'currency', currency: 'EUR'}).formatToParts(-1234.5).map(p => p.type + ':' + p.value).join(' ')", "minusSign:- currency:€ integer:1 group:, integer:234 decimal:. fraction:50"},
	{"new Intl.NumberFormat('en-DE').format(1234.5)", "1.234,5"},
	{"new Intl.NumberFormat('en-DE', {style: 'percent'}).format(12.5)", "1.250\u00a0%"},
	{"new Intl.NumberFormat('en-150', {style: 'currency', currency: 'EUR'}).format(-1234567.891)", "-€1,234,567.89"},
	{"new Intl.NumberFormat('en-150', {style: 'currency', currency: 'USD'}).format(-1234.5)", "-1,234.50\u00a0US$"},
	{"new Intl.NumberFormat('en-150', {style: 'currency', currency: 'USD', currencyDisplay: 'name'}).format(2)", "2.00 US dollars"},
	{"new Intl.NumberFormat('en-CH', {style: 'currency', currency: 'EUR'}).format(-1234567.891)", "-€1,234,567.89"},
	{"new Intl.NumberFormat('en-CH', {style: 'currency', currency: 'CHF'}).format(-1234.5)", "CHF-1'234.50"},
	{"new Intl.NumberFormat('en-AT', {style: 'currency', currency: 'USD'}).format(-1234.5)", "-US$\u00a01.234,50"},
	{"new Intl.NumberFormat('en-AT', {style: 'currency', currency: 'USD', currencyDisplay: 'name'}).format(2)", "US$\u00a02,00 US dollars"},
	{"new Intl.NumberFormat('tr', {style: 'currency', currency: 'TRY', currencySign: 'accounting'}).format(-1234.5)", "-₺1.234,50"},
	{"new Intl.NumberFormat('tr', {style: 'currency', currency: 'TRY', currencyDisplay: 'name'}).format(-1234.5)", "-₺1.234,50 Türk lirası"},
	{"new Intl.NumberFormat('tr', {style: 'currency', currency: 'USD', currencySign: 'accounting'}).format(-1234.5)", "($1.234,50)"},
	{"new Intl.NumberFormat('tr-TR', {style: 'currency', currency: 'USD', currencyDisplay: 'name'}).format(2)", "$2,00 ABD doları"},
	{"new Intl.NumberFormat('tr-TR', {style: 'currency', currency: 'USD', currencySign: 'accounting'}).format(-1234.5)", "-$1.234,50"},
	{"new Intl.NumberFormat('tr', {style: 'currency', currency: 'USD', currencyDisplay: 'name'}).format(2)", "2,00 ABD doları"},
	{"new Intl.NumberFormat('tr-CY', {style: 'currency', currency: 'USD', currencyDisplay: 'name'}).format(2)", "2,00 ABD doları"},
	{"new Intl.NumberFormat('tr-CY', {style: 'currency', currency: 'TRY', currencySign: 'accounting'}).format(-1234.5)", "-₺1.234,50"},
	{"new Intl.NumberFormat('pt-CV', {style: 'currency', currency: 'CVE'}).format(-1234567.891)", "-1\u00a0234\u00a0567$89\u00a0\u200b"},
	{"new Intl.NumberFormat('pt-CV', {style: 'currency', currency: 'EUR', currencySign: 'accounting'}).format(-1234567.891)", "-1\u00a0234\u00a0567$89\u00a0€"},
	{"new Intl.NumberFormat('pt-CV', {style: 'currency', currency: 'USD', currencyDisplay: 'name'}).format(2)", "2$00\u00a0US$ dólares dos Estados Unidos"},
	{"new Intl.NumberFormat('pt-CV').format(-1234567.891)", "-1\u00a0234\u00a0567,891"},
	{"new Intl.NumberFormat('pt', {style: 'currency', currency: 'CVE'}).format(-1234567.891)", "-CVE\u00a01.234.567,89"},
}

// currencyDigitsByNotationTests are the cases of TestCurrencyDigitsByNotation.
var currencyDigitsByNotationTests = [][2]string{
	{"new Intl.NumberFormat('en', {style: 'currency', currency: 'EUR', notation: 'scientific'}).format(5000)", "€5E3"},
	{"new Intl.NumberFormat('en', {style: 'currency', currency: 'EUR', notation: 'scientific'}).format(5432.1)", "€5.432E3"},
	{"new Intl.NumberFormat('en', {style: 'currency', currency: 'JPY', notation: 'scientific'}).format(5432.1)", "¥5.432E3"},
	{"new Intl.NumberFormat('de', {style: 'currency', currency: 'EUR', notation: 'engineering'}).format(54321)", "54,321E3\u00a0€"},
	{"new Intl.NumberFormat('en', {style: 'currency', currency: 'EUR', notation: 'compact', minimumFractionDigits: 1}).format(1234)", "€1.234K"},
	{"new Intl.NumberFormat('en', {style: 'currency', currency: 'EUR', notation: 'compact', maximumFractionDigits: 1}).format(1234)", "€1.2K"},
	{"new Intl.NumberFormat('en', {style: 'currency', currency: 'EUR', notation: 'scientific'}).resolvedOptions().minimumFractionDigits", "0"},
	{"new Intl.NumberFormat('en', {style: 'currency', currency: 'EUR', notation: 'scientific'}).resolvedOptions().maximumFractionDigits", "3"},
	{"new Intl.NumberFormat('en', {style: 'currency', currency: 'EUR'}).resolvedOptions().minimumFractionDigits", "2"},
}

// maximumFractionDigitsAloneTests are the cases of TestMaximumFractionDigitsAlone.
var maximumFractionDigitsAloneTests = [][2]string{
	{"new Intl.NumberFormat('zh-CN', {style: 'currency', currency: 'CNY', maximumFractionDigits: 0}).format(428.5)", "¥429"},
	{"new Intl.NumberFormat('en-US', {style: 'currency', currency: 'USD', maximumFractionDigits: 1}).format(1.25)", "$1.3"},
	{"new Intl.NumberFormat('en-US', {style: 'currency', currency: 'USD', maximumFractionDigits: 1}).resolvedOptions().minimumFractionDigits", "1"},
	{"new Intl.NumberFormat('en-US', {style: 'currency', currency: 'JPY', maximumFractionDigits: 0}).format(5.5)", "¥6"},
	{"(() => { try { new Intl.NumberFormat('en-US', {style: 'currency', currency: 'USD', minimumFractionDigits: 3, maximumFractionDigits: 1}) } catch (e) { return e.constructor.name } })()", "RangeError"},
}

// fractionDigitsUnreadNextToSignificantTests are the cases of TestFractionDigitsUnreadNextToSignificant.
var fractionDigitsUnreadNextToSignificantTests = [][2]string{
	{"new Intl.NumberFormat('en', {maximumSignificantDigits: 3, maximumFractionDigits: 200}).format(1.2345)", "1.23"},
	{"new Intl.NumberFormat('en', {notation: 'compact', minimumSignificantDigits: 2, minimumFractionDigits: -1}).format(1234)", "1.234K"},
	{"new Intl.PluralRules('en', {maximumSignificantDigits: 3, maximumFractionDigits: 200}).select(1)", "one"},
	{"(() => { try { new Intl.NumberFormat('en', {maximumFractionDigits: 200}) } catch (e) { return e.constructor.name } })()", "RangeError"},
}

// pluralRulesTests are the cases of TestPluralRules.
var pluralRulesTests = [][2]string{
	{"new Intl.PluralRules('en').select(1)", "one"},
	{"new Intl.PluralRules('en').select(-1)", "one"},
	{"new Intl.PluralRules('en').select(1.5)", "other"},
	{"new Intl.PluralRules('en', {minimumFractionDigits: 1}).select(1)", "other"},
	{"new Intl.PluralRules('en', {type: 'ordinal'}).select(22)", "two"},
	{"new Intl.PluralRules('ar').select(11)", "many"},
	{"new Intl.PluralRules('ru').select(21)", "one"},
	{"new Intl.PluralRules('fr').select(1000000)", "many"},
	{"new Intl.PluralRules('fr', {notation: 'compact'}).select(1000)", "other"},
	{"new Intl.PluralRules('fr', {notation: 'compact'}).select(1e6)", "many"},
	{"new Intl.PluralRules('fr', {notation: 'compact'}).select(1.5e6)", "many"},
	{"new Intl.PluralRules('fr', {notation: 'compact'}).select(1234567)", "many"},
	{"new Intl.PluralRules('fr', {notation: 'compact'}).select(1.5)", "one"},
	{"new Intl.PluralRules('fr', {notation: 'compact', compactDisplay: 'long'}).select(1e6)", "many"},
	{"new Intl.PluralRules('fr', {notation: 'scientific'}).select(1e5)", "other"},
	{"new Intl.PluralRules('fr', {notation: 'scientific'}).select(0.5)", "many"},
	{"new Intl.PluralRules('fr', {notation: 'engineering'}).select(0.00015)", "many"},
	{"new Intl.PluralRules('fr', {notation: 'scientific', minimumFractionDigits: 2}).select(1.5e6)", "many"},
	{"new Intl.PluralRules('ru', {notation: 'scientific'}).select(1e3)", "many"},
	{"new Intl.PluralRules('ru', {notation: 'compact'}).select(12345)", "many"},
	{"new Intl.PluralRules('ru', {notation: 'compact'}).select(0)", "many"},
	{"new Intl.PluralRules('pl', {notation: 'engineering'}).select(-1e6)", "many"},
	{"new Intl.PluralRules('sl', {notation: 'scientific'}).select(0.5)", "few"},
	{"new Intl.PluralRules('sl', {notation: 'scientific'}).select(1e21)", "other"},
	{"new Intl.PluralRules('pt', {notation: 'engineering'}).select(1.5)", "one"},
	{"new Intl.PluralRules('es', {notation: 'compact'}).select(0)", "other"},
	{"new Intl.PluralRules('en', {notation: 'compact'}).select(1e6)", "other"},
	{"new Intl.PluralRules('fr').select(1000)", "other"},
	{"new Intl.PluralRules('zh-CN').resolvedOptions().locale", "zh"},
	{"new Intl.PluralRules('pl').resolvedOptions().pluralCategories.join(' ')", "one few many other"},
	{"JSON.stringify(new Intl.PluralRules('en').resolvedOptions())", "{\"locale\":\"en\",\"type\":\"cardinal\",\"notation\":\"standard\",\"minimumIntegerDigits\":1,\"minimumFractionDigits\":0,\"maximumFractionDigits\":3,\"pluralCategories\":[\"one\",\"other\"],\"roundingIncrement\":1,\"roundingMode\":\"halfExpand\",\"roundingPriority\":\"auto\",\"trailingZeroDisplay\":\"auto\"}"},
	{"new Intl.PluralRules('ru').selectRange(1, 5)", "many"},
	{"new Intl.PluralRules('en').selectRange(0, 1)", "other"},
	{"new Intl.PluralRules('en').select(NaN)", "other"},
	{"Intl.PluralRules.supportedLocalesOf(['cy', 'xx']).join(' ')", "cy"},
	{"(() => { try { Intl.PluralRules('en') } catch (e) { return e.constructor.name } })()", "TypeError"},
	{"(() => { try { new Intl.PluralRules('en').selectRange(1) } catch (e) { return e.constructor.name } })()", "TypeError"},
	{"(() => { try { new Intl.PluralRules('en').selectRange(NaN, 1) } catch (e) { return e.constructor.name } })()", "RangeError"},
	{"(() => { try { new Intl.PluralRules('en', {type: 'x'}) } catch (e) { return e.constructor.name } })()", "RangeError"},
	{"Object.prototype.toString.call(new Intl.PluralRules())", "[object Intl.PluralRules]"},
}

// icuRulesTests are the cases of TestICURules.
var icuRulesTests = [][2]string{
	{"new Intl.DateTimeFormat('en-US', {minute: 'numeric', second: 'numeric'}).format(new Date(1500))", "00:01"},
	{"new Intl.DateTimeFormat('en-US', {minute: 'numeric'}).format(new Date(1500))", "0"},
	{"new Intl.DateTimeFormat('en-US', {hour: 'numeric', minute: 'numeric'}).resolvedOptions().minute", "2-digit"},
	{"new Intl.DateTimeFormat('en-US', {minute: 'numeric'}).resolvedOptions().minute", "numeric"},
	{"new Intl.DateTimeFormat('en-US', {hour: 'numeric', hourCycle: 'h23'}).resolvedOptions().hour", "2-digit"},
	{"new Intl.DateTimeFormat('en-US', {hour: 'numeric'}).resolvedOptions().hour", "numeric"},
	{"new Date(0).toLocaleTimeString('en-US', {weekday: 'long'})", "Thursday 12:00:00 AM"},
	{"new Date(0).toLocaleString('en-US', {month: 'long', day: 'numeric', hour: 'numeric'})", "January 1 at 12 AM"},
	{"new Intl.NumberFormat('en-US', {notation: 'compact'}).format(0.0001)", "0.0001"},
	{"new Intl.NumberFormat('en-US', {notation: 'compact'}).format(0.55555)", "0.56"},
	{"new Intl.NumberFormat('en-US', {notation: 'compact'}).format(12.345)", "12"},
	{"new Intl.NumberFormat('en-US', {notation: 'scientific'}).format(999999)", "1E6"},
	{"new Intl.NumberFormat('en-US', {notation: 'engineering'}).format(999999999)", "1E9"},
	{"new Intl.NumberFormat('en-US', {notation: 'engineering'}).format(0.5)", "500E-3"},
	{"new Intl.NumberFormat('en-US', {signDisplay: 'exceptZero'}).format(-1e-7)", "0"},
	{"new Intl.NumberFormat('en-US', {signDisplay: 'exceptZero'}).format(-Infinity)", "-∞"},
	{"new Intl.NumberFormat('en-US', {signDisplay: 'exceptZero'}).format(NaN)", "NaN"},
	{"new Intl.NumberFormat('en-US', {signDisplay: 'always'}).format(NaN)", "+NaN"},
	{"new Intl.NumberFormat('en-US', {style: 'currency', currency: 'USD', currencyDisplay: 'code'}).format(1)", "USD\u00a01.00"},
	{"new Intl.NumberFormat('en-US', {style: 'currency', currency: 'USD', currencyDisplay: 'code'}).format(NaN)", "USDNaN"},
	{"new Intl.NumberFormat('en-US', {style: 'currency', currency: 'SEK'}).format(1)", "SEK\u00a01.00"},
	{"new Intl.NumberFormat('en-US', {style: 'currency', currency: 'SEK', currencyDisplay: 'narrowSymbol'}).format(1)", "kr\u00a01.00"},
	{"new Intl.NumberFormat('en-US', {style: 'currency', currency: 'IQD'}).format(1)", "IQD\u00a01"},
	{"new Intl.NumberFormat('en-US', {style: 'unit', unit: 'celsius', unitDisplay: 'narrow'}).format(20)", "20°C"},
	{"new Intl.NumberFormat('en-US', {style: 'unit', unit: 'celsius'}).format(20)", "20°C"},
	{"new Intl.NumberFormat('en-US', {style: 'unit', unit: 'byte'}).format(20)", "20 byte"},
	{"new Intl.NumberFormat('en-US', {minimumSignificantDigits: 3}).format(0)", "0.00"},
	{"new Intl.DateTimeFormat('en-US', {timeZone: 'Asia/Shanghai', timeZoneName: 'short'}).format(0)", "1/1/1970, GMT+8"},
	{"new Intl.DateTimeFormat('en-US', {timeZone: 'America/Chicago', timeZoneName: 'short'}).format(0)", "12/31/1969, CST"},
	{"new Intl.NumberFormat('en-US', {maximumFractionDigits: 2, roundingMode: 'halfEven'}).format(1.005)", "1"},
	{"new Intl.NumberFormat('en-US', {maximumFractionDigits: 0, roundingMode: 'ceil'}).format(-2.5)", "-2"},
	{"new Intl.NumberFormat('en-US', {maximumFractionDigits: 0, roundingMode: 'floor'}).format(2.5)", "2"},
	{"new Intl.NumberFormat('en-US', {maximumFractionDigits: 0, roundingMode: 'halfEven'}).format(2.5)", "2"},
	{"new Intl.NumberFormat('en-US', {maximumFractionDigits: 0, roundingMode: 'halfEven'}).format(3.5)", "4"},
	{"new Intl.NumberFormat('en-US', {minimumFractionDigits: 2, trailingZeroDisplay: 'stripIfInteger'}).format(3)", "3"},
	{"new Intl.NumberFormat('en-US', {maximumFractionDigits: 0, roundingMode: 'ceil'}).format(0.0001)", "1"},
	{"new Intl.NumberFormat('en-US', {maximumFractionDigits: 2, roundingMode: 'expand'}).format(1e-7)", "0.01"},
	{"new Intl.NumberFormat('en-US', {maximumFractionDigits: 0, roundingMode: 'ceil'}).format(-0.0001)", "-0"},
}

// styleTimeGeneratedTests are the cases of TestStyleTimeGenerated.
var styleTimeGeneratedTests = [][2]string{
	{"new Intl.DateTimeFormat('kok-IN', {timeStyle: 'short', timeZone: 'UTC'}).format(Date.UTC(2026, 6, 5, 3, 7))", "3:07 AM"},
	{"new Intl.DateTimeFormat('kok-IN', {dateStyle: 'medium', timeStyle: 'short', timeZone: 'UTC'}).format(Date.UTC(2026, 6, 5, 3, 7))", "2026 M07 5 3:07 AM"},
	{"new Intl.DateTimeFormat('kok-Deva-IN', {dateStyle: 'medium', timeStyle: 'short', timeZone: 'UTC'}).format(Date.UTC(2026, 6, 5, 3, 7))", "2026 M07 5 03:07 AM"},
	{"new Intl.DateTimeFormat('kok', {timeStyle: 'short', timeZone: 'UTC'}).format(Date.UTC(2026, 6, 5, 3, 7))", "03:07 AM"},
	{"new Intl.DateTimeFormat('zh-MY', {dateStyle: 'full', timeStyle: 'long', timeZone: 'America/New_York'}).format(Date.UTC(2026, 0, 5, 15, 7, 9))", "2026年1月5日星期一 GMT-5上午10:07:09"},
	{"new Intl.DateTimeFormat('de', {timeStyle: 'short', hourCycle: 'h12', timeZone: 'UTC'}).format(Date.UTC(2026, 6, 5, 3, 7))", "03:07 AM"},
	{"new Intl.DateTimeFormat('de-u-hc-h12', {timeStyle: 'short', timeZone: 'UTC'}).format(Date.UTC(2026, 6, 5, 3, 7))", "3:07 AM"},
}

// patternTiesTests are the cases of TestPatternTies.
var patternTiesTests = [][2]string{
	{"new Intl.DateTimeFormat('en-AU', {weekday: 'short', month: 'long', day: 'numeric', timeZone: 'UTC'}).format(Date.UTC(2026, 6, 5))", "Sun 5 July"},
	{"new Intl.DateTimeFormat('en-ZA', {month: 'numeric', day: '2-digit', timeZone: 'UTC'}).format(Date.UTC(2026, 6, 5))", "07/05"},
	{"new Intl.DateTimeFormat('en-ZA', {month: '2-digit', day: 'numeric', timeZone: 'UTC'}).format(Date.UTC(2026, 6, 5))", "07/05"},
	{"new Intl.DateTimeFormat('es-MX', {month: 'long', day: '2-digit', timeZone: 'UTC'}).format(Date.UTC(2026, 6, 5))", "05-julio"},
	{"new Intl.DateTimeFormat('es-US', {month: 'long', day: '2-digit', timeZone: 'UTC'}).format(Date.UTC(2026, 6, 5))", "05-julio"},
	{"new Intl.DateTimeFormat('es-CO', {month: 'long', day: '2-digit', timeZone: 'UTC'}).format(Date.UTC(2026, 6, 5))", "5 de julio"},
}

// dateTimeFormatLocalesTests are the cases of TestDateTimeFormatLocales.
var dateTimeFormatLocalesTests = [][2]string{
	{"new Intl.DateTimeFormat('zh-CN', {dateStyle: 'full', timeStyle: 'long', timeZone: 'Asia/Shanghai'}).format(0)", "1970年1月1日星期四 GMT+8 08:00:00"},
	{"new Intl.DateTimeFormat('zh-TW', {hour: 'numeric', minute: 'numeric', timeZone: 'Asia/Taipei'}).format(0)", "上午8:00"},
	{"new Intl.DateTimeFormat('ja', {dateStyle: 'medium', timeStyle: 'short', timeZone: 'Asia/Tokyo'}).format(0)", "1970/01/01 9:00"},
	{"new Intl.DateTimeFormat('ja-JP', {hour: 'numeric', hour12: true, timeZone: 'UTC'}).format(0)", "午前0時"},
	{"new Intl.DateTimeFormat('de', {weekday: 'long', day: 'numeric', month: 'long', timeZone: 'Europe/Berlin'}).format(0)", "Donnerstag, 1. Januar"},
	{"new Intl.DateTimeFormat('ar-EG', {dateStyle: 'short', timeZone: 'Africa/Cairo'}).format(0)", "١\u200f/١\u200f/١٩٧٠"},
	{"new Intl.DateTimeFormat('zh-CN', {dayPeriod: 'long', hour: 'numeric', timeZone: 'UTC'}).format(0)", "0时"},
	{"new Date(0).toLocaleString('zh-CN', {timeZone: 'Asia/Shanghai'})", "1970/1/1 08:00:00"},
	{"new Date(0).toLocaleDateString('de-DE', {timeZone: 'UTC'})", "1.1.1970"},
	{"new Date(0).toLocaleTimeString('fr', {timeZone: 'UTC'})", "00:00:00"},
	{"new Intl.DateTimeFormat('fa', {dateStyle: 'long', timeZone: 'Asia/Tehran'}).format(0)", "۱۱ دی ۱۳۴۸"},
	{"new Intl.DateTimeFormat('fa').resolvedOptions().calendar", "persian"},
	{"new Intl.DateTimeFormat('th', {dateStyle: 'long', timeZone: 'Asia/Bangkok'}).format(0)", "1 มกราคม 2513"},
	{"new Intl.DateTimeFormat('ja-JP-u-ca-japanese').resolvedOptions().locale", "ja-JP-u-ca-japanese"},
	{"new Intl.DateTimeFormat('ja-JP-u-ca-japanese').resolvedOptions().calendar", "japanese"},
	{"new Intl.DateTimeFormat('en-US-u-hc-h23', {hour: 'numeric', minute: 'numeric', timeZone: 'UTC'}).format(0)", "00:00"},
	{"new Intl.DateTimeFormat('en-US-u-hc-h23', {hour: 'numeric', hour12: true}).resolvedOptions().locale", "en-US"},
	{"new Intl.DateTimeFormat('ko', {timeZoneName: 'long', timeZone: 'Asia/Seoul'}).format(0)", "1970. 1. 1. 한국 표준시"},
	{"new Intl.DateTimeFormat('en-IN', {timeZoneName: 'short', timeZone: 'America/New_York'}).format(0)", "31/12/1969, GMT-5"},
	{"new Intl.DateTimeFormat('pt-BR', {timeZoneName: 'shortGeneric', timeZone: 'America/Sao_Paulo'}).format(0)", "31/12/1969, BRT"},
	{"new Intl.DateTimeFormat('en', {timeZoneName: 'shortGeneric', timeZone: 'Asia/Hong_Kong'}).format(0)", "1/1/1970, Hong Kong Time"},
	{"new Intl.DateTimeFormat('en-US', {timeZoneName: 'shortOffset', timeZone: 'UTC'}).format(0)", "1/1/1970, GMT+0"},
	{"new Intl.DateTimeFormat('en', {timeZoneName: 'long', timeZone: 'Asia/Shanghai'}).format(-62198755200000)", "1/1/2, GMT+08:05:43"},
	{"new Intl.DateTimeFormat('en-GB', {timeZoneName: 'long', timeZone: 'Europe/London'}).format(Date.UTC(2025, 0, 15))", "15/01/2025, GMT+00:00"},
}

// resolvedOptionsTests are the cases of TestResolvedOptions.
var resolvedOptionsTests = [][2]string{
	{"new Intl.DateTimeFormat('cy').resolvedOptions().locale", "en-US"},
	{"new Intl.DateTimeFormat('de-DE').resolvedOptions().locale", "de-DE"},
	{"new Intl.NumberFormat(['fr', 'de']).resolvedOptions().locale", "fr"},
	{"new Intl.NumberFormat(['cy', 'de']).resolvedOptions().locale", "de"},
	{"new Intl.NumberFormat('zh-Hans-CN').resolvedOptions().locale", "zh-Hans-CN"},
	{"new Intl.NumberFormat('ar-EG-u-nu-latn').resolvedOptions().locale", "ar-EG-u-nu-latn"},
	{"new Intl.NumberFormat('ar-EG').resolvedOptions().numberingSystem", "arab"},
	{"new Intl.NumberFormat('en-US-u-hc-h23').resolvedOptions().locale", "en-US"},
	{"new Intl.DateTimeFormat().resolvedOptions().timeZone", "UTC"},
	{"new Intl.DateTimeFormat('en', {timeZone: 'america/new_york'}).resolvedOptions().timeZone", "America/New_York"},
	{"new Intl.DateTimeFormat('en', {timeZone: 'ASIA/CALCUTTA'}).resolvedOptions().timeZone", "Asia/Calcutta"},
	{"new Intl.DateTimeFormat('en', {timeZone: 'Asia/Kolkata'}).resolvedOptions().timeZone", "Asia/Calcutta"},
	{"new Intl.DateTimeFormat('en', {timeZone: 'US/Pacific'}).resolvedOptions().timeZone", "America/Los_Angeles"},
	{"new Intl.DateTimeFormat('en', {timeZone: 'america/port-au-prince'}).resolvedOptions().timeZone", "America/Port-au-Prince"},
	{"new Intl.DateTimeFormat('en', {timeZone: 'etc/gmt+5'}).resolvedOptions().timeZone", "Etc/GMT+5"},
	{"new Intl.DateTimeFormat('en', {timeZone: 'GMT'}).resolvedOptions().timeZone", "UTC"},
	{"new Intl.DateTimeFormat('en').resolvedOptions().calendar", "gregory"},
	{"JSON.stringify(new Intl.DateTimeFormat('en-US').resolvedOptions())", "{\"locale\":\"en-US\",\"calendar\":\"gregory\",\"numberingSystem\":\"latn\",\"timeZone\":\"UTC\",\"year\":\"numeric\",\"month\":\"numeric\",\"day\":\"numeric\"}"},
	{"new Intl.DateTimeFormat('en', {hour: 'numeric'}).resolvedOptions().hourCycle", "h12"},
	{"new Intl.DateTimeFormat('en', {hour: 'numeric', hour12: false}).resolvedOptions().hourCycle", "h23"},
	{"new Intl.DateTimeFormat('en', {hour: 'numeric', hour12: false}).resolvedOptions().hour12", "false"},
	{"new Intl.DateTimeFormat('en', {dateStyle: 'full'}).resolvedOptions().dateStyle", "full"},
	{"new Intl.DateTimeFormat('en', {dateStyle: 'full'}).resolvedOptions().weekday", "undefined"},
	{"JSON.stringify(new Intl.NumberFormat('en-US').resolvedOptions())", "{\"locale\":\"en-US\",\"numberingSystem\":\"latn\",\"style\":\"decimal\",\"minimumIntegerDigits\":1,\"minimumFractionDigits\":0,\"maximumFractionDigits\":3,\"useGrouping\":\"auto\",\"notation\":\"standard\",\"signDisplay\":\"auto\",\"roundingIncrement\":1,\"roundingMode\":\"halfExpand\",\"roundingPriority\":\"auto\",\"trailingZeroDisplay\":\"auto\"}"},
	{"JSON.stringify(new Intl.NumberFormat('en-US', {notation: 'compact'}).resolvedOptions())", "{\"locale\":\"en-US\",\"numberingSystem\":\"latn\",\"style\":\"decimal\",\"minimumIntegerDigits\":1,\"minimumFractionDigits\":0,\"maximumFractionDigits\":0,\"minimumSignificantDigits\":1,\"maximumSignificantDigits\":2,\"useGrouping\":\"min2\",\"notation\":\"compact\",\"compactDisplay\":\"short\",\"signDisplay\":\"auto\",\"roundingIncrement\":1,\"roundingMode\":\"halfExpand\",\"roundingPriority\":\"morePrecision\",\"trailingZeroDisplay\":\"auto\"}"},
	{"Object.keys(new Intl.NumberFormat('en', {notation: 'compact'}).resolvedOptions()).filter(k => k[0] === '_').length", "0"},
	{"Intl.getCanonicalLocales(['EN-us', 'de-de']).join(' ')", "en-US de-DE"},
	{"Intl.NumberFormat.supportedLocalesOf(['de', 'en-US', 'cy']).join(' ')", "de en-US"},
	{"Intl.DateTimeFormat.supportedLocalesOf('en-GB').join(' ')", "en-GB"},
}

// supportedLocalesOfOptionsTests are the cases of TestSupportedLocalesOfOptions.
var supportedLocalesOfOptionsTests = [][2]string{
	{"null", "TypeError"},
	{"undefined", "[\"en\",\"de\"]"},
	{"{}", "[\"en\",\"de\"]"},
	{"5", "[\"en\",\"de\"]"},
	{"\"x\"", "[\"en\",\"de\"]"},
	{"{localeMatcher: 'bogus'}", "RangeError"},
	{"{localeMatcher: 'lookup'}", "[\"en\",\"de\"]"},
	{"{localeMatcher: 'best fit'}", "[\"en\",\"de\"]"},
	{"{localeMatcher: undefined}", "[\"en\",\"de\"]"},
	{"{localeMatcher: null}", "RangeError"},
	{"{localeMatcher: {toString() { return 'lookup'; }}}", "[\"en\",\"de\"]"},
	{"Object.create({localeMatcher: 'x'})", "RangeError"},
}

// errorsTests are the cases of TestErrors.
var errorsTests = [][2]string{
	{"new Intl.DateTimeFormat('en', {timeZone: 'Mars/Olympus'})", "RangeError:"},
	{"new Intl.DateTimeFormat('en', {timeZone: 'Local'})", "RangeError:"},
	{"new Intl.DateTimeFormat('en', {timeZone: 'Etc/Unknown'})", "RangeError:"},
	{"new Intl.DateTimeFormat('en', {weekday: 'wide'})", "RangeError:"},
	{"new Intl.DateTimeFormat('en', {dateStyle: 'full', year: 'numeric'})", "TypeError:"},
	{"new Date(0).toLocaleDateString('en', {timeStyle: 'short'})", "TypeError:"},
	{"new Intl.DateTimeFormat('en').format(new Date(NaN))", "RangeError:"},
	{"new Intl.DateTimeFormat('123')", "RangeError:"},
	{"new Intl.NumberFormat('en', {style: 'currency'})", "TypeError:"},
	{"new Intl.NumberFormat('en', {style: 'currency', currency: 'US'})", "RangeError:"},
	{"new Intl.NumberFormat('en', {minimumFractionDigits: 3, maximumFractionDigits: 1})", "RangeError:"},
	{"Intl.DateTimeFormat.prototype.resolvedOptions.call({})", "TypeError:"},
	{"new Intl.NumberFormat(null)", "TypeError:"},
	{"new Intl.DateTimeFormat(null)", "TypeError:"},
	{"new Intl.PluralRules(null)", "TypeError:"},
	{"Intl.getCanonicalLocales(null)", "TypeError:"},
	{"Intl.NumberFormat.supportedLocalesOf(null)", "TypeError:"},
	{"(1).toLocaleString(null)", "TypeError:"},
	{"new Date(0).toLocaleString(null)", "TypeError:"},
	{"Intl.getCanonicalLocales(5).length", "no error"},
	// The toLocale* methods keep the formatter of no locale apart from that
	// of the empty tag, which is no tag.
	{"(1).toLocaleString()", "no error"},
	{"(1).toLocaleString('')", "RangeError:"},
	{"new Date(0).toLocaleString()", "no error"},
	{"new Date(0).toLocaleString('')", "RangeError:"},
	{"'a'.localeCompare('b', 'en')", "no error"},
	{"'a'.localeCompare('b', '')", "RangeError:"},
}

// defaultTimeZoneTests are the cases of TestDefaultTimeZone.
var defaultTimeZoneTests = [][2]string{
	{"new Intl.DateTimeFormat().resolvedOptions().timeZone", "America/New_York"},
	{"new Date(0).toLocaleString('en-US')", "12/31/1969, 7:00:00 PM"},
	{"new Date(0).toLocaleString('en-US', {timeZone: 'UTC'})", "1/1/1970, 12:00:00 AM"},
}

// utcByDefaultTests are the cases of TestUTCByDefault.
var utcByDefaultTests = [][2]string{
	{"new Intl.DateTimeFormat().resolvedOptions().timeZone", "UTC"},
	{"new Date(0).toLocaleString('en-US')", "1/1/1970, 12:00:00 AM"},
}

// registerPerRealmTests are the cases of TestRegisterPerRealm.
var registerPerRealmTests = [][2]string{
	{"new Intl.DateTimeFormat('en-US').format(new Date(0))", "1/1/1970"},
	{"new Date(0).toLocaleString('en-US', {timeZone: 'America/New_York'})", "12/31/1969, 7:00:00 PM"},
	{"(1234.5).toLocaleString('en-US')", "1,234.5"},
}
