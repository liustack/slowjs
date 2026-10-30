// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build testhooks

package cengine

import (
	"errors"
	"strings"
	"testing"
)

// TestInterruptStopsLongKeys interrupts the first use of a long string as a
// property key. TestInterruptStopsLongBuiltins runs a call over and over, and
// the first run of these changes the string for the next ones (it becomes an
// atom, which later runs find at once), so each place of the trial gets a VM
// of its own, with the string as new as in the first run.
func TestInterruptStopsLongKeys(t *testing.T) {
	needPolls(t)
	// slack is how many polls more than builtinStopPolls a stop may take
	cases := []struct {
		name, setup, call, top string
		slack                  int64
	}{
		// the hash of the key is walked
		{"o[f] = 1", `var f = 'x'.repeat(1 << 26), o = {}`, `o[f] = 1`, "<eval>", 0},
		{"f in o", `var f = 'x'.repeat(1 << 26), o = {}`, `f in o`, "<eval>", 0},
		{"o[f] read", `var f = 'x'.repeat(1 << 26), o = {}`, `o[f]`, "<eval>", 0},
		// the key is the atom of an equal string, which is compared with it
		{"o[g] = 1, an atom equal to g", `var f = 'x'.repeat(1 << 25), g = 'x'.repeat(1 << 25), o = {}; o[f] = 1`, `o[g] = 2`, "<eval>", 0},
		// The characters of a long string of the text are taken in runs. Two copies of tens of MiB are
		// memcpy calls, which the generated code polls (up to about 440 polls) and in which the engine
		// reads no flag: the text as a C string, before the parse, and the last doubling of the buffer
		// that holds the string.
		{"JSON.parse, a long key", `var j = '{"' + 'x'.repeat(1 << 26) + '":1}'`, `JSON.parse(j)`, "parse", 500},
		{"JSON.parse, a long string", `var j = '{"a":"' + 'x'.repeat(1 << 26) + '"}'`, `JSON.parse(j)`, "parse", 500},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// start returns a VM with the setup done and the memory settled, and its host
			start := func() (*VM, func()) {
				v := newVM(t, Options{Seed: 1})
				if _, err := v.Eval(c.setup, "setup.js"); err != nil {
					t.Fatal(err)
				}
				v.SetGCThreshold(^uintptr(0))
				v.RunGC()
				return v, func() { closeVM(t, v) }
			}
			v, done := start()
			h := v.Host()
			before := h.Polls()
			if _, err := v.Eval(c.call+"; 0", "call.js"); err != nil {
				t.Fatal(err)
			}
			polls := h.Polls() - before
			done()
			if polls < int64(builtinCallPolls) {
				t.Fatalf("one call took %d polls, want at least %d", polls, builtinCallPolls)
			}
			t.Logf("one call took %d polls", polls)
			wantTop := "    at " + c.top + " (native)"
			if c.top == "<eval>" {
				wantTop = "    at <eval> (stop.js"
			}
			for trial := range builtinTrials {
				v, done := start()
				h := v.Host()
				at := (2*int64(trial) + 1) * polls / (2 * builtinTrials)
				h.InterruptAtPoll(h.Polls() + at)
				_, err := v.Eval(c.call, "stop.js")
				var je *JSError
				switch stop := h.StopPolls(); {
				case errors.Is(err, ErrKilled):
					t.Errorf("trial %d: the script did not stop and was killed, %d polls after Interrupt", trial, stop)
				case !errors.As(err, &je) || je.Message != "InternalError: interrupted" || !je.Uncatchable:
					t.Errorf("trial %d: Eval gave %#v, want the error interrupted", trial, err)
				case !strings.HasPrefix(je.Stack, wantTop):
					t.Errorf("trial %d: interrupted at poll %d with the stack %q, not in %s", trial, at, je.Stack, c.top)
				case stop > builtinStopPolls+c.slack:
					t.Errorf("trial %d: stopped %d polls after Interrupt at poll %d, want at most %d", trial, stop, at, builtinStopPolls+c.slack)
				default:
					t.Logf("trial %d: stopped %d polls after Interrupt at poll %d", trial, stop, at)
				}
				if err != nil && errors.Is(err, ErrKilled) {
					return
				}
				done()
			}
		})
	}
}
