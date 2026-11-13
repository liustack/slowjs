// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package quickjs // import "github.com/liustack/quickjs"

import (
	"errors"
	"fmt"
	"math/rand"
	"runtime"
	"strings"
	"testing"
	"time"
	"unsafe"
)

// longJSStrings are long strings of javascript, one for each way a string of
// the engine becomes a Go string: 16-bit units of three bytes of UTF-8,
// surrogate pairs, lone surrogates, 8-bit characters from U+0080 and ASCII.
var longJSStrings = []struct{ name, js string }{
	{"cjk", `'中'.repeat(1 << 24)`},
	{"pairs", `'😀'.repeat(1 << 23)`},
	{"lone", `'\ud800a'.repeat(1 << 23)`},
	{"latin1", `'é'.repeat(1 << 24)`},
	{"ascii", `'a'.repeat(1 << 25)`},
}

// longGoStrings returns long Go strings, one for each way NewString makes a
// string of the engine: characters from U+0800, characters above U+FFFF,
// characters below U+0100, ASCII and invalid UTF-8.
func longGoStrings() []struct{ name, s string } {
	return []struct{ name, s string }{
		{"cjk", strings.Repeat("中", 1<<24)},
		{"pairs", strings.Repeat("😀", 1<<23)},
		{"latin1", strings.Repeat("é", 1<<24)},
		{"ascii", strings.Repeat("a", 1<<25)},
		{"invalid", strings.Repeat("\xff中", 1<<23)},
	}
}

// The stops a conversion must see: Interrupt from the Go function itself,
// and the eval timeout, which the Go function waits for.
var conversionStops = []struct {
	name  string
	setup func(m *VM)               // before the call
	stop  func(a Args)              // in the Go function, before the conversion
	end   func(t *testing.T, m *VM) // after the call
}{
	{"interrupt", func(m *VM) {}, func(a Args) { a.VM().Interrupt() }, func(t *testing.T, m *VM) {}},
	{"timeout", func(m *VM) {
		if err := m.SetEvalTimeout(5 * time.Millisecond); err != nil {
			panic(err)
		}
	}, func(a Args) {
		// A wait for the deadline, which a hang leaves to go test.
		for !a.VM().h.Stopping() {
		}
	}, func(t *testing.T, m *VM) {
		if err := m.SetEvalTimeout(0); err != nil {
			t.Fatal(err)
		}
	}},
}

// wantStopped fails unless err is the uncatchable interrupt, not a kill.
func wantStopped(t *testing.T, what string, err error) {
	t.Helper()
	var e *Error
	if !errors.As(err, &e) || !e.Uncatchable || !e.Interrupted || errors.Is(err, ErrKilled) {
		t.Errorf("%s: %v, want the interrupt", what, err)
	}
}

// A long string argument read with Args.String or Args.WTF8 after Interrupt
// or the eval timeout is no string: ok is false, and the call ends with the
// interrupt, not killed, whatever the Go function returns. The VM runs
// scripts after.
func TestArgStringStops(t *testing.T) {
	for _, stop := range conversionStops {
		t.Run(stop.name, func(t *testing.T) {
			m := newTestVM(t)
			var got string
			var ok bool
			read := func(wtf8 bool) func(a Args) (Value, error) {
				return func(a Args) (Value, error) {
					stop.stop(a)
					if wtf8 {
						got, ok = a.WTF8(0)
					} else {
						got, ok = a.String(0)
					}
					return a.VM().NewString("read")
				}
			}
			defineBuiltin(t, m, "readString", NewBuiltin("readString", 1, read(false)))
			defineBuiltin(t, m, "readWTF8", NewBuiltin("readWTF8", 1, read(true)))
			for _, c := range longJSStrings {
				if _, err := m.Eval(`globalThis.s = `+c.js, EvalGlobal); err != nil {
					t.Fatal(err)
				}
				for _, f := range []string{"readString", "readWTF8"} {
					got, ok = "", true
					stop.setup(m)
					_, err := m.Eval(f+`(s)`, EvalGlobal)
					stop.end(t, m)
					if ok || got != "" {
						t.Errorf("%s(%s): %d bytes, ok %v, want no string", f, c.name, len(got), ok)
					}
					wantStopped(t, f+"("+c.name+")", err)
					if got := evalString(t, m, `String(1 + 1)`); got != "2" {
						t.Errorf("after %s(%s): %q", f, c.name, got)
					}
				}
			}
		})
	}
}

// NewString of a long string in a Go function after Interrupt or the eval
// timeout returns the interrupt and no string, and the call ends with it.
// Outside a call, also after Interrupt while the VM is idle, it makes the
// whole string.
func TestNewStringStops(t *testing.T) {
	long := longGoStrings()
	for _, stop := range conversionStops {
		t.Run(stop.name, func(t *testing.T) {
			m := newTestVM(t)
			var s string
			var got Value
			var gotErr error
			defineBuiltin(t, m, "make", NewBuiltin("make", 0, func(a Args) (Value, error) {
				stop.stop(a)
				got, gotErr = a.VM().NewString(s)
				return got, gotErr
			}))
			for _, c := range long {
				s = c.s
				stop.setup(m)
				_, err := m.Eval(`make()`, EvalGlobal)
				stop.end(t, m)
				if got != (Value{}) {
					t.Errorf("NewString(%s): a string, want none", c.name)
				}
				wantStopped(t, "NewString("+c.name+") in the call", gotErr)
				wantStopped(t, "the call of NewString("+c.name+")", err)
			}
		})
	}
	m := newTestVM(t)
	for _, idle := range []bool{false, true} {
		for _, c := range long {
			if idle {
				m.Interrupt()
			}
			v, err := m.NewString(c.s)
			if err != nil {
				t.Fatalf("NewString(%s) outside a call, idle %v: %v", c.name, idle, err)
			}
			n, err := m.goString(v.v)
			v.Free()
			if err != nil || n != strings.ToValidUTF8(c.s, "\ufffd") {
				t.Errorf("NewString(%s) outside a call, idle %v: %d bytes back, %v", c.name, idle, len(n), err)
			}
		}
	}
}

// A stop that comes while a long string is converted, from another
// goroutine, is seen at the next read of whether to stop: the conversion
// of a string read or made in a Go function ends there with the interrupt.
func TestConversionStopsMidway(t *testing.T) {
	m := newTestVM(t)
	reads := 0
	stringStepHook = func() {
		reads++
		if reads == 1 {
			done := make(chan struct{})
			go func() {
				m.Interrupt()
				close(done)
			}()
			<-done
		}
	}
	t.Cleanup(func() { stringStepHook = nil })
	var ok bool
	var gotErr error
	defineBuiltin(t, m, "read", NewBuiltin("read", 1, func(a Args) (Value, error) {
		_, ok = a.String(0)
		return UndefinedValue, nil
	}))
	var s string
	defineBuiltin(t, m, "make", NewBuiltin("make", 0, func(a Args) (Value, error) {
		var v Value
		v, gotErr = a.VM().NewString(s)
		return v, gotErr
	}))
	for _, c := range longJSStrings {
		if _, err := m.Eval(`globalThis.s = `+c.js, EvalGlobal); err != nil {
			t.Fatal(err)
		}
		reads, ok = 0, true
		_, err := m.Eval(`read(s)`, EvalGlobal)
		if ok || reads != 1 {
			t.Errorf("read(%s): ok %v after %d reads of the stop, want a stop at the first", c.name, ok, reads)
		}
		wantStopped(t, "read("+c.name+")", err)
	}
	for _, c := range longGoStrings() {
		s, reads = c.s, 0
		_, err := m.Eval(`make()`, EvalGlobal)
		if reads != 1 {
			t.Errorf("NewString(%s): %d reads of the stop, want a stop at the first", c.name, reads)
		}
		wantStopped(t, "NewString("+c.name+")", gotErr)
		wantStopped(t, "the call of NewString("+c.name+")", err)
	}
}

// Converted a few characters at a time, a string comes out as converted
// whole, wherever the pieces end: in a surrogate pair, in a character of
// UTF-8 or in a run of bytes that are not UTF-8.
func TestConversionPieces(t *testing.T) {
	m := newTestVM(t)
	r := rand.New(rand.NewSource(7))
	units := [][2]int{{0, 0x80}, {0x80, 0x100}, {0x100, 0x800}, {0x800, 0xD800}, {0xD800, 0xDC00}, {0xDC00, 0xE000}, {0xE000, 0x10000}}
	pieces := []string{"a", "é", "ÿ", "中", "😀", "\xff", "\x80", "\x80\x80\x80\x80", "\xe4\xb8", "\xf0\x9f\x98", "\xed\xa0\x80", "\xc0\xaf"}
	for k := 0; k < 4000; k++ {
		var u []uint16
		for len(u) < r.Intn(60) {
			kind := units[r.Intn(len(units))]
			for range 1 + r.Intn(4) {
				u = append(u, uint16(kind[0]+r.Intn(kind[1]-kind[0])))
			}
		}
		var b []byte
		for len(b) < r.Intn(60) {
			b = append(b, byte(r.Intn(256)))
		}
		var g strings.Builder
		for g.Len() < r.Intn(80) {
			g.WriteString(pieces[r.Intn(len(pieces))])
		}
		gs := g.String()
		if k%2 == 0 {
			// only characters below U+0100, which make a string of 8 bits
			gs = strings.Map(func(c rune) rune { return c & 0xFF }, strings.ToValidUTF8(gs, "x"))
		}

		type result struct {
			u8, u16, l1 string
			chars       []byte
		}
		convert := func() (res result) {
			res.u8, _ = utf16String(nil, u, false)
			res.u16, _ = utf16String(nil, u, true)
			res.l1, _ = latin1String(nil, b)
			v := m.newStringFast(gs)
			if isException(v) {
				t.Fatalf("newStringFast(%q): %v", gs, m.errFromException())
			}
			p, n, wide, _ := stringChars(v)
			if wide {
				n *= 2
			}
			res.chars = append([]byte{byte(n), fmt.Sprint(wide)[0]}, unsafe.Slice((*byte)(addr(p)), n)...)
			m.jsFreeValue(v)
			return res
		}
		whole := convert()
		for _, step := range []int{1, 2, 3, 4, 5, 7} {
			stringStep = step
			got := convert()
			stringStep = 1 << 18
			if got.u8 != whole.u8 || got.u16 != whole.u16 || got.l1 != whole.l1 || string(got.chars) != string(whole.chars) {
				t.Fatalf("pieces of %d: units %x, bytes %x, string %q: %+v, want %+v", step, u, b, gs, got, whole)
			}
		}
	}
}

// NewStringLatin1 and NewStringUTF16 of many characters in a Go function
// after Interrupt or the eval timeout return the interrupt and no string,
// as NewString does: a Go function that read a long argument with
// Args.Latin1 or Args.UTF16 and returns it so stops too.
func TestNewStringCharsStops(t *testing.T) {
	b := []byte(strings.Repeat("\xe9", 1<<25))
	narrow := make([]uint16, 1<<25)
	wide := make([]uint16, 1<<25)
	for i := range narrow {
		narrow[i], wide[i] = 0xe9, 0x4e2d
	}
	makers := []struct {
		name string
		make func(m *VM) (Value, error)
	}{
		{"Latin1", func(m *VM) (Value, error) { return m.NewStringLatin1(b) }},
		{"UTF16 narrow", func(m *VM) (Value, error) { return m.NewStringUTF16(narrow) }},
		{"UTF16 wide", func(m *VM) (Value, error) { return m.NewStringUTF16(wide) }},
	}
	for _, stop := range conversionStops {
		t.Run(stop.name, func(t *testing.T) {
			m := newTestVM(t)
			var maker func(m *VM) (Value, error)
			var got Value
			var gotErr error
			defineBuiltin(t, m, "make", NewBuiltin("make", 0, func(a Args) (Value, error) {
				stop.stop(a)
				got, gotErr = maker(a.VM())
				return got, gotErr
			}))
			for _, c := range makers {
				maker = c.make
				stop.setup(m)
				_, err := m.Eval(`make()`, EvalGlobal)
				stop.end(t, m)
				if got != (Value{}) {
					t.Errorf("NewString%s: a string, want none", c.name)
				}
				wantStopped(t, "NewString"+c.name+" in the call", gotErr)
				wantStopped(t, "the call of NewString"+c.name, err)
			}
		})
	}
	m := newTestVM(t)
	m.Interrupt()
	for _, c := range makers {
		v, err := c.make(m)
		if err != nil {
			t.Fatalf("NewString%s outside a call: %v", c.name, err)
		}
		if _, n, _, _ := stringChars(v.v); n != 1<<25 {
			t.Errorf("NewString%s outside a call: %d characters", c.name, n)
		}
		v.Free()
	}
}

// A read of a long string reads the stop once more after it counted the
// bytes of the result and before Go allocates it, the one step that cannot
// be cut: a stop that comes by then allocates nothing.
func TestArgStringStopsBeforeAllocation(t *testing.T) {
	m := newTestVM(t)
	const n = 1 << 24
	pieces := n / stringStep
	reads := 0
	stringStepHook = func() {
		reads++
		// The reads of the count come between its pieces, and the one
		// before the allocation after them.
		if reads == pieces {
			m.Interrupt()
		}
	}
	t.Cleanup(func() { stringStepHook = nil })
	var ok bool
	var allocated uint64
	defineBuiltin(t, m, "read", NewBuiltin("read", 1, func(a Args) (Value, error) {
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		_, ok = a.String(0)
		runtime.ReadMemStats(&after)
		allocated = after.TotalAlloc - before.TotalAlloc
		return UndefinedValue, nil
	}))
	for _, c := range []struct{ name, js string }{
		{"ascii", `'a'.repeat(1 << 24)`},
		{"latin1", `'é'.repeat(1 << 24)`},
		{"cjk", `'中'.repeat(1 << 24)`},
		{"lone", `'\ud800'.repeat(1 << 24)`},
	} {
		if _, err := m.Eval(`globalThis.s = `+c.js, EvalGlobal); err != nil {
			t.Fatal(err)
		}
		reads, ok = 0, true
		_, err := m.Eval(`read(s)`, EvalGlobal)
		if ok || reads != pieces || allocated > n/4 {
			t.Errorf("%s: ok %v after %d reads of the stop, %d bytes allocated, want a stop at read %d and less than %d bytes", c.name, ok, reads, allocated, pieces, n/4)
		}
		wantStopped(t, "read("+c.name+")", err)
	}
}

// A long 8-bit string that is ASCII but for its end converts in steps, the
// ASCII before the first other character too: the memory of the result is a
// block, whose pages the OS hands over as they are written (hostmem.Take),
// and a copy of it in one go would take them all with no read of the stop.
func TestLatin1PrefixSteps(t *testing.T) {
	const n = 1 << 25
	b := []byte(strings.Repeat("a", n) + "\xe9")
	reads := 0
	stringStepHook = func() { reads++ }
	t.Cleanup(func() { stringStepHook = nil })
	s, ok := latin1String(nil, b)
	if !ok || len(s) != n+2 || s[n:] != "é" {
		t.Fatalf("latin1String of %d bytes: %d bytes, ok %v", len(b), len(s), ok)
	}
	// The ASCII is read twice, once to find where it ends and once to copy.
	if want := 2 * (n / stringStep); reads < want {
		t.Errorf("latin1String of %d bytes read the stop %d times, want at least %d", len(b), reads, want)
	}
}
