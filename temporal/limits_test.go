// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package temporal // import "github.com/liustack/quickjs/temporal"

import (
	"errors"
	"fmt"
	"runtime"
	"strings"
	"testing"

	"github.com/liustack/quickjs"
	"github.com/liustack/quickjs/intl"
)

// A string argument longer than the cap of a string of Temporal is refused
// with a RangeError before it is read: the rope of 32 MiB is not joined, in
// a VM limited to 16 MiB. And one under the cap, in a VM with less room than
// the door counts for the call, ends the call with the out of memory error:
// the bridge does not parse it as the empty string.
func TestStringArgumentOutOfMemory(t *testing.T) {
	for _, js := range []string{
		`Temporal.PlainDate.from(s)`,
		`Temporal.Duration.from(s)`,
		`Temporal.PlainDate.from('2024-01-05').toLocaleString(s)`,
	} {
		t.Run(js, func(t *testing.T) {
			vm := newVM(t)
			eval(t, vm, `Temporal.PlainDate.from('2024-01-05').toLocaleString('en'); JSON`)
			vm.SetMemoryLimit(16 << 20)
			got, err := vm.Eval(`
				var r = "ok";
				try {
					let s = "1".repeat(1024);
					for (let i = 0; i < 15; i++) s = s + s;
					`+js+`;
				} catch (e) { r = "caught " + e; }
				r`, quickjs.EvalGlobal)
			if r, _ := got.(string); err != nil || !strings.HasPrefix(r, "caught RangeError") {
				t.Fatalf("got %v, %v, want a RangeError", got, err)
			}
			vm.SetMemoryLimit(vm.MemorySize() + 16<<10)
			_, err = vm.Eval(`var s = "1".repeat(100); try { `+js+` } catch (e) { "caught " + e }`, quickjs.EvalGlobal)
			var e *quickjs.Error
			if !errors.As(err, &e) || !e.Uncatchable || !e.OutOfMemory {
				t.Fatalf("got %v, want the uncatchable out of memory error", err)
			}
		})
	}
}

// What a VM asks Intl to format a Temporal value with stays in the caches of
// the process, which every VM shares, only within a small bound: a failed
// request with an option of 1 MiB, and a format of a list of 10,000 locales,
// each in a VM closed right after, leave a few MiB at most.
func TestFormatCachesAfterClose(t *testing.T) {
	for _, c := range []struct{ name, js string }{
		{"bad calendar", `try { Temporal.PlainDate.from('2024-01-05').toLocaleString('en', {calendar: 'X'.repeat(1 << 20) + %d}) } catch (e) {}`},
		{"long list", `new Intl.DateTimeFormat(Array.from({length: 10000}, (_, i) => 'en-x-' + i.toString(36) + %d)).format(Temporal.PlainDate.from('2024-01-05'))`},
		{"long list toLocaleString", `Temporal.PlainDate.from('2024-01-05').toLocaleString(Array.from({length: 10000}, (_, i) => 'en-x-' + i.toString(36) + %d))`},
	} {
		t.Run(c.name, func(t *testing.T) {
			runtime.GC()
			var before, after runtime.MemStats
			runtime.ReadMemStats(&before)
			const n = 40
			for i := range n {
				vm, err := quickjs.NewVM()
				if err != nil {
					t.Fatal(err)
				}
				if err := intl.Register(vm, intl.TimeZone("UTC")); err != nil {
					t.Fatal(err)
				}
				if err := Register(vm); err != nil {
					t.Fatal(err)
				}
				_, err = vm.Eval(fmt.Sprintf(c.js, i), quickjs.EvalGlobal)
				vm.Close()
				if err != nil {
					t.Fatal(err)
				}
			}
			runtime.GC()
			runtime.GC()
			runtime.ReadMemStats(&after)
			kept := int64(after.HeapAlloc) - int64(before.HeapAlloc)
			t.Logf("%d VMs closed, the heap kept %d KiB", n, kept>>10)
			if kept > 8<<20 {
				t.Fatalf("the caches keep %d MiB after the VMs closed", kept>>20)
			}
		})
	}
}

// The strings Temporal writes are bounded whatever the value: a year of
// at most six digits, nine decimals of a second, the fields of a valid
// duration, of at most 2**32 years, months or weeks and 2**53 seconds,
// and a time zone and a calendar of the table or an offset, the longest
// of 32 characters. So the door needs no count for them.
const toStringLength = 256

func TestToStringBounded(t *testing.T) {
	vm := newVM(t)
	for _, js := range []string{
		`Temporal.Duration.from({seconds: 9007199254740991, nanoseconds: 999999999}).toString()`,
		`Temporal.Duration.from({years: -4294967295, months: -4294967295, weeks: -4294967295, days: -104249991374, hours: -1, milliseconds: -1}).toString({fractionalSecondDigits: 9})`,
		`Temporal.Duration.from({milliseconds: 9007199254740991, microseconds: 9007199254740991, nanoseconds: 9007199254740991}).toString()`,
		`Temporal.Duration.from({seconds: 9007199254740991}).toLocaleString('hi-u-nu-adlm', {style: 'long'})`,
		`Temporal.PlainDateTime.from('-271821-04-20T00:00:00.123456789[u-ca=islamic-umalqura]').toString({fractionalSecondDigits: 9, calendarName: 'critical'})`,
		`new Temporal.ZonedDateTime(-8640000000000000000000n, 'America/Argentina/ComodRivadavia', 'islamic-umalqura').toString({fractionalSecondDigits: 9, calendarName: 'critical', timeZoneName: 'critical', offset: 'auto'})`,
		`Temporal.Instant.fromEpochNanoseconds(8640000000000000000000n).toString({timeZone: '-23:59', fractionalSecondDigits: 9})`,
		`Temporal.PlainMonthDay.from({monthCode: 'M05L', day: 30, calendar: 'chinese'}).toString({calendarName: 'critical'})`,
		`Temporal.PlainYearMonth.from({year: 275700, month: 13, calendar: 'ethiopic'}).toString({calendarName: 'critical'})`,
		`Temporal.PlainDate.from('+275760-09-13').toLocaleString('hi-u-nu-adlm', {era: 'long', year: 'numeric', month: 'long', day: 'numeric', weekday: 'long', calendar: 'ethiopic'})`,
	} {
		got, err := vm.Eval(js, quickjs.EvalGlobal)
		if err != nil {
			t.Errorf("%s: %v", js, err)
			continue
		}
		if s, _ := got.(string); s == "" || len(s) > toStringLength {
			t.Errorf("%s = %q, longer than %d", js, got, toStringLength)
		}
	}
}
