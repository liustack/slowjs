// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package slowjs // import "github.com/liustack/slowjs"

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// escapeReference is escapeTextForBrowser as React 19's production build of
// the string renderer has it. engine/test/unit-api.c checks the C function
// against the same text.
const escapeReference = `
var matchHtmlRegExp = /["'&<>]/;
function reactEscape(text) {
  if ("boolean" === typeof text || "number" === typeof text || "bigint" === typeof text)
    return "" + text;
  text = "" + text;
  var match = matchHtmlRegExp.exec(text);
  if (match) {
    var html = "", index, lastIndex = 0;
    for (index = match.index; index < text.length; index++) {
      switch (text.charCodeAt(index)) {
        case 34: match = "&quot;"; break;
        case 38: match = "&amp;"; break;
        case 39: match = "&#x27;"; break;
        case 60: match = "&lt;"; break;
        case 62: match = "&gt;"; break;
        default: continue;
      }
      lastIndex !== index && (html += text.slice(lastIndex, index));
      lastIndex = index + 1;
      html += match;
    }
    text = lastIndex !== index ? html + text.slice(lastIndex, index) : html;
  }
  return text;
}
`

// escapeDriver runs esc and reactEscape on the same inputs and says whether
// they give the same text or the same error and make the same calls on the
// objects they are given.
const escapeDriver = `
var bad = 0, count = 0, first = '';
function run1(f, make) {
  var log = [], r;
  try { r = { v: f(make(log)) } } catch (e) { r = { e: e instanceof Error ? e.name + ': ' + e.message : 'thrown ' + String(e) } }
  r.log = log.join();
  return r;
}
function same(make) {
  count++;
  var a = run1(esc, make), b = run1(reactEscape, make);
  var ok = a.log === b.log && a.e === b.e && (a.e !== undefined || (typeof a.v === 'string' && a.v === b.v));
  if (!ok) { bad++; if (!first) first = JSON.stringify(a) + ' against ' + JSON.stringify(b); }
}
function str(s) { same(function () { return s }) }
var alphabet = ['a', '&', '<', '>', '"', "'", 'é', 'Ā', '\ud800', '\udc00', '😀', ';', '\u0000'];
function gen(prefix, depth) {
  str(prefix);
  if (depth) for (var c of alphabet) gen(prefix + c, depth - 1);
}
gen('', 4);
var sizes = [1, 7, 4095, 4096, 4097, 8191, 8192, 8193, 12288, 100003];
var marks = ['&', '<', '>', '"', "'"];
for (var size of sizes) {
  for (var wide of ['', 'Ā']) {
    var filler = 'x'.repeat(size - 1) + wide;
    str(filler);
    for (var at of [0, 1, 4095, 4096, 4097, 8192, size - 2, size - 1]) {
      if (at < 0 || at >= size) continue;
      for (var m of marks) str(filler.slice(0, at) + m + filler.slice(at + 1));
    }
    str(marks.join('').repeat(Math.ceil(size / 5)).slice(0, size) + wide);
    str('&'.repeat(size) + wide);
  }
}
var piece = 'ab<cd' + 'e'.repeat(3000), rope = piece;
for (var i = 0; i < 12; i++) rope = rope + rope + i;
str(rope);
str(rope + 'Ā');
var others = [0, -0, 1, -1, 1.5, 1e21, 1e-7, NaN, Infinity, -Infinity, 2 ** 31, 123456789012345680000, 0.1 + 0.2,
  true, false, null, undefined, 10n, -(2n ** 70n), {}, [], [1, 2], ['<', '&'], new Date(0), /a<b/g, function () {},
  new String('a<b'), new Number(4), Object.create(null), Symbol('x'), Symbol.iterator, new Boolean(false), [undefined, null], new Error('<e>')];
for (var x of others) same(function () { return x });
same(function (log) { return { toString() { log.push('t'); return '<b>' } } });
same(function (log) { return { valueOf() { log.push('v'); return '&' } } });
same(function (log) { return { valueOf() { log.push('v'); return 7 }, toString() { log.push('t'); return 'x&y' } } });
same(function (log) { return { valueOf() { log.push('v'); return {} }, toString() { log.push('t'); return 'x"y' } } });
same(function (log) { return { valueOf() { log.push('v'); return {} }, toString() { log.push('t'); return {} } } });
same(function (log) { return { [Symbol.toPrimitive](hint) { log.push(hint); return hint + '<' } } });
same(function (log) { return { [Symbol.toPrimitive](hint) { log.push(hint); return Symbol('s') } } });
same(function (log) { return { [Symbol.toPrimitive](hint) { log.push(hint); return {} } } });
same(function (log) { return { toString() { log.push('t'); throw new RangeError('boom') } } });
same(function (log) { return { toString() { log.push('t'); throw 'plain' } } });
same(function (log) { return { toString() { log.push('t'); return rope } } });
same(function (log) { return { toString() { log.push('t'); return 'a'.repeat(5000) + '&' } } });
same(function (log) { return { get [Symbol.toPrimitive]() { log.push('g'); return undefined }, toString() { log.push('t'); return "'" } } });
same(function (log) { return new Proxy({}, { get(t, k) { log.push(String(k)); return k === 'toString' ? function () { return '>' } : undefined } }) });
var noArgs = [esc(), reactEscape()];
if (noArgs[0] !== noArgs[1] || noArgs[0] !== 'undefined') { bad++; first = first || 'no argument: ' + noArgs; }
var more = [esc('<', 'ignored', '&'), esc.call({ toString() { return 'this' } }, '<'), new Function('esc', 'return esc("a&b")')(esc)];
if (more.join() !== '&lt;,&lt;,a&amp;b') { bad++; first = first || 'arguments and this: ' + more; }
bad === 0 ? 'ok ' + (count > 20000) : bad + ' of ' + count + ' differ, the first: ' + first
`

// The function gives what React DOM's escapeTextForBrowser gives, for strings
// of both widths and every length and place of the five characters, ropes, and
// everything that is not a string.
func TestEscapeText(t *testing.T) {
	m := newTestVM(t)
	if err := m.RegisterEscapeText("esc"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Eval(escapeReference, EvalGlobal); err != nil {
		t.Fatal(err)
	}
	if got := evalString(t, m, escapeDriver); got != "ok true" {
		t.Fatal(got)
	}
}

// The function is on the global object only for a host that registers it,
// under the name the host gives, as a built-in function.
func TestRegisterEscapeText(t *testing.T) {
	m := newTestVM(t)
	if got := evalString(t, m, `typeof __escape`); got != "undefined" {
		t.Fatalf("a VM that registered nothing has __escape: %s", got)
	}
	if err := m.RegisterEscapeText("__escape"); err != nil {
		t.Fatal(err)
	}
	got := evalString(t, m, `
	  var d = Object.getOwnPropertyDescriptor(globalThis, '__escape');
	  JSON.stringify([d.writable, d.enumerable, d.configurable, __escape.name, __escape.length,
	    Object.hasOwn(__escape, 'prototype'), String(__escape), Object.getPrototypeOf(__escape) === Function.prototype,
	    (() => { try { new __escape('a'); return 'made' } catch (e) { return e.name } })(),
	    Object.keys(globalThis).includes('__escape'), __escape('<a href="x">&\'</a>')])`)
	const want = `[true,false,true,"__escape",1,false,"function __escape() {\n    [native code]\n}",true,"TypeError",false,"&lt;a href=&quot;x&quot;&gt;&amp;&#x27;&lt;/a&gt;"]`
	if got != want {
		t.Fatalf("got %s\nwant %s", got, want)
	}

	// another VM is unaffected, and a second call replaces the function
	other := newTestVM(t)
	if got := evalString(t, other, `typeof __escape`); got != "undefined" {
		t.Fatalf("another VM has __escape: %s", got)
	}
	if _, err := m.Eval(`var first = __escape`, EvalGlobal); err != nil {
		t.Fatal(err)
	}
	if err := m.RegisterEscapeText("__escape"); err != nil {
		t.Fatal(err)
	}
	if got := evalString(t, m, `String(first !== __escape) + __escape('&')`); got != "true&amp;" {
		t.Fatal(got)
	}

	// it replaces what a script assigned, and keeps what cannot be replaced
	if _, err := m.Eval(`globalThis.assigned = 1; var declared = 2`, EvalGlobal); err != nil {
		t.Fatal(err)
	}
	if err := m.RegisterEscapeText("assigned"); err != nil {
		t.Fatal(err)
	}
	if got := evalString(t, m, `typeof assigned + assigned('<')`); got != "function&lt;" {
		t.Fatal(got)
	}
	for _, name := range []string{"undefined", "NaN", "declared"} {
		if err := m.RegisterEscapeText(name); err == nil {
			t.Errorf("RegisterEscapeText(%q) returned no error", name)
		}
	}
	if got := evalString(t, m, `String(declared) + typeof undefined`); got != "2undefined" {
		t.Fatal(got)
	}
	if err := m.RegisterEscapeText(""); err == nil {
		t.Error("an empty name returned no error")
	}
	if err := m.RegisterEscapeText("a\x00b"); err == nil {
		t.Error("a name with a NUL byte returned no error")
	}
	// invalid UTF-8 in the name becomes U+FFFD as it does for RegisterFunc
	if err := m.RegisterEscapeText("bad\xc0name"); err != nil {
		t.Fatal(err)
	}
	if got := evalString(t, m, `typeof globalThis['bad�name']`); got != "function" {
		t.Fatal(got)
	}
}

// The eval timeout stops the function on long strings, in one call and in a
// loop of calls.
func TestEscapeTextStops(t *testing.T) {
	for _, c := range []struct{ name, setup, loop string }{
		{"dirty", `var s = '<'.repeat(1 << 24)`, `for (;;) esc(s)`},
		{"clean", `var s = 'a'.repeat(1 << 24)`, `for (;;) esc(s)`},
		{"16-bit", `var s = 'Ā&'.repeat(1 << 23)`, `for (;;) esc(s)`},
		{"short strings", `var s = 'a<b'.repeat(1 << 12)`, `for (;;) esc(s)`},
	} {
		t.Run(c.name, func(t *testing.T) {
			m := newTestVM(t)
			if err := m.RegisterEscapeText("esc"); err != nil {
				t.Fatal(err)
			}
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
			if took > stopLimit {
				t.Errorf("stopped %v after the call started, want about 10ms", took)
			}
		})
	}
}

// A string that is too long once escaped throws the RangeError of a string
// that cannot be built, with no memory taken for it.
func TestEscapeTextTooLong(t *testing.T) {
	m := newTestVM(t)
	if err := m.RegisterEscapeText("esc"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Eval(`var s = '&'.repeat(1 << 28)`, EvalGlobal); err != nil {
		if strings.Contains(err.Error(), "out of memory") {
			t.Skip("no memory for a string of 2**28 units")
		}
		t.Fatal(err)
	}
	got := evalString(t, m, `try { esc(s); 'made' } catch (e) { e.name + ': ' + e.message }`)
	if got != "RangeError: invalid string length" {
		t.Fatal(got)
	}
}
