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
	// Strings of one hash: the pairs A and B add the same to a hash made as h * 263 + c
	// (256*263+519 == 257*263+256), so keys that differ in which of them they hold, in any order,
	// have one hash, and the key asked for differs from every one of them at the first pair.
	const chainSetup = `var A = 'Āȇ', B = 'āĀ', P = 'x'.repeat(65532);
		function tail(i) { var r = []; for (var j = 0; j < 8; j++) r.push((i >> j & 1) ? A : B); return r.join('') }
		function ckey(i) { return [P, B, tail(i)].join('') }
		function cquery() { return [P, A, tail(0)].join('') }`
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
		// The characters of a long string of the text are taken in runs, the text is made a C string in
		// steps, and a buffer that holds tens of MiB is copied in steps when it grows.
		{"JSON.parse, a long key", `var j = '{"' + 'x'.repeat(1 << 26) + '":1}'`, `JSON.parse(j)`, "parse", 0},
		{"JSON.parse, a long string", `var j = '{"a":"' + 'x'.repeat(1 << 26) + '"}'`, `JSON.parse(j)`, "parse", 0},
		// the escapes count toward the step as the characters do, so a string of nothing else is a run of steps
		{"JSON.parse, escapes", `var j = '"' + '\\n'.repeat(1 << 24) + '"'`, `JSON.parse(j)`, "parse", 0},
		{"JSON.parse, runs and escapes", `var j = '"' + ('a'.repeat(30000) + '\\n').repeat(1 << 10) + '"'`, `JSON.parse(j)`, "parse", 0},
		// a wide character after tens of MiB of ASCII widens them all, in steps
		{"JSON.parse, a wide character last", `var j = ['"', 'a'.repeat(1 << 26), '中"'].join('')`, `JSON.parse(j)`, "parse", 0},
		{"JSON.parse, a wide escape last", `var j = '"' + 'a'.repeat(1 << 26) + '\\u4e2d"'`, `JSON.parse(j)`, "parse", 0},
		// padEnd writes the 64 MiB of s first with one memcpy that reads no flag (150 to 450 polls), and then
		// widens them in steps. padStart writes 32M characters of the filler first, since the widening of s alone
		// is under 640 polls. The first trial lands in the allocation of the buffers (96 MiB, then 192 MiB), which takes
		// up to 100 polls and reads no flag
		{"padEnd, a wide filler", `var s = 'a'.repeat(1 << 26)`, `s.padEnd((1 << 26) + 1, '中')`, "padEnd", 500},
		{"padStart, a wide filler", `var s = 'a'.repeat(1 << 26)`, `s.padStart((1 << 26) + (1 << 25), '中')`, "padStart", 100},
		// 256 keys of one hash and one length, each different from the key asked for at the first step: the
		// compares of the chain share one budget, and one that ends at a difference counts what it did
		{"Map.get, a chain of keys of one hash", chainSetup + `; var cm = new Map(); for (var i = 0; i < 256; i++) cm.set(ckey(i), i)`, `cm.get(cquery())`, "get", 0},
		{"Map.delete, a chain of keys of one hash", chainSetup + `; var cm = new Map(); for (var i = 0; i < 256; i++) cm.set(ckey(i), i)`, `cm.delete(cquery())`, "delete", 0},
		{"o[k] = 1, a chain of keys of one hash", chainSetup + `; var co = {}; for (var i = 0; i < 256; i++) co[ckey(i)] = i`, `co[cquery()] = 1`, "<eval>", 0},
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
