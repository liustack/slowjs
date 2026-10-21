// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package libquickjs // import "github.com/liustack/quickjs/lib"

import (
	"strconv"
	"testing"
	"time"

	"modernc.org/libc"
)

// processStart is about when the process started: the package of the
// engine and its clock are initialized just before it.
var processStart = time.Now()

// newClockContext returns a context with the helpers of quickjs-libc,
// performance.now among them.
func newClockContext(t *testing.T) (*libc.TLS, uintptr) {
	t.Helper()
	tls := libc.NewTLS()
	rt := XJS_NewRuntime(tls)
	ctx := XJS_NewContext(tls, rt)
	Xjs_std_add_helpers(tls, ctx, -1, 0)
	t.Cleanup(func() {
		XJS_FreeContext(tls, ctx)
		XJS_FreeRuntime(tls, rt)
		tls.Close()
	})
	return tls, ctx
}

// setClock makes the wall clock of the engine read now until the test ends.
func setClock(t *testing.T, now func() time.Time) {
	t.Helper()
	timeNow = now
	t.Cleanup(func() { timeNow = time.Now })
}

// evalNumber evaluates js and returns its value, a number.
func evalNumber(t *testing.T, tls *libc.TLS, ctx uintptr, js string) float64 {
	t.Helper()
	s := eval1(t, tls, ctx, js)
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		t.Fatalf("%s = %s, not a number", js, s)
	}
	return f
}

// Date.now, new Date() and Date() read the wall clock of time.Now, in
// whole milliseconds since 1970, rounded down as gettimeofday's are.
func TestDateNowReadsTimeNow(t *testing.T) {
	for _, c := range []struct {
		now  time.Time
		want string
	}{
		{time.Unix(1_700_000_000, 123_456_789), "1700000000123"},
		{time.Unix(0, 999_999), "0"},
		{time.Unix(-1, 500_000_000), "-500"},
		{time.Unix(-1, 999), "-1000"},
	} {
		setClock(t, func() time.Time { return c.now })
		tls, ctx := newClockContext(t)
		for _, js := range []string{
			`String(Date.now())`,
			`String(new Date().getTime())`,
			`String(+new Date())`,
		} {
			if got := eval1(t, tls, ctx, js); got != c.want {
				t.Errorf("at %v: %s = %s, want %s", c.now, js, got, c.want)
			}
		}
		js := `String(Date() === new Date(` + c.want + `).toString())`
		if got := eval1(t, tls, ctx, js); got != "true" {
			t.Errorf("at %v: %s = %s", c.now, js, got)
		}
	}
}

// Date.now agrees with Go's time.Now, and counts whole milliseconds.
func TestDateNowAgreesWithGo(t *testing.T) {
	tls, ctx := newClockContext(t)
	for _, js := range []string{`Date.now()`, `new Date().getTime()`} {
		before := time.Now().UnixMilli()
		got := evalNumber(t, tls, ctx, js)
		after := time.Now().UnixMilli()
		if got < float64(before) || got > float64(after) || got != float64(int64(got)) {
			t.Errorf("%s = %v, want a whole number in [%d, %d]", js, got, before, after)
		}
	}
}

// A new context seeds Math.random with the time in microseconds since
// 1970, and with 1 for a seed of 0, on which the generator would only
// return 0.
func TestMathRandomSeed(t *testing.T) {
	first := func(now time.Time) string {
		setClock(t, func() time.Time { return now })
		tls, ctx := newClockContext(t)
		return eval1(t, tls, ctx, `Math.random() + ' ' + Math.random()`)
	}
	at := time.Unix(1_700_000_000, 123_456_000)
	if a, b := first(at), first(at.Add(999*time.Nanosecond)); a != b {
		t.Errorf("two contexts made in the same microsecond: Math.random gives %s and %s", a, b)
	}
	if a, b := first(at), first(at.Add(time.Microsecond)); a == b {
		t.Errorf("two contexts made a microsecond apart: Math.random gives %s in both", a)
	}
	if a, b := first(time.Unix(0, 0)), first(time.Unix(0, 1000)); a != b || a == "0 0" {
		t.Errorf("seeds 0 and 1: Math.random gives %s and %s, want the same, not 0 0", a, b)
	}
}

// performance.now (through get_time_ns) reads a monotonic clock in
// milliseconds with a fraction: it goes on when the wall clock stands still, never goes
// back, keeps pace with Go's monotonic clock, and counts from the start
// of the process, as in Node. Before it read the engine's own clocks it
// counted from the boot of the system, or from 1970 on Windows and
// FreeBSD.
func TestMonotonicClock(t *testing.T) {
	wall := time.Now()
	setClock(t, func() time.Time { return wall })
	tls, ctx := newClockContext(t)

	p := evalNumber(t, tls, ctx, `performance.now()`)
	if max := float64(time.Since(processStart))/1e6 + 1; p > max {
		t.Errorf("performance.now() = %vms, the process started %vms ago", p, max-1)
	}

	g0 := time.Now()
	p0 := evalNumber(t, tls, ctx, `performance.now()`)
	time.Sleep(20 * time.Millisecond)
	p1 := evalNumber(t, tls, ctx, `performance.now()`)
	g1 := time.Now()
	const slack = 1 // ms, the two clocks may tick at slightly different rates
	if d, max := p1-p0, float64(g1.Sub(g0))/1e6; d < 20-slack || d > max+slack {
		t.Errorf("over a sleep of 20ms with the wall clock stopped: performance.now went %vms, Go's monotonic clock %vms", d, max)
	}

	if got := eval1(t, tls, ctx, `
		(() => {
			let last = performance.now(), frac = false;
			for (let i = 0; i < 10000; i++) {
				const t = performance.now();
				if (t < last) return 'back from ' + last + ' to ' + t;
				if (t !== Math.floor(t)) frac = true;
				last = t;
			}
			return frac ? 'ok' : 'whole milliseconds only';
		})()`); got != "ok" {
		t.Errorf("10000 reads of performance.now: %s", got)
	}

	m0 := monoNanos()
	ns := _get_time_ns(tls)
	m1 := monoNanos()
	if ns < m0 || ns > m1 {
		t.Errorf("monoNanos %d, get_time_ns %d, monoNanos %d: not the same clock", m0, ns, m1)
	}
}
