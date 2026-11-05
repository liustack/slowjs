// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package quickjs // import "github.com/liustack/quickjs"

import (
	"fmt"
	"strings"
	"testing"
)

// builtinStrings are the strings BenchmarkBuiltinString passes, as
// javascript expressions evaluated once before the loop. The rope is made
// by concatenation, so it stays a rope.
var builtinStrings = []struct{ name, js string }{
	{"ascii16", `'hello, world 123'`},
	{"ascii1k", `'abcdefghijklmnop'.repeat(64)`},
	{"latin16", `'café crème brûlé'`},
	{"latin1k", `'café crème brûlé'.repeat(64)`},
	{"cjk16", `'中文字符串测试中文字符串测试一二'`},
	{"cjk1k", `'中文字符串测试中文字符串测试一二'.repeat(64)`},
	{"rope1k", `'abcdefghijklmnop'.repeat(32) + 'qrstuvwxyz012345'.repeat(32)`},
	{"html16", `'Tom & Jerry <3>!'`},
	{"html1k", `'<p class="x">Tom & Jerry</p>'.repeat(36).slice(0, 1024)`},
	{"cjkhtml16", `'中文 & 字符串 <测试> 一二'`},
}

// reactEscape is escapeTextForBrowser in React DOM 19.3 (MIT, Copyright (c)
// Meta Platforms, Inc. and affiliates) for a string, with its regexp made
// once, as React's module makes it.
const reactEscape = `(() => { var matchHtmlRegExp = /["'&<>]/; return (text) => {
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
} })()`

// escapeHTML is reactEscape for the characters s, one byte or one 16-bit
// unit each: s itself, unchanged, when it has none of " & ' < >.
func escapeHTML[T byte | uint16](s []T) (out []T, changed bool) {
	i := 0
	for i < len(s) {
		if c := s[i]; c == '"' || c == '&' || c == '\'' || c == '<' || c == '>' {
			break
		}
		i++
	}
	if i == len(s) {
		return s, false
	}
	out = make([]T, i, len(s)+len(s)/4+8)
	copy(out, s)
	for _, c := range s[i:] {
		var e string
		switch c {
		case '"':
			e = "&quot;"
		case '&':
			e = "&amp;"
		case '\'':
			e = "&#x27;"
		case '<':
			e = "&lt;"
		case '>':
			e = "&gt;"
		default:
			out = append(out, c)
			continue
		}
		for j := 0; j < len(e); j++ {
			out = append(out, T(e[j]))
		}
	}
	return out, true
}

// escapeOld is the escape of the react-hot study: WTF8, then NewString.
func escapeOld(a Args) (Value, error) {
	s, ok := a.WTF8(0)
	if !ok {
		return Value{}, nil
	}
	out, changed := escapeHTML([]byte(s))
	if !changed {
		return a.Value(0).Dup(), nil
	}
	return a.VM().NewString(string(out))
}

// escapeView reads its argument with Latin1 or UTF16 and makes the result
// with NewStringLatin1 or NewStringUTF16.
func escapeView(a Args) (Value, error) {
	if b, ok := a.Latin1(0); ok {
		out, changed := escapeHTML(b)
		if !changed {
			return a.Value(0).Dup(), nil
		}
		return a.VM().NewStringLatin1(out)
	}
	if u, ok := a.UTF16(0); ok {
		out, changed := escapeHTML(u)
		if !changed {
			return a.Value(0).Dup(), nil
		}
		return a.VM().NewStringUTF16(out)
	}
	return Value{}, nil
}

// builtinStringKinds are the functions BenchmarkBuiltinString calls with each
// string: a javascript function, a Builtin that returns its argument as it
// is, one that reads it and returns its length, and ones that read it and
// return it as a new string.
var builtinStringKinds = []struct {
	name string
	fn   func(Args) (Value, error)
}{
	{"js", nil},
	{"dup", func(a Args) (Value, error) { return a.Value(0).Dup(), nil }},
	{"in", func(a Args) (Value, error) {
		s, _ := a.String(0)
		return a.VM().NewInt(len(s)), nil
	}},
	{"inWTF8", func(a Args) (Value, error) {
		s, _ := a.WTF8(0)
		return a.VM().NewInt(len(s)), nil
	}},
	{"echo", func(a Args) (Value, error) {
		s, _ := a.String(0)
		return a.VM().NewString(s)
	}},
	{"view", func(a Args) (Value, error) {
		if b, ok := a.Latin1(0); ok {
			return a.VM().NewInt(len(b)), nil
		}
		u, _ := a.UTF16(0)
		return a.VM().NewInt(len(u)), nil
	}},
	{"viewEcho", func(a Args) (Value, error) {
		if b, ok := a.Latin1(0); ok {
			return a.VM().NewStringLatin1(b)
		}
		if u, ok := a.UTF16(0); ok {
			return a.VM().NewStringUTF16(u)
		}
		s, _ := a.String(0)
		return a.VM().NewString(s)
	}},
	{"escapeJS", nil},
	{"escapeOld", escapeOld},
	{"escapeView", escapeView},
}

// BenchmarkBuiltinString times one call of a Builtin with a string, for each
// kind of function and each string, and one with a number for comparison.
// ns/op is per call, loop included.
func BenchmarkBuiltinString(b *testing.B) {
	m, err := NewVM()
	if err != nil {
		b.Fatal(err)
	}
	defer m.Close()

	defineBuiltin(b, m, "num", NewBuiltin("num", 1, func(a Args) (Value, error) {
		x, _ := a.Number(0)
		return a.VM().NewFloat64(x + 1), nil
	}))
	b.Run("number", func(b *testing.B) {
		benchLoop(b, m, `num`, `1.5`)
	})
	for _, k := range builtinStringKinds {
		f := `(s) => s`
		if k.name == "escapeJS" {
			f = reactEscape
		}
		if k.fn != nil {
			f = "f_" + k.name
			defineBuiltin(b, m, f, NewBuiltin(f, 1, k.fn))
		}
		for _, s := range builtinStrings {
			b.Run(k.name+"/"+s.name, func(b *testing.B) {
				benchLoop(b, m, f, s.js)
			})
		}
	}
}

// benchLoop calls f(arg) b.N times in javascript.
func benchLoop(b *testing.B, m *VM, f, arg string) {
	if _, err := m.Eval(`globalThis.run = (() => { const f = `+f+`, s = `+arg+`; return (n) => { for (let i = 0; i < n; i++) f(s) } })()`, EvalGlobal); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	if _, err := m.Eval(fmt.Sprintf(`run(%d)`, b.N), EvalGlobal); err != nil {
		b.Fatal(err)
	}
}

// escapeView escapes every string it reads as React does, lone surrogates
// included, and leaves ropes to javascript.
func TestEscapeView(t *testing.T) {
	m := newTestVM(t)
	defineBuiltin(t, m, "escapeView", NewBuiltin("escapeView", 1, escapeView))
	got := evalString(t, m, `(() => {
  const reactEscape = `+reactEscape+`
  const strings = `+jsStringCases+`.concat(['Tom & Jerry <3>!', '"\'&<>', 'é&ÿ', '\ud800<', '中<\udc00>'.repeat(100)])
  let wrong = 0, ropes = 0
  for (const s of strings) {
    const go = escapeView(s)
    if (go === undefined) ropes++
    else if (go !== reactEscape(s)) wrong++
  }
  return strings.length + ' ' + wrong + ' ' + (ropes > 0)
})()`)
	if !strings.HasSuffix(got, " 0 true") {
		t.Errorf("strings, wrong escapes and whether ropes went to javascript: %s", got)
	}
}
