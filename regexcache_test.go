// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package slowjs // import "github.com/liustack/slowjs"

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
// and is usable after the call, with the cache in use.
func TestRegExpCacheMemoryLimit(t *testing.T) {
	m := newTestVM(t)
	if _, err := m.Eval(makeRegExps, EvalGlobal); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Call("make", 300); err != nil {
		t.Fatal(err)
	}
	m.SetMemoryLimit(uintptr(vmMemory(m).Fmalloc_size) + 256<<10)
	_, err := m.Eval(`var a = []; for (;;) { try { a.push(new RegExp('z' + a.length + '+', 'g'), 'x'.repeat(64) + a.length) } catch (e) { a.push(e) } }`, EvalGlobal)
	var e *Error
	if !errors.As(err, &e) || !e.OutOfMemory || !e.Uncatchable {
		t.Fatalf("got %v, want an uncatchable out of memory error", err)
	}
	if _, err := m.Eval(`a = null`, EvalGlobal); err != nil {
		t.Fatal(err)
	}
	m.SetMemoryLimit(^uintptr(0))
	if v, err := m.Call("make", 300); err != nil || fmt.Sprint(v) != "1200" {
		t.Errorf("after: %v, %v", v, err)
	}
}

// The eval timeout and Interrupt stop a loop that makes regexps the cache
// keeps, as one that compiles them, within a few milliseconds: compiling a
// long pattern counts its steps toward the interrupt check, so the check
// does not wait for thousands of compiles, and one long compile stops in
// the middle. A catch block does not take the interrupt.
func TestRegExpCacheStops(t *testing.T) {
	for _, js := range []string{
		`for (;;) new RegExp(p, 'g')`,
		`for (let i = 0; ; i++) new RegExp(p + (i % 1000), 'g')`,
		`for (let i = 0; ; i++) try { new RegExp(p + i, 'g') } catch (e) {}`,
		`for (let i = 0; ; i++) eval('/' + p + i + '/g')`,
		`new RegExp(p.replaceAll('(', '(?:').repeat(6000))`,
	} {
		for _, c := range []struct {
			name string
			arm  func(m *VM) (stop func())
		}{
			{"timeout", func(m *VM) func() {
				m.SetEvalTimeout(10 * time.Millisecond)
				return func() {}
			}},
			{"interrupt", func(m *VM) func() {
				timer := time.AfterFunc(10*time.Millisecond, m.Interrupt)
				return func() { timer.Stop() }
			}},
		} {
			t.Run(c.name, func(t *testing.T) {
				m := newTestVM(t)
				if _, err := m.Eval(`var p = '(a|b|c)+d'.repeat(200); 0`, EvalGlobal); err != nil {
					t.Fatal(err)
				}
				defer c.arm(m)()
				start := time.Now()
				_, err := m.Eval(js, EvalGlobal)
				took := time.Since(start)
				var e *Error
				if !errors.As(err, &e) || !e.Interrupted || !e.Uncatchable || e.Message != "interrupted" {
					t.Fatalf("%s: got %v, want interrupted", js, err)
				}
				if took > 50*time.Millisecond {
					t.Errorf("%s: stopped %v after the call started, want about 10ms", js, took)
				}
			})
		}
	}
}
