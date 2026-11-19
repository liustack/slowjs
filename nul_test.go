// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

package slowjs // import "github.com/liustack/slowjs"

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// clip quotes v for a test message: whole when it is short or not a string,
// and its first 20 bytes and its length when it is a long string.
func clip(v any) string {
	s, ok := v.(string)
	if !ok || len(s) <= 40 {
		return fmt.Sprintf("%q", v)
	}
	return fmt.Sprintf("%q... (%d bytes)", s[:20], len(s))
}

// A string of javascript keeps its U+0000 on the way to Go, wherever it goes:
// a result, an argument of a Go function, a property, the JSON of an object,
// an error. Strings of one byte and two byte characters, and a rope.
func TestStringNUL(t *testing.T) {
	m, err := NewVM()
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()

	rope := strings.Repeat("x", 600) + "\x00" + strings.Repeat("y", 600)
	for _, c := range []struct{ src, want string }{
		{`"a" + String.fromCharCode(0) + "b"`, "a\x00b"},
		{`"é" + String.fromCharCode(0) + "日本"`, "é\x00日本"},
		{`"x".repeat(600) + String.fromCharCode(0) + "y".repeat(600)`, rope},
	} {
		got, err := m.Eval(c.src, EvalGlobal)
		if err != nil || got != c.want {
			t.Errorf("%s: got %s, %v, want %s", c.src, clip(got), err, clip(c.want))
		}
	}

	var gotHost any
	if err := m.RegisterHostFunc("echo", func(args []any) (any, error) {
		gotHost = args[0]
		s := args[0].(string)
		return s + s, nil
	}); err != nil {
		t.Fatal(err)
	}
	r, err := m.Eval(`const back = echo("a\u0000b"); [back.length, back.charCodeAt(1), back.charCodeAt(4), back === "a\u0000ba\u0000b"]`, EvalGlobal)
	if err != nil {
		t.Fatal(err)
	}
	if gotHost != "a\x00b" {
		t.Errorf("the host function got %s", clip(gotHost))
	}
	if o, ok := r.(*Object); !ok || o.json != `[6,0,0,true]` {
		t.Errorf("back in javascript: %v", r)
	}

	var gotArg string
	if err := m.RegisterFunc("length", func(s string) int { gotArg = s; return len(s) }, false); err != nil {
		t.Fatal(err)
	}
	if n, err := m.Eval(`length("é" + String.fromCharCode(0) + "日本")`, EvalGlobal); err != nil || n != len("é\x00日本") {
		t.Errorf("length: got %v, %v, want %d", n, err, len("é\x00日本"))
	}
	if gotArg != "é\x00日本" {
		t.Errorf("the string parameter got %s", clip(gotArg))
	}

	obj, err := m.EvalValue(`({k: "a" + String.fromCharCode(0) + "b"})`, EvalGlobal)
	if err != nil {
		t.Fatal(err)
	}
	defer obj.Free()
	k, err := m.NewAtom("k")
	if err != nil {
		t.Fatal(err)
	}
	if got, err := obj.Property(k); err != nil || got != "a\x00b" {
		t.Errorf("Property: got %s, %v", clip(got), err)
	}
	pv, err := m.GetPropertyValue(obj, k)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := pv.Any(); err != nil || got != "a\x00b" {
		t.Errorf("GetPropertyValue: got %s, %v", clip(got), err)
	}
	pv.Free()

	o, err := m.Eval(`({k: "a" + String.fromCharCode(0) + "b"})`, EvalGlobal)
	if err != nil {
		t.Fatal(err)
	}
	if s := fmt.Sprint(o); s != `{"k":"a\u0000b"}` {
		t.Errorf("*Object: %s", s)
	}
	var into struct{ K string }
	if err := o.(*Object).Into(&into); err != nil || into.K != "a\x00b" {
		t.Errorf("Into: %s, %v", clip(into.K), err)
	}

	b, err := json.Marshal(obj)
	if err != nil || string(b) != `{"k":"a\u0000b"}` {
		t.Fatalf("json.Marshal: %s, %v", b, err)
	}
	// The bytes are Go's own: what the engine does next leaves them alone.
	kept := string(b)
	if _, err := m.Eval(`for (let i = 0; i < 1000; i++) "z".repeat(100 + i) + i`, EvalGlobal); err != nil {
		t.Fatal(err)
	}
	if string(b) != kept {
		t.Errorf("the JSON changed to %s", b)
	}

	_, err = m.Eval(`throw new Error("x" + String.fromCharCode(0) + "y")`, EvalGlobal)
	var e *Error
	if !errors.As(err, &e) || e.Name != "Error" || e.Message != "x\x00y" || err.Error() != "Error: x\x00y" {
		t.Errorf("a thrown Error: %#v", err)
	}
	_, err = m.Eval(`throw "p" + String.fromCharCode(0) + "q"`, EvalGlobal)
	if err == nil || err.Error() != "p\x00q" {
		t.Errorf("a thrown string: %v", err)
	}

	if n, err := m.Eval(`1n << 70n`, EvalGlobal); err != nil || fmt.Sprint(n) != "1180591620717411303424" {
		t.Errorf("1n << 70n: %v, %v", n, err)
	}
}
