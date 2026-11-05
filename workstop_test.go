// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package quickjs // import "github.com/liustack/quickjs"

import (
	"testing"
	"time"
)

// The eval timeout stops a loop of calls of built-ins that parse source or
// build long strings within a few polls: each call counts its work toward
// the interrupt check, so the check does not wait for thousands of calls.
func TestBuiltinWorkStops(t *testing.T) {
	for _, c := range []struct {
		name, setup, loop string
		limit             int64
	}{
		{"JSON.parse", `var s = JSON.stringify(Array.from({length: 200}, (_, i) => ({id: i, name: 'item ' + i})))`, `for (;;) JSON.parse(s)`, stopPolls},
		{"JSON.stringify", `var o = Array.from({length: 200}, (_, i) => ({id: i, name: 'item ' + i}))`, `for (;;) JSON.stringify(o)`, stopPolls},
		{"eval", `var src = Array.from({length: 50}, (_, i) => 'function f' + i + '(a, b) { return a * ' + i + ' + b.x }').join('\n')`, `for (;;) eval(src)`, stopPolls},
		{"new Function", `var src = Array.from({length: 50}, (_, i) => 'var v' + i + ' = a * ' + i + ' + b.x').join('\n')`, `for (;;) new Function('a', 'b', src)`, stopPolls},
		// repeat writes one character at a time and reads the flag every 65,536
		{"repeat", ``, `for (;;) 'x'.repeat(1e5)`, stepStopPolls},
		{"long repeat", ``, `for (;;) 'x'.repeat(1 << 21)`, stepStopPolls},
		{"padStart", ``, `for (;;) ''.padStart(1e5, 'ab')`, stopPolls},
		{"replaceAll", `var s = 'ab'.repeat(1e4)`, `for (;;) s.replaceAll('a', 'c')`, stopPolls},
		{"join", `var a = Array.from({length: 1e4}, (_, i) => i)`, `for (;;) a.join(',')`, stopPolls},
	} {
		t.Run(c.name, func(t *testing.T) {
			m := newTestVM(t)
			if _, err := m.Eval(c.setup+`; 0`, EvalGlobal); err != nil {
				t.Fatal(err)
			}
			m.SetEvalTimeout(10 * time.Millisecond)
			start := time.Now()
			_, err := m.Eval(c.loop, EvalGlobal)
			checkStopped(t, m, err, time.Since(start), c.limit, "")
		})
	}
}

// The eval timeout stops a loop of searches in a long string within a few
// polls: each search counts the indexes it looks through toward the
// interrupt check.
func TestStringSearchStops(t *testing.T) {
	for _, c := range []struct {
		name, setup, loop string
		limit             int64
	}{
		{"indexOf", `var s = 'a'.repeat(1 << 20)`, `for (;;) s.indexOf('b')`, stopPolls},
		// the search from the end, and a search in a string of 16 bits, read the flag every 65,536 indexes
		{"lastIndexOf", `var s = 'a'.repeat(1 << 20)`, `for (;;) s.lastIndexOf('b')`, stepStopPolls},
		{"includes", `var s = 'a'.repeat(1 << 20)`, `for (;;) s.includes('b')`, stopPolls},
		{"16-bit", `var s = '\u0100'.repeat(1 << 20)`, `for (;;) s.indexOf('b')`, stepStopPolls},
	} {
		t.Run(c.name, func(t *testing.T) {
			m := newTestVM(t)
			if _, err := m.Eval(c.setup+`; 0`, EvalGlobal); err != nil {
				t.Fatal(err)
			}
			m.SetEvalTimeout(10 * time.Millisecond)
			start := time.Now()
			_, err := m.Eval(c.loop, EvalGlobal)
			checkStopped(t, m, err, time.Since(start), c.limit, "")
		})
	}
}
