// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package slowjs // import "github.com/liustack/slowjs"

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"strings"
	"testing"
	"unicode/utf16"
	"unicode/utf8"
)

// jsUnits returns a JS expression for the string of the UTF-16 code units u.
func jsUnits(u []uint16) string {
	parts := make([]string, len(u))
	for i, c := range u {
		parts[i] = fmt.Sprint(c)
	}
	return "String.fromCharCode(" + strings.Join(parts, ",") + ")"
}

// A JS string can hold a lone surrogate, a code unit of a surrogate pair
// without its other half: page code that cuts a string in the middle of an
// emoji makes one. A Go string from a JS string is valid UTF-8 with U+FFFD for
// each lone surrogate, as Go's utf16.Decode, Node's Buffer and TextEncoder
// make it, and pairs and other characters are as they were.
func TestLoneSurrogatesBecomeReplacementCharacters(t *testing.T) {
	m, err := NewVM()
	if err != nil {
		t.Fatal(err)
	}

	defer m.Close()

	check := func(what string, got any, u []uint16) {
		t.Helper()
		want := string(utf16.Decode(u))
		s, ok := got.(string)
		if !ok || s != want || !utf8.ValidString(s) {
			t.Errorf("%s: got %s, want %s", what, clip(got), clip(want))
		}
	}

	cases := [][]uint16{
		{0xd800},
		{0xdc00, 0xdc00},
		{'x', 0xdbff},
		{0xd800, 0xd83d, 0xde00},         // a lone high surrogate, then a pair
		{0xd83d, 0xde00, 0xde00},         // a pair, then a lone low surrogate
		{0xd55c, 0xad6d, 0xc5b4},         // Hangul, which UTF-8 starts with 0xED too
		{0xd7ff, 0xe000, 0xfffd, 0xd800}, // around the surrogates
	}
	rng := rand.New(rand.NewPCG(1, 2))
	for range 200 {
		u := make([]uint16, rng.IntN(12))
		for i := range u {
			switch rng.IntN(4) {
			case 0:
				u[i] = uint16(0xd800 + rng.IntN(0x800))
			case 1:
				u[i] = uint16(0xd000 + rng.IntN(0x800))
			case 2:
				u[i] = uint16(rng.IntN(0x80))
			default:
				u[i] = uint16(rng.IntN(0x10000))
			}
		}
		cases = append(cases, u)
	}

	var arg string
	mustHost(t, m, "probe", func(args []any) (any, error) {
		arg = args[0].(string)
		return nil, nil
	})
	for _, u := range cases {
		js := jsUnits(u)
		v, err := m.Eval(js, EvalGlobal)
		if err != nil {
			t.Fatal(err)
		}
		check("Eval("+js+")", v, u)

		// A rope is flattened before it is converted.
		long := append(append([]uint16{}, u...), utf16.Encode([]rune(strings.Repeat("é", 600)))...)
		if v, err = m.Eval(jsUnits(u)+` + "é".repeat(600)`, EvalGlobal); err != nil {
			t.Fatal(err)
		}
		check("rope of "+js, v, long)

		if _, err := m.Eval("probe("+js+")", EvalGlobal); err != nil {
			t.Fatal(err)
		}
		check("host function argument "+js, arg, u)

		_, err = m.Eval("throw new Error("+js+")", EvalGlobal)
		var e *Error
		if !errors.As(err, &e) {
			t.Fatalf("want *Error, got %T: %v", err, err)
		}
		check("error message "+js, e.Message, u)
	}
}

// The module normalizer and loader get the names of modules as valid UTF-8
// too.
func TestModuleNamesWithLoneSurrogates(t *testing.T) {
	m, err := NewVM()
	if err != nil {
		t.Fatal(err)
	}

	defer m.Close()

	var normalized, loaded []string
	m.SetModuleLoader(
		func(m *VM, name string) (string, error) {
			loaded = append(loaded, name)
			return "export default 1", nil
		},
		func(m *VM, baseName, name string) (string, error) {
			normalized = append(normalized, name)
			return name, nil
		},
	)
	if _, err := m.Eval(`import x from "m𐀀\udc00"; globalThis.x = x`, EvalModule); err != nil {
		t.Fatal(err)
	}

	want := "m\U00010000�"
	if len(normalized) != 1 || normalized[0] != want || len(loaded) != 1 || loaded[0] != want {
		t.Errorf("normalizer got %q, loader got %q, want %q", normalized, loaded, want)
	}
}

// wellFormed leaves alone what JS_ToCStringLen2 does not write: a 0xED at
// the end of the bytes, with no room for the two after it.
func TestWellFormedAtTheEnd(t *testing.T) {
	for _, b := range []string{"\xed", "a\xed", "\xed\xa0", "a\xed\xa0", "\xed\x95\x9c\xed"} {
		if got := wellFormed([]byte(b)); got != b {
			t.Errorf("wellFormed(%q) = %q", b, got)
		}
	}
	if got := wellFormed([]byte("\xed\xa0\x80\xed")); got != "�\xed" {
		t.Errorf("wellFormed = %q", got)
	}
}
