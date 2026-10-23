// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package slowjs // import "github.com/liustack/slowjs"

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"unicode/utf16"
)

// invalidUTF8 holds Go strings that are not valid UTF-8, with valid strings
// around them that must come through unchanged.
var invalidUTF8 = []string{
	"\xc0\x80",                 // overlong NUL, as in modified UTF-8
	"\x80a",                    // a lone continuation byte before ASCII
	"a\x80\x80b",               // two of them
	"\xe4\xb8",                 // a sequence cut short at the end
	"\xe4\xb8a",                // and before ASCII
	"\xf0\x9f\x98",             // a cut emoji
	"a\xed\xa0\x80b",           // a surrogate written as UTF-8 (CESU-8)
	"\xed\xa0\xbd\xed\xb8\x80", // a surrogate pair written so
	"\xff",                     // a byte UTF-8 never has
	"\xf4\x90\x80\x80",         // past U+10FFFF
	"\xe0\x80\x80",             // overlong
	"\xf8\x88\x80\x80\x80",     // a five-byte form
	"x\xc2",                    // a lead byte at the end
	"\xc2\xc2\xa9",             // a lead byte before a valid sequence
	"中\xe4文",                   // between characters
	"�",                        // a real U+FFFD
	"😀 中文 é",                   // valid
}

// TestGoInvalidUTF8 passes Go strings that are not valid UTF-8 into
// javascript on every path a Go string becomes a JS string. Each byte that
// does not start a valid sequence becomes one U+FFFD, as Go itself decodes
// it (for range, []rune, utf16.Encode and encoding/json). The engine used to
// skip the bytes that followed an invalid one: "\x80a" lost the "a", and
// "\xc0\x80" made a single U+FFFD. It took the three-byte form of a surrogate
// as that surrogate, so two of them made an emoji.
func TestGoInvalidUTF8(t *testing.T) {
	m, err := NewVM()
	if err != nil {
		t.Fatal(err)
	}

	defer m.Close()

	var cur string
	for name, f := range map[string]any{
		"str":    func() string { return cur },
		"key":    func() map[string]int { return map[string]int{cur: 1} },
		"val":    func() map[string]any { return map[string]any{"k": cur} },
		"field":  func() struct{ K string } { return struct{ K string }{cur} },
		"errStr": func() error { return errors.New(cur) },
	} {
		if err := m.RegisterFunc(name, f, false); err != nil {
			t.Fatal(err)
		}
	}
	if err := m.RegisterHostFunc("fail", func([]any) (any, error) { return nil, errors.New(cur) }); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Eval(`globalThis.units = s => Array.from({length: s.length}, (_, i) => s.charCodeAt(i)).join(' ')`, EvalGlobal); err != nil {
		t.Fatal(err)
	}

	for i, s := range invalidUTF8 {
		cur = s
		var want []string
		for _, c := range utf16.Encode([]rune(s)) {
			want = append(want, fmt.Sprint(c))
		}
		check := func(path string, got any, err error) {
			t.Helper()
			if err != nil {
				t.Errorf("%q, %s: %v", s, path, err)
				return
			}
			if got != strings.Join(want, " ") {
				t.Errorf("%q, %s: got code units %v, want %v", s, path, got, strings.Join(want, " "))
			}
		}

		got, err := m.Call("units", s)
		check("argument", got, err)
		got, err = m.Eval(`units(str())`, EvalGlobal)
		check("result", got, err)
		got, err = m.Eval(`units(Object.keys(key())[0])`, EvalGlobal)
		check("map key", got, err)
		got, err = m.Eval(`units(val().k)`, EvalGlobal)
		check("map value", got, err)
		got, err = m.Eval(`units(field().K)`, EvalGlobal)
		check("struct field", got, err)
		got, err = m.Eval(`units(errStr())`, EvalGlobal)
		check("error result", got, err)
		got, err = m.Eval(`try { fail() } catch (e) { units(e.message) }`, EvalGlobal)
		check("thrown error", got, err)

		v, err := m.NewString(s)
		if err != nil {
			t.Fatal(err)
		}
		got, err = m.Call("units", v)
		v.Free()
		check("NewString", got, err)

		// An atom is a property name.
		a, err := m.NewAtom(s)
		if err != nil {
			t.Fatal(err)
		}
		g := m.GlobalObject()
		err = m.SetProperty(g, a, 1)
		g.Free()
		if err != nil {
			t.Fatal(err)
		}
		got, err = m.Eval(`units(Object.keys(globalThis).find(k => globalThis[k] === 1))`, EvalGlobal)
		check("NewAtom", got, err)
		if _, err := m.Eval(`for (const k of Object.keys(globalThis)) if (globalThis[k] === 1) delete globalThis[k]`, EvalGlobal); err != nil {
			t.Fatal(err)
		}

		// So is the name of a Go function. A VM takes a name once, and some
		// of the strings are the same name in javascript ("\xff" and "\uFFFD"
		// are both U+FFFD), so each gets a prefix of its own.
		if _, err := m.Eval(`Object.defineProperty(globalThis, 'before', {value: new Set(Object.getOwnPropertyNames(globalThis)), configurable: true, writable: true})`, EvalGlobal); err != nil {
			t.Fatal(err)
		}
		prefix := fmt.Sprintf("f%d_", i)
		if err := m.RegisterFunc(prefix+s, func() int { return 2 }, false); err != nil {
			t.Fatal(err)
		}
		got, err = m.Eval(fmt.Sprintf(`units(Object.getOwnPropertyNames(globalThis).find(k => !before.has(k) && k !== 'before').slice(%d))`, len(prefix)), EvalGlobal)
		check("function name", got, err)
		if _, err := m.Eval(`for (const k of Object.getOwnPropertyNames(globalThis)) if (!before.has(k) && k !== 'before') delete globalThis[k]`, EvalGlobal); err != nil {
			t.Fatal(err)
		}
	}
}

// TestGoInvalidUTF8ModuleName gives the engine a module name that is not
// valid UTF-8 from the normalizer. The engine keeps the name in the form Go
// decodes it, and hands the loader that form.
func TestGoInvalidUTF8ModuleName(t *testing.T) {
	m, err := NewVM()
	if err != nil {
		t.Fatal(err)
	}

	defer m.Close()

	var loaded string
	m.SetModuleLoader(
		func(_ *VM, name string) (string, error) {
			loaded = name
			return `export default 1;`, nil
		},
		func(_ *VM, _, name string) (string, error) {
			return name + "\x80a", nil
		},
	)
	if _, err := m.Eval(`import x from "mod";`, EvalModule); err != nil {
		t.Fatal(err)
	}

	if want := "mod�a"; loaded != want {
		t.Fatalf("the loader got %q, want %q", loaded, want)
	}
}
