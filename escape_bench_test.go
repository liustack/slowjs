// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package slowjs // import "github.com/liustack/slowjs"

import (
	"strings"
	"testing"
)

// tifaFallback is escapeTextForBrowser as tifa's Vite plugin leaves it in a VM without the function of
// RegisterEscapeText: a cache of the escapes in front of it, and the five replaceAll calls in place of its
// loop over the characters.
const tifaFallback = `
var tifaEscapes = new Map();
function tifaCached(text) {
  if ("string" !== typeof text) return reactEscape(text);
  if (null === matchHtmlRegExp.exec(text)) return text;
  var escaped = tifaEscapes.get(text);
  if (void 0 === escaped) {
    escaped = text.replaceAll("&", "&amp;").replaceAll("<", "&lt;").replaceAll(">", "&gt;").replaceAll('"', "&quot;").replaceAll("'", "&#x27;");
    tifaEscapes.set(text, escaped);
  }
  return escaped;
}
`

// BenchmarkEscapeText times one call of the function of RegisterEscapeText against React DOM's own
// escapeTextForBrowser written in JavaScript (react) and against tifa's cache and replaceAll in front of it
// (tifa), on the texts of a page: a short one with nothing to escape, a class name with characters to
// escape, a sentence, a number, and long texts. ns/op is per call. A loop of the empty function (empty) is
// the cost of the loop and the call, which all of them pay. The cache of tifa answers the same text each
// time, the best case for it.
func BenchmarkEscapeText(b *testing.B) {
	inputs := []struct{ name, js string }{
		{"short-clean", `'data-slot'`},
		{"class-dirty", `'[&_svg]:size-4 has-[>svg]:px-3'`},
		{"sentence-clean", `'Pour-over kettle with a gooseneck spout and a thermometer'`},
		{"sentence-dirty", `'Pour-over kettle & dripper: "gooseneck" <1.2 L> it\'s a set'`},
		{"number", `1234.5`},
		{"512-clean", `'a'.repeat(512)`},
		{"512-dirty", `'a&b'.repeat(171)`},
		{"4096-clean", `'a'.repeat(4096)`},
	}
	for _, in := range inputs {
		for _, f := range []string{"empty", "react", "tifa", "esc"} {
			b.Run(in.name+"/"+f, func(b *testing.B) {
				m, err := NewVM()
				if err != nil {
					b.Fatal(err)
				}
				b.Cleanup(func() { m.Close() })
				if err := m.RegisterEscapeText("esc"); err != nil {
					b.Fatal(err)
				}
				if _, err := m.Eval(escapeReference+tifaFallback+`
function empty(t) { return t }
var input = `+in.js+`;
function loop(n, f) { for (var i = 0; i < n; i++) f(input) }
function loopEmpty(n) { loop(n, empty) }
function loopReact(n) { loop(n, reactEscape) }
function loopTifa(n) { loop(n, tifaCached) }
function loopEsc(n) { loop(n, esc) }
`, EvalGlobal); err != nil {
					b.Fatal(err)
				}
				name := map[string]string{"empty": "loopEmpty", "react": "loopReact", "tifa": "loopTifa", "esc": "loopEsc"}[f]
				if strings.Contains(in.name, "4096") {
					b.SetBytes(4096)
				}
				b.ResetTimer()
				if _, err := m.Call(name, b.N); err != nil {
					b.Fatal(err)
				}
			})
		}
	}
}
