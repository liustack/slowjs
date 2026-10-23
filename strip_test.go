// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package slowjs // import "github.com/liustack/slowjs"

import (
	"regexp"
	"testing"
)

const stripSource = `function f(a) { return new Error('x').stack }`

// A function compiled with StripSource shows no source, as the engine's own
// built-ins do, also from bytecode another VM loads. Functions compiled
// before keep theirs.
func TestStripSource(t *testing.T) {
	m := newTestVM(t)
	evalString(t, m, `function before() { return 1 }`)
	m.SetStrip(StripSource)
	evalString(t, m, stripSource)
	native := evalString(t, m, `String(Math.max).replace('max', 'f')`)
	if got := evalString(t, m, `String(f)`); got != native {
		t.Errorf("got %q, want %q", got, native)
	}
	if got := evalString(t, m, `String(before)`); got != `function before() { return 1 }` {
		t.Errorf("a function compiled before lost its source: %q", got)
	}

	code, err := m.Compile(`function g() { return 2 }`, EvalGlobal)
	if err != nil {
		t.Fatal(err)
	}

	other := newTestVM(t)
	if _, err := other.EvalBytecode(code); err != nil {
		t.Fatal(err)
	}
	if got, want := evalString(t, other, `String(g)`), evalString(t, other, `String(Math.max).replace('max', 'g')`); got != want {
		t.Errorf("from bytecode: got %q, want %q", got, want)
	}

	// The page's own eval and new Function compile without source too.
	for _, js := range []string{`String(eval('(function e() { return 4 })'))`, `String(new Function('return 5')).replace('anonymous', 'e')`} {
		if got, want := evalString(t, m, js), evalString(t, m, `String(Math.max).replace('max', 'e')`); got != want {
			t.Errorf("%s: got %q, want %q", js, got, want)
		}
	}

	m.SetStrip(0)
	evalString(t, m, `function h() { return 3 }`)
	if got := evalString(t, m, `String(h)`); got != `function h() { return 3 }` {
		t.Errorf("after SetStrip(0): %q", got)
	}
}

// StripDebug leaves out where a function is, so its stack frames have no
// line and column, and its source too.
func TestStripDebug(t *testing.T) {
	where := regexp.MustCompile(`at f \(<eval>:\d+:\d+\)`)
	m := newTestVM(t)
	evalString(t, m, stripSource)
	if got := evalString(t, m, `f()`); !where.MatchString(got) {
		t.Fatalf("the stack has no line and column without StripDebug: %q", got)
	}

	m = newTestVM(t)
	m.SetStrip(StripDebug)
	evalString(t, m, stripSource)
	if got := evalString(t, m, `f()`); where.MatchString(got) || !regexp.MustCompile(`at f\b`).MatchString(got) {
		t.Errorf("got stack %q", got)
	}
	if got, want := evalString(t, m, `String(f)`), evalString(t, m, `String(Math.max).replace('max', 'f')`); got != want {
		t.Errorf("StripDebug kept the source: %q", got)
	}
}
