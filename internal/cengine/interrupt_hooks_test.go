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
// stopsSoftly interrupts an endless script. It comes after a RunGC, which
// sets the threshold of the next collection, so the Eval starts none before
// the script runs.
const endlessAt = 200

// stopsSoftly runs the endless script c, has the poll endlessAt into it ask
// to stop, and checks the stop (stoppedSoftly), with the kill grace given.
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
	v.RunGC()
	v.Host().InterruptAtPoll(v.Host().Polls() + endlessAt)
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
// interrupts it at places spread evenly over the first call of the loop,
// counted in polls: the polls of a call are the same on any machine, and the
// poll at the place asks to stop itself. Each stop must come within
// builtinStopPolls polls, in the built-in (the top frame of the stack), and
// not by the kill. The stop is measured in polls, not on the clock, so a
// machine that does not run the thread for a while does not fail it. The
// input is cheap to mark and to free, made before the loop and kept alive,
// and the loop makes nothing that the engine would collect, so that the stop
// is the built-in reading the flag: what a large heap costs when a script
// stops is TestKill's and the kill's business. The built-ins that make a
// result of many pieces are the exception, and may take the free of the
// result besides (bigResults).
func TestInterruptStopsLongBuiltins(t *testing.T) {
	needPolls(t)
	// What the iterators below walk: the iterator of an array, a Set or a
	// Map is a built-in one, which JS_IteratorNext2 calls without
	// JS_CallInternal, and these built-ins call nothing else for each item.
	const (
		bigArray     = `var a = new Array(1 << 24).fill(1)`
		setOfNumbers = `var s = new Set(); for (var i = 0; i < 1 << 21; i++) s.add(i)`
		mapOfNumbers = `var m = new Map(); for (var i = 0; i < 1 << 20; i++) m.set(i, i)`
		// a little under a power of two, so that no add of the result starts the rehash of a table that holds 1M records
		twoSets = `var s = new Set(), t = new Set(); for (var i = 0; i < (1 << 20) - 1024; i++) { s.add(i); t.add(-i - 1) }`
	)
	cases := []struct{ name, setup, call, top string }{
		// The first replace of a VM takes the generic path, which calls exec and so reads the flag. The
		// rest, after one has run, take the one that matches in a loop of its own, inside
		// RegExp.prototype[Symbol.replace].
		{"replace", `'a'.replace(/a/g, 'b'); var s = 'ab'.repeat(1 << 22)`, `s.replace(/a/g, 'cc')`, "[Symbol.replace]"},
		{"fill", `var a = new Array(1 << 23).fill(1)`, `a.fill(0)`, "fill"},
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
		// isWellFormed and toWellFormed look through the string for a lone
		// surrogate. toWellFormed then copies the string and repairs what
		// follows the first one it found.
		{"isWellFormed", `var s = '中'.repeat(1 << 23)`, `s.isWellFormed()`, "isWellFormed"},
		{"toWellFormed, a lone surrogate last", `var s = '中'.repeat(1 << 23) + '\ud800'; s.indexOf('x')`, `s.toWellFormed()`, "toWellFormed"},
		{"toWellFormed, a lone surrogate first", `var s = '\ud800' + '中'.repeat(1 << 23); s.indexOf('x')`, `s.toWellFormed()`, "toWellFormed"},
		// A pair is two characters of one turn of the repair loop, which has to
		// count its work apart from the place it is at.
		{"toWellFormed, a lone surrogate first, then pairs", `var s = ['\ud800', '😀'.repeat(1 << 23)].join('')`, `s.toWellFormed()`, "toWellFormed"},
		{"split, a separator that is not there", `var s = '中'.repeat(1 << 24)`, `s.split('b')`, "split"},
		{"padStart", `var s = 'x'`, `s.padStart(1 << 24, 'ab')`, "padStart"},
		{"padEnd, one character", `var s = 'x'`, `s.padEnd(1 << 24)`, "padEnd"},
		{"trim", `var s = ' '.repeat(1 << 24)`, `s.trim()`, "trim"},
		{"trimEnd", `var s = ' '.repeat(1 << 24)`, `s.trimEnd()`, "trimEnd"},
		{"anchor", `var v = 'a'.repeat(1 << 25)`, `'x'.anchor(v)`, "anchor"},
		{"encodeURI", `var s = 'a/'.repeat(1 << 23)`, `encodeURI(s)`, "encodeURI"},
		{"decodeURI", `var s = '%41%C3%A9'.repeat(1 << 22)`, `decodeURI(s)`, "decodeURI"},
		{"escape", `var s = 'a@*_+-./'.repeat(1 << 21)`, `escape(s)`, "escape"},
		{"unescape", `var s = '%u00E9%41%E9'.repeat(1 << 22)`, `unescape(s)`, "unescape"},
		// Two ropes made by doubling: each holds the same strings over and over
		// and takes no memory. (Making a rope flat takes few polls, since it is a
		// memcpy of big strings: test_rope_polls of engine/test/unit.c counts
		// the readings of the flag in it.)
		{"two ropes compared", `var f = function () { var r = 'ab'; for (var i = 0; i < 25; i++) r = r + r; return r }; var r1 = f(), r2 = f()`, `r1 < r2`, "<eval>"},
		// Equality is an operator of the interpreter, which stops in the frame that runs it. Two equal ropes are
		// compared leaf by leaf, and two equal flat strings in steps.
		{"two ropes ===", `var f = function () { var r = 'ab'; for (var i = 0; i < 25; i++) r = r + r; return r }; var r1 = f(), r2 = f()`, `r1 === r2`, "<eval>"},
		{"two ropes ==", `var f = function () { var r = 'ab'; for (var i = 0; i < 25; i++) r = r + r; return r }; var r1 = f(), r2 = f()`, `r1 == r2`, "<eval>"},
		{"switch on a rope", `var f = function () { var r = 'ab'; for (var i = 0; i < 25; i++) r = r + r; return r }; var r1 = f(), r2 = f()`, `switch (r1) { case 1: break; case r2: break }`, "<eval>"},
		{"two flat strings ===", `var a = 'ab'.repeat(1 << 24), b = 'ab'.repeat(1 << 24)`, `a === b`, "<eval>"},
		// The hash of the key of a Map or Set walks all its characters.
		{"Set.has, a rope for the key", `var r = 'ab'; for (var i = 0; i < 26; i++) r = r + r; var s = new Set()`, `s.has(r)`, "has"},
		{"Map.get, a long string for the key", `var f = 'ab'.repeat(1 << 25); var m = new Map()`, `m.get(f)`, "get"},
		// the key is taken out again: comparing a key with an equal rope, as a Set that holds it does, reads no flag
		{"Map.set, a rope for the key", `var r = 'ab'; for (var i = 0; i < 26; i++) r = r + r; var m = new Map()`, `m.set(r, 1), m.clear()`, "set"},
		// a key equal to the one the Map holds is compared with it after it is hashed
		{"Map.get, an equal rope for the key", `var f = function () { var r = 'ab'; for (var i = 0; i < 25; i++) r = r + r; return r }; var r1 = f(), r2 = f(), m = new Map([[r1, 1]])`, `m.get(r2)`, "get"},
		{"Set.has, an equal flat string for the key", `var a = 'ab'.repeat(1 << 24), b = 'ab'.repeat(1 << 24), s = new Set([a])`, `s.has(b)`, "has"},
		{"Math.sumPrecise, an Array", bigArray, `Math.sumPrecise(a)`, "sumPrecise"},
		{"Iterator drop, an Array", bigArray, `a.values().drop(1 << 24).next()`, "next"},
		// A set-like object that lends the iterator of an array: isSupersetOf asks this Set
		// about each item, in C.
		{"Set isSupersetOf", bigArray + `; var s = new Set([1]), like = { size: 1, has: () => true, keys: () => a.values() }`, `s.isSupersetOf(like)`, "isSupersetOf"},
	}
	// These make a result of as many pieces as the input has. Freeing it takes
	// polls, up to a third of the polls of the call or more, and so many
	// after a stop as the call had made by then. A result that grows by
	// doubling is also copied now and then, which reads no flag: the copy of
	// 16 MiB is a tenth of such a call. So a stop may take, besides
	// builtinStopPolls, the free of a whole result and a quarter of the call,
	// and what these tests hold to is the frame the stop comes in: the
	// built-in that reads the flag, not the interpreter after it. A result is
	// kept in a variable, so that the free of the one before it comes after
	// the built-in, where no place of the trials is.
	bigResults := []struct{ name, setup, call, top, also string }{
		{"split", `var s = 'ab,'.repeat(1 << 21)`, `s.split(',')`, "split", ""},
		{"split by nothing", `var s = 'a'.repeat(1 << 22)`, `s.split('')`, "split", ""},
		{"Array.from, a string", `var s = 'ab'.repeat(1 << 21)`, `Array.from(s)`, "from", ""},
		{"Array.from, an array-like", `var o = { length: 1 << 22 }`, `Array.from(o)`, "from", ""},
		// A typed array of an iterable is made from an array of its pieces, which
		// is freed when the call stops.
		{"Uint8Array.from, a string", `var s = '1'.repeat(1 << 21)`, `Uint8Array.from(s)`, "from", `Array.from(s)`},
		{"new Uint8Array, an Array", `var a = new Array(1 << 22).fill(1)`, `new Uint8Array(a)`, "Uint8Array", `Array.from(a)`},
		// A replacement that is a string calls nothing for each match, and the result grows by doubling.
		{"replaceAll, a string for the pattern", `var s = 'ab'.repeat(1 << 23)`, `s.replaceAll('a', 'cc')`, "replaceAll", ""},
		{"Array.from, a Set", setOfNumbers, `Array.from(s)`, "from", ""},
		{"Object.fromEntries, a Map", mapOfNumbers, `Object.fromEntries(m)`, "fromEntries", ""},
		{"Iterator toArray, a Set", setOfNumbers, `s.values().toArray()`, "toArray", ""},
		// Two Sets with no item in common, so that the result is as large as what the call holds when it stops.
		{"Set union", twoSets, `s.union(t)`, "union", ""},
		{"Set symmetricDifference", twoSets, `s.symmetricDifference(t)`, "symmetricDifference", ""},
		// A spread is an operator of the interpreter, which stops in the frame that runs it.
		{"spread of a Set", setOfNumbers, `[...s]`, "<eval>", ""},
	}
	run := func(t *testing.T, name, setup, callSrc, top, also string, big bool) {
		t.Run(name, func(t *testing.T) {
			v := newVM(t, Options{Seed: 1})
			defer closeVM(t, v)
			h := v.Host()
			if _, err := v.Eval(setup, "setup.js"); err != nil {
				t.Fatal(err)
			}
			// What the loop runs. A big result is kept in r.
			loop := callSrc
			if big {
				if _, err := v.Eval("var r", "setup.js"); err != nil {
					t.Fatal(err)
				}
				// a collection in the middle of a call is polls of its own that the
				// places of the trials do not allow for; RunGC leaves this alone
				v.SetGCThreshold(^uintptr(0))
				loop = "r = " + callSrc
			}
			// prep puts the VM where each trial starts: the loop frees no result
			// before the call it interrupts, and a collection has left the
			// caches of the allocator empty.
			prep := func() {
				if big {
					if _, err := v.Eval("r = null; 0", "free.js"); err != nil {
						t.Fatal(err)
					}
				}
				v.RunGC()
			}
			// once runs the call, and returns its polls.
			once := func() int64 {
				before := h.Polls()
				if _, err := v.Eval(loop+"; 0", "call.js"); err != nil {
					t.Fatal(err)
				}
				return h.Polls() - before
			}
			// A trial starts as this does, from prep, and the loop's first call is
			// the one that runs here: the places are in it, so that the polls up to
			// them are the same on any machine.
			prep()
			polls := once()
			// A result of many pieces is held to the frame of the stop, not to
			// the polls, so a call of a fourth of them tells the places apart.
			least := int64(builtinCallPolls)
			if big {
				least /= 4
			}
			if polls < least {
				t.Fatalf("one call took %d polls, want at least %d", polls, least)
			}
			t.Logf("one call took %d polls", polls)
			// The polls of the call that the places of the trials are spread over,
			// and what a stop may take besides builtinStopPolls.
			span := polls
			allowed := int64(builtinStopPolls)
			if big {
				freed := func(src string) int64 {
					prep()
					if _, err := v.Eval("r = "+src+"; 0", "call.js"); err != nil {
						t.Fatal(err)
					}
					before := h.Polls()
					if _, err := v.Eval("r = null; 0", "free.js"); err != nil {
						t.Fatal(err)
					}
					return h.Polls() - before
				}
				free := freed(callSrc)
				if also != "" {
					freeAlso := freed(also)
					free += freeAlso
					// the call frees its array of pieces at its end, where no place
					// of the trials is
					span -= freeAlso
				}
				t.Logf("the free of the result took %d polls", free)
				allowed += free + polls/4
			}
			// An operator of the interpreter stops in the frame that runs it.
			wantTop := "    at " + top + " (native)"
			if top == "<eval>" {
				wantTop = "    at <eval> (stop.js"
			}
			for trial := range builtinTrials {
				prep()
				// The first call of the loop, at (trial+1/2)/builtinTrials of it (of
				// what comes before the free of its pieces).
				at := (2*int64(trial) + 1) * span / (2 * builtinTrials)
				h.InterruptAtPoll(h.Polls() + at)
				_, err := v.Eval(`for (;;) { `+loop+` }`, "stop.js")
				var je *JSError
				switch stop := h.StopPolls(); {
				case errors.Is(err, ErrKilled):
					t.Fatalf("trial %d: the script did not stop and was killed, %d polls after Interrupt", trial, stop)
				case !errors.As(err, &je) || je.Message != "InternalError: interrupted" || !je.Uncatchable:
					t.Fatalf("trial %d: Eval gave %#v, want the error interrupted", trial, err)
				case !strings.HasPrefix(je.Stack, wantTop):
					t.Errorf("trial %d: interrupted at poll %d of the loop with the stack %q, not in %s", trial, at, je.Stack, top)
				case stop > allowed:
					t.Errorf("trial %d: stopped %d polls after Interrupt at poll %d of the loop, want at most %d", trial, stop, at, allowed)
				default:
					t.Logf("trial %d: stopped %d polls after Interrupt at poll %d of the loop", trial, stop, at)
				}
			}
		})
	}
	for _, c := range cases {
		run(t, c.name, c.setup, c.call, c.top, "", false)
	}
	for _, c := range bigResults {
		run(t, c.name, c.setup, c.call, c.top, c.also, true)
	}
}
