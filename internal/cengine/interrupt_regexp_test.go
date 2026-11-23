// Copyright (c) 2026 The liustack/beastjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

//go:build testhooks

package cengine

import (
	"errors"
	"fmt"
	"testing"
)

// regexpStopPolls is how many polls may see the request to stop before the
// compile of a regexp has stopped. The longest steps of a compile are the
// conversion of the pattern to UTF-8, which reads the flag every 65,536
// characters with about a tick for each, 7 polls.
const regexpStopPolls = 9

// TestInterruptStopsRegExpCompile compiles regexps of large classes and
// interrupts each compile at places spread evenly over it, counted in polls
// (InterruptAtPoll). Each stop must come within regexpStopPolls, with the
// error interrupted: the compile reads the flag in every long loop, and what
// it frees when it stops is a few blocks. Each case was a stretch of more
// polls before: the sort of the strings of a class by length (146 for the
// case of many lengths), the regrowth of their table (34) and their free one
// by one after a stop (139), the walk of the empty buckets of their table
// (51 when an intersection had left none of them), the merge of each
// character into the ranges of a class so far (18 at the end of a class of
// 60,000), and the move of the points of a class inverted (10 for 330,000
// characters).
func TestInterruptStopsRegExpCompile(t *testing.T) {
	needPolls(t)
	// The patterns are made before the trials and kept. Each compile adds a
	// number to its pattern, so that the regexp cache does not keep it.
	const (
		strings4  = `var M = []; for (var i = 0; i < 1 << 19; i++) M.push((i + 1e6).toString(36)); var P = '[\\q{' + M.join('|') + '}]'; M = null`
		emptied   = `var M = []; for (var i = 0; i < 1 << 19; i++) M.push((i + 1e6).toString(36)); var P = '[[\\q{' + M.join('|') + '}]&&[\\q{zz}]&&[\\q{yy|'; M = null`
		lengths   = `var M = []; for (var i = 0; i < 1 << 18; i++) M.push((i + 1e6).toString(36) + 'x'.repeat(i * 7919 % 64)); var P = '[\\q{' + M.join('|') + '}]'; M = null`
		upward    = `var a = []; for (var c = 0x10000; c < 0x10000 + 120000; c += 2) a.push(String.fromCodePoint(c)); var P = '[' + a.join(''); a = null`
		downward  = `var a = []; for (var c = 0x10000 + 120000; c > 0x10000; c -= 2) a.push(String.fromCodePoint(c)); var P = '[' + a.join(''); a = null`
		twoRanges = `var a = []; for (var c = 0x10000; c < 0x10000 + 120000; c += 2) a.push(String.fromCodePoint(c)); a = a.join(''); var P = '[[' + a + ']&&[' + a; a = null`
		inverted  = `var a = []; for (var c = 0x10000; c < 0x10000 + 660000; c += 2) a.push(String.fromCodePoint(c)); var P = '[[^' + a.join(''); a = null`
	)
	cases := []struct {
		name, setup, call string
		trials            int64
	}{
		{"a class of 524,288 strings of 4 characters", strings4, `new RegExp(P.slice(0, -2) + '|' + n++ + '}]', 'v')`, 100},
		{"a class of 262,144 strings of 4 to 67 characters", lengths, `new RegExp(P.slice(0, -2) + '|' + n++ + '}]', 'v')`, 100},
		{"a class of 524,288 strings intersected with two that have none of them", emptied, `new RegExp(P + n++ + '}]]', 'v')`, 100},
		{"a class of 60,000 characters upward", upward, `new RegExp(P + n++ + ']', 'u')`, 10},
		{"a class of 60,000 characters downward", downward, `new RegExp(P + n++ + ']', 'u')`, 10},
		{"the intersection of two classes of 60,000 characters", twoRanges, `new RegExp(P + n++ + ']]', 'v')`, 10},
		{"a class of 330,000 characters inverted, intersected", inverted, `new RegExp(P + n++ + ']&&[\\u{10000}-\\u{10010}]]', 'v')`, 400},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			v := newVM(t, Options{Seed: 1})
			defer closeVM(t, v)
			h := v.Host()
			if _, err := v.Eval(c.setup+`; var n = 0; 0`, "setup.js"); err != nil {
				t.Fatal(err)
			}
			// a collection in the middle of a compile is polls of its own
			v.SetGCThreshold(^uintptr(0))
			v.RunGC()
			before := h.Polls()
			if _, err := v.Eval(c.call+`; 0`, "call.js"); err != nil {
				t.Fatal(err)
			}
			span := h.Polls() - before
			t.Logf("one compile took %d polls", span)
			for trial := range c.trials {
				at := (2*trial + 1) * span / (2 * c.trials)
				v.RunGC()
				h.InterruptAtPoll(h.Polls() + at)
				_, err := v.Eval(c.call+`; 0`, "call.js")
				h.InterruptAtPoll(0)
				what := fmt.Sprintf("the stop at poll %d of %d", at, span)
				var je *JSError
				if !errors.As(err, &je) || je.Message != "InternalError: interrupted" || !je.Uncatchable {
					t.Fatalf("%s: Eval gave %v, want the error interrupted", what, err)
				}
				if n := h.StopPolls(); n > regexpStopPolls {
					t.Errorf("%s: %d polls saw the request, want at most %d", what, n, regexpStopPolls)
				}
			}
		})
	}
}

// TestInterruptStopsRegExpFlags makes regexps with flags of 16 Mi
// characters, in the constructor and in a literal, and interrupts each at
// places spread evenly over it (InterruptAtPoll). Each stop must come within
// regexpStopPolls, also where the call would end with the SyntaxError of
// the flags. The compile converted the flags to UTF-8 to check them, which
// counted the non-ASCII characters of the string with no reading of the
// flag, about 140 polls for these.
func TestInterruptStopsRegExpFlags(t *testing.T) {
	needPolls(t)
	const caught = `try { %s } catch (e) { if (!(e instanceof SyntaxError)) throw e }`
	for _, c := range []struct{ name, call string }{
		{"the constructor", `new RegExp('a' + n++, F)`},
		{"a literal", `(0, eval)('/a' + n++ + '/' + F)`},
	} {
		t.Run(c.name, func(t *testing.T) {
			v := newVM(t, Options{Seed: 1})
			defer closeVM(t, v)
			h := v.Host()
			if _, err := v.Eval(`var F = 'g'.repeat(1 << 24), n = 0; F.length`, "setup.js"); err != nil {
				t.Fatal(err)
			}
			v.SetGCThreshold(^uintptr(0))
			v.RunGC()
			before := h.Polls()
			if _, err := v.Eval(fmt.Sprintf(caught, c.call)+`; 0`, "call.js"); err != nil {
				t.Fatal(err)
			}
			span := h.Polls() - before
			t.Logf("one call took %d polls", span)
			const trials = 100
			for trial := range int64(trials) {
				at := (2*trial + 1) * span / (2 * trials)
				v.RunGC()
				h.InterruptAtPoll(h.Polls() + at)
				// the loop reads the flag after a call that ended with the
				// SyntaxError of the flags without reading it
				_, err := v.Eval(`for (;;) `+fmt.Sprintf(caught, c.call), "call.js")
				h.InterruptAtPoll(0)
				what := fmt.Sprintf("the stop at poll %d of %d", at, span)
				var je *JSError
				if !errors.As(err, &je) || je.Message != "InternalError: interrupted" || !je.Uncatchable {
					t.Fatalf("%s: Eval gave %v, want the error interrupted", what, err)
				}
				if n := h.StopPolls(); n > regexpStopPolls {
					t.Errorf("%s: %d polls saw the request, want at most %d", what, n, regexpStopPolls)
				}
			}
		})
	}
}

// scanOffsetCases are the scans of the source that read the flag where their
// pointer meets a multiple of JS_ROPE_STEP (js_poll_cross and js_poll_at of
// engine/quickjs.c) and are followed by work that counts its own steps. The
// last reading of such a scan can fall nearly a step before its end,
// depending on the address of the source, and the work after it reads only
// after a step of its own: the scan must end with a reading. Each case is the
// setup and the source of one call, made from P, a padding of the length
// the test sets, and n, a number that changes at each call (the regexp cache
// and the atoms keep nothing between two calls).
var scanOffsetCases = []struct{ name, setup, source string }{
	// js_parse_regexp, then the hash of the body and the flags in the compile
	// (js_regexp_cache_hash): 11 polls on one darwin/arm64 machine of CI with
	// 16 Mi flags, 13 here before the reading at its end
	{"a regexp literal with 1 Mi flags", `var F = 'g'.repeat(1 << 20)`, `'/' + P + n++ + '/' + F`},
}

// TestInterruptStopsScanAtEveryOffset evaluates the source of each case of
// scanOffsetCases, with P padded by 0 to 60 KiB in steps of 4 KiB to put the
// end of the scan at every place of a step, and stops it at every poll of
// the call: each stop must come within regexpStopPolls.
func TestInterruptStopsScanAtEveryOffset(t *testing.T) {
	needPolls(t)
	const caught = `try { (0, eval)(%s) } catch (e) { if (!(e instanceof SyntaxError)) throw e }`
	for _, c := range scanOffsetCases {
		t.Run(c.name, func(t *testing.T) {
			v := newVM(t, Options{Seed: 1})
			defer closeVM(t, v)
			h := v.Host()
			if _, err := v.Eval(c.setup+`; var P, n = 0; 0`, "setup.js"); err != nil {
				t.Fatal(err)
			}
			v.SetGCThreshold(^uintptr(0))
			call := fmt.Sprintf(caught, c.source)
			var worst int64
			var worstAt string
			for pad := 0; pad < 1<<16; pad += 1 << 12 {
				if _, err := v.Eval(fmt.Sprintf(`P = 'a'.repeat(%d); 0`, pad), "pad.js"); err != nil {
					t.Fatal(err)
				}
				v.RunGC()
				before := h.Polls()
				if _, err := v.Eval(call+`; 0`, "call.js"); err != nil {
					t.Fatal(err)
				}
				span := h.Polls() - before
				for at := int64(1); at <= span; at++ {
					v.RunGC()
					h.InterruptAtPoll(h.Polls() + at)
					// the loop reads the flag after a call that ended with
					// a SyntaxError without reading it
					_, err := v.Eval(`for (;;) `+call, "call.js")
					h.InterruptAtPoll(0)
					what := fmt.Sprintf("a padding of %d bytes, the stop at poll %d of %d", pad, at, span)
					var je *JSError
					if !errors.As(err, &je) || je.Message != "InternalError: interrupted" || !je.Uncatchable {
						t.Fatalf("%s: Eval gave %v, want the error interrupted", what, err)
					}
					if n := h.StopPolls(); n > worst {
						worst, worstAt = n, what
					}
				}
			}
			t.Logf("at most %d polls saw the request (%s)", worst, worstAt)
			if worst > regexpStopPolls {
				t.Errorf("%s: %d polls saw the request, want at most %d", worstAt, worst, regexpStopPolls)
			}
		})
	}
}
