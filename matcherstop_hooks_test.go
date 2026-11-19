// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

//go:build testhooks

package slowjs // import "github.com/liustack/slowjs"

import (
	"errors"
	"testing"
)

// stopCase is a call that runs long in one step of the engine: setup makes
// what it needs, and call, made again and again, is what a stop must end.
type stopCase struct{ name, setup, call string }

// matcherStops are regexps whose match runs long between two places of the
// matcher that read the flag on the engine before lre_insert_checks: a run
// that only goes forward in the code (a class or a character written out
// many times, many groups or lookaheads in a row), the loop of a back
// reference over a long capture, and a loop of x+ or x{n,m} that matches
// many times. On the engine before, up to 26 to 102 polls saw a stop in
// each, or 13 and 14 for the groups and the classes of 256 ranges.
var matcherStops = []stopCase{
	{"class written out, 8-bit", `var re = new RegExp('[ab]'.repeat(1 << 18)), s = 'a'.repeat(1 << 18)`, `re.test(s)`},
	{"class written out, 16-bit", `var re = new RegExp('中' + '[ab]'.repeat(1 << 18)), s = '中' + 'a'.repeat(1 << 18)`, `re.test(s)`},
	{"dot written out, 16-bit", `var re = new RegExp('.'.repeat(1 << 18)), s = '中'.repeat(1 << 18)`, `re.test(s)`},
	{"literal, ignoring case", `var re = new RegExp('a'.repeat(1 << 18), 'i'), s = 'A'.repeat(1 << 18)`, `re.test(s)`},
	{"class, ignoring case, unicode", `var re = new RegExp('中' + '[ab]'.repeat(1 << 18), 'iu'), s = '中' + 'A'.repeat(1 << 18)`, `re.test(s)`},
	{"32767 groups", `var re = new RegExp('(a)'.repeat(32767)), s = 'a'.repeat(32767)`, `re.test(s)`},
	{"lookaheads in a row", `var re = new RegExp('(?=a)a'.repeat(1 << 17)), s = 'a'.repeat(1 << 17)`, `re.test(s)`},
	{"classes of 256 ranges", `var c = ''; for (var i = 0; i < 256; i++) c += String.fromCharCode(0x100 + 2 * i); ` +
		`var re = new RegExp(('[' + c + ']').repeat(1 << 14)), s = String.fromCharCode(0x100 + 2 * 255).repeat(1 << 14)`, `re.test(s)`},
	{"back reference", `var re = /^(a+)b\1$/, s = 'a'.repeat(1 << 18) + 'b' + 'a'.repeat(1 << 18)`, `re.test(s)`},
	{"back reference, ignoring case", `var re = /^(a+)b\1$/i, s = 'a'.repeat(1 << 18) + 'b' + 'A'.repeat(1 << 18)`, `re.test(s)`},
	{"back reference, folding", `var re = /^(\u00e9+)b\1$/i, s = '\u00e9'.repeat(1 << 16) + 'b' + '\u00c9'.repeat(1 << 16)`, `re.test(s)`},
	{"back reference in a lookbehind", `var re = /(?<=^\1b(a+))/y, s = 'a'.repeat(1 << 18) + 'b' + 'a'.repeat(1 << 18)`, `re.lastIndex = s.length, re.test(s)`},
	{"a+", `var re = /^a+/, s = 'a'.repeat(1 << 18)`, `re.test(s)`},
	{"a+ that fails", `var re = /^a+$/, s = 'a'.repeat(1 << 18) + 'b'`, `re.test(s)`},
	{"\\w+, 16-bit", `var re = /^\w+/, s = 'a'.repeat(1 << 18) + '中'`, `re.test(s)`},
	{"a{0,n}", `var re = /^(?:a){0,2000000}/, s = 'a'.repeat(1 << 18)`, `re.test(s)`},
	{"a{2,}", `var re = /^(?:a){2,}/, s = 'a'.repeat(1 << 18)`, `re.test(s)`},
	{"a+ in a lookbehind", `var re = /(?<=^a+)/y, s = 'a'.repeat(1 << 18)`, `re.lastIndex = s.length, re.test(s)`},
	{"(?=a+)", `var re = /(?=a+)/y, s = 'a'.repeat(1 << 18)`, `re.lastIndex = 0, re.test(s)`},
	{"(?!a+)", `var re = /(?!a+)/y, s = 'a'.repeat(1 << 18)`, `re.lastIndex = 0, re.test(s)`},
}

// TestMatcherStops: a stop that comes anywhere in a long match ends it
// within stepStopPolls polls that saw the request outside a collection and
// the freeing of an object (dstCleanup): the code of a regexp gets a goto,
// which reads the flag, wherever a run forward reaches LRE_CHECK_RUN ticks,
// and the loops of the matcher read it every LRE_EXEC_STEP steps
// (engine/libregexp.c). The stop comes at a fixed poll of the call
// (InterruptAtPoll), at 8 points spread over it.
func TestMatcherStops(t *testing.T) {
	testStops(t, matcherStops, 8, stepStopPolls)
}

// testStops makes each call of cases once to count its polls, then runs it
// in a loop with the stop at each of points points spread evenly over a
// call, and fails when more than limit polls outside cleanup saw the
// request. The call must end with the interrupt, and the VM go on.
func testStops(t *testing.T, cases []stopCase, points, limit int64, cleanup ...string) {
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := newTestVM(t)
			if _, err := m.Eval(c.setup+`; 0`, EvalGlobal); err != nil {
				t.Fatal(err)
			}
			// a first call makes what the call keeps (the code of the
			// regexp, the shapes), and the second is the one counted
			if _, err := m.Eval(c.call+`; 0`, EvalGlobal); err != nil {
				t.Fatal(err)
			}
			m.RunGC()
			before := m.h.Polls()
			if _, err := m.Eval(c.call+`; 0`, EvalGlobal); err != nil {
				t.Fatal(err)
			}
			span := m.h.Polls() - before
			for k := int64(0); k < points; k++ {
				at := (2*k + 1) * span / (2 * points)
				m.RunGC()
				var loop int64
				where := map[string]int{}
				m.h.SetStopTrace(func() {
					if inCleanup() {
						return
					}
					for _, f := range cleanup {
						if underFunc(f) {
							return
						}
					}
					loop++
					where[innermostEngineFunc()]++
				})
				m.h.InterruptAtPoll(m.h.Polls() + at)
				_, err := m.Eval(`for (;;) { `+c.call+` }`, EvalGlobal)
				m.h.InterruptAtPoll(0)
				m.h.SetStopTrace(nil)
				if errors.Is(err, ErrKilled) {
					t.Fatalf("stop at poll %d of %d: killed, want the interrupt (%d polls of the call saw the request, %v)", at, span, loop, where)
				}
				var e *Error
				if !errors.As(err, &e) || !e.Interrupted || !e.Uncatchable {
					t.Fatalf("stop at poll %d of %d: got %v, want interrupted", at, span, err)
				}
				t.Logf("stop at poll %d of %d: %d polls of the call, %d in all, %v", at, span, loop, m.h.StopPolls(), where)
				if loop > limit {
					t.Errorf("stop at poll %d of %d: %d polls of the call saw the request to stop, want at most %d (%d in all, %v)",
						at, span, loop, limit, m.h.StopPolls(), where)
				}
				if v, err := m.Eval(`1+1`, EvalGlobal); err != nil || v != 2 {
					t.Fatalf("the VM after the stop: 1+1 got %v, %v", v, err)
				}
			}
		})
	}
}
