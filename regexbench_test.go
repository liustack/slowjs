// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

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
	js, err := os.ReadFile("lib/testdata/regexsplit.js")
	if err != nil {
		b.Fatal(err)
	}
	start := strings.Index(string(js), "var colors = [")
	end := strings.Index(string(js)[start:], "];")
	if start < 0 || end < 0 {
		b.Fatal("no colors in lib/testdata/regexsplit.js")
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
