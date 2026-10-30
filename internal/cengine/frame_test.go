// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package cengine

import (
	"fmt"
	"strings"
	"testing"
)

// frameValues are the kinds of value a local variable holds at the end of a
// call: those with a reference count and those without one.
var frameValues = []struct{ make, show string }{
	{"({a: 1})", "[object Object]"},
	{"'ab'.repeat(3)", "ababab"},
	{"[1, 2, 3]", "1,2,3"},
	{"(() => 1)", "() => 1"},
	{"10n ** 30n", "1000000000000000000000000000000"},
	{"Symbol('s')", "Symbol(s)"},
	{"1.5", "1.5"},
	{"7", "7"},
	{"true", "true"},
	{"null", "null"},
	{"undefined", "undefined"},
}

// TestFrameValuesFreed ends calls of functions with every number of local
// variables up to 70 and a few more, one of the locals holding each kind of
// value at each place, and the stack of the function holding values as well.
// Every value must be freed once: closeVM fails on a block that is left, and
// a value freed twice breaks the engine's counts.
func TestFrameValuesFreed(t *testing.T) {
	v := newVM(t, Options{Seed: 1})
	var src strings.Builder
	src.WriteString("var failures = [];\n")
	src.WriteString("function check(name, got, want) { if (got !== want) failures.push(name + ': ' + got + ' != ' + want) }\n")
	counts := []int{}
	for k := 0; k <= 70; k++ {
		counts = append(counts, k)
	}
	counts = append(counts, 127, 128, 129, 200)
	for _, k := range counts {
		var decl strings.Builder
		for i := 0; i < k; i++ {
			fmt.Fprintf(&decl, "var v%d;", i)
		}
		// Places: each of the first and last 9, and a few between.
		for j := 0; j < k; j++ {
			if j >= 9 && j < k-9 && j%11 != 0 {
				continue
			}
			for n, kind := range frameValues {
				fmt.Fprintf(&src, "check('%d/%d/%d', String(new Function('mk', %q)(() => %s)), %q);\n",
					k, j, n, fmt.Sprintf("%s v%d = mk(); return v%d;", decl.String(), j, j), kind.make, kind.show)
			}
		}
		// Every local holds a value with a reference count.
		var all strings.Builder
		for i := 0; i < k; i++ {
			fmt.Fprintf(&all, "v%d = {n: %d};", i, i)
		}
		fmt.Fprintf(&src, "check('%d/all', String(new Function(%q)()), '%d');\n", k, decl.String()+all.String()+" return "+fmt.Sprint(k), k)
	}
	// Values left on the stack when the call ends, and locals that closures
	// keep, which close_var_refs copies out first.
	src.WriteString(`
function tail(n, o) { return n == 0 ? o.v : tail(n - 1, {v: o.v + 1}) }
check('tail', tail(50, {v: 0}), 50);
function forOf(a) { var s = [], t = {}; for (var x of a) { var u = {x}; return u.x } }
check('forOf', forOf([{}, {}]) instanceof Object, true);
function inFinally() { var a = {}, b = 'x'.repeat(5); try { return a } finally { b = {} } }
check('finally', typeof inFinally(), 'object');
function captured() { var a = {n: 1}, b = [2], c = 'cc'.repeat(2), d = 4, e = {n: 5}, f = [6], g = 7, h = {n: 8}, i = [9];
  return [() => a.n, () => b[0], () => c, () => d, () => e.n, () => f[0], () => g, () => h.n, () => i[0]] }
check('captured', captured().map((f) => f()).join(), '1,2,cccc,4,5,6,7,8,9');
function withStack(a, b) { return a + (b ? {valueOf() { return 1 }} : 2) + [3].length }
check('stack', withStack(1, true) + withStack(1, false), 7);
function throws() { var a = {}, b = [1], c = 'z'.repeat(3); null.x }
for (var i = 0; i < 10; i++) { try { throws() } catch (e) { check('throws', e instanceof TypeError, true) } }
function* gen() { var a = {}, b = [1], c = 'z'.repeat(3), d = 1; yield a; yield b; return c }
check('gen', [...gen()].length, 2);
async function asy() { var a = {}, b = [1], c = 'z'.repeat(3), d = 1; await 0; return b[0] }
var asyResult; asy().then((r) => { asyResult = r });
`)
	src.WriteString("failures.join('\\n') + '|' + failures.length")
	got, err := v.Eval(src.String(), "frame.js")
	if err != nil {
		t.Fatal(err)
	}
	if got != "|0" {
		t.Errorf("failures:\n%s", got)
	}
	closeVM(t, v)
}

// TestLocalsFreedInFewTurns holds the loops that set the local variables of
// a call and free its values to a few turns. With an interval of 1 each turn
// that counts polls, so the polls of calls of a function with 200 locals,
// less those of calls of a function with none, are the turns of the two
// loops for each call.
func TestLocalsFreedInFewTurns(t *testing.T) {
	needPolls(t)
	var decl strings.Builder
	for i := 0; i < 200; i++ {
		fmt.Fprintf(&decl, "var v%d;", i)
	}
	run := func(f string) int64 {
		v := newVM(t, Options{Seed: 1})
		v.Host().SetPollInterval(1)
		src := fmt.Sprintf("function f(a) { %s return a } var s = 0; for (var i = 0; i < 1000; i++) s += f(i); s", f)
		got, err := v.Eval(src, "turns.js")
		if err != nil || got != "499500" {
			t.Fatalf("%q, %v", got, err)
		}
		n := v.Host().Counts().Polls
		closeVM(t, v)
		return n
	}
	turns := float64(run(decl.String())-run("")) / 1000
	t.Logf("%.1f turns for each call of a function with 200 locals", turns)
	// Setting eight to a turn takes 25, looking at eight to a turn 25. Setting
	// four to a turn and freeing two to a turn took 150.
	if turns > 60 {
		t.Errorf("a call of a function with 200 locals takes %.1f turns, want 60 or fewer", turns)
	}
}
