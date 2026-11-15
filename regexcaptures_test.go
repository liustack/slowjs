// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package slowjs // import "github.com/liustack/slowjs"

import (
	"errors"
	"fmt"
	"testing"
	"time"
)

// A regexp takes as many capture groups as V8 does, 32767 and group 0 (a
// capture index is a u16 in the bytecode, engine/libregexp.c), and one more
// is a SyntaxError. Upstream QuickJS stops at 254, which shiki's JavaScript
// engine passes for the grammars of JavaScript (410 groups), TypeScript (555)
// and HTML (912).
func TestRegExpManyCaptures(t *testing.T) {
	m := newTestVM(t)
	for _, c := range []struct {
		n    int
		want string
	}{
		{254, "255 u"},
		{255, "256 a"},
		{256, "257 a"},
		{912, "913 a"},
		{32767, "32768 a"},
		{32768, "SyntaxError"},
		{65536, "SyntaxError"},
	} {
		v, err := m.Eval(fmt.Sprintf(`(function () {
			try {
				var m = new RegExp("(a)".repeat(%d)).exec("a".repeat(%[1]d));
				return m.length + " " + (m[255] === undefined ? "u" : m[255]);
			} catch (e) {
				return e.name;
			}
		})()`, c.n), EvalGlobal)
		if err != nil {
			t.Fatalf("%d groups: %v", c.n, err)
		}
		if fmt.Sprint(v) != c.want {
			t.Errorf("%d groups: got %v, want %s", c.n, v, c.want)
		}
	}
}

// Groups nested 1000 deep compile and match. Deeper nesting, up to and past
// the limit of groups, ends the parse with a SyntaxError of the depth the
// parser counts (lre_depth_enter), and the VM goes on.
func TestRegExpDeepCaptures(t *testing.T) {
	m := newTestVM(t)
	for _, c := range []struct {
		n    int
		want string
	}{
		{1000, "1001"},
		{2000, "SyntaxError: stack overflow"},
		{32767, "SyntaxError: stack overflow"},
		{32768, "SyntaxError: stack overflow"},
		{100000, "SyntaxError: stack overflow"},
	} {
		v, err := m.Eval(fmt.Sprintf(`(function () {
			try {
				return String(new RegExp("(".repeat(%d) + "a" + ")".repeat(%[1]d)).exec("a").length);
			} catch (e) {
				return e.name + ": " + e.message;
			}
		})()`, c.n), EvalGlobal)
		if err != nil {
			t.Fatalf("%d deep: %v", c.n, err)
		}
		if s := fmt.Sprint(v); s != c.want {
			t.Errorf("%d deep: got %s, want %s", c.n, s, c.want)
		}
	}
	if v, err := m.Eval(`/(a)(b)/.exec("ab").length`, EvalGlobal); err != nil || fmt.Sprint(v) != "3" {
		t.Fatalf("after: %v, %v", v, err)
	}
}

// The captures of one exec of a regexp of 32767 groups, 512 KiB, count
// against the memory limit: with less room the exec ends with the
// uncatchable out of memory error, and with the limit lifted it runs.
func TestRegExpManyCapturesMemoryLimit(t *testing.T) {
	m := newTestVM(t)
	if _, err := m.Eval(`var re = new RegExp("(a)".repeat(32767)), s = "a".repeat(32767), r; 0`, EvalGlobal); err != nil {
		t.Fatal(err)
	}
	m.SetMemoryLimit(uintptr(vmMemory(m).Fmalloc_size) + 256<<10)
	_, err := m.Eval(`try { r = re.exec(s) } catch (e) { r = e } 0`, EvalGlobal)
	var e *Error
	if !errors.As(err, &e) || !e.OutOfMemory || !e.Uncatchable {
		t.Fatalf("got %v, want an uncatchable out of memory error", err)
	}
	m.SetMemoryLimit(^uintptr(0))
	if v, err := m.Eval(`re.exec(s).length`, EvalGlobal); err != nil || fmt.Sprint(v) != "32768" {
		t.Fatalf("after: %v, %v", v, err)
	}
}

// The eval timeout and Interrupt stop a regexp of many groups that backtracks
// for a long time.
func TestRegExpManyCapturesStop(t *testing.T) {
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
		for _, w := range []struct{ name, setup, loop string }{
			{"backtrack", `var re = new RegExp("^(?:(a)" + "|(a)".repeat(299) + ")*$"), s = "a".repeat(30) + "b"; 0`, `re.exec(s)`},
		} {
			t.Run(c.name+"/"+w.name, func(t *testing.T) {
				m := newTestVM(t)
				if _, err := m.Eval(w.setup, EvalGlobal); err != nil {
					t.Fatal(err)
				}
				stop := c.arm(m)
				defer stop()
				start := time.Now()
				_, err := m.Eval(w.loop, EvalGlobal)
				checkStopped(t, m, err, time.Since(start), stepStopPolls, "")
			})
		}
	}
}
