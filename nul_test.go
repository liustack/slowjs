// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package quickjs // import "modernc.org/quickjs"

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// clip quotes v for a failure message, abbreviating a long string.
func clip(v any) string {
	s, ok := v.(string)
	if !ok || len(s) <= 40 {
		return fmt.Sprintf("%q", v)
	}

	return fmt.Sprintf("%q... (%d bytes)", s[:20], len(s))
}

// TestStringNUL guards the JS -> Go string conversion against treating U+0000
// as a terminator. A JavaScript string is a sequence of UTF-16 code units and a
// zero code unit is an ordinary one - a binary string built with
// String.fromCharCode, btoa's argument, pixel data - so it must reach Go whole
// wherever a JS value becomes a Go string: host function arguments, Eval
// results, property reads, error messages. The Go -> JS direction has always
// been length-based; the round trips below confirm both halves agree.
//
// It also runs under the memgrind allocator (TestMemgrind2), so it must create
// and Close its own VM.
func TestStringNUL(t *testing.T) {
	m, err := NewVM()
	if err != nil {
		t.Fatal(err)
	}

	defer m.Close()

	// Three shapes of JS string, because JS_ToCStringLen2 has three code paths:
	// 8-bit (Latin-1) strings are handed out in place, 16-bit strings are
	// transcoded to UTF-8 into a fresh buffer, and ropes are flattened first.
	const js8 = `"a" + String.fromCharCode(0) + "b"`
	const js16 = `"é" + String.fromCharCode(0) + "日本"`
	// (A right operand longer than JS_STRING_ROPE_SHORT_LEN, 512, makes the
	// concatenation a rope.)
	const jsRope = `"x".repeat(600) + String.fromCharCode(0) + "y".repeat(600)`
	want8 := "a\x00b"
	want16 := "é\x00日本"
	wantRope := strings.Repeat("x", 600) + "\x00" + strings.Repeat("y", 600)

	// Eval result: the string case of the value conversion.
	for _, test := range []struct {
		js   string
		want string
	}{
		{js8, want8},
		{js16, want16},
		{jsRope, wantRope},
	} {
		v, err := m.Eval(test.js, EvalGlobal)
		if err != nil {
			t.Fatalf("%s: %v", test.js, err)
		}

		if g, e := v, test.want; g != e {
			t.Errorf("Eval(%s) = %s (%T), want %s", test.js, clip(g), g, clip(e))
		}
	}

	// Host function argument (RegisterHostFunc): the case from the report, a
	// 3-byte Go string. The value is echoed back doubled so the Go -> JS
	// direction is checked in the same breath.
	var gotHost string
	mustHost(t, m, "probe", func(args []any) (any, error) {
		s, ok := args[0].(string)
		if !ok {
			return nil, fmt.Errorf("want a string argument, got %T", args[0])
		}

		gotHost = s
		return s + s, nil
	})
	v, err := m.Eval(`const back = probe(`+js8+`); [back.length, back.charCodeAt(1), back.charCodeAt(4), back === "a\u0000ba\u0000b"]`, EvalGlobal)
	if err != nil {
		t.Fatal(err)
	}

	if g, e := gotHost, want8; g != e {
		t.Errorf("host function argument = %q (len %d), want %q (len %d)", g, len(g), e, len(e))
	}
	if g, e := fmt.Sprint(v), `[6,0,0,true]`; g != e {
		t.Errorf("host function result seen from JS = %s, want %s", g, e)
	}

	// Typed Go function (RegisterFunc): the same conversion, reflect-assigned.
	var gotTyped string
	if err := m.RegisterFunc("typed", func(s string) int {
		gotTyped = s
		return len(s)
	}, false); err != nil {
		t.Fatal(err)
	}
	v, err = m.Eval(`typed(`+js16+`)`, EvalGlobal)
	if err != nil {
		t.Fatal(err)
	}

	if g, e := gotTyped, want16; g != e {
		t.Errorf("typed function argument = %q, want %q", g, e)
	}
	if g, e := v, len(want16); g != e {
		t.Errorf("typed function saw len %v, want %v", g, e)
	}

	// Property read and Value.Any.
	obj, err := m.EvalValue(`({k: `+js8+`})`, EvalGlobal)
	if err != nil {
		t.Fatal(err)
	}

	defer obj.Free()

	k, err := m.NewAtom("k")
	if err != nil {
		t.Fatal(err)
	}

	p, err := obj.Property(k)
	if err != nil {
		t.Fatal(err)
	}

	if g, e := p, want8; g != e {
		t.Errorf("Property = %q, want %q", g, e)
	}

	pv, err := obj.GetPropertyValue(k)
	if err != nil {
		t.Fatal(err)
	}

	a, err := pv.Any()
	pv.Free()
	if err != nil {
		t.Fatal(err)
	}

	if g, e := a, want8; g != e {
		t.Errorf("Value.Any = %q, want %q", g, e)
	}

	// Objects cross as JSON, where U+0000 is written as \u0000 and so was never
	// at risk; check the JSON paths still carry it and that the bytes a
	// MarshalJSON returns are a copy and not a view of a freed JS buffer.
	o, err := m.Eval(`({k: `+js8+`})`, EvalGlobal)
	if err != nil {
		t.Fatal(err)
	}

	if g, e := fmt.Sprint(o), `{"k":"a\u0000b"}`; g != e {
		t.Errorf("Object = %s, want %s", g, e)
	}

	var decoded struct{ K string }
	if err := o.(*Object).Into(&decoded); err != nil {
		t.Fatal(err)
	}

	if g, e := decoded.K, want8; g != e {
		t.Errorf("Object.Into = %q, want %q", g, e)
	}

	b, err := json.Marshal(obj)
	if err != nil {
		t.Fatal(err)
	}

	// Churn the JS heap before looking at the bytes.
	if _, err := m.Eval(`for (let i = 0; i < 1000; i++) { globalThis["g" + i] = "z".repeat(100) + i }`, EvalGlobal); err != nil {
		t.Fatal(err)
	}

	if g, e := string(b), `{"k":"a\u0000b"}`; g != e {
		t.Errorf("Value.MarshalJSON = %s, want %s", g, e)
	}

	// ErrorFromValue: an Error whose message contains a NUL keeps the message
	// whole, in Message and in Error().
	_, err = m.Eval(`throw new Error("x" + String.fromCharCode(0) + "y")`, EvalGlobal)
	var e *Error
	if !errors.As(err, &e) {
		t.Fatalf("want *Error, got %T: %v", err, err)
	}

	if g, w := e.Name, "Error"; g != w {
		t.Errorf("Name = %q, want %q", g, w)
	}
	if g, w := e.Message, "x\x00y"; g != w {
		t.Errorf("Message = %q, want %q", g, w)
	}
	if g, w := e.Error(), "Error: x\x00y"; g != w {
		t.Errorf("Error() = %q, want %q", g, w)
	}

	// A thrown string, the other thing ErrorFromValue sees in the wild.
	_, err = m.Eval(`throw "p" + String.fromCharCode(0) + "q"`, EvalGlobal)
	if !errors.As(err, &e) {
		t.Fatalf("want *Error, got %T: %v", err, err)
	}

	if g, w := e.Error(), "p\x00q"; g != w {
		t.Errorf("thrown string Error() = %q, want %q", g, w)
	}

	// The only other JS -> Go string conversion, a BigInt's hex digits, has no
	// NUL to lose; it shares the length-based path and must keep working.
	v, err = m.Eval(`(1n << 70n)`, EvalGlobal)
	if err != nil {
		t.Fatal(err)
	}

	if g, e := fmt.Sprint(v), "1180591620717411303424"; g != e {
		t.Errorf("BigInt = %s, want %s", g, e)
	}
}
