// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build testhooks

package cengine

// These tests interrupt a script at a set poll (gohost.VM.InterruptAtPoll),
// which only a build with the tag testhooks has: the poll itself asks to
// stop, on the goroutine that runs the engine, so the place in the script is
// the same on any machine and however the goroutines are scheduled.

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// endlessAt is the poll, counted from the start of Eval, at which
// stopsSoftly interrupts an endless script, on top of the polls of a full
// collection: JS_RunGC leaves the threshold of the next collection where it
// was, so the first allocation of the Eval may start one more before the
// script runs.
const endlessAt = 200

// stopsSoftly runs the endless script c, has the poll endlessAt (plus a
// collection) into it ask to stop, and checks the stop (stoppedSoftly),
// with the kill grace given.
func stopsSoftly(t *testing.T, c endlessScript, grace time.Duration) {
	t.Helper()
	v := newVM(t, Options{Seed: 1})
	defer closeVM(t, v)
	v.Host().SetKillGrace(grace)
	if c.setup != "" {
		if _, err := v.Eval(c.setup, "setup.js"); err != nil {
			t.Fatalf("%s: setup: %v", c.name, err)
		}
	}
	v.Host().InterruptAtPoll(v.Host().Polls() + runGC(v) + endlessAt)
	_, err := v.Eval(c.src, "stop.js")
	stoppedSoftly(t, v, c, err)
}

// TestInterrupt: Interrupt stops each endless script with the error
// "interrupted", which the engine throws where it reads the flag the poll
// set, in the part of the script that runs on, and the VM goes on.
func TestInterrupt(t *testing.T) {
	needPolls(t)
	for _, c := range endless {
		stopsSoftly(t, c, -1)
	}
}

// TestNoKillBeforeThePolls: with a grace of 0, Interrupt still stops each
// endless script with the error "interrupted" and kills none: each reads the
// flag, or frees what it made, well within gohost.KillPolls polls of it being
// set.
func TestNoKillBeforeThePolls(t *testing.T) {
	needPolls(t)
	for _, c := range endless {
		stopsSoftly(t, c, 0)
	}
}

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
// interrupts it at places spread evenly over the second call of the loop,
// counted in polls: the polls of a call are the same on any machine, and the
// poll at the place asks to stop itself. Each stop must come within
// builtinStopPolls polls, in the built-in (the top frame of the stack), and
// not by the kill. The stop is measured in polls, not on the clock, so a
// machine that does not run the thread for a while does not fail it. The
// input is cheap to mark and to free, made before the loop and kept alive,
// and the loop makes nothing that the engine would collect, so that the stop
// is the built-in reading the flag: what a large heap costs when a script
// stops is TestKill's and the kill's business.
func TestInterruptStopsLongBuiltins(t *testing.T) {
	needPolls(t)
	cases := []struct{ name, setup, call, top string }{
		// The first replace of a VM takes the generic path, which calls exec and so reads the flag. The
		// rest, after one has run, take the one that matches in a loop of its own, inside
		// RegExp.prototype[Symbol.replace].
		{"replace", `'a'.replace(/a/g, 'b'); var s = 'ab'.repeat(1 << 22)`, `s.replace(/a/g, 'cc')`, "[Symbol.replace]"},
		{"fill", `var a = new Array(1 << 22).fill(1)`, `a.fill(0)`, "fill"},
		// An array-like object: join reads each index as it would an
		// Array's, and there are no elements to mark.
		{"join", `var a = { length: 1 << 23 }`, `Array.prototype.join.call(a, ',')`, "join"},
		{"normalize", `var s = 'éå'.repeat(1 << 21)`, `s.normalize('NFC')`, "normalize"},
		// localeCompare normalizes both strings, as normalize does, then
		// compares them.
		{"localeCompare", `var s = 'éå'.repeat(1 << 20), t = s + 'x'`, `s.localeCompare(t)`, "localeCompare"},
		{"toUpperCase", `var s = 'aé'.repeat(1 << 21)`, `s.toUpperCase()`, "toUpperCase"},
		{"toLowerCase", `var s = 'AÉ'.repeat(1 << 21)`, `s.toLowerCase()`, "toLowerCase"},
		// Whether a capital sigma ends a word looks past the characters
		// that case ignores (U+0301 is one) after it and before it.
		{"toLowerCase, a sigma before a long run", `var s = 'AΣ' + '́'.repeat(1 << 22)`, `s.toLowerCase()`, "toLowerCase"},
		{"toLowerCase, a sigma after a long run", `var s = 'A' + '́'.repeat(1 << 22) + 'Σ'`, `s.toLowerCase()`, "toLowerCase"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			v := newVM(t, Options{Seed: 1})
			defer closeVM(t, v)
			h := v.Host()
			if _, err := v.Eval(c.setup, "setup.js"); err != nil {
				t.Fatal(err)
			}
			before := h.Polls()
			if _, err := v.Eval(c.call, "call.js"); err != nil {
				t.Fatal(err)
			}
			call := h.Polls() - before
			if call < builtinCallPolls {
				t.Fatalf("one call took %d polls, want at least %d", call, builtinCallPolls)
			}
			t.Logf("one call took %d polls", call)
			for trial := range builtinTrials {
				v.RunGC()
				// The second call of the loop, at (trial+1/2)/builtinTrials
				// of it.
				at := call + (2*int64(trial)+1)*call/(2*builtinTrials)
				h.InterruptAtPoll(h.Polls() + at)
				_, err := v.Eval(`for (;;) { `+c.call+` }`, "stop.js")
				var je *JSError
				switch polls := h.StopPolls(); {
				case errors.Is(err, ErrKilled):
					t.Fatalf("trial %d: the script did not stop and was killed, %d polls after Interrupt", trial, polls)
				case !errors.As(err, &je) || je.Message != "InternalError: interrupted" || !je.Uncatchable:
					t.Fatalf("trial %d: Eval gave %#v, want the error interrupted", trial, err)
				case !strings.HasPrefix(je.Stack, "    at "+c.top+" (native)"):
					t.Errorf("trial %d: interrupted at poll %d of the loop with the stack %q, not in %s", trial, at, je.Stack, c.top)
				case polls > builtinStopPolls:
					t.Errorf("trial %d: stopped %d polls after Interrupt at poll %d of the loop, want at most %d", trial, polls, at, builtinStopPolls)
				default:
					t.Logf("trial %d: stopped %d polls after Interrupt at poll %d of the loop", trial, polls, at)
				}
			}
		})
	}
}
