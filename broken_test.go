// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package quickjs // import "github.com/liustack/quickjs"

import (
	"runtime"
	"testing"
	"time"

	"github.com/liustack/quickjs/internal/faulttest"
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
	faulttest.Skip(t)
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

// A fault in the engine below the address 0x1000, as a NULL pointer in the C
// makes, is a runtime.Error that names no address: the runtime makes such a
// fault the panic of a nil dereference before it looks at SetPanicOnFault.
// The VM is broken all the same, since the call left the engine halfway. The
// tests make the fault by giving the engine a context at address 0.

// lowFault checks that p is the panic of a fault at a low address.
func lowFault(t *testing.T, p any) {
	t.Helper()
	if _, ok := p.(runtime.Error); !ok {
		t.Fatalf("panicked with %T %v, want the runtime.Error of a fault", p, p)
	}
	if _, ok := p.(interface{ Addr() uintptr }); ok {
		t.Fatalf("the fault %v names an address: the test does not make a fault at a low address", p)
	}
}

// withoutContext runs f with the context of m at address 0, so that the
// first call into the engine in f faults.
func withoutContext(m *VM, f func()) {
	ctx := m.ctx
	m.ctx = 0
	defer func() { m.ctx = ctx }()
	f()
}

func TestLowFaultBreaksVM(t *testing.T) {
	faulttest.Skip(t)
	m := newTestVM(t)
	var p any
	withoutContext(m, func() { p = evalRecover(m, `1`) })
	lowFault(t, p)
	wantBroken(t, m, "a fault at a low address")
}

// A Go function called from javascript calls into the engine itself, which
// faults at a low address: the panic goes on up through the engine that
// called the Go function, and does not become the uncatchable error of a
// panic of the Go function.
func TestHostLowFaultBreaksVM(t *testing.T) {
	faulttest.Skip(t)
	m := newTestVM(t)
	defineBuiltin(t, m, "inner", NewBuiltin("inner", 0, func(a Args) (Value, error) {
		m := a.VM()
		withoutContext(m, func() { m.NewObjectValue() })
		return m.NewInt(0), nil
	}))
	lowFault(t, evalRecover(m, `inner()`))
	wantBroken(t, m, "a fault at a low address in a Go function")
}

// The same for a fault at a low address under a nested call, which the Go
// function recovers.
func TestNestedLowFaultRecoveredBreaksVM(t *testing.T) {
	faulttest.Skip(t)
	m := newTestVM(t)
	var inner any
	defineBuiltin(t, m, "inner", NewBuiltin("inner", 0, func(a Args) (Value, error) {
		m := a.VM()
		withoutContext(m, func() { inner = evalRecover(m, `1`) })
		return m.NewInt(0), nil
	}))
	if p := evalRecover(m, `inner()`); p != framesLeftPanic {
		t.Fatalf("Eval panicked with %v, want %q", p, framesLeftPanic)
	}
	lowFault(t, inner)
	wantBroken(t, m, "a recovered fault at a low address")
}

// zeroContext is an argument whose JSON takes the context of m away, so that
// the next call into the engine in the Call that converts it faults at a low
// address.
type zeroContext struct{ m *VM }

func (z zeroContext) MarshalJSON() ([]byte, error) {
	z.m.ctx = 0
	return []byte(`1`), nil
}

// After a call into the engine left it halfway, the engine is not entered
// again: Call freed the function it evaluated, in a defer that runs before
// leave, on the engine the fault had left. inEngine counts the call that
// faulted, and a call made after it, which the context at 0 makes fault too,
// would still count.
func TestLowFaultInCallFreesNothing(t *testing.T) {
	faulttest.Skip(t)
	m := newTestVM(t)
	ctx := m.ctx
	p := func() (p any) {
		defer func() { p = recover() }()
		m.Call(`(() => () => 1)()`, zeroContext{m})
		return nil
	}()
	m.ctx = ctx
	lowFault(t, p)
	if m.inEngine != 1 {
		t.Fatalf("%d calls into the engine left halfway, want 1: the engine was entered again after the fault", m.inEngine)
	}
	wantBroken(t, m, "a fault at a low address in Call")
}

// A Go function that recovers a fault of its own call into the engine cannot
// call into the engine again.
func TestHostLowFaultRecoveredRefusesEngine(t *testing.T) {
	faulttest.Skip(t)
	m := newTestVM(t)
	var fault, again any
	var calls int
	defineBuiltin(t, m, "inner", NewBuiltin("inner", 0, func(a Args) (Value, error) {
		m := a.VM()
		withoutContext(m, func() {
			defer func() { fault = recover() }()
			m.NewObjectValue()
		})
		func() {
			defer func() { again = recover() }()
			m.NewObjectValue()
		}()
		calls = m.inEngine
		return m.NewInt(0), nil
	}))
	if p := evalRecover(m, `inner()`); p != framesLeftPanic && p != brokenPanic {
		t.Fatalf("Eval panicked with %v, want %q or %q", p, framesLeftPanic, brokenPanic)
	}
	lowFault(t, fault)
	if again != brokenPanic {
		t.Fatalf("a call into the engine after the fault panicked with %v, want %q", again, brokenPanic)
	}
	if calls != 1 {
		t.Fatalf("%d calls into the engine left halfway, want 1", calls)
	}
	wantBroken(t, m, "a recovered fault at a low address in a Go function")
}
