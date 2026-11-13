// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package temporal

import (
	"errors"
	"testing"

	"github.com/liustack/quickjs"
	"github.com/liustack/quickjs/internal/stopsweep"
	"github.com/liustack/quickjs/internal/vmhook"
)

// The arithmetic of Temporal in a calendar other than ISO 8601 goes a month
// or a year at a time, and a duration of millions of days makes millions of
// turns. It reads vmhook.Stop at least once every turnsPerStop turns, or
// refuses the duration with a RangeError. The test counts the reads and
// reads no clock (docs/testing.md, "Time in a test").

// turnsPerStop is the most turns, months or years, the arithmetic of a
// calendar goes through between two reads of vmhook.Stop.
const turnsPerStop = 4096

// countStops evaluates src on vm and returns how many times it read
// vmhook.Stop. The tests that call it do not run in parallel, so no other
// call reads vmhook.Stop.
func countStops(t *testing.T, vm *quickjs.VM, src string) (int, error) {
	t.Helper()
	n := 0
	stop := vmhook.Stop
	vmhook.Stop = func(vm any) bool {
		n++
		return stop(vm)
	}
	defer func() { vmhook.Stop = stop }()
	_, err := vm.Eval("(function () { "+src+"; })(); 0", quickjs.EvalGlobal)
	return n, err
}

func isRangeError(err error) bool {
	var e *quickjs.Error
	return errors.As(err, &e) && !e.Uncatchable && e.Name == "RangeError"
}

// Each call goes through millions of months or hundreds of thousands of
// years, which took 0.1 to 0.4 seconds in Go with no check: a date of the
// Chinese calendar plus 190 million days ran 64 ms past an eval timeout of
// 20 ms.
func TestCalendarArithmeticStops(t *testing.T) {
	// The loops go over months when the call counts days, and over years when
	// it counts months: 190 million days are at least 6,129,032 months, of 31
	// days at most, 6.7 million months at least 515,384 years, of 13 months at
	// most, and the years from -271000 to 275000 are 546,000.
	const monthsOfDays, yearsOfMonths, yearsBetween = 190_000_000 / 31, 6_700_000 / 13, 546_000
	const setup = `
		var first = Temporal.PlainDate.from({year: -271000, month: 1, day: 1, calendar: 'chinese'});
		var last = Temporal.PlainDate.from({year: 275000, month: 1, day: 1, calendar: 'chinese'});
		var hebrew = Temporal.PlainDate.from({year: -268000, month: 1, day: 1, calendar: 'hebrew'});
		var dangi = Temporal.PlainDateTime.from({year: -271000, month: 1, day: 1, calendar: 'dangi'});
		var zfirst = Temporal.ZonedDateTime.from('-271000-01-01T00:00[UTC]').withCalendar('chinese');
		var zlast = Temporal.ZonedDateTime.from('+275000-01-01T00:00[UTC]').withCalendar('chinese');
		0`
	for _, c := range []struct {
		call  string
		turns int
	}{
		{`first.add({days: 1.9e8})`, monthsOfDays},
		{`first.add({months: 6.7e6})`, yearsOfMonths},
		{`last.subtract({days: 1.9e8})`, monthsOfDays},
		{`first.until(last, {largestUnit: 'months'})`, yearsBetween},
		{`hebrew.add({days: 1.9e8})`, monthsOfDays},
		{`dangi.add({days: 1.9e8})`, monthsOfDays},
		{`Temporal.Duration.from({months: 6.7e6}).total({unit: 'days', relativeTo: first})`, yearsOfMonths},
		{`Temporal.Duration.from({days: 1.9e8}).total({unit: 'months', relativeTo: first})`, monthsOfDays},
		{`Temporal.Duration.from({days: 1.9e8}).round({largestUnit: 'months', relativeTo: first})`, monthsOfDays},
		{`zfirst.until(zlast, {largestUnit: 'months'})`, yearsBetween},
	} {
		t.Run(c.call, func(t *testing.T) {
			vm := newVM(t)
			eval(t, vm, setup)
			n, err := countStops(t, vm, c.call)
			switch {
			case isRangeError(err):
			case err != nil:
				t.Fatalf("%s: %v", c.call, err)
			case n < c.turns/turnsPerStop:
				t.Errorf("%s read vmhook.Stop %d times for at least %d turns, want at least %d", c.call, n, c.turns, c.turns/turnsPerStop)
			}
		})
	}
}

// Each call of the arithmetic of a calendar stops within the grace of
// stopsweep at every eval timeout from 2 ms to the time it takes whole, so
// that a part of it with no check shows wherever it lies.
func TestCalendarArithmeticStopsAtEveryTimeout(t *testing.T) {
	if testing.Short() {
		t.Skip("times each call at many timeouts")
	}
	const setup = `
		var first = Temporal.PlainDate.from({year: -271000, month: 1, day: 1, calendar: 'chinese'});
		var last = Temporal.PlainDate.from({year: 275000, month: 1, day: 1, calendar: 'chinese'});
		var hebrew = Temporal.PlainDate.from({year: -268000, month: 1, day: 1, calendar: 'hebrew'});
		var islamic = Temporal.PlainDate.from({year: -271000, month: 1, day: 1, calendar: 'islamic-civil'});
		var zfirst = Temporal.ZonedDateTime.from('-271000-01-01T00:00[UTC]').withCalendar('chinese');
		var zlast = Temporal.ZonedDateTime.from('+275000-01-01T00:00[UTC]').withCalendar('chinese');
		0`
	const monthsOfDays, yearsOfMonths, yearsBetween = 190_000_000 / 31, 6_700_000 / 13, 546_000
	for _, c := range []struct {
		call  string
		turns int
	}{
		{`first.add({days: 1.9e8}).year`, monthsOfDays},
		{`first.add({months: 6.7e6}).year`, yearsOfMonths},
		{`first.until(last, {largestUnit: 'months'}).months`, yearsBetween},
		{`hebrew.add({days: 1.9e8}).year`, monthsOfDays},
		{`islamic.add({days: 1.9e8}).year`, monthsOfDays},
		{`Temporal.Duration.from({days: 1.9e8}).total({unit: 'months', relativeTo: first})`, monthsOfDays},
		{`Temporal.Duration.from({days: 1.9e8}).round({largestUnit: 'months', relativeTo: first}).months`, monthsOfDays},
		{`zfirst.until(zlast, {largestUnit: 'months'}).months`, yearsBetween},
		{`first.toPlainYearMonth().add({months: 6.7e6}).year`, yearsOfMonths},
	} {
		t.Run(c.call, func(t *testing.T) {
			vm := newVM(t)
			eval(t, vm, setup)
			stopsweep.Sweep(t, vm, c.call, c.turns, turnsPerStop)
		})
	}
}
