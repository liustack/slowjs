// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build slowjs_cengine && linux && amd64

package slowjs // import "github.com/liustack/slowjs"

import (
	"runtime"
	"testing"
	"time"
)

// A panic or a Goexit that goes up through the engine's frames leaves the VM
// good only for Close, wherever it starts and whatever Go code recovers it
// above: the engine still points at the frames that are gone.

// wantBroken checks that m is good only for Close now.
func wantBroken(t *testing.T, m *VM, after string) {
	t.Helper()
	if p := evalRecover(m, `1`); p != brokenPanic {
		t.Fatalf("Eval after %s panicked with %v, want %q", after, p, brokenPanic)
	}
}

// The clock of SetClock runs inside the engine's frames, called by the
// engine (host_now), not as a function javascript calls.
func TestClockPanicBreaksVM(t *testing.T) {
	m := newTestVM(t)
	m.SetClock(func() time.Time { panic("clock panic") })
	if p := evalRecover(m, `Date.now()`); p != "clock panic" {
		t.Fatalf("Eval panicked with %v, want the clock's panic", p)
	}
	wantBroken(t, m, "a panic of the clock")
}

func TestClockGoexitBreaksVM(t *testing.T) {
	m := newTestVM(t)
	m.SetClock(func() time.Time { runtime.Goexit(); return time.Time{} })
	done := make(chan struct{})
	go func() {
		defer close(done)
		m.Eval(`Date.now()`, EvalGlobal)
		t.Error("Eval returned after a Goexit")
	}()
	<-done
	wantBroken(t, m, "a Goexit of the clock")
}

// A Go function that recovers the panic of javascript it evaluated in turn,
// which went up through the engine, cannot go on as if nothing happened: its
// call panics when it returns, and the VM is broken.
func TestNestedClockPanicRecoveredBreaksVM(t *testing.T) {
	m := newTestVM(t)
	defineBuiltin(t, m, "inner", NewBuiltin("inner", 0, func(a Args) (Value, error) {
		m := a.VM()
		func() {
			defer func() { recover() }()
			m.SetClock(func() time.Time { panic("clock panic") })
			m.Eval(`Date.now()`, EvalGlobal)
		}()
		m.SetClock(nil)
		return m.NewInt(0), nil
	}))
	if p := evalRecover(m, `inner()`); p != framesLeftPanic {
		t.Fatalf("Eval panicked with %v, want %q", p, framesLeftPanic)
	}
	wantBroken(t, m, "a recovered panic of the clock")
}

// The same for a memory fault in the engine under a nested call: the nested
// call is given a shadow stack with nothing left, so its first call into the
// engine faults on the guard page.
func TestNestedFaultRecoveredBreaksVM(t *testing.T) {
	m := newTestVM(t)
	defineBuiltin(t, m, "inner", NewBuiltin("inner", 0, func(a Args) (Value, error) {
		m := a.VM()
		sp := m.sp
		func() {
			defer func() { recover() }()
			m.sp = m.end - 8
			m.Eval(`1`, EvalGlobal)
		}()
		m.sp = sp
		return m.NewInt(0), nil
	}))
	if p := evalRecover(m, `inner()`); p != framesLeftPanic {
		t.Fatalf("Eval panicked with %v, want %q", p, framesLeftPanic)
	}
	wantBroken(t, m, "a recovered fault")
}

// A panic of a Go function that javascript called is not one: it became an
// uncatchable error, the javascript ended, and the panic goes on in the Go
// code that called javascript. The VM goes on, also when that is a Go
// function that javascript called, which recovers it: the uncatchable error
// stays pending and ends the javascript that called it.
func TestNestedHostPanicRecoveredKeepsVM(t *testing.T) {
	m := newTestVM(t)
	defineBuiltin(t, m, "boom", NewBuiltin("boom", 0, func(a Args) (Value, error) { panic("boom") }))
	defineBuiltin(t, m, "inner", NewBuiltin("inner", 0, func(a Args) (Value, error) {
		m := a.VM()
		func() {
			defer func() { recover() }()
			m.Eval(`boom()`, EvalGlobal)
		}()
		return m.NewInt(7), nil
	}))
	_, err := m.Eval(`inner()`, EvalGlobal)
	if e, ok := err.(*Error); !ok || !e.Uncatchable {
		t.Fatalf("inner() failed with %v, want the uncatchable error of the panic", err)
	}
	if got, err := m.Eval(`1 + 1`, EvalGlobal); err != nil || got != 2 {
		t.Fatalf("after the recovered panic: %v, %v", got, err)
	}
}
