// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package quickjs // import "github.com/liustack/quickjs"

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// The eval timeout stops the compare of two long strings, the copy of a long
// string into a string buffer and the key of Symbol.for of a long string
// within a few polls, wherever in the call it comes: they read the
// interrupt flag every 65,536 characters, or every MiB of a plain copy. So
// do the substitution of a long replacement, the flags of a regexp and
// String.raw of a long length, and a call whose copy was stopped leaves at
// once.
// Each stop is made in a new VM, as the first Symbol.for of a key makes the
// atom that the next ones find. Before, s < s2 of two strings of 64 Mi
// characters ran for thousands of polls without reading it, past the grace
// of the kill.
func TestStringCompareCopyStops(t *testing.T) {
	const n = `var n = 1 << 26`
	const s, w = n + `, s = 'a'.repeat(n)`, n + `, w = '\u3000'.repeat(n >> 1)`
	for _, c := range []struct{ name, setup, call string }{
		{"less than", s + `, s2 = 'a'.repeat(n)`, `s < s2`},
		{"less than of wide strings", w + `, w2 = '\u3000'.repeat(n >> 1)`, `w < w2`},
		{"sort", s + `, s2 = 'a'.repeat(n)`, `[s2, s].sort()`},
		{"sort of wide strings", w + `, w2 = '\u3000'.repeat(n >> 1)`, `[w2, w].sort()`},
		{"Symbol.for", s, `Symbol.for(s)`},
		{"join", s, `[s, 'x'].join('')`},
		{"join of wide strings", w, `[w, 'x'].join('')`},
		{"join of one", s, `[s].join()`},
		{"padEnd", s, `s.padEnd(n + 1)`},
		{"replace", s, `s.replace('a', 'c')`},
		{"replace in a wide string", w, `w.replace('\u3000', 'c')`},
		{"String.raw", s, `String.raw({raw: [s, s]}, 1)`},
		{"Function", n + `, sp = ' '.repeat(n)`, `Function(sp)`},
		{"replace with a long replacement", n + `, r = 'a'.repeat(1 << 20) + '$$' + 'b'.repeat(n)`, `'x'.replace('x', r)`},
		{"replace with a replacement of no $", n + `, r = 'b'.repeat(n)`, `'x'.replace('x', r)`},
		{"replace with many $$", n + `, r = '$$'.repeat(n >> 2)`, `'x'.replace('x', r)`},
		{"replace with many $< of a group", `var r = /(?<a>x)/, rp = '$<'.repeat(1 << 14)`, `(r.lastIndex = '0', 'x'.replace(r, rp))`},
		{"flags of a regexp", n + `, o = {flags: 'x'.repeat(n), exec() { return null }}`, `RegExp.prototype[Symbol.replace].call(o, 'x', 'y')`},
		{"String.raw of a long length", `var n = 0`, `String.raw({raw: {length: 1 << 22}})`},
	} {
		t.Run(c.name, func(t *testing.T) {
			vm := func() *VM {
				m, err := NewVM()
				if err != nil {
					t.Fatal(err)
				}
				if _, err := m.Eval(c.setup+`; 0`, EvalGlobal); err != nil {
					m.Close()
					t.Fatal(err)
				}
				return m
			}
			// one call, then a stop at each sixth of it
			m := vm()
			start := time.Now()
			_, err := m.Eval(c.call+`; 0`, EvalGlobal)
			call := time.Since(start)
			m.Close()
			if err != nil {
				t.Fatal(err)
			}
			for k := 1; k <= 5; k++ {
				m := vm()
				if err := m.SetEvalTimeout(max(call*time.Duration(k)/6, time.Millisecond)); err != nil {
					t.Fatal(err)
				}
				start := time.Now()
				_, err := m.Eval(`for (;;) `+c.call, EvalGlobal)
				took := time.Since(start)
				polls := m.h.StopPolls()
				if errors.Is(err, ErrKilled) {
					m.Close()
					t.Fatalf("killed after %v of a call of %v, want the interrupt", took, call)
				}
				var e *Error
				if !errors.As(err, &e) || !e.Interrupted || !e.Uncatchable {
					m.Close()
					t.Fatalf("got %v after %v, want interrupted", err, took)
				}
				if polls > copyStopPolls {
					t.Errorf("%d polls saw the request to stop, want at most %d (stopped %v after the start, in a call of %v)", polls, copyStopPolls, took, call)
				}
				t.Logf("stopped after %v of a call of %v, %d polls saw the request", took, call, polls)
				if err := m.SetEvalTimeout(0); err != nil {
					t.Fatal(err)
				}
				if v, err := m.Eval(`1+1`, EvalGlobal); err != nil || v != 2 {
					t.Fatalf("the VM after the stop: 1+1 got %v, %v", v, err)
				}
				m.Close()
			}
		})
	}
}

// copyStopPolls is the most polls TestStringCompareCopyStops lets see the
// request, the limit of the parse (parseStopPolls): Function parses what it
// copies.
const copyStopPolls = parseStopPolls

// The eval timeout stops the sort of the export names of a module, which a
// namespace import makes, within a few polls: two names of 64 Mi characters
// that differ in the last one are compared in steps that read the interrupt
// flag. The exporting module is the first one named <eval>, which the
// importing one finds by that name. Before, the compare ran for thousands of
// polls without reading it.
func TestModuleExportNamesStop(t *testing.T) {
	p := strings.Repeat("a", 1<<26)
	exporter := `const x = 0; export {x as "` + p + `a", x as "` + p + `b"}`
	const importer = `import * as ns from "<eval>"; globalThis.keys = Object.keys(ns).map(k => k.slice(-2)).join(); for (;;) {}`
	vm := func() *VM {
		m, err := NewVM()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := m.Eval(exporter, EvalModule); err != nil {
			m.Close()
			t.Fatal(err)
		}
		return m
	}
	// one import, which loops once it has the names, then a stop at each
	// sixth of the time it takes to get there
	m := vm()
	if err := m.SetEvalTimeout(time.Second); err != nil {
		t.Fatal(err)
	}
	_, err := m.Eval(importer, EvalModule)
	var e *Error
	if !errors.As(err, &e) || !e.Interrupted {
		m.Close()
		t.Fatalf("the import got %v, want interrupted in its loop", err)
	}
	if err := m.SetEvalTimeout(0); err != nil {
		t.Fatal(err)
	}
	v, err := m.Eval(`keys`, EvalGlobal)
	m.Close()
	if err != nil || v != "aa,ab" {
		t.Fatalf("the keys of the namespace: got %v, %v, want aa,ab", v, err)
	}
	m = vm()
	start := time.Now()
	if _, err := m.Eval(`import * as ns from "<eval>"; globalThis.n = Object.keys(ns).length`, EvalModule); err != nil {
		m.Close()
		t.Fatal(err)
	}
	call := time.Since(start)
	m.Close()
	for k := 1; k <= 5; k++ {
		m := vm()
		if err := m.SetEvalTimeout(max(call*time.Duration(k)/6, time.Millisecond)); err != nil {
			t.Fatal(err)
		}
		start := time.Now()
		_, err := m.Eval(importer, EvalModule)
		took := time.Since(start)
		polls := m.h.StopPolls()
		if errors.Is(err, ErrKilled) {
			m.Close()
			t.Fatalf("killed after %v of an import of %v, want the interrupt", took, call)
		}
		var e *Error
		if !errors.As(err, &e) || !e.Interrupted || !e.Uncatchable {
			m.Close()
			t.Fatalf("got %v after %v, want interrupted", err, took)
		}
		if polls > copyStopPolls {
			t.Errorf("%d polls saw the request to stop, want at most %d (stopped %v after the start, in an import of %v)", polls, copyStopPolls, took, call)
		}
		t.Logf("stopped after %v of an import of %v, %d polls saw the request", took, call, polls)
		if err := m.SetEvalTimeout(0); err != nil {
			t.Fatal(err)
		}
		if v, err := m.Eval(`1+1`, EvalGlobal); err != nil || v != 2 {
			t.Fatalf("the VM after the stop: 1+1 got %v, %v", v, err)
		}
		m.Close()
	}
}

// A compare, a copy and a key that read the flag on their way give what
// they gave: strings of 200,000 characters and more, which cross the places
// where they read it.
func TestStringCompareCopyLongResults(t *testing.T) {
	m := newTestVM(t)
	if _, err := m.Eval(`var a = 'a'.repeat(200000), w = '　'.repeat(100000) + 'é'.repeat(100000); 0`, EvalGlobal); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ expr, want string }{
		{`[a + 'a' < a + 'b', a + 'b' < a + 'a', a < a + 'a', a + 'a' < a, a < a.slice(0)].join()`, `true,false,true,false,false`},
		{`[w + 'a' < w + 'b', w + '、' < w + '　', w < a, a < w].join()`, `true,false,false,true`},
		{`[a + 'c', a + 'a', w, a + 'b'].sort().map(x => x.slice(-1)).join()`, `a,b,c,é`},
		{`[Symbol.for(a) === Symbol.for(a.slice(0)), Symbol.keyFor(Symbol.for(a)) === a, Symbol.for(a) !== Symbol.for(a + 'x')].join()`, `true,true,true`},
		{`[a, 'x'].join('') === a + 'x'`, `true`},
		{`[w, 'x', a].join('-') === w + '-x-' + a`, `true`},
		{`a.padEnd(200001, 'z').slice(-2)`, `az`},
		{`w.replace('　', 'c').slice(0, 2) + w.replace('é', 'c').slice(99999, 100002)`, `c　　cé`},
		{`String.raw({raw: [a, w]}, 1) === a + '1' + w`, `true`},
		{`String.raw({raw: {length: 3, 0: 'a', 1: 'b', 2: 'c'}}, 1, 2)`, `a1b2c`},
		{`'x'.replace('x', a + '$$' + w + '$&$` + "`" + `$\'') === a + '$' + w + 'x'`, `true`},
		{`['x'.replace('x', '$$'.repeat(3000)).length, 'x'.replace('x', '$<'.repeat(3000)).length].join()`, `3000,6000`},
		{`['$<$<$<$<$<$<a>$<', '$<a>$<$<'].map(p => { var r = /(?<a>x)/; r.lastIndex = '0'; return 'x'.replace(r, p) }).join()`, `$<,x$<$<`},
		{`(r => (r.lastIndex = '0', a.replace(r, 'b')))(/a/g) === 'b'.repeat(200000)`, `true`},
		{`RegExp.prototype[Symbol.replace].call({flags: 'x'.repeat(70000) + 'g', lastIndex: 0, exec() { return this.d ? null : (this.d = 1, Object.assign(['a'], {index: 0})) }}, 'ab', 'c')`, `cb`},
		{`Function(' '.repeat(200000) + 'return 7')()`, `7`},
	} {
		v, err := m.Eval(`String(`+c.expr+`)`, EvalGlobal)
		if err != nil {
			t.Errorf("%s: %v", c.expr, err)
			continue
		}
		if v != c.want {
			t.Errorf("%s = %v, want %s", c.expr, v, c.want)
		}
	}
}
