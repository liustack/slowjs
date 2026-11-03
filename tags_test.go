// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package slowjs // import "github.com/liustack/slowjs"

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

// The functions of RegisterStartTag and RegisterEndTag give what React DOM
// 19.3's pushStartGenericElement and endChunkForTag give, or leave the call to
// them. The script is the one the C tree runs
// (engine/test/compare/start_tag.js), and its text is the file next to it.
func TestStartTagLikeReact(t *testing.T) {
	script, err := os.ReadFile("engine/test/compare/start_tag.js")
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("engine/test/compare/start_tag.txt")
	if err != nil {
		t.Fatal(err)
	}
	m := newTestVM(t)
	if err := m.RegisterStartTag("startTag"); err != nil {
		t.Fatal(err)
	}
	if err := m.RegisterEndTag("endTag"); err != nil {
		t.Fatal(err)
	}
	got := evalString(t, m, string(script))
	if got != strings.TrimRight(string(want), "\n") {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
}

// Each function is on the global object only for a host that registers it,
// under the name the host gives, as a built-in function.
func TestRegisterTags(t *testing.T) {
	for _, c := range []struct {
		name     string
		register func(m *VM, name string) error
		length   string
		call     string // a call of __f, and what it gives
		want     string
	}{
		{"start", (*VM).RegisterStartTag, "6",
			`(() => { var t = [], d = {}; var r = __f(t, { className: 'a', children: 'x<' }, 'p', { viewTransition: null }, Object.prototype.hasOwnProperty, d);
			  return JSON.stringify([r, t, __f(t, { a: 1 }, 'p', {}, Object.prototype.hasOwnProperty, d) === d]) })()`,
			`[null,["<p class=\"a\">x&lt;"],true]`},
		{"end", (*VM).RegisterEndTag, "1", `__f('div') + __f(7)`, `</div></7>`},
	} {
		t.Run(c.name, func(t *testing.T) {
			m := newTestVM(t)
			if got := evalString(t, m, `typeof __f`); got != "undefined" {
				t.Fatalf("a VM that registered nothing has __f: %s", got)
			}
			if err := c.register(m, "__f"); err != nil {
				t.Fatal(err)
			}
			got := evalString(t, m, `
			  var d = Object.getOwnPropertyDescriptor(globalThis, '__f');
			  JSON.stringify([d.writable, d.enumerable, d.configurable, __f.name, __f.length,
			    Object.hasOwn(__f, 'prototype'), String(__f), Object.getPrototypeOf(__f) === Function.prototype,
			    (() => { try { new __f(); return 'made' } catch (e) { return e.name } })(),
			    Object.keys(globalThis).includes('__f')])`)
			want := `[true,false,true,"__f",` + c.length + `,false,"function __f() {\n    [native code]\n}",true,"TypeError",false]`
			if got != want {
				t.Fatalf("got %s\nwant %s", got, want)
			}
			if got := evalString(t, m, c.call); got != c.want {
				t.Fatalf("got %s\nwant %s", got, c.want)
			}

			// another VM is unaffected, and a second call replaces the function
			other := newTestVM(t)
			if got := evalString(t, other, `typeof __f`); got != "undefined" {
				t.Fatalf("another VM has __f: %s", got)
			}
			if _, err := m.Eval(`var first = __f`, EvalGlobal); err != nil {
				t.Fatal(err)
			}
			if err := c.register(m, "__f"); err != nil {
				t.Fatal(err)
			}
			if got := evalString(t, m, `String(first !== __f) + (`+c.call+`)`); got != "true"+c.want {
				t.Fatal(got)
			}

			// it replaces what a script assigned, and keeps what cannot be replaced
			if _, err := m.Eval(`globalThis.assigned = 1; var declared = 2`, EvalGlobal); err != nil {
				t.Fatal(err)
			}
			if err := c.register(m, "assigned"); err != nil {
				t.Fatal(err)
			}
			if got := evalString(t, m, `typeof assigned`); got != "function" {
				t.Fatal(got)
			}
			for _, name := range []string{"undefined", "NaN", "declared"} {
				if err := c.register(m, name); err == nil {
					t.Errorf("registering %q returned no error", name)
				}
			}
			if got := evalString(t, m, `String(declared) + typeof undefined`); got != "2undefined" {
				t.Fatal(got)
			}
			if err := c.register(m, ""); err == nil {
				t.Error("an empty name returned no error")
			}
			if err := c.register(m, "a\x00b"); err == nil {
				t.Error("a name with a NUL byte returned no error")
			}
			// invalid UTF-8 in the name becomes U+FFFD as it does for RegisterFunc
			if err := c.register(m, "bad\xc0name"); err != nil {
				t.Fatal(err)
			}
			if got := evalString(t, m, `typeof globalThis['bad�name']`); got != "function" {
				t.Fatal(got)
			}
		})
	}
}

// The functions of a VM keep working across its collections, with the names,
// styles and end tags they keep, and a VM closed after it registered them
// leaves nothing behind.
func TestTagsAcrossCollections(t *testing.T) {
	m := newTestVM(t)
	if err := m.RegisterStartTag("st"); err != nil {
		t.Fatal(err)
	}
	if err := m.RegisterEndTag("et"); err != nil {
		t.Fatal(err)
	}
	got := evalString(t, m, `
	  (() => {
	  var hop = Object.prototype.hasOwnProperty, d = {}, fc = { viewTransition: null }, total = 0;
	  for (var round = 0; round < 50; round++) {
	    var out = [];
	    for (var i = 0; i < 200; i++) {
	      var tag = 'x-' + (i % 70);
	      if (st(out, { id: 'r' + round, ['data-k' + i]: i, style: { ['w' + (i % 600)]: i }, children: 'item ' + i }, tag, fc, hop, d) === d) return 'declined';
	      out.push(et(tag));
	    }
	    var html = out.join('');
	    if (html.indexOf('<x-3 id="r' + round + '" data-k3="3" style="w3:3px">item 3</x-3>') < 0) return html.slice(0, 200);
	    total += out.length;
	  }
	  return String(total) })()`)
	if got != "20000" {
		t.Fatal(got)
	}
}

// A long string stops at an interrupt or the eval timeout within the
// function, wherever it is: the text of children or of an attribute, a style
// value or name, the tag, an end tag.
func TestTagsStop(t *testing.T) {
	for _, c := range []struct{ name, setup, loop string }{
		{"dirty children", `var p = { children: '<'.repeat(1 << 24) }`, `for (;;) st([], p, 'div', fc, hop, d)`},
		{"clean children", `var p = { children: 'a'.repeat(1 << 24) }`, `for (;;) st([], p, 'div', fc, hop, d)`},
		{"16-bit children", `var p = { children: 'Ā&'.repeat(1 << 23) }`, `for (;;) st([], p, 'div', fc, hop, d)`},
		{"attribute", `var p = { title: '"'.repeat(1 << 24) }`, `for (;;) st([], p, 'div', fc, hop, d)`},
		{"style value", `var p = { style: { color: ' '.repeat(1 << 25) + 'x' + ' '.repeat(1 << 25) } }`, `for (;;) st([], p, 'div', fc, hop, d)`},
		{"style name", `var p = { style: { ['aB'.repeat(1 << 23)]: 1 } }`, `for (;;) st([], p, 'div', fc, hop, d)`},
		{"custom style name", `var p = { style: { ['--' + '&'.repeat(1 << 23)]: 1 } }`, `for (;;) st([], p, 'div', fc, hop, d)`},
		{"tag", `var p = {}, tag = 'a'.repeat(1 << 25)`, `for (;;) st([], p, tag, fc, hop, d)`},
		{"end tag", `var s = 'a'.repeat(1 << 25), o = { toString() { return s } }`, `for (;;) et(o)`},
	} {
		t.Run(c.name, func(t *testing.T) {
			m := newTestVM(t)
			if err := m.RegisterStartTag("st"); err != nil {
				t.Fatal(err)
			}
			if err := m.RegisterEndTag("et"); err != nil {
				t.Fatal(err)
			}
			setup := `var hop = Object.prototype.hasOwnProperty, d = {}, fc = { viewTransition: null }; ` + c.setup + `; 0`
			if _, err := m.Eval(setup, EvalGlobal); err != nil {
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
			if took > stopLimit {
				t.Errorf("stopped %v after the call started, want about 10ms", took)
			}
		})
	}
}

// A start tag that could be longer than a string can be once escaped is left
// to React's functions, with target unchanged. Each string counts 6 units and
// 8192 are kept: children of 178955604 units after "<div>" fit, one more does
// not.
func TestStartTagTooLong(t *testing.T) {
	m := newTestVM(t)
	if err := m.RegisterStartTag("st"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Eval(`var s = 'a'.repeat(178955605)`, EvalGlobal); err != nil {
		if strings.Contains(err.Error(), "out of memory") {
			t.Skip("no memory for a string of 178955605 units")
		}
		t.Fatal(err)
	}
	got := evalString(t, m, `
	  var hop = Object.prototype.hasOwnProperty, d = {}, fc = { viewTransition: null }, t = ['x'];
	  var short = s.slice(1);
	  JSON.stringify([st(t, { children: s }, 'div', fc, hop, d) === d, t.length,
	    st(t, { title: s }, 'div', fc, hop, d) === d, t.length,
	    st(t, { children: short }, 'div', fc, hop, d), t.length, t[1].length])`)
	if got != `[true,1,true,1,null,2,178955609]` {
		t.Fatal(got)
	}
}
