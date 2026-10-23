// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package slowjs // import "github.com/liustack/slowjs"

import (
	"errors"
	"testing"
	"time"
)

// The eval timeout stops a loop of calls of built-ins that parse source or
// build long strings within a few milliseconds: each call counts its work
// toward the interrupt check, so the check does not wait for thousands of
// calls.
func TestBuiltinWorkStops(t *testing.T) {
	for _, c := range []struct{ name, setup, loop string }{
		{"JSON.parse", `var s = JSON.stringify(Array.from({length: 200}, (_, i) => ({id: i, name: 'item ' + i})))`, `for (;;) JSON.parse(s)`},
		{"JSON.stringify", `var o = Array.from({length: 200}, (_, i) => ({id: i, name: 'item ' + i}))`, `for (;;) JSON.stringify(o)`},
		{"eval", `var src = Array.from({length: 50}, (_, i) => 'function f' + i + '(a, b) { return a * ' + i + ' + b.x }').join('\n')`, `for (;;) eval(src)`},
		{"new Function", `var src = Array.from({length: 50}, (_, i) => 'var v' + i + ' = a * ' + i + ' + b.x').join('\n')`, `for (;;) new Function('a', 'b', src)`},
		{"repeat", ``, `for (;;) 'x'.repeat(1e5)`},
		{"padStart", ``, `for (;;) ''.padStart(1e5, 'ab')`},
		{"replaceAll", `var s = 'ab'.repeat(1e4)`, `for (;;) s.replaceAll('a', 'c')`},
		{"join", `var a = Array.from({length: 1e4}, (_, i) => i)`, `for (;;) a.join(',')`},
	} {
		t.Run(c.name, func(t *testing.T) {
			m := newTestVM(t)
			if _, err := m.Eval(c.setup+`; 0`, EvalGlobal); err != nil {
				t.Fatal(err)
			}
			m.SetEvalTimeout(10 * time.Millisecond)
			start := time.Now()
			_, err := m.Eval(c.loop, EvalGlobal)
			took := time.Since(start)
			var e *Error
			if !errors.As(err, &e) || !e.Interrupted || !e.Uncatchable {
				t.Fatalf("got %v, want interrupted", err)
			}
			if took > 50*time.Millisecond {
				t.Errorf("stopped %v after the call started, want about 10ms", took)
			}
		})
	}
}

// The eval timeout stops a loop of searches in a long string within a few
// milliseconds: each search counts the indexes it looks through toward the
// interrupt check.
func TestStringSearchStops(t *testing.T) {
	for _, c := range []struct{ name, setup, loop string }{
		{"indexOf", `var s = 'a'.repeat(1 << 20)`, `for (;;) s.indexOf('b')`},
		{"lastIndexOf", `var s = 'a'.repeat(1 << 20)`, `for (;;) s.lastIndexOf('b')`},
		{"includes", `var s = 'a'.repeat(1 << 20)`, `for (;;) s.includes('b')`},
		{"16-bit", `var s = '\u0100'.repeat(1 << 20)`, `for (;;) s.indexOf('b')`},
	} {
		t.Run(c.name, func(t *testing.T) {
			m := newTestVM(t)
			if _, err := m.Eval(c.setup+`; 0`, EvalGlobal); err != nil {
				t.Fatal(err)
			}
			m.SetEvalTimeout(10 * time.Millisecond)
			start := time.Now()
			_, err := m.Eval(c.loop, EvalGlobal)
			took := time.Since(start)
			var e *Error
			if !errors.As(err, &e) || !e.Interrupted || !e.Uncatchable {
				t.Fatalf("got %v, want interrupted", err)
			}
			if took > 50*time.Millisecond {
				t.Errorf("stopped %v after the call started, want about 10ms", took)
			}
		})
	}
}
