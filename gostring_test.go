// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package slowjs // import "github.com/liustack/slowjs"

import (
	"errors"
	"strconv"
	"testing"

	lib "github.com/liustack/slowjs/lib"
)

// stringOOMSize is the length of the string the tests below convert. Its
// characters are not ASCII, so the UTF-8 copy JS_ToCStringLen2 makes takes two
// bytes per character for "é" and three for "中".
const stringOOMSize = 1 << 20

// newStringOOMVM returns a VM holding globalThis.s, a string of
// stringOOMSize copies of ch, and globalThis.o = {s}. Its memory limit then
// leaves room for 'room' bytes more.
func newStringOOMVM(t *testing.T, ch string, room uintptr) *VM {
	t.Helper()
	m, err := NewVM()
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { m.Close() })

	if _, err := m.Eval(`globalThis.s = "`+ch+`".repeat(`+strconv.Itoa(stringOOMSize)+`); globalThis.o = {s}; undefined`, EvalGlobal); err != nil {
		t.Fatal(err)
	}

	m.SetMemoryLimit(uintptr(vmMemory(m).Fmalloc_size) + room)
	return m
}

// wantOutOfMemory fails the test unless err is the engine's out of memory
// error.
func wantOutOfMemory(t *testing.T, what string, err error) {
	t.Helper()
	var e *Error
	if !errors.As(err, &e) || e.Name != "InternalError" || e.Message != "out of memory" {
		t.Fatalf("%s: got error %v, want InternalError: out of memory", what, err)
	}
}

// wantNoPendingException fails the test if an exception is still pending in
// m's context. A failed call must take its exception out of the context: one
// left there is what the next caller of JS_GetException finds, and it keeps
// the thrown value alive.
func wantNoPendingException(t *testing.T, m *VM) {
	t.Helper()
	if lib.XJS_HasException(m.runtime.tls, m.cContext) != 0 {
		t.Fatal("an exception is still pending in the context")
	}
}

// wantUsable lifts m's memory limit and checks that the VM converts s now.
func wantUsable(t *testing.T, m *VM) {
	t.Helper()
	m.SetMemoryLimit(^uintptr(0))
	v, err := m.Eval("s", EvalGlobal)
	if err != nil {
		t.Fatalf("reading s with no memory limit: %v", err)
	}

	if s, ok := v.(string); !ok || len([]rune(s)) != stringOOMSize {
		t.Fatalf("reading s with no memory limit: got a %T of %d bytes", v, len(s))
	}
}

// A string returned to Go that has no room for its UTF-8 copy must come back
// as the out of memory error, not as "".
func TestGoStringOutOfMemory(t *testing.T) {
	for _, ch := range []string{"é", "中"} {
		t.Run(ch, func(t *testing.T) {
			// The UTF-8 copy needs stringOOMSize*2 or *3 bytes.
			m := newStringOOMVM(t, ch, stringOOMSize/2)
			v, err := m.Eval("s", EvalGlobal)
			if err == nil {
				t.Fatalf("Eval returned a %T of %d bytes, want an error", v, len(v.(string)))
			}

			wantOutOfMemory(t, "Eval", err)
			wantNoPendingException(t, m)
			wantUsable(t, m)
		})
	}
}

// The JSON of an object returned to Go that has no room for its UTF-8 copy
// must come back as the out of memory error, not as an empty Object, and so
// must Value.MarshalJSON.
func TestObjectJSONOutOfMemory(t *testing.T) {
	for _, c := range []struct {
		name string
		get  func(m *VM) (string, error)
	}{
		{"Eval", func(m *VM) (string, error) {
			v, err := m.Eval("o", EvalGlobal)
			if err != nil {
				return "", err
			}

			return v.(*Object).String(), nil
		}},
		{"MarshalJSON", func(m *VM) (string, error) {
			v, err := m.EvalValue("o", EvalGlobal)
			if err != nil {
				return "", err
			}

			defer v.Free()

			b, err := v.MarshalJSON()
			return string(b), err
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			// Room for JSON.stringify(o), about stringOOMSize bytes and up to
			// half as much again while its buffer grows, but not for the
			// stringOOMSize*2 bytes of its UTF-8 copy next to it.
			m := newStringOOMVM(t, "é", stringOOMSize*2)
			// JSON.stringify itself fits, so what fails below is the copy.
			if v, err := m.Eval("JSON.stringify(o).length", EvalGlobal); err != nil || v != stringOOMSize+8 {
				t.Fatalf("JSON.stringify(o).length: %v, %v", v, err)
			}

			json, err := c.get(m)
			if err == nil {
				t.Fatalf("got %d bytes of JSON, want an error", len(json))
			}

			wantOutOfMemory(t, c.name, err)
			wantNoPendingException(t, m)
			wantUsable(t, m)
		})
	}
}

// A host function whose string argument has no room for its UTF-8 copy must
// not be called with "". Running out of memory there ends the call: page code
// cannot catch it.
func TestHostFuncArgumentOutOfMemory(t *testing.T) {
	// The UTF-8 copy needs stringOOMSize*2 bytes.
	m := newStringOOMVM(t, "é", stringOOMSize/2)
	called := false
	if err := m.RegisterFunc("take", func(s string) { called = true }, false); err != nil {
		t.Fatal(err)
	}

	_, err := m.Eval("try { take(s) } catch (e) { globalThis.caught = true } finally { globalThis.finished = true }", EvalGlobal)
	var e *Error
	if !errors.As(err, &e) || e.Name != "InternalError" || e.Message != "out of memory" {
		t.Fatalf("got error %v, want the out of memory error", err)
	}
	m.SetMemoryLimit(^uintptr(0))
	if v, err := m.Eval("typeof caught + typeof finished", EvalGlobal); err != nil || v != "undefinedundefined" {
		t.Fatalf("page code saw the out of memory error: %v, %v", v, err)
	}

	if called {
		t.Fatal("take was called")
	}

	wantNoPendingException(t, m)
	wantUsable(t, m)
}

// Reading a thrown value whose toString throws must not leave that second
// exception pending.
func TestErrorFromValueToStringThrows(t *testing.T) {
	m, err := NewVM()
	if err != nil {
		t.Fatal(err)
	}

	defer m.Close()

	_, err = m.Eval(`throw {toString() { throw new Error("from toString") }}`, EvalGlobal)
	var e *Error
	if !errors.As(err, &e) {
		t.Fatalf("got error %v, want an *Error", err)
	}

	wantNoPendingException(t, m)
	if v, err := m.Eval("1 + 1", EvalGlobal); err != nil || v != 2 {
		t.Fatalf("1 + 1 after the failed read: %v, %v", v, err)
	}
}

// A BigInt is read through its toString. When looking toString up or calling
// it throws, that exception is the error, and it does not stay pending.
func TestBigIntToStringThrows(t *testing.T) {
	for _, tc := range []struct{ name, setup, message string }{
		{"lookup", `Object.defineProperty(BigInt.prototype, "toString", {get() { throw new Error("from the getter") }})`, "from the getter"},
		{"call", `BigInt.prototype.toString = function() { throw new Error("from toString") }`, "from toString"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, err := NewVM()
			if err != nil {
				t.Fatal(err)
			}

			defer m.Close()

			if _, err := m.Eval(tc.setup, EvalGlobal); err != nil {
				t.Fatal(err)
			}

			_, err = m.Eval("1n", EvalGlobal)
			var e *Error
			if !errors.As(err, &e) || e.Name != "Error" || e.Message != tc.message {
				t.Fatalf("got error %v, want Error: %s", err, tc.message)
			}

			wantNoPendingException(t, m)
		})
	}
}

// ErrorFromValue reads properties of the thrown value. A getter that throws
// leaves that field empty and its exception must not stay pending.
func TestErrorFromValueGetterThrows(t *testing.T) {
	for _, prop := range []string{"stack", "lineNumber"} {
		t.Run(prop, func(t *testing.T) {
			m, err := NewVM()
			if err != nil {
				t.Fatal(err)
			}

			defer m.Close()

			_, err = m.Eval(`var e = new Error("thrown"); Object.defineProperty(e, "`+prop+`", {get() { throw new Error("from the getter") }}); throw e`, EvalGlobal)
			var e *Error
			if !errors.As(err, &e) || e.Message != "thrown" {
				t.Fatalf("got error %v, want Error: thrown", err)
			}

			if prop == "stack" && e.Stack != "" {
				t.Fatalf("got stack %q, want none", e.Stack)
			}

			wantNoPendingException(t, m)
		})
	}
}

// A thrown value that is not an object has no name or message to read, and
// reading them must not leave an exception in the context. Near the memory
// limit the engine throws null.
func TestThrownPrimitiveLeavesNoException(t *testing.T) {
	m, err := NewVM()
	if err != nil {
		t.Fatal(err)
	}

	defer m.Close()

	for _, src := range []string{"throw null", "throw undefined", "throw Symbol('x')"} {
		if _, err := m.Eval(src, EvalGlobal); err == nil {
			t.Fatalf("%s: no error", src)
		}

		wantNoPendingException(t, m)
		if v, err := m.Eval("1 + 1", EvalGlobal); err != nil || v != 2 {
			t.Fatalf("after %s: got %v, %v, want 2", src, v, err)
		}
	}
}
