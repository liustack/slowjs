// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package intl // import "github.com/liustack/quickjs/intl"

import (
	"errors"
	"runtime"
	"strconv"
	"testing"

	"github.com/liustack/quickjs"
	"github.com/liustack/quickjs/internal/vmhook"
)

// A Builtin of Intl that does work in Go in proportion to a long input reads
// vmhook.Stop between pieces of at most workPerStop units of that input, or
// refuses the input with a RangeError. It never runs the whole input with no
// check, and between two checks it allocates in Go no more than the VM has
// room for. These tests count the checks and what Go allocated between two
// of them. They read no clock (docs/testing.md, "Time in a test").

// workPerStop is the most UTF-16 units of input a Builtin works on between
// two reads of vmhook.Stop.
const workPerStop = 1 << 16

// stopSpans is what measureStops saw of a call: how many times it read
// vmhook.Stop, and the most Go allocated from the start of the call to the
// first read, between two reads, or from the last read to the end.
type stopSpans struct {
	stops    int
	maxAlloc uint64
}

// measureStops evaluates src on vm with vmhook.Stop counted. The tests that
// call it do not run in parallel, so no other call reads vmhook.Stop.
func measureStops(t *testing.T, vm *quickjs.VM, src string) (stopSpans, error) {
	t.Helper()
	var s stopSpans
	var ms runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&ms)
	last := ms.TotalAlloc
	span := func() {
		runtime.ReadMemStats(&ms)
		s.maxAlloc = max(s.maxAlloc, ms.TotalAlloc-last)
		last = ms.TotalAlloc
	}
	stop := vmhook.Stop
	vmhook.Stop = func(vm any) bool {
		s.stops++
		span()
		return stop(vm)
	}
	defer func() { vmhook.Stop = stop }()
	_, err := vm.Eval("(function () { "+src+"; })(); 0", quickjs.EvalGlobal)
	span()
	return s, err
}

// isRangeError reports whether err is a RangeError a script can catch.
func isRangeError(err error) bool {
	var e *quickjs.Error
	return errors.As(err, &e) && !e.Uncatchable && e.Name == "RangeError"
}

// longWork are calls whose work in Go grows with an input of about N units,
// set up by the first string and made by the second.
var longWork = []struct{ name, setup, call string }{
	{"localeCompare after a shared run of accents",
		`var a = 'a' + '́'.repeat(N) + '̀', b = 'a' + '́'.repeat(N) + '̂'`,
		`a.localeCompare(b, 'en')`},
	{"Collator.compare of two long strings",
		`var a = 'ab'.repeat(N / 2) + 'c', b = 'ab'.repeat(N / 2) + 'd', c = new Intl.Collator('en')`,
		`c.compare(a, b)`},
	{"Segmenter, the word containing 0 in Thai",
		`var s = 'สวัสดีครับ'.repeat(N / 10), g = new Intl.Segmenter('th', {granularity: 'word'})`,
		`g.segment(s).containing(0).segment`},
	{"toLocaleUpperCase in Turkish",
		`var s = 'istanbul '.repeat(N / 9)`,
		`s.toLocaleUpperCase('tr')`},
	{"toLocaleLowerCase in Turkish of I and marks",
		`var s = 'I' + '̣'.repeat(N)`,
		`s.toLocaleLowerCase('tr')`},
	{"toLocaleUpperCase in Greek",
		`var s = 'άέήί '.repeat(N / 5)`,
		`s.toLocaleUpperCase('el')`},
	{"toLocaleLowerCase in Lithuanian",
		`var s = 'ÌÍĨ '.repeat(N / 4)`,
		`s.toLocaleLowerCase('lt')`},
	{"getCanonicalLocales of one long tag",
		`var s = 'en-x-' + 'abcdefgh-'.repeat(N / 9) + 'a'`,
		`Intl.getCanonicalLocales(s)`},
	{"DurationFormat of a duration string of zeros",
		`var s = 'PT' + '0'.repeat(N) + '1S', df = new Intl.DurationFormat('en')`,
		`df.format(s)`},
	{"NumberFormat.format of a long numeric string",
		`var s = '1'.repeat(N), nf = new Intl.NumberFormat('en')`,
		`nf.format(s)`},
}

// waiting holds the calls of longWork that still run whole in Go, with no
// check or no count of the room, each until the change that cuts its work
// into pieces. A test skips them, and that change takes its call out.
var waiting = map[string]bool{
	"Collator.compare of two long strings": true,
}

// skipWaiting skips the call name of longWork while it waits for its change.
func skipWaiting(t *testing.T, name string) {
	t.Helper()
	if waiting[name] {
		t.Skip("still runs whole in Go, with no check")
	}
}

// holdWhole holds the calls of longWork that keep their whole input in Go,
// as arrays as long as it, while they check the call and count those arrays
// against the room: a compare keeps the code points and the collation
// elements of its strings, a Segments object its text in UTF-16, and a case
// mapping the code points of its string before and after. They check the
// call between pieces of the work, but what they hold between two checks
// grows with the input, which TestLongWorkHeldPerPiece skips them for.
var holdWhole = map[string]bool{
	"localeCompare after a shared run of accents": true,
	"Collator.compare of two long strings":        true,
	"Segmenter, the word containing 0 in Thai":    true,
	"toLocaleUpperCase in Turkish":                true,
	"toLocaleLowerCase in Turkish of I and marks": true,
	"toLocaleUpperCase in Greek":                  true,
	"toLocaleLowerCase in Lithuanian":             true,
}

// setUp makes the input of setup, of n units, in vm.
func setUp(t *testing.T, vm *quickjs.VM, setup string, n int) {
	t.Helper()
	eval(t, vm, "var N = "+strconv.Itoa(n)+"; "+setup+"; 0")
}

// Each long call reads vmhook.Stop at least once per workPerStop units of
// its input, or refuses the input with a RangeError. With no check, the
// eval timeout and Interrupt wait for the end of the work: localeCompare
// after 8 Mi accents ran 2.1 s past a timeout of 20 ms.
func TestLongWorkStops(t *testing.T) {
	const n = 2 << 20
	for _, c := range longWork {
		t.Run(c.name, func(t *testing.T) {
			skipWaiting(t, c.name)
			vm := newVM(t)
			setUp(t, vm, c.setup, n)
			s, err := measureStops(t, vm, c.call)
			switch {
			case isRangeError(err):
			case err != nil:
				t.Fatalf("%s: %v", c.call, err)
			case s.stops < n/workPerStop:
				t.Errorf("%s read vmhook.Stop %d times for %d units, want at least %d", c.call, s.stops, n, n/workPerStop)
			}
		})
	}
}

// Between two reads of vmhook.Stop, a long call allocates in Go no more than
// the room the VM has under its memory limit: what Go allocates is outside
// the limit, and the call has to stop before it takes more. localeCompare
// after 8 Mi accents allocated 376 MiB in Go in a VM limited to 256 MiB.
func TestLongWorkRoom(t *testing.T) {
	const n = 2 << 20
	const room = 16 << 20
	for _, c := range longWork {
		t.Run(c.name, func(t *testing.T) {
			skipWaiting(t, c.name)
			vm := newVM(t)
			setUp(t, vm, c.setup, n)
			runtime.GC()
			vm.SetMemoryLimit(vm.MemorySize() + room)
			s, err := measureStops(t, vm, c.call)
			if err != nil && !isRangeError(err) && !isOutOfMemory(err) {
				t.Fatalf("%s: %v", c.call, err)
			}
			if s.maxAlloc > room {
				t.Errorf("%s allocated %d MiB in Go between two reads of vmhook.Stop, with %d MiB of room", c.call, s.maxAlloc>>20, room>>20)
			}
		})
	}
}

// isOutOfMemory reports whether err ended the call as running out of memory.
func isOutOfMemory(err error) bool {
	var e *quickjs.Error
	return errors.As(err, &e) && e.Uncatchable && e.OutOfMemory
}

// What a long call holds in Go between two reads of vmhook.Stop does not
// grow with its input: it works on a piece at a time and keeps no list as
// long as the input, as Collator.compare kept all the collation elements
// of both strings.
func TestLongWorkHeldPerPiece(t *testing.T) {
	const n = 1 << 20
	for _, c := range longWork {
		t.Run(c.name, func(t *testing.T) {
			skipWaiting(t, c.name)
			if holdWhole[c.name] {
				t.Skip("holds its whole input in Go, counted against the room")
			}
			var at [2]stopSpans
			for i, size := range []int{n, 4 * n} {
				vm := newVM(t)
				setUp(t, vm, c.setup, size)
				s, err := measureStops(t, vm, c.call)
				if isRangeError(err) {
					return
				}
				if err != nil {
					t.Fatalf("%s: %v", c.call, err)
				}
				at[i] = s
			}
			if at[1].maxAlloc > at[0].maxAlloc+at[0].maxAlloc/2+1<<20 {
				t.Errorf("%s allocated %d KiB in Go between two reads of vmhook.Stop for %d units, and %d KiB for %d", c.call, at[0].maxAlloc>>10, n, at[1].maxAlloc>>10, 4*n)
			}
		})
	}
}
