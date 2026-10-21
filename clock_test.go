// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package quickjs // import "github.com/liustack/quickjs"

import (
	"math"
	"sync/atomic"
	"testing"
	"time"
	"unsafe"
)

// wallShifted returns t with its wall clock reading moved by d, a whole
// number of seconds, and its monotonic reading left alone: what time.Now
// returns after the system clock was set while the process ran. It writes
// the seconds field of time.Time (see the time package source).
// checkWallShifted checks that it does what it means to.
func wallShifted(t time.Time, d time.Duration) time.Time {
	type timeLayout struct {
		wall uint64
		ext  int64
		loc  *time.Location
	}
	const secShift = 30 // the seconds above the 30 bits of nanoseconds
	(*timeLayout)(unsafe.Pointer(&t)).wall += uint64(int64(d/time.Second) << secShift)
	return t
}

func checkWallShifted(tb testing.TB, d time.Duration) {
	tb.Helper()
	t := time.Now()
	s := wallShifted(t, d)
	if s.UnixNano()-t.UnixNano() != int64(d) || s.Sub(t) != 0 {
		tb.Fatalf("the layout of time.Time changed: shifted %v, the wall clock moved %v, the monotonic clock %v", d, time.Duration(s.UnixNano()-t.UnixNano()), s.Sub(t))
	}
}

// The eval timeout runs on the monotonic clock. The system clock can be set
// back or forward while a script runs (NTP, a person, a VM resumed): set
// back, a deadline on the wall clock fired that much later, or never, and
// set forward it fired at once.
func TestEvalTimeoutIgnoresWallClock(t *testing.T) {
	for _, jump := range []time.Duration{-time.Hour, time.Hour} {
		t.Run(jump.String(), func(t *testing.T) {
			checkWallShifted(t, jump)
			var shift atomic.Int64
			timeNow = func() time.Time { return wallShifted(time.Now(), time.Duration(shift.Load())) }
			t.Cleanup(func() { timeNow = time.Now })

			m := newTestVM(t)
			if err := m.RegisterFunc("jump", func() { shift.Store(int64(jump)) }, false); err != nil {
				t.Fatal(err)
			}

			const timeout = 200 * time.Millisecond
			m.SetEvalTimeout(timeout)
			start := time.Now()
			done := make(chan error, 1)
			go func() {
				_, err := m.Eval(`jump(); for (;;);`, EvalGlobal)
				done <- err
			}()
			var err error
			select {
			case err = <-done:
			case <-time.After(5 * time.Second):
				m.Interrupt()
				<-done
				t.Fatalf("the clock went %v: the timeout of %v did not fire in 5s", jump, timeout)
			}
			elapsed := time.Since(start)
			wantError(t, err, "InternalError", "interrupted")
			if elapsed < timeout {
				t.Fatalf("the clock went %v: the timeout of %v fired after %v", jump, timeout, elapsed)
			}
		})
	}
}

// monoNow moves with the monotonic clock only.
func TestMonoNowIgnoresWallClock(t *testing.T) {
	for _, jump := range []time.Duration{-time.Hour, time.Hour} {
		checkWallShifted(t, jump)
		var shift time.Duration
		timeNow = func() time.Time { return wallShifted(time.Now(), shift) }
		t.Cleanup(func() { timeNow = time.Now })
		before := monoNow()
		shift = jump
		after := monoNow()
		if d := time.Duration(after - before); d < 0 || d > time.Second {
			t.Fatalf("the wall clock went %v: monoNow moved %v", jump, d)
		}
	}
}

// A timeout further away than the clock reaches is no timeout at all. The
// deadline used to wrap around to the past, so the call was interrupted at
// once.
func TestHugeEvalTimeout(t *testing.T) {
	for _, d := range []time.Duration{math.MaxInt64, math.MaxInt64 - time.Second, 290 * 365 * 24 * time.Hour} {
		m := newTestVM(t)
		m.SetEvalTimeout(d)
		if _, err := m.Eval(`let s = 0; for (let i = 0; i < 1e6; i++) s += i; s`, EvalGlobal); err != nil {
			t.Fatalf("timeout %v: %v", d, err)
		}
	}
}
