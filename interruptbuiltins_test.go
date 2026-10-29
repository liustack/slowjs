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

// builtinTrials is how many places of a call TestInterruptStopsLongBuiltins
// interrupts, spread evenly over it: a part of a call that does not read the
// flag and is longer than 1/builtinTrials of it is hit every time.
const builtinTrials = 6

// TestInterruptStopsLongBuiltins runs one call of a built-in over and over and
// interrupts it, at places spread over the second call of the loop. Both the
// place and the stop are measured in the engine's polls, not on the clock:
// the polls of a call are the same on any machine, and one that does not run
// the thread for a while does not fail it. The Interrupt comes only once the
// loop has polled, so the script has entered and the Interrupt is not
// cleared by the entry. The input is cheap to mark and to free, made before
// the loop and kept alive, and the loop makes nothing that the engine would
// collect, so that the stop is the built-in reading the flag: what a large
// heap costs when a script stops is TestKill's and the kill's business.
func TestInterruptStopsLongBuiltins(t *testing.T) {
	cases := []struct{ name, setup, call string }{
		// The first replace of a VM takes the generic path, which calls exec and so reads the flag. The
		// rest, after one has run, take the one that matches in a loop of its own.
		{"replace", `'a'.replace(/a/g, 'b'); var s = 'ab'.repeat(1 << 22)`, `s.replace(/a/g, 'cc')`},
		{"fill", `var a = new Array(1 << 22).fill(1)`, `a.fill(0)`},
		// An array-like object: join reads each index as it would an
		// Array's, and there are no elements to mark.
		{"join", `var a = { length: 1 << 23 }`, `Array.prototype.join.call(a, ',')`},
		{"normalize", `var s = 'éå'.repeat(1 << 21)`, `s.normalize('NFC')`},
		// localeCompare normalizes both strings, as normalize does, then
		// compares them.
		{"localeCompare", `var s = 'éå'.repeat(1 << 20), t = s + 'x'`, `s.localeCompare(t)`},
		{"toUpperCase", `var s = 'aé'.repeat(1 << 21)`, `s.toUpperCase()`},
		{"toLowerCase", `var s = 'AÉ'.repeat(1 << 21)`, `s.toLowerCase()`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := newTestVM(t)
			if _, err := m.Eval(c.setup, EvalGlobal); err != nil {
				t.Fatal(err)
			}
			before := m.h.Polls()
			if _, err := m.Eval(c.call, EvalGlobal); err != nil {
				t.Fatal(err)
			}
			call := m.h.Polls() - before
			if call < builtinCallPolls {
				t.Fatalf("one call took %d polls, want at least %d", call, builtinCallPolls)
			}
			t.Logf("one call took %d polls", call)
			for trial := range builtinTrials {
				m.RunGC()
				// The second call of the loop, at (trial+1/2)/builtinTrials
				// of it.
				at := call + (2*int64(trial)+1)*call/(2*builtinTrials)
				begin := m.h.Polls()
				done := make(chan error, 1)
				go func() {
					_, err := m.Eval(`for (;;) { `+c.call+` }`, EvalGlobal)
					done <- err
				}()
				for m.h.Polls()-begin < at {
					select {
					case err := <-done:
						t.Fatalf("trial %d: the loop ended before the Interrupt: %v", trial, err)
					default:
						time.Sleep(20 * time.Microsecond)
					}
				}
				m.Interrupt()
				err := <-done
				var e *Error
				switch polls := m.h.StopPolls(); {
				case errors.Is(err, ErrKilled):
					t.Fatalf("trial %d: the script did not stop and was killed, %d polls after Interrupt", trial, polls)
				case !errors.As(err, &e) || !e.Interrupted:
					t.Fatalf("trial %d: %v", trial, err)
				case polls > builtinStopPolls:
					t.Errorf("trial %d: stopped %d polls after Interrupt at poll %d of the loop, want at most %d", trial, polls, at, builtinStopPolls)
				default:
					t.Logf("trial %d: stopped %d polls after Interrupt at poll %d of the loop", trial, polls, at)
				}
			}
		})
	}
}
