// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package quickjs // import "github.com/liustack/quickjs"

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"
	"unicode/utf8"

	lib "github.com/liustack/quickjs/lib"
)

// jsStringCases makes, in javascript, strings of every form the engine
// holds: empty, ASCII, 8-bit with characters up to U+00FF, 16-bit with
// characters of every UTF-8 length, surrogate pairs, lone surrogates at the
// start, the middle and the end, 16-bit strings whose characters are all
// below U+0100, ropes of each, and long strings, plus random ones.
const jsStringCases = `(() => {
  const out = ['', 'a', 'hello, world', 'a\0b', '\x7f\x80', 'é', 'café crème', '\xff'.repeat(9), 'ÿ\x00\x80',
    '中文', 'a中', '\u07ff\u0800', '\uffff', '\ufffd', '😀', 'x😀y', '\ud800', '\udc00', 'a\ud800', '\ud800a',
    '\udc00\ud800', '\ud800\ud800\udc00', '\udbff\udfff', '\ud83d' + '\ude00', 'ab\ud83d',
    '\u0100é'.slice(1), 'é\u0100'.slice(0, 1), 'abcdefghij'.repeat(1000), 'é'.repeat(70000), '中'.repeat(70000),
    '😀'.repeat(40000), 'a'.repeat(1 << 16) + '\ud800', 'é' + 'a'.repeat(7), 'é' + 'a'.repeat(8), 'é' + 'a'.repeat(9) + 'ÿ',
    'é' + 'abcdefghijklmnopq' + 'ÿ' + 'ab', 'café crème brûlé'.repeat(64), '中文字符串测试 abc'.repeat(30)]
  const ropes = []
  for (const s of out) ropes.push('r'.repeat(600) + s, s + 'é'.repeat(600), s + '中'.repeat(600) + s)
  let seed = 7
  const next = (n) => Math.floor((seed = (seed * 1103515245 + 12345) % 2147483648) / 65536) % n
  for (let k = 0; k < 2000; k++) {
    let s = ''
    const len = next(k < 1500 ? 40 : 3000)
    const kind = next(4)
    for (let i = 0; i < len; i++) {
      const r = next(10)
      s += String.fromCharCode(r < 4 ? 32 + next(95) : r < 6 || kind === 0 ? next(0x100) : r < 8 ? next(0x10000) : 0xd800 + next(0x800))
    }
    out.push(s)
  }
  return out.concat(ropes)
})()`

// A string argument of a Builtin reads as JS_ToCStringLen2 converts it, in
// both forms, whatever form the engine holds it in.
func TestArgStringSameAsEngine(t *testing.T) {
	m := newTestVM(t)
	var forms [4]int
	defineBuiltin(t, m, "compare", NewBuiltin("compare", 1, func(a Args) (Value, error) {
		v := a.arg(0)
		if _, n, wide, flat := lib.StringChars(v); !flat {
			forms[0]++
		} else if n > 0 && !wide {
			forms[1]++
		} else if wide {
			forms[2]++
		}
		tls, ctx := a.vm.runtime.tls, a.vm.cContext
		for _, wtf8 := range []bool{false, true} {
			want, wok := toCString(tls, ctx, v, wtf8)
			var got string
			var gok bool
			if wtf8 {
				got, gok = a.WTF8(0)
			} else {
				got, gok = a.String(0)
			}
			if got != want || gok != wok {
				return a.VM().NewString(fmt.Sprintf("wtf8=%v: got %q %v, want %q %v", wtf8, trim(got), gok, trim(want), wok))
			}
		}
		forms[3]++
		return Value{}, nil
	}))
	got := evalString(t, m, `(() => { const r = []; for (const s of `+jsStringCases+`) { const e = compare(s); if (e !== undefined) r.push(e) } return r.slice(0, 5).join('\n') })()`)
	if got != "" {
		t.Error(got)
	}
	if forms[0] == 0 || forms[1] == 0 || forms[2] == 0 {
		t.Errorf("ropes, 8-bit and 16-bit strings: %v, want some of each", forms[:3])
	}
	t.Logf("%d strings: %d ropes, %d 8-bit, %d 16-bit", forms[3], forms[0], forms[1], forms[2])
}

func trim(s string) string {
	if len(s) > 60 {
		return s[:60] + "..."
	}
	return s
}

// goStringCases are Go strings of every kind newString takes: ASCII, UTF-8
// with characters of every length, invalid UTF-8 of every kind (a byte that
// starts no sequence, a sequence cut short, an overlong form, the three bytes
// of a surrogate, past U+10FFFF), long strings and random bytes.
func goStringCases() []string {
	out := []string{"", "a", "hello, world", "a\x00b", "\x7f", "é", "café", "ÿ", "\u0100", "中文", "😀", "a😀b",
		"\uffff", "\ufffd", "\x80", "\x80a", "a\xff", "\xc0\x80", "\xe4\xb8", "\xe4\xb8a", "\xed\xa0\x80",
		"\xed\xa0\x80\xed\xb0\x80", "\xf4\x90\x80\x80", "\xf0\x9f\x98", "é\x80", "\xc3", "\xc3\xa9\xc3",
		strings.Repeat("abcdefgh", 2000), strings.Repeat("é", 70000), strings.Repeat("中", 70000),
		strings.Repeat("😀", 40000), strings.Repeat("a", 1<<16) + "\xff", "é" + strings.Repeat("a", 17) + "ÿ",
		strings.Repeat("café crème brûlé", 64), strings.Repeat("中文字符串测试 abc", 30)}
	r := rand.New(rand.NewSource(3))
	for k := 0; k < 3000; k++ {
		n := r.Intn(40)
		if k >= 2500 {
			n = r.Intn(3000)
		}
		var b []byte
		for range n {
			switch r.Intn(6) {
			case 0, 1:
				b = append(b, byte(32+r.Intn(95)))
			case 2:
				b = append(b, byte(r.Intn(256)))
			case 3:
				b = utf8.AppendRune(b, rune(r.Intn(0x100)))
			case 4:
				b = utf8.AppendRune(b, rune(r.Intn(0x10000)))
			default:
				b = utf8.AppendRune(b, rune(0x10000+r.Intn(0x100000)))
			}
		}
		out = append(out, string(b))
	}
	return out
}

// NewString makes the string JS_NewStringLen made of validUTF8(s): the same
// characters, held as 8-bit characters or 16-bit units as the engine held
// them.
func TestNewStringSameAsEngine(t *testing.T) {
	m := newTestVM(t)
	eq, err := m.EvalValue(`(a, b) => a === b`, EvalGlobal)
	if err != nil {
		t.Fatal(err)
	}
	defer eq.Free()
	bad := 0
	for _, s := range goStringCases() {
		got, err := m.NewString(s)
		if err != nil {
			t.Fatal(err)
		}
		v, err := m.newStringEngine(s)
		if err != nil {
			t.Fatal(err)
		}
		want := Value{vm: m, v: v}
		same, err := eq.Call(UndefinedValue, got, want)
		if err != nil {
			t.Fatal(err)
		}
		_, gn, gwide, gflat := lib.StringChars(got.v)
		_, wn, wwide, wflat := lib.StringChars(want.v)
		if same != true || gn != wn || gflat != wflat || gwide != wwide {
			bad++
			if bad <= 5 {
				t.Errorf("%q: same %v, got %d wide=%v flat=%v, want %d wide=%v flat=%v", trim(s), same, gn, gwide, gflat, wn, wwide, wflat)
			}
		}
		got.Free()
		want.Free()
	}
}

// A Builtin returning the strings it is given, and new strings made of
// them, leaks nothing, ropes and long strings included (run under memgrind
// by TestMemgrind2).
func TestBuiltinStringRoundTrip(t *testing.T) {
	m := newTestVM(t)
	defineBuiltin(t, m, "twice", NewBuiltin("twice", 1, func(a Args) (Value, error) {
		s, ok := a.String(0)
		w, wok := a.WTF8(0)
		if !ok || !wok {
			return Value{}, fmt.Errorf("not a string")
		}
		return a.VM().NewString(s + w)
	}))
	got := evalString(t, m, `(() => {
  let bad = 0, n = 0
  for (const s of `+jsStringCases+`) {
    n++
    // A lone surrogate in the WTF-8 half is three bytes NewString makes three U+FFFD of.
    const want = s.toWellFormed() + s.replace(/[\ud800-\udbff](?![\udc00-\udfff])|(?<![\ud800-\udbff])[\udc00-\udfff]/g, '\ufffd\ufffd\ufffd')
    if (twice(s) !== want) bad++
  }
  return n + ' ' + bad
})()`)
	if !strings.HasSuffix(got, " 0") {
		t.Errorf("strings and wrong round trips: %s", got)
	}
}

// utf8Units finds the strings Go finds valid UTF-8, every one of up to three
// bytes and random longer ones, and counts their UTF-16 units.
func TestUTF8Units(t *testing.T) {
	check := func(b []byte) {
		units, latin1, valid := utf8Units(b)
		if valid != utf8.Valid(b) {
			t.Fatalf("% x: valid %v", b, valid)
		}
		if !valid {
			return
		}
		r := []rune(string(b))
		n, max := len(r), rune(0)
		for _, c := range r {
			if c > 0xFFFF {
				n++
			}
			max = maxRune(max, c)
		}
		if units != n || latin1 != (max < 0x100) {
			t.Fatalf("% x: %d units, latin1 %v", b, units, latin1)
		}
	}
	for x := range 1 << 24 {
		b := []byte{byte(x >> 16), byte(x >> 8), byte(x)}
		check(b)
		if x&0xFFFF == 0 {
			check(b[:1])
		}
		if x&0xFF == 0 {
			check(b[:2])
		}
	}
	r := rand.New(rand.NewSource(5))
	b := make([]byte, 6)
	for range 1 << 20 {
		for i := range b {
			b[i] = byte(0x80 + r.Intn(0x80))
		}
		check(b[:4+r.Intn(3)])
	}
}
