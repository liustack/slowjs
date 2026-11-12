// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package quickjs // import "github.com/liustack/quickjs"

import (
	"errors"
	"fmt"
	"math"
	"testing"
	"time"
)

// makeRegExps makes regexps of n patterns again and again, as a highlighter
// made anew for each call does, and returns how many matched.
const makeRegExps = `
function make(n) {
	let hits = 0
	for (let round = 0; round < 4; round++) {
		for (let i = 0; i < n; i++) {
			if (new RegExp('(?:kw' + i + '|id' + i + ')\\b[\\w$]*', 'g').test('x kw' + i + ' y')) hits++
		}
	}
	return hits
}
`

// A new VM keeps the regexps it compiled, at most DefaultRegExpCache of
// them, and after SetRegExpCache(0, 0) the memory they took is free again.
func TestRegExpCache(t *testing.T) {
	m := newTestVM(t)
	if _, err := m.Eval(makeRegExps, EvalGlobal); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Call("make", 1000); err != nil {
		t.Fatal(err)
	}
	with := vmMemory(m).Fmalloc_size
	if err := m.SetRegExpCache(0, 0); err != nil {
		t.Fatal(err)
	}
	without := vmMemory(m).Fmalloc_size
	if with-without < DefaultRegExpCache*100 || with-without > DefaultRegExpCacheBytes+64<<10 {
		t.Errorf("%d bytes with the cache, %d without", with, without)
	}
	for _, c := range [][2]int{{-1, 10}, {10, -1}, {math.MaxInt32/8 + 1, 1 << 20}} {
		if err := m.SetRegExpCache(c[0], c[1]); err == nil {
			t.Errorf("SetRegExpCache(%d, %d) took it", c[0], c[1])
		}
	}
	// The results are the same with the cache off, small and at its
	// defaults.
	for _, c := range [][2]int{{0, 0}, {16, 1 << 20}, {math.MaxInt32 / 8, 4 << 10}, {DefaultRegExpCache, DefaultRegExpCacheBytes}} {
		if err := m.SetRegExpCache(c[0], c[1]); err != nil {
			t.Fatal(err)
		}
		v, err := m.Call("make", 300)
		if err != nil || fmt.Sprint(v) != "1200" {
			t.Errorf("cache %v: %v, %v", c, v, err)
		}
	}
}

// What the cache keeps counts against the memory limit: a VM that runs out
// of memory with the cache full reports it as out of memory, uncatchable,
// and is usable after the call, with the cache in use. The limit is lifted
// before the array is let go: the VM is full to within one allocation, and
// small blocks come in arenas of 4 KiB (docs/allocator.md), so even
// `a = null` needs room the loop may not have left, by how its last
// allocations fell. Each limit makes the loop run out at another
// allocation, with 2.8 to 12 KiB left.
func TestRegExpCacheMemoryLimit(t *testing.T) {
	for _, room := range []uintptr{200 << 10, 256 << 10, 260 << 10, 300 << 10} {
		t.Run(fmt.Sprint(room>>10, "KiB"), func(t *testing.T) {
			m := newTestVM(t)
			if _, err := m.Eval(makeRegExps, EvalGlobal); err != nil {
				t.Fatal(err)
			}
			if _, err := m.Call("make", 300); err != nil {
				t.Fatal(err)
			}
			m.SetMemoryLimit(uintptr(vmMemory(m).Fmalloc_size) + room)
			_, err := m.Eval(`var a = []; for (;;) { try { a.push(new RegExp('z' + a.length + '+', 'g'), 'x'.repeat(64) + a.length) } catch (e) { a.push(e) } }`, EvalGlobal)
			var e *Error
			if !errors.As(err, &e) || !e.OutOfMemory || !e.Uncatchable {
				t.Fatalf("got %v, want an uncatchable out of memory error", err)
			}
			m.SetMemoryLimit(^uintptr(0))
			if _, err := m.Eval(`a = null`, EvalGlobal); err != nil {
				t.Fatal(err)
			}
			if v, err := m.Call("make", 300); err != nil || fmt.Sprint(v) != "1200" {
				t.Errorf("after: %v, %v", v, err)
			}
		})
	}
}

// The eval timeout and Interrupt stop a loop that makes regexps the cache
// keeps, as one that compiles them, within a few polls: compiling a
// long pattern counts its steps toward the interrupt check, so the check
// does not wait for thousands of compiles, and one long compile stops in
// the middle. A catch block does not take the interrupt.
func TestRegExpCacheStops(t *testing.T) {
	for _, r := range []struct {
		js    string
		limit int64
	}{
		{`for (;;) new RegExp(p, 'g')`, stopPolls},
		{`for (let i = 0; ; i++) new RegExp(p + (i % 1000), 'g')`, stopPolls},
		{`for (let i = 0; ; i++) try { new RegExp(p + i, 'g') } catch (e) {}`, stopPolls},
		{`for (let i = 0; ; i++) eval('/' + p + i + '/g')`, stopPolls},
		// one compile of a pattern of 13.2M characters: its conversion to
		// UTF-8 ran 41 polls with no reading of the flag, and the work after
		// the parse about 1,360, past the 1000 after which a poll kills the
		// script
		{`new RegExp(p.replaceAll('(', '(?:').repeat(6000))`, stopPolls},
		// strings of one hash and one length in a class: a string was
		// compared with the ones before it in one pass of memcmp, a byte
		// at a time, up to 31 times 128 KiB with no reading of the flag.
		// The pattern is a string of 16 bits, which the compile converts
		// to UTF-8 in steps of 65,536 characters, about a tick each: up to
		// 7 polls (see regexpStopPolls in internal/cengine), so a stop that
		// comes in the conversion is held to stepStopPolls
		{`new RegExp('[\\q{' + q + '}]', 'v')`, stepStopPolls},
	} {
		js := r.js
		for _, c := range []struct {
			name string
			arm  func(m *VM) (stop func())
		}{
			{"timeout", func(m *VM) func() {
				m.SetEvalTimeout(10 * time.Millisecond)
				return func() {}
			}},
			{"interrupt", func(m *VM) func() { return interruptAfter(m, 10*time.Millisecond) }},
		} {
			t.Run(c.name, func(t *testing.T) {
				m := newTestVM(t)
				if _, err := m.Eval(`var p = '(a|b|c)+d'.repeat(200), q = []; for (let i = 0; i < 32; i++) { let s = 'x'.repeat(1 << 15); for (let j = 0; j < 5; j++) s += i >> j & 1 ? 'b\u00f9' : 'a\u0200'; q.push(s) } q = q.join('|'); 0`, EvalGlobal); err != nil {
					t.Fatal(err)
				}
				defer c.arm(m)()
				start := time.Now()
				_, err := m.Eval(js, EvalGlobal)
				took := time.Since(start)
				if e := (*Error)(nil); !errors.As(err, &e) || e.Message != "interrupted" {
					t.Fatalf("%s: got %v, want interrupted", js, err)
				}
				checkStopped(t, m, err, took, r.limit, js+": ")
			})
		}
	}
}
