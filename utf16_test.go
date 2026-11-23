// Copyright (c) 2026 The liustack/beastjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

package beastjs

import (
	"strings"
	"testing"
	"unicode/utf16"
)

// utf16Cases returns units with every boundary the conversion has: the
// edges of each length of UTF-8 (U+007F, U+0080, U+07FF, U+0800, U+FFFF,
// U+10000, U+10FFFF), lone high and low surrogates at the start, the middle
// and the end, and a pair or a lone surrogate at each place of a run of
// ASCII, so that it falls on every unit of a block of 8 or 4 and on both
// sides of where one ends.
func utf16Cases() [][]uint16 {
	enc := func(s string) []uint16 { return utf16.Encode([]rune(s)) }
	cases := [][]uint16{
		nil,
		{0},
		{0x7F}, {0x80}, {0xFF}, {0x100}, {0x7FF}, {0x800}, {0xD7FF}, {0xE000}, {0xFFFD}, {0xFFFE}, {0xFFFF},
		{0xD800, 0xDC00}, {0xDBFF, 0xDFFF}, {0xD83D, 0xDE00},
		{0xD800}, {0xDBFF}, {0xDC00}, {0xDFFF},
		{0xD800, 0xD800}, {0xDC00, 0xD800}, {0xDC00, 0xDC00}, {0xDBFF, 0xDBFF, 0xDFFF},
		{0xD800, 'a'}, {'a', 0xDC00}, {0xDFFF, 0xDBFF},
		enc("ascii only"), enc("Latin-1: café, naïve, ½ ÿ"), enc("Ελληνικά и кириллица"), enc("中文和日本語のテキスト"),
		enc("emoji 😀🎉 and 𝄞 and \U0010FFFF"),
	}
	units := [][]uint16{
		{0xD83D, 0xDE00}, // a pair
		{0xD800},         // a lone high surrogate
		{0xDC00},         // a lone low surrogate
		{0xE9},           // two bytes
		{0x4E2D},         // three bytes
		{0xFFFF},
	}
	for _, x := range units {
		for before := 0; before <= 20; before++ {
			for _, after := range []int{0, 1, 7, 8, 9, 17} {
				u := append(enc(strings.Repeat("a", before)), x...)
				cases = append(cases, append(u, enc(strings.Repeat("b", after))...))
			}
		}
	}
	// Runs of each kind, short and long, and pieces of each kind in turn.
	for _, s := range []string{"x", "é", "中", "😀"} {
		for _, n := range []int{3, 4, 5, 7, 8, 9, 15, 16, 17, 63, 64, 65, 1000} {
			cases = append(cases, enc(strings.Repeat(s, n)))
		}
	}
	for _, n := range []int{1, 3, 8, 9, 30} {
		var b strings.Builder
		for range 40 {
			b.WriteString(strings.Repeat("<div>", n) + "订单" + strings.Repeat("y", n) + "é😀")
		}
		cases = append(cases, enc(b.String()))
	}
	// A lone surrogate at the start, the middle and the end of long ASCII.
	long := enc(strings.Repeat("<p>text</p>", 100))
	for _, s := range []uint16{0xD800, 0xDC00} {
		for _, at := range []int{0, len(long) / 2, len(long)} {
			u := append(append(append([]uint16(nil), long[:at]...), s), long[at:]...)
			cases = append(cases, u)
		}
	}
	return cases
}

// The conversion writes what v0.30.6 wrote (utf16StringRef), byte for byte,
// for every case, in one go and in pieces of every size up to 9 units; and
// what Go's utf16.Decode gives, a lone surrogate as U+FFFD, without wtf8.
func TestUTF16Boundaries(t *testing.T) {
	for _, u := range utf16Cases() {
		checkUTF16(t, u)
	}
}

func checkUTF16(t *testing.T, u []uint16) {
	t.Helper()
	for _, wtf8 := range []bool{false, true} {
		want := utf16StringRef(u, wtf8)
		if !wtf8 {
			if d := string(utf16.Decode(u)); d != want {
				t.Fatalf("units %x: the reference gives %q, utf16.Decode %q", u, want, d)
			}
		}
		if got, ok := utf16String(nil, u, wtf8); got != want || !ok {
			t.Fatalf("units %x, wtf8 %v: got %q %v, want %q", u, wtf8, got, ok, want)
		}
		if n := utf16Len(u); n != len(want) {
			t.Fatalf("units %x: utf16Len %d, want %d", u, n, len(want))
		}
		for step := 1; step <= 9; step++ {
			stringStep = step
			got, ok := utf16Steps(nil, u, wtf8, goOut)
			stringStep = 1 << 18
			if got != want || !ok {
				t.Fatalf("units %x, wtf8 %v, pieces of %d: got %q %v, want %q", u, wtf8, step, got, ok, want)
			}
		}
	}
}

// FuzzUTF16String compares the conversion with the one of v0.30.6 on units
// the fuzzer makes: two bytes a unit as they are when raw, or else two bytes
// that pick a kind of unit (mostly ASCII, as in a page) and its value.
func FuzzUTF16String(f *testing.F) {
	f.Add([]byte("<div>hello</div>"), false)
	f.Add([]byte{0x00, 'a', 0x80, 0x01, 0xA0, 0x2D, 0xC0, 0x00, 0xE0, 0x00, 0x00, 'z'}, false)
	f.Add([]byte{0x3D, 0xD8, 0x00, 0xDE, 'a', 0, 0x00, 0xD8}, true)
	f.Fuzz(func(t *testing.T, b []byte, raw bool) {
		u := make([]uint16, 0, len(b)/2)
		for i := 0; i+1 < len(b); i += 2 {
			if raw {
				u = append(u, uint16(b[i])|uint16(b[i+1])<<8)
			} else {
				u = append(u, fuzzUnit(b[i], b[i+1]))
			}
		}
		checkUTF16(t, u)
	})
}

// fuzzUnit makes a unit of the kind the top 3 bits of k pick: ASCII (half
// of them), two bytes, three bytes (an edge when the low bits of k are all
// set), a high or a low surrogate.
func fuzzUnit(k, v byte) uint16 {
	low := uint16(k&0x1F)<<8 | uint16(v)
	switch k >> 5 {
	case 4:
		return 0x80 + low%0x780
	case 5:
		if k&0x1F == 0x1F {
			return [...]uint16{0x800, 0xD7FF, 0xE000, 0xFFFD, 0xFFFF}[v%5]
		}
		return 0x4E00 + low
	case 6:
		return 0xD800 + low&0x3FF
	case 7:
		return 0xDC00 + low&0x3FF
	}
	return uint16(v & 0x7F)
}
