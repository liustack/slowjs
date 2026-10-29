// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package slowjs // import "github.com/liustack/slowjs"

// A built-in that works on a long input in one call must read the interrupt
// flag as it goes. If it does not, the call runs to its end, or to the kill,
// which ends the VM instead of the call: the Interrupt did not stop the
// script, the kill did.

import (
	"errors"
	"testing"
	"time"
)

// builtinStopPolls is how many polls may see the request to stop before a
// built-in that reads the flag has stopped. Each call below takes at least
// builtinCallPolls, so one that does not read the flag runs on for half of
// that on average.
const (
	builtinStopPolls = 16
	builtinCallPolls = 40 * builtinStopPolls
)

// TestInterruptStopsLongBuiltins runs one call of a built-in over and over and
// interrupts it, at a different place each time. The stop is measured in the
// engine's polls, not on the clock, so a machine that does not run the
// thread for a while does not fail it. The input is cheap to mark and to
// free, made before the loop and kept alive, and the loop makes nothing that
// the engine would collect, so that the stop is the built-in reading the
// flag: what a large heap costs when a script stops is TestKill's and the
// kill's business.
func TestInterruptStopsLongBuiltins(t *testing.T) {
	cases := []struct{ name, setup, call, gap string }{
		// The first replace of a VM takes the generic path, which calls exec and so reads the flag. The
		// rest, after one has run, take the one that matches in a loop of its own.
		{"replace", `'a'.replace(/a/g, 'b'); var s = 'ab'.repeat(1 << 22)`, `s.replace(/a/g, 'cc')`, ""},
		{"fill", `var a = new Array(1 << 22).fill(1)`, `a.fill(0)`, ""},
		// An array-like object: join reads each index as it would an
		// Array's, and there are no elements to mark.
		{"join", `var a = { length: 1 << 23 }`, `Array.prototype.join.call(a, ',')`, ""},
		// normalize reads the flag in its three passes and as it builds the
		// result, but JS_ToUTF32String (engine/quickjs.c), which turns the
		// input into code points first, reads none: an Interrupt there runs on
		// for up to half the call, 650 polls of 1300 here.
		{"normalize", `var s = 'éå'.repeat(1 << 21)`, `s.normalize('NFC')`, "JS_ToUTF32String reads no flag"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if c.gap != "" {
				t.Skip("the engine does not stop it in time yet: " + c.gap)
			}
			for trial := range 3 {
				m := newTestVM(t)
				if _, err := m.Eval(c.setup, EvalGlobal); err != nil {
					t.Fatal(err)
				}
				before := m.h.Counts().Polls
				if _, err := m.Eval(c.call, EvalGlobal); err != nil {
					t.Fatal(err)
				}
				if n := m.h.Counts().Polls - before; n < builtinCallPolls {
					t.Fatalf("one call took %d polls, want at least %d", n, builtinCallPolls)
				} else if trial == 0 {
					t.Logf("one call took %d polls", n)
				}
				m.RunGC()
				done := make(chan error, 1)
				go func() {
					_, err := m.Eval(`for (;;) { `+c.call+` }`, EvalGlobal)
					done <- err
				}()
				// A different place of the call each time.
				time.Sleep(50*time.Millisecond + time.Duration(trial)*37*time.Millisecond)
				m.Interrupt()
				err := <-done
				var e *Error
				switch polls := m.h.StopPolls(); {
				case errors.Is(err, ErrKilled):
					t.Errorf("trial %d: the script did not stop and was killed, %d polls after Interrupt", trial, polls)
				case !errors.As(err, &e) || !e.Interrupted:
					t.Errorf("trial %d: %v", trial, err)
				case polls > builtinStopPolls:
					t.Errorf("trial %d: stopped %d polls after Interrupt, want at most %d", trial, polls, builtinStopPolls)
				default:
					t.Logf("trial %d: stopped %d polls after Interrupt", trial, polls)
				}
			}
		})
	}
}
