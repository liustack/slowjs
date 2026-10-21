// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package temporal // import "github.com/liustack/quickjs/temporal"

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/liustack/quickjs"
	"github.com/liustack/quickjs/intl"
)

func newVM(tb testing.TB, opts ...Option) *quickjs.VM {
	tb.Helper()
	vm, err := quickjs.NewVM()
	if err != nil {
		tb.Fatal(err)
	}

	tb.Cleanup(func() { vm.Close() })
	if err := intl.Register(vm, intl.TimeZone("UTC")); err != nil {
		tb.Fatal(err)
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

// evalTests runs src/want pairs, each in the same VM. A want starting with
// "!" is the error src throws, "!RangeError" say, or its start.
func evalTests(t *testing.T, vm *quickjs.VM, tests [][2]string) {
	t.Helper()
	for _, test := range tests {
		src, want := test[0], test[1]
		got, err := vm.Eval(src, quickjs.EvalGlobal)
		switch {
		case strings.HasPrefix(want, "!"):
			if err == nil {
				t.Errorf("%s\n\tgot  %q\n\twant error %s", src, fmt.Sprint(got), want[1:])
			} else if !strings.HasPrefix(err.Error(), want[1:]) {
				t.Errorf("%s\n\tgot error %v\n\twant error %s", src, err, want[1:])
			}
		case err != nil:
			t.Errorf("%s\n\tgot error %v\n\twant %q", src, err, want)
		case fmt.Sprint(got) != want:
			t.Errorf("%s\n\tgot  %q\n\twant %q", src, fmt.Sprint(got), want)
		}
	}
}

func TestLazyInstall(t *testing.T) {
	vm := newVM(t)
	before := shimInstalls.Load()
	evalTests(t, vm, [][2]string{
		{`typeof Object.getOwnPropertyDescriptor(globalThis, 'Temporal').get`, "function"},
		{`Object.keys(globalThis).includes('Temporal')`, "false"},
	})
	if n := shimInstalls.Load() - before; n != 0 {
		t.Fatalf("installed %d times before first use", n)
	}
	evalTests(t, vm, [][2]string{
		{`typeof Temporal`, "object"},
		{`String(Temporal)`, "[object Temporal]"},
		{`typeof Object.getOwnPropertyDescriptor(globalThis, 'Temporal').value`, "object"},
		{`Temporal === Temporal`, "true"},
		{`Temporal.PlainDate.from('2024-01-05').toString()`, "2024-01-05"},
	})
	if n := shimInstalls.Load() - before; n != 1 {
		t.Fatalf("installed %d times, want 1", n)
	}
}

func TestLazyInstallFromDate(t *testing.T) {
	vm := newVM(t)
	evalTests(t, vm, [][2]string{
		{`new Date(1704460800123).toTemporalInstant().toString()`, "2024-01-05T13:20:00.123Z"},
		{`Temporal.Instant.name`, "Instant"},
		{`Date.prototype.toTemporalInstant.length`, "0"},
	})
}

func TestPageTemporalKept(t *testing.T) {
	vm := newVM(t)
	before := shimInstalls.Load()
	evalTests(t, vm, [][2]string{
		{`Temporal = {mine: true}; Temporal.mine`, "true"},
	})
	if n := shimInstalls.Load() - before; n != 0 {
		t.Fatalf("installed %d times", n)
	}
}

// What a page does to Temporal or Date.prototype.toTemporalInstant before
// the first use stays as V8 has it: the first use replaces a stand-in only
// while it is still there and can be replaced, and a stand-in left in place
// keeps working. Each case runs in a VM of its own.
func TestStandInsKeepPageCode(t *testing.T) {
	const epoch = "1970-01-01T00:00:00Z"
	for _, test := range [][2]string{
		{`Date.prototype.toTemporalInstant = function () { return 'mine'; }; Temporal.Now; new Date(0).toTemporalInstant()`, "mine"},
		{`Date.prototype.toTemporalInstant = function () { return 'mine'; }; Temporal.PlainDate; [new Date(0).toTemporalInstant(), Date.prototype.toTemporalInstant.name].join()`, "mine,"},
		{`Object.defineProperty(globalThis, 'Temporal', {value: 42, writable: true, configurable: false}); String(new Date(0).toTemporalInstant()) + ' ' + Temporal`, epoch + " 42"},
		{`Object.defineProperty(globalThis, 'Temporal', {value: 42, writable: true, configurable: true}); String(new Date(0).toTemporalInstant()) + ' ' + Temporal`, epoch + " 42"},
		{`delete globalThis.Temporal; String(new Date(0).toTemporalInstant()) + ' ' + typeof Temporal`, epoch + " undefined"},
		{`(0, eval)('function Temporal() {}'); String(new Date(0).toTemporalInstant()) + ' ' + typeof Temporal`, epoch + " function"},
		{`Temporal = 5; [Temporal, String(new Date(0).toTemporalInstant())].join()`, "5," + epoch},
		{`Object.freeze(Date.prototype); [String(new Date(0).toTemporalInstant()), new Date(0).toTemporalInstant() instanceof Temporal.Instant].join()`, epoch + ",true"},
		{`Object.preventExtensions(globalThis); [typeof Temporal, typeof Object.getOwnPropertyDescriptor(globalThis, 'Temporal').value].join()`, "object,object"},
		{`Object.freeze(globalThis); [typeof Temporal.Now, Temporal === Temporal, new Date(0).toTemporalInstant() instanceof Temporal.Instant].join()`, "object,true,true"},
		{`var f = Date.prototype.toTemporalInstant; Temporal.Now; [String(f.call(new Date(0))), f.call(new Date(0)) instanceof Temporal.Instant].join()`, epoch + ",true"},
	} {
		evalTests(t, newVM(t), [][2]string{test})
	}
}

func TestRegisterTwice(t *testing.T) {
	vm := newVM(t)
	if err := Register(vm); err == nil {
		t.Fatal("second Register: no error")
	}
}

func TestClock(t *testing.T) {
	at := time.Date(2024, 1, 5, 13, 5, 7, 123456789, time.UTC)
	vm := newVM(t, Clock(func() time.Time { return at }))
	evalTests(t, vm, [][2]string{
		{`Temporal.Now.instant().toString()`, "2024-01-05T13:05:07.123456789Z"},
		{`Temporal.Now.instant().epochNanoseconds === 1704459907123456789n`, "true"},
		{`Temporal.Now.timeZoneId()`, "UTC"},
		{`Temporal.Now.plainDateISO().toString()`, "2024-01-05"},
		{`Temporal.Now.plainTimeISO('Asia/Tokyo').toString()`, "22:05:07.123456789"},
		{`Temporal.Now.plainDateTimeISO('-05:00').toString()`, "2024-01-05T08:05:07.123456789"},
		{`Temporal.Now.zonedDateTimeISO().toString()`, "2024-01-05T13:05:07.123456789+00:00[UTC]"},
		{`Temporal.Now.zonedDateTimeISO('America/New_York').toString()`,
			"2024-01-05T08:05:07.123456789-05:00[America/New_York]"},
	})
	// Before the epoch, the nanoseconds past the floored millisecond.
	at = time.Date(1969, 12, 31, 23, 59, 59, 999999999, time.UTC)
	evalTests(t, vm, [][2]string{
		{`Temporal.Now.instant().epochNanoseconds`, "-1"},
		{`Temporal.Now.instant().epochMilliseconds`, "-1"},
	})
}

// A clock anywhere in Temporal.Instant's range gives its time, past the
// years 1678 to 2262 that UnixNano holds, and one outside throws a
// RangeError rather than a wrong time.
func TestClockRange(t *testing.T) {
	for _, test := range []struct {
		at   time.Time
		want string
	}{
		{time.Date(2300, 1, 1, 0, 0, 0, 0, time.UTC), "2300-01-01T00:00:00Z"},
		{time.Date(1600, 1, 1, 0, 0, 0, 0, time.UTC), "1600-01-01T00:00:00Z"},
		{time.Time{}, "0001-01-01T00:00:00Z"},
		{time.Date(1969, 12, 31, 23, 59, 59, 999_999_999, time.UTC), "1969-12-31T23:59:59.999999999Z"},
		{time.UnixMilli(8.64e15), "+275760-09-13T00:00:00Z"},
		{time.UnixMilli(-8.64e15), "-271821-04-20T00:00:00Z"},
		{time.UnixMilli(8.64e15).Add(time.Nanosecond), "!RangeError"},
		{time.UnixMilli(-8.64e15).Add(-time.Nanosecond), "!RangeError"},
		{time.Date(300000, 1, 1, 0, 0, 0, 0, time.UTC), "!RangeError"},
	} {
		vm := newVM(t, Clock(func() time.Time { return test.at }))
		evalTests(t, vm, [][2]string{{`Temporal.Now.instant().toString()`, test.want}})
	}
}

// Close drops a VM's clock, a clock that references its VM too.
func TestClockDroppedOnClose(t *testing.T) {
	count := func() int {
		n := 0
		clocks.Range(func(any, any) bool { n++; return true })
		return n
	}
	before := count()
	for range 20 {
		vm, err := quickjs.NewVM()
		if err != nil {
			t.Fatal(err)
		}
		if err := Register(vm, Clock(func() time.Time { _ = vm; return time.Unix(0, 0) })); err != nil {
			t.Fatal(err)
		}
		if n := count(); n != before+1 {
			t.Fatalf("%d clocks with a VM open, want %d", n, before+1)
		}
		vm.Close()
		if n := count(); n != before {
			t.Fatalf("%d clocks after Close, want %d", n, before)
		}
	}
}

// A panic in a Clock reaches the caller of Eval, and the VM works after it.
func TestClockPanics(t *testing.T) {
	vm := newVM(t, Clock(func() time.Time { panic("clock broke") }))
	func() {
		defer func() {
			if r := recover(); r != "clock broke" {
				t.Errorf("recovered %v", r)
			}
		}()
		v, err := vm.Eval(`Temporal.Now.instant().toString()`, quickjs.EvalGlobal)
		t.Errorf("no panic: %v %v", v, err)
	}()
	evalTests(t, vm, [][2]string{{`Temporal.PlainDate.from('2024-01-05').add({days: 1}).toString()`, "2024-01-06"}})
}

func TestClockPerVM(t *testing.T) {
	a := newVM(t, Clock(func() time.Time { return time.Unix(0, 0) }))
	b := newVM(t)
	evalTests(t, a, [][2]string{{`Temporal.Now.instant().toString()`, "1970-01-01T00:00:00Z"}})
	evalTests(t, b, [][2]string{{`Temporal.Now.instant().epochMilliseconds > 1.7e12`, "true"}})
}

func TestBasics(t *testing.T) {
	vm := newVM(t)
	evalTests(t, vm, [][2]string{
		{`new Temporal.PlainDate(2024, 2, 29).add({years: 1}).toString()`, "2025-02-28"},
		{`Temporal.PlainDate.from('2024-01-05').until('2024-03-01').toString()`, "P56D"},
		{`Temporal.PlainDate.from('2024-01-05').until('2024-03-01', {largestUnit: 'month'}).toString()`, "P1M25D"},
		{`Temporal.PlainDate.from('2024-01-05').dayOfWeek`, "5"},
		{`Temporal.PlainDate.from('2024-01-05').weekOfYear`, "1"},
		{`Temporal.PlainDate.from({year: 2024, month: 13, day: 1}).toString()`, "2024-12-01"},
		{`Temporal.PlainDate.from({year: 2024, month: 13, day: 1}, {overflow: 'reject'})`, "!RangeError"},
		{`Temporal.PlainDate.from('2024-01-05').with({day: 31}).toString()`, "2024-01-31"},
		{`Temporal.PlainDate.compare('2024-01-05', '2024-01-06')`, "-1"},
		{`Temporal.PlainTime.from('13:05:07.5').round('second').toString()`, "13:05:08"},
		{`new Temporal.PlainTime(1, 2, 3).toString({smallestUnit: 'minute'})`, "01:02"},
		{`Temporal.PlainDateTime.from('2024-01-05T13:05').toString()`, "2024-01-05T13:05:00"},
		{`Temporal.PlainDateTime.from('2024-01-05T13:05').round({smallestUnit: 'day'}).toString()`,
			"2024-01-06T00:00:00"},
		{`Temporal.PlainYearMonth.from('2024-02').daysInMonth`, "29"},
		{`Temporal.PlainYearMonth.from('2024-02').toPlainDate({day: 3}).toString()`, "2024-02-03"},
		{`Temporal.PlainMonthDay.from('02-29').toPlainDate({year: 2023}).toString()`, "2023-02-28"},
		{`Temporal.PlainMonthDay.from('--12-25').toString()`, "12-25"},
		{`String(Temporal.Instant.from('2024-01-05T13:05:00Z').epochMilliseconds)`, "1704459900000"},
		{`Temporal.Instant.fromEpochMilliseconds(0).add({hours: 1}).toString()`, "1970-01-01T01:00:00Z"},
		{`new Temporal.Instant(1n).toString()`, "1970-01-01T00:00:00.000000001Z"},
		{`new Temporal.Instant(-1n).epochMilliseconds`, "-1"},
		{`Temporal.Instant.from('2024-01-05T13:05:00Z').toString({timeZone: 'Asia/Shanghai'})`,
			"2024-01-05T21:05:00+08:00"},
		{`Temporal.ZonedDateTime.from('2024-03-10T01:30[America/New_York]').add({hours: 1}).toString()`,
			"2024-03-10T03:30:00-04:00[America/New_York]"},
		{`Temporal.ZonedDateTime.from('2024-03-10T12:00[America/New_York]').hoursInDay`, "23"},
		{`Temporal.ZonedDateTime.from('2024-03-10T12:00[America/New_York]').offset`, "-04:00"},
		{`Temporal.ZonedDateTime.from('2024-03-10T12:00[America/New_York]').getTimeZoneTransition('previous').toString()`,
			"2024-03-10T03:00:00-04:00[America/New_York]"},
		{`Temporal.ZonedDateTime.from('2024-01-05T12:00[UTC]').getTimeZoneTransition('next')`, "<nil>"},
		{`Temporal.ZonedDateTime.from('2024-01-05T12:00[Europe/Paris]').startOfDay().toString()`,
			"2024-01-05T00:00:00+01:00[Europe/Paris]"},
		{`Temporal.ZonedDateTime.from('2024-01-05T12:00[Europe/Paris]').with({hour: 3}).toString()`,
			"2024-01-05T03:00:00+01:00[Europe/Paris]"},
		{`Temporal.ZonedDateTime.from('2024-01-05T12:00[Europe/Paris]').epochNanoseconds`, "1704452400000000000"},
		{`Temporal.Duration.from({hours: 25}).round({largestUnit: 'day'}).toString()`, "P1DT1H"},
		{`Temporal.Duration.from('P1M').total({unit: 'day', relativeTo: '2024-02-01'})`, "29"},
		{`Temporal.Duration.compare({hours: 24}, {days: 1})`, "0"},
		{`Temporal.Duration.from({minutes: 90}).toString()`, "PT90M"},
		{`new Temporal.Duration(1, 2).negated().toString()`, "-P1Y2M"},
		{`Temporal.Duration.from('PT1.5S').milliseconds`, "500"},
		{`Temporal.PlainDate.from('2024-01-05').withCalendar('japanese').era`, "reiwa"},
		{`Temporal.PlainDate.from('2024-01-05[u-ca=hebrew]').monthCode`, "M04"},
		{`Temporal.PlainDate.from('2024-01-05').valueOf()`, "!TypeError"},
		{`Temporal.PlainDate('2024-01-05')`, "!TypeError"},
		{`JSON.stringify({d: Temporal.PlainDate.from('2024-01-05')})`, `{"d":"2024-01-05"}`},
	})
}

func TestToLocaleString(t *testing.T) {
	vm := newVM(t)
	evalTests(t, vm, [][2]string{
		{`Temporal.PlainDate.from('2024-01-05').toLocaleString('en-US')`, "1/5/2024"},
		{`Temporal.PlainTime.from('13:05').toLocaleString('en-US')`, "1:05:00 PM"},
		{`Temporal.PlainDateTime.from('2024-01-05T13:05').toLocaleString('en-US')`, "1/5/2024, 1:05:00 PM"},
		{`Temporal.ZonedDateTime.from('2024-01-05T13:05[America/New_York]').toLocaleString('en-US')`,
			"1/5/2024, 1:05:00 PM EST"},
		{`Temporal.Instant.from('2024-01-05T13:05Z').toLocaleString('en-US')`, "1/5/2024, 1:05:00 PM"},
		{`Temporal.PlainDate.from('2024-01-05').toLocaleString('de', {month: 'long'})`, "Januar"},
		{`Temporal.ZonedDateTime.from('2024-01-05T13:05[America/New_York]').toLocaleString('en-US', {timeZone: 'UTC'})`,
			"!TypeError"},
	})
}

// A ZonedDateTime's toLocaleString shows the wall clock and offset its
// toString does, also where Go's zone data has another offset for the
// zone: Temporal's data is ICU's, package intl's is Go's.
func TestToLocaleStringZonedOffset(t *testing.T) {
	vm := newVM(t)
	for _, at := range [][2]string{
		{"1908-04-30T21:32:44Z", "Africa/Addis_Ababa"}, // ICU and IANA's backzone differ
		{"1908-04-30T21:32:44Z", "Africa/Kampala"},     // likewise
		{"2026-11-01T07:00:00Z", "America/Winnipeg"},   // stays on -05 from 2026e
		{"2026-07-01T00:00:00Z", "Europe/Paris"},
	} {
		src := `(function () {
			const z = Temporal.Instant.from('` + at[0] + `').toZonedDateTimeISO('` + at[1] + `');
			const want = z.toPlainDateTime().toLocaleString('en-US') + ' GMT' + z.offset;
			const got = z.toLocaleString('en-US', {timeZoneName: 'longOffset'});
			return got === want ? 'same' : got + ' != ' + want + ' (' + z.toString() + ')';
		})()`
		evalTests(t, vm, [][2]string{{src, "same"}})
	}
}

// A function in a locale list is an object, as ECMA-402's
// CanonicalizeLocaleList has it, so it throws a RangeError.
func TestLocaleListFunction(t *testing.T) {
	vm := newVM(t)
	evalTests(t, vm, [][2]string{
		{`Temporal.PlainDate.from('2024-01-05').toLocaleString([function () {}])`, "!RangeError"},
		{`Temporal.PlainDate.from('2024-01-05').toLocaleString([1])`, "!TypeError"},
		{`var f = function () {}; f.toString = () => 'de'; Temporal.PlainDate.from('2024-01-05').toLocaleString([f])`, "5.1.2024"},
	})
}

// TestDefaultZone checks the zones of a VM whose Intl has a default zone
// other than UTC: Temporal.Now takes Intl's zone, ECMA-402's
// DefaultTimeZone, while Date's local time stays UTC, and an Instant's
// toLocaleString formats in Intl's zone, as a Date's does.
func TestDefaultZone(t *testing.T) {
	vm, err := quickjs.NewVM()
	if err != nil {
		t.Fatal(err)
	}

	defer vm.Close()

	if err := intl.Register(vm, intl.TimeZone("Asia/Tokyo")); err != nil {
		t.Fatal(err)
	}
	if err := Register(vm, Clock(func() time.Time { return time.Unix(0, 0) })); err != nil {
		t.Fatal(err)
	}
	evalTests(t, vm, [][2]string{
		{`Temporal.Now.timeZoneId()`, "Asia/Tokyo"},
		{`Temporal.Now.plainDateTimeISO().toString()`, "1970-01-01T09:00:00"},
		{`Temporal.Now.zonedDateTimeISO().toString()`, "1970-01-01T09:00:00+09:00[Asia/Tokyo]"},
		{`Temporal.Now.plainDateISO('UTC').toString()`, "1970-01-01"},
		{`new Date(0).getHours()`, "0"},
		{`Temporal.Instant.fromEpochMilliseconds(0).toLocaleString('en-US')`, "1/1/1970, 9:00:00 AM"},
		{`new Date(0).toLocaleString('en-US')`, "1/1/1970, 9:00:00 AM"},
		{`Temporal.PlainDateTime.from('1970-01-01T00:00').toLocaleString('en-US')`, "1/1/1970, 12:00:00 AM"},
		{`Temporal.ZonedDateTime.from('1970-01-01T00:00[UTC]').toLocaleString('en-US')`, "1/1/1970, 12:00:00 AM UTC"},
	})
}
