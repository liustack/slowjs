// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package quickjs // import "github.com/liustack/quickjs"

import (
	"os"
	"strings"
	"testing"
)

// BenchmarkRegexColors times one replace with the regexp react-spring tests
// its values against, an alternation of 148 color names, on a value with no
// name (a shadow) and on one with a name in it. ns/op is per replace.
func BenchmarkRegexColors(b *testing.B) {
	js, err := os.ReadFile("engine/test/compare/recorded/regexsplit.js")
	if err != nil {
		b.Fatal(err)
	}
	start := strings.Index(string(js), "var colors = [")
	end := strings.Index(string(js)[start:], "];")
	if start < 0 || end < 0 {
		b.Fatal("no colors in engine/test/compare/recorded/regexsplit.js")
	}
	colors := string(js)[start : start+end+2]
	for _, c := range []struct{ name, s string }{
		{"shadow", `0px 2px 4px rgba(0,0,0,0.2)`},
		{"name", `1px solid darkslategray`},
	} {
		b.Run(c.name, func(b *testing.B) {
			m := newCallVM(b, colors+` const re = new RegExp('(' + colors.join('|') + ')(?!\\w)', 'g'); const s = '`+c.s+`'`, `s.replace(re, 'x')`)
			b.ResetTimer()
			if _, err := m.Call("loop", b.N); err != nil {
				b.Fatal(err)
			}
		})
	}
}

// regexShapes are regexps of the shapes that cost most in tifa's pages, with
// a value like the ones they get there. Each runs as its library runs it.
var regexShapes = []struct{ name, setup, call string }{
	// seroval's test for characters to escape, on a serialized value
	{"escape-class", `const re = /[\x00-\x07\x0b\x0e-\x1f<\u2028\u2029\ud800-\udfff]/; const s = '{"id":1,"name":"Linen shirt","tags":["new","sale"]}'.repeat(17)`, `re.test(s)`},
	// React's escape of text, on a short label
	{"html-escape", `const re = /["'&<>]/; const s = 'Add to cart'`, `re.exec(s)`},
	// core-js's URL parser, the trailing spaces of a URL
	{"url-trailing", `const re = /(^|[^\u0000-\u0020])[\u0000-\u0020]+$/; const s = 'https://shop.example/p/42'`, `s.replace(re, '$1')`},
	// emotion's labels, in the styles of a component
	{"emotion-label", `const re = /label:\s*([^\s;{]+)\s*(;|$)/g; const s = 'display:flex;flex-direction:column;align-items:center;justify-content:space-between;padding:16px 24px;border-bottom:1px solid rgba(0,0,0,0.12);background-color:#fff;color:rgba(0,0,0,0.87);font-family:Roboto,Helvetica,Arial,sans-serif;font-weight:400;font-size:1rem;line-height:1.5;letter-spacing:0.00938em;min-height:56px;'`, `{ re.lastIndex = 0; while (re.exec(s) !== null) {} }`},
	// react-spring's numbers in a value
	{"number-match", `const re = /[+\-]?(?:0|[1-9]\d*)(?:\.\d*)?(?:[eE][+\-]?\d+)?/g; const s = 'rgba(0, 0, 0, 0.5)'`, `s.match(re)`},
}

// BenchmarkRegexShapes times one run of each of regexShapes. ns/op is per
// run.
func BenchmarkRegexShapes(b *testing.B) {
	for _, c := range regexShapes {
		b.Run(c.name, func(b *testing.B) {
			m := newCallVM(b, c.setup, c.call)
			b.ResetTimer()
			if _, err := m.Call("loop", b.N); err != nil {
				b.Fatal(err)
			}
		})
	}
}
