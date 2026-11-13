// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package quickjs // import "github.com/liustack/quickjs"

import (
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"
)

// longResults are Go functions whose results make many elements or
// properties, one for each loop that converts a Go value: a slice
// (jsArray), small slices in a slice, a map (jsMap), and a slice and a map
// that can hold a Value (buildArray and buildObject of the snapshot). n is
// how many elements or properties the result has.
func longResults(stop func()) []struct {
	name string
	f    any
	n    int
} {
	const n = 4 * newArrayStep
	keys := make([]string, n)
	for i := range keys {
		keys[i] = "k" + strconv.Itoa(i)
	}
	return []struct {
		name string
		f    any
		n    int
	}{
		{"slice", func() []int { stop(); return make([]int, n) }, n},
		{"rows", func() [][]int {
			stop()
			rows := make([][]int, n/2)
			for i := range rows {
				rows[i] = []int{i, i}
			}
			return rows
		}, n / 2},
		{"map", func() map[string]int {
			stop()
			r := make(map[string]int, n)
			for _, k := range keys {
				r[k] = 1
			}
			return r
		}, n},
		{"anys", func() []any { stop(); return make([]any, n) }, n},
		{"anymap", func() map[string]any {
			stop()
			r := make(map[string]any, n)
			for _, k := range keys {
				r[k] = 1
			}
			return r
		}, n},
	}
}

// A long result of a Go function after Interrupt or the eval timeout is not
// converted to the end: the call ends with the interrupt, not killed. Without
// a stop it converts whole, and the VM runs scripts after.
func TestResultConversionStops(t *testing.T) {
	stops := []struct {
		name  string
		setup func(m *VM)
		stop  func(m *VM)
		end   func(m *VM)
	}{
		{"interrupt", func(m *VM) {}, func(m *VM) { m.Interrupt() }, func(m *VM) {}},
		{"timeout", func(m *VM) {
			if err := m.SetEvalTimeout(5 * time.Millisecond); err != nil {
				t.Fatal(err)
			}
		}, func(m *VM) {
			// A wait for the deadline, which a hang leaves to go test.
			for !m.h.Stopping() {
			}
		}, func(m *VM) {
			if err := m.SetEvalTimeout(0); err != nil {
				t.Fatal(err)
			}
		}},
		{"none", func(m *VM) {}, func(m *VM) {}, func(m *VM) {}},
	}
	for _, stop := range stops {
		t.Run(stop.name, func(t *testing.T) {
			m := newTestVM(t)
			reads := 0
			stringStepHook = func() { reads++ }
			t.Cleanup(func() { stringStepHook = nil })
			for _, c := range longResults(func() { stop.stop(m) }) {
				if err := m.RegisterFunc("f_"+c.name, c.f, false); err != nil {
					t.Fatal(err)
				}
				reads = 0
				stop.setup(m)
				// The length is all the script does after the call, and it
				// reads no interrupt flag before the end.
				r, err := m.Eval(`(r => Array.isArray(r) ? r.length : Object.keys(r).length)(f_`+c.name+`())`, EvalGlobal)
				stop.end(m)
				if stop.name == "none" {
					if err != nil || r != c.n {
						t.Errorf("f_%s: %v, %v, want %d", c.name, r, err, c.n)
					}
					continue
				}
				if reads != 1 {
					t.Errorf("f_%s: %d reads of the stop, want a stop at the first", c.name, reads)
				}
				wantStopped(t, "f_"+c.name, err)
				if got := evalString(t, m, `String(1 + 1)`); got != "2" {
					t.Errorf("after f_%s: %q", c.name, got)
				}
			}
		})
	}
}

// A stop that comes from another goroutine while a result converts, at the
// first read, ends the conversion there.
func TestResultConversionStopsMidway(t *testing.T) {
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
	for _, c := range longResults(func() {}) {
		if err := m.RegisterFunc("f_"+c.name, c.f, false); err != nil {
			t.Fatal(err)
		}
		reads = 0
		_, err := m.Eval(`f_`+c.name+`().length`, EvalGlobal)
		if reads != 1 {
			t.Errorf("f_%s: %d reads of the stop, want a stop at the first", c.name, reads)
		}
		wantStopped(t, "f_"+c.name, err)
	}
}

// NewString of a string of more characters than the engine holds throws
// the error JS_NewStringLen throws for it, however the string is made, and
// makes a string of as many characters as it holds.
func TestNewStringTooLong(t *testing.T) {
	m := newTestVM(t)
	// Only a string longer than stringStep can be too long, as stringStep is
	// far below stringLenMax.
	stringLenLimit, stringStep = 8, 1
	t.Cleanup(func() { stringLenLimit, stringStep = stringLenMax, 1<<18 })
	for _, c := range []struct {
		s  string
		ok bool
	}{
		{strings.Repeat("a", 8), true},
		{strings.Repeat("a", 9), false},
		{strings.Repeat("é", 8), true},
		{strings.Repeat("é", 9), false},
		{strings.Repeat("中", 8), true},
		{strings.Repeat("中", 9), false},
		{strings.Repeat("😀", 4), true},
		{"a" + strings.Repeat("😀", 4), false},
		{strings.Repeat("\xff", 8), true},
		{strings.Repeat("\xff", 9), false},
	} {
		v, err := m.NewString(c.s)
		if c.ok {
			if err != nil {
				t.Errorf("NewString(%q): %v", c.s, err)
				continue
			}
			v.Free()
			continue
		}
		var e *Error
		if !errors.As(err, &e) || e.Name != "InternalError" || e.Message != "string too long" {
			t.Errorf("NewString(%q): %v, want InternalError: string too long", c.s, err)
		}
	}
}

// A key of a result made a string a piece at a time is the property the key
// is whole: invalid UTF-8, an index, __proto__ and NULs too. A long key
// after Interrupt ends the call with the interrupt.
func TestResultLongKeys(t *testing.T) {
	m := newTestVM(t)
	keys := map[string]int{"\xffab\xe4\xb8": 1, "1234": 2, "__proto__": 3, "a\x00中😀b": 4, "ab": 5, "": 6, "k" + strings.Repeat("é", 9): 7}
	if err := m.RegisterFunc("keys", func() map[string]int { return keys }, false); err != nil {
		t.Fatal(err)
	}
	if err := m.RegisterFunc("anyKeys", func() map[string]any {
		r := map[string]any{}
		for k, v := range keys {
			r[k] = v
		}
		return r
	}, false); err != nil {
		t.Fatal(err)
	}
	const js = `[keys(), anyKeys()].map(o => JSON.stringify(Object.entries(o)) + Object.getPrototypeOf(o).constructor.name).join()`
	want := evalString(t, m, js)
	for _, n := range []int{1, 2, 3, 4} {
		stringStep = n
		got := evalString(t, m, js)
		stringStep = 1 << 18
		if got != want {
			t.Errorf("pieces of %d: %s, want %s", n, got, want)
		}
	}
	for _, s := range []string{"\xffab", "1234", "__proto__", "a\x00中😀b"} {
		short, err := m.NewAtom(s)
		if err != nil {
			t.Fatal(err)
		}
		stringStep = 1
		long, err := m.NewAtom(s)
		stringStep = 1 << 18
		if err != nil {
			t.Fatal(err)
		}
		if short != long {
			t.Errorf("NewAtom(%q): %d in pieces, %d whole", s, long, short)
		}
		m.jsFreeAtom(short)
		m.jsFreeAtom(long)
	}

	reads := 0
	stringStepHook = func() { reads++ }
	t.Cleanup(func() { stringStepHook = nil })
	long := strings.Repeat("中", 1<<24)
	if err := m.RegisterFunc("longKey", func() map[string]int {
		m.Interrupt()
		return map[string]int{long: 1}
	}, false); err != nil {
		t.Fatal(err)
	}
	_, err := m.Eval(`longKey()`, EvalGlobal)
	if reads != 1 {
		t.Errorf("%d reads of the stop, want a stop at the first", reads)
	}
	wantStopped(t, "longKey", err)
}

// A long string argument of a Go function that a stop comes for while it
// converts ends the call with the interrupt, and the Go function does not
// run. So does a long string it returns.
func TestRegisterFuncStringStops(t *testing.T) {
	m := newTestVM(t)
	reads := 0
	stringStepHook = func() {
		reads++
		if reads == 1 {
			m.Interrupt()
		}
	}
	t.Cleanup(func() { stringStepHook = nil })
	ran := false
	if err := m.RegisterFunc("takes", func(s string) int { ran = true; return len(s) }, false); err != nil {
		t.Fatal(err)
	}
	if err := m.RegisterFunc("gives", func() string { return strings.Repeat("中", 1<<24) }, false); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Eval(`globalThis.s = '中'.repeat(1 << 24)`, EvalGlobal); err != nil {
		t.Fatal(err)
	}
	reads = 0
	_, err := m.Eval(`takes(s)`, EvalGlobal)
	if ran || reads != 1 {
		t.Errorf("takes: ran %v, %d reads of the stop, want a stop at the first", ran, reads)
	}
	wantStopped(t, "takes", err)
	reads = 0
	_, err = m.Eval(`gives().length`, EvalGlobal)
	if reads != 1 {
		t.Errorf("gives: %d reads of the stop, want a stop at the first", reads)
	}
	wantStopped(t, "gives", err)
}
