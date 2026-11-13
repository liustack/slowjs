// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package whatwgurl

import (
	"runtime"
	"runtime/debug"
	"strings"
	"testing"
	"time"
)

// mustInput is the Input of s, which the test knows is not too long.
func mustInput(s string) Input {
	in, ok := NewInput(s)
	if !ok {
		panic("whatwgurl test: an input longer than MaxUnits")
	}
	return in
}

// parse is Parse of a string.
func parse(s string, base *URL) (*URL, error) { return Parse(mustInput(s), base) }

// fill is prefix, unit repeated, and suffix, as long as fits in n UTF-16 units,
// and false when not one unit fits.
func fill(prefix, unit, suffix string, n int) (string, bool) {
	room := n - units(prefix) - units(suffix)
	if unit == "" || room < units(unit) {
		return "", false
	}
	return prefix + strings.Repeat(unit, room/units(unit)) + suffix, true
}

// callBound is the longest a call may take: the engine aims to look at its
// stop flag every 12.5 ms, a quarter of the grace it gives a call that is to
// end, and a call of Go is between two looks. It is scaled by slowness.
const callBound = 12500 * time.Microsecond

// slowness is how many times slower than usual the code of this package runs
// now, 1 for the usual: the fuzzer builds it with counters in every loop, the
// race detector slows it, and a loaded machine runs it at a share of a core,
// which changes from one moment to the next, so it is measured each time. A
// parse of a path of MaxUnits takes about 0.4 ms usually.
func slowness() time.Duration {
	in := mustInput("http://h/" + strings.Repeat("a", MaxUnits-9))
	best := time.Hour
	for i := 0; i < 5; i++ {
		start := time.Now()
		Parse(in, nil)
		best = min(best, time.Since(start))
	}
	return max(1, best/time.Millisecond)
}

// quickest is the shortest of five runs of f, each after a collection and
// with the collector off, as the collection a large allocation starts is not
// the call's: a gap the machine made is not there the next time, and one the
// code made is.
func quickest(f func()) time.Duration {
	gcPercent := debug.SetGCPercent(-1)
	defer debug.SetGCPercent(gcPercent)
	best := time.Hour
	for try := 0; try < 5; try++ {
		runtime.GC()
		start := time.Now()
		f()
		best = min(best, time.Since(start))
	}
	return best
}

// withinBound fails t when f takes longer than callBound, scaled.
func withinBound(t testing.TB, what string, f func()) {
	t.Helper()
	if d, bound := quickest(f), callBound*slowness(); d > bound {
		t.Errorf("%s took %v, more than %v", what, d, bound)
	}
}
