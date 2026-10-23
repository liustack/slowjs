// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package intl // import "github.com/liustack/slowjs/intl"

import (
	"encoding/json"
	"testing"

	"github.com/liustack/slowjs/internal/intlhook"
)

// TestTemporalHook formats through intlhook.Format as package temporal
// does, with the options of each Temporal type, and checks the errors of
// ECMA-402's calendar check and of a type no option suits.
func TestTemporalHook(t *testing.T) {
	var list struct {
		Names []string
		Kinds string
	}
	if err := json.Unmarshal([]byte(intlhook.Options), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Names) != len(list.Kinds) || len(list.Names) == 0 {
		t.Fatalf("intlhook.Options: %s", intlhook.Options)
	}

	vm := newVM(t, TimeZone("Asia/Tokyo"))
	f, err := vm.NewBuiltinFunction(intlhook.Format)
	if err != nil {
		t.Fatal(err)
	}

	defer f.Free()

	atom, err := vm.NewAtom("format")
	if err != nil {
		t.Fatal(err)
	}
	if err := vm.SetPropertyValue(vm.GlobalObject(), atom, f); err != nil {
		t.Fatal(err)
	}
	names, err := json.Marshal(list.Names)
	if err != nil {
		t.Fatal(err)
	}
	// opts lists the options in the order of intlhook.Options.
	eval(t, vm, `var names = `+string(names)+`;
		function opts(o) { return names.map(n => o[n]); }`)
	jan5 := `Date.UTC(2024, 0, 5, 12)`
	evalTests(t, vm, [][2]string{
		{`format('date', 'iso8601', ` + jan5 + `, undefined, undefined, 'en-US', undefined, ...opts({}))`, "1/5/2024"},
		{`format('date', 'gregory', ` + jan5 + `, undefined, undefined, 'de', undefined, ...opts({dateStyle: 'long'}))`, "5. Januar 2024"},
		{`format('date', 'iso8601', ` + jan5 + `, undefined, undefined, undefined, '2:ja5:en-US', ...opts({}))`, "2024/1/5"},
		{`format('time', 'iso8601', 13 * 3600e3 + 5 * 60e3, undefined, undefined, 'en-US', undefined, ...opts({}))`, "1:05:00 PM"},
		{`format('yearMonth', 'gregory', ` + jan5 + `, undefined, undefined, 'en-US', undefined, ...opts({}))`, "1/2024"},
		{`format('monthDay', 'gregory', ` + jan5 + `, undefined, undefined, 'en-US', undefined, ...opts({}))`, "1/5"},
		// An instant takes the zone Register was given, a zoned value its own.
		{`format('instant', 'iso8601', 0, undefined, undefined, 'en-US', undefined, ...opts({}))`, "1/1/1970, 9:00:00 AM"},
		{`format('zoned', 'iso8601', 0, 'America/New_York', -5 * 3600e9, 'en-US', undefined, ...opts({}))`, "12/31/1969, 7:00:00 PM EST"},
		// A zoned value at another offset than Go's data has for its zone
		// is shown at that offset, under the offset's name.
		{`format('zoned', 'iso8601', 0, 'America/New_York', -4 * 3600e9, 'en-US', undefined, ...opts({}))`, "12/31/1969, 8:00:00 PM GMT-4"},
		{`try { format('yearMonth', 'iso8601', 0, undefined, undefined, 'en-US', undefined, ...opts({})) } catch (e) { e.name + ': ' + e.message }`,
			"RangeError: Mismatched calendars."},
		{`try { format('date', 'hebrew', 0, undefined, undefined, 'en-US', undefined, ...opts({})) } catch (e) { e.name + ': ' + e.message }`,
			"RangeError: Mismatched calendars."},
		{`try { format('date', 'iso8601', 0, undefined, undefined, 'en-US', undefined, ...opts({timeStyle: 'short'})) } catch (e) { e.name + ': ' + e.message }`,
			"TypeError: Invalid option : timeStyle"},
		{`try { format('time', 'iso8601', 0, undefined, undefined, 'en-US', undefined, ...opts({year: 'numeric'})) } catch (e) { e.name + ': ' + e.message }`,
			"TypeError: Invalid argument for Temporal [object Temporal.PlainTime]"},
		{`try { format('zoned', 'iso8601', 0, 'UTC', 0, 'en-US', undefined, ...opts({timeZone: 'UTC'})) } catch (e) { e.name + ': ' + e.message }`,
			"TypeError: Invalid time zone specified: UTC"},
	})
}
