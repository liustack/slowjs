// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package slowjs // import "github.com/liustack/slowjs"

import (
	"errors"
	"strconv"
	"strings"
	"testing"
)

// stringOOMSize is the length of the string the tests below convert. Its
// characters are not ASCII: "é" is one byte in the engine and two in UTF-8,
// "中" two bytes in the engine and three in UTF-8.
const stringOOMSize = 1 << 20

// unitBytes is how many bytes the engine keeps for each character of ch.
func unitBytes(ch string) uintptr {
	if ch == "é" {
		return 1
	}
	return 2
}

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

// A string returned to Go is read where the engine keeps it: it needs no
// room in the VM for a UTF-8 copy, so a result as large as the memory left
// comes back whole. A rope, a string the engine keeps as the pieces it was
// joined from, needs room for one copy of its characters, joined, and no
// more.
func TestGoStringNeedsNoRoom(t *testing.T) {
	for _, ch := range []string{"é", "中"} {
		t.Run(ch, func(t *testing.T) {
			// Less room than the characters of s take, let alone their UTF-8.
			m := newStringOOMVM(t, ch, stringOOMSize/2)
			v, err := m.Eval("s", EvalGlobal)
			if err != nil {
				t.Fatalf("Eval(s): %v", err)
			}
			if v != strings.Repeat(ch, stringOOMSize) {
				t.Fatalf("Eval(s) returned a %T of %d bytes", v, len(v.(string)))
			}

			// Room for the joined rope, half a MiB to spare, but not for its
			// UTF-8 next to it.
			m.SetMemoryLimit(^uintptr(0))
			m.SetMemoryLimit(uintptr(vmMemory(m).Fmalloc_size) + stringOOMSize*unitBytes(ch) + stringOOMSize/2)
			v, err = m.Eval("s + '!'", EvalGlobal)
			if err != nil {
				t.Fatalf("Eval(s + '!'): %v", err)
			}
			if v != strings.Repeat(ch, stringOOMSize)+"!" {
				t.Fatalf("Eval(s + '!') returned a %T of %d bytes", v, len(v.(string)))
			}
			wantNoPendingException(t, m)
		})
	}
}

// A rope returned to Go that has no room to be joined must come back as the
// out of memory error, not as "".
func TestGoStringOutOfMemory(t *testing.T) {
	for _, ch := range []string{"é", "中"} {
		t.Run(ch, func(t *testing.T) {
			// Joining s + '!' needs stringOOMSize or twice that.
			m := newStringOOMVM(t, ch, stringOOMSize/2)
			v, err := m.Eval("s + '!'", EvalGlobal)
			if err == nil {
				t.Fatalf("Eval returned a %T of %d bytes, want an error", v, len(v.(string)))
			}

			wantOutOfMemory(t, "Eval", err)
			wantNoPendingException(t, m)
			wantUsable(t, m)
		})
	}
}

// The JSON of an object returned to Go, and that of Value.MarshalJSON, is
// read where JSON.stringify left it: with room for JSON.stringify but none for
// a UTF-8 copy of its result, the JSON comes back whole. With no room for
// JSON.stringify it is the out of memory error, not an empty Object.
func TestObjectJSON(t *testing.T) {
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
			if v, err := m.Eval("JSON.stringify(o).length", EvalGlobal); err != nil || v != stringOOMSize+8 {
				t.Fatalf("JSON.stringify(o).length: %v, %v", v, err)
			}

			json, err := c.get(m)
			if err != nil {
				t.Fatal(err)
			}
			if want := `{"s":"` + strings.Repeat("é", stringOOMSize) + `"}`; json != want {
				t.Fatalf("got %d bytes of JSON, want %d", len(json), len(want))
			}

			// No room for JSON.stringify(o) itself.
			m.SetMemoryLimit(^uintptr(0))
			m.SetMemoryLimit(uintptr(vmMemory(m).Fmalloc_size) + stringOOMSize/2)
			json, err = c.get(m)
			if err == nil {
				t.Fatalf("got %d bytes of JSON, want an error", len(json))
			}

			wantOutOfMemory(t, c.name, err)
			wantNoPendingException(t, m)
			wantUsable(t, m)
		})
	}
}

// A host function's string argument is read where the engine keeps it, so it
// needs no room. One that is a rope with no room to be joined must not be
// passed as "": running out of memory there ends the call, and page code
// cannot catch it.
func TestHostFuncArgumentOutOfMemory(t *testing.T) {
	// Less room than s takes: joining s + '!' needs stringOOMSize bytes.
	m := newStringOOMVM(t, "é", stringOOMSize/2)
	var got []int
	if err := m.RegisterFunc("take", func(s string) { got = append(got, len(s)) }, false); err != nil {
		t.Fatal(err)
	}

	if _, err := m.Eval("take(s)", EvalGlobal); err != nil {
		t.Fatalf("take(s): %v", err)
	}
	if len(got) != 1 || got[0] != 2*stringOOMSize {
		t.Fatalf("take(s) got strings of %v bytes, want one of %d", got, 2*stringOOMSize)
	}

	_, err := m.Eval("try { take(s + '!') } catch (e) { globalThis.caught = true } finally { globalThis.finished = true }", EvalGlobal)
	var e *Error
	if !errors.As(err, &e) || e.Name != "InternalError" || e.Message != "out of memory" {
		t.Fatalf("got error %v, want the out of memory error", err)
	}
	m.SetMemoryLimit(^uintptr(0))
	if v, err := m.Eval("typeof caught + typeof finished", EvalGlobal); err != nil || v != "undefinedundefined" {
		t.Fatalf("page code saw the out of memory error: %v, %v", v, err)
	}

	if len(got) != 1 {
		t.Fatal("take was called with the rope")
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
