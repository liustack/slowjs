// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build slowjs_cengine && ((linux && (amd64 || arm64)) || (darwin && arm64))

package slowjs // import "github.com/liustack/slowjs"

// On the C engine a script that runs on 50 ms past its deadline or an
// Interrupt is killed: the call returns an interrupted error that matches
// ErrKilled, and the VM is good only for Close. On lib it runs on.

import (
	"errors"
	"testing"
	"time"
)

// killGrace is how long the C engine lets a script run on.
const killGrace = 50 * time.Millisecond

// killedVM returns a VM a kill left good only for Close, with the error of
// the call that was killed: a host function sleeps past the deadline and the
// grace, and the first poll of the javascript it runs then kills.
func killedVM(t *testing.T, nested func(m *VM) error) (*VM, error) {
	t.Helper()
	return killedVMWith(t, newTestVM(t), nested)
}

func killedVMWith(t *testing.T, m *VM, nested func(m *VM) error) (*VM, error) {
	t.Helper()
	const timeout = 20 * time.Millisecond
	if err := m.SetEvalTimeout(timeout); err != nil {
		t.Fatal(err)
	}
	returned := false
	mustHost(t, m, "nested", func([]any) (any, error) {
		time.Sleep(timeout + killGrace + pastDeadline)
		nested(m)
		returned = true
		return nil, nil
	})
	if _, err := m.Eval(`globalThis.loop = function() { for (let i = 0; i < 1e5; i++) {} }`, EvalGlobal); err != nil {
		t.Fatal(err)
	}
	_, err := m.Eval(`nested(); loop(); 'done'`, EvalGlobal)
	if returned {
		t.Fatal("the nested call returned to the host function after the kill")
	}
	return m, err
}

// TestNestedEvalPastGrace runs javascript from a host function past the
// deadline of the outer call and the grace after it: the kill goes up
// through the nested call and the host function, and the outer call returns
// it.
func TestNestedEvalPastGrace(t *testing.T) {
	for name, nested := range nestedEntries {
		t.Run(name, func(t *testing.T) {
			_, err := killedVM(t, nested)
			var e *Error
			if !errors.As(err, &e) || !e.Interrupted || !e.Uncatchable || !errors.Is(err, ErrKilled) {
				t.Fatalf("the outer call got %#v, want an interrupted error that matches ErrKilled", err)
			}
			if err.Error() != "InternalError: interrupted" {
				t.Fatalf("the outer call got %q, want the text of an interrupted error", err)
			}
		})
	}
}

// After a kill the methods that return an error return one that matches
// ErrKilled, those that do not panic, and Close closes the VM and lets go of
// the Go functions it held, of a handler of Then too, which never runs.
func TestKilledVM(t *testing.T) {
	// Closed by the test, which a second Close would not take.
	m, err := NewVM()
	if err != nil {
		t.Fatal(err)
	}
	v, err := m.EvalValue(`({a: 1, f() {}, then() {}})`, EvalGlobal)
	if err != nil {
		t.Fatal(err)
	}
	atom, err := m.NewAtom("a")
	if err != nil {
		t.Fatal(err)
	}
	// A handler of Then on a promise resolved before the kill, whose job
	// never runs.
	pc, err := m.NewPromiseCapability()
	if err != nil {
		t.Fatal(err)
	}
	res, err := pc.Promise.Then(func(Value) { t.Error("a handler ran after the kill") }, nil)
	if err != nil {
		t.Fatal(err)
	}
	res.Free()
	if _, err := pc.Resolve.Call(UndefinedValue, 1); err != nil {
		t.Fatal(err)
	}
	pc.Free()
	h := m.h
	if _, err := killedVMWith(t, m, nestedEntries["Eval"]); !errors.Is(err, ErrKilled) {
		t.Fatalf("the killed call got %v", err)
	}
	for name, call := range map[string]func() error{
		"Eval":                 func() error { _, err := m.Eval(`1`, EvalGlobal); return err },
		"EvalValue":            func() error { _, err := m.EvalValue(`1`, EvalGlobal); return err },
		"Call":                 func() error { _, err := m.Call("loop"); return err },
		"Compile":              func() error { _, err := m.Compile(`1`, EvalGlobal); return err },
		"EvalBytecode":         func() error { _, err := m.EvalBytecode([]byte{1}); return err },
		"ExecutePendingJobs":   func() error { _, err := m.ExecutePendingJobs(); return err },
		"NewString":            func() error { _, err := m.NewString("s"); return err },
		"NewStringLatin1":      func() error { _, err := m.NewStringLatin1([]byte("s")); return err },
		"NewStringUTF16":       func() error { _, err := m.NewStringUTF16([]uint16{'s'}); return err },
		"NewObjectValue":       func() error { _, err := m.NewObjectValue(); return err },
		"NewArray":             func() error { _, err := m.NewArray(); return err },
		"NewAtom":              func() error { _, err := m.NewAtom("a"); return err },
		"NewPromiseCapability": func() error { _, err := m.NewPromiseCapability(); return err },
		"NewBuiltinFunction":   func() error { _, err := m.NewBuiltinFunction(testBuiltin); return err },
		"RegisterFunc":         func() error { return m.RegisterFunc("f", func() {}, false) },
		"RegisterHostFunc":     func() error { return m.RegisterHostFunc("h", func([]any) (any, error) { return nil, nil }) },
		"ReleaseMemory":        func() error { _, err := m.ReleaseMemory(0); return err },
		"SetShapeCache":        func() error { return m.SetShapeCache(1) },
		"SetRegExpCache":       func() error { return m.SetRegExpCache(1, 1<<10) },
		"SetEvalTimeout":       func() error { return m.SetEvalTimeout(time.Second) },
		"GetProperty":          func() error { _, err := m.GetProperty(v, atom); return err },
		"SetProperty":          func() error { return m.SetProperty(v, atom, 1) },
		"SetPropertyValue":     func() error { return m.SetPropertyValue(v, atom, UndefinedValue) },
		"GetPropertyValue":     func() error { _, err := m.GetPropertyValue(v, atom); return err },
		"Value.Property":       func() error { _, err := v.Property(atom); return err },
		"Value.Any":            func() error { _, err := v.Any(); return err },
		"Value.MarshalJSON":    func() error { _, err := v.MarshalJSON(); return err },
		"Value.Call":           func() error { _, err := v.Call(UndefinedValue); return err },
		"Value.CallValue":      func() error { _, err := v.CallValue(UndefinedValue); return err },
		"Value.Then":           func() error { _, err := v.Then(func(Value) {}, nil); return err },
	} {
		if err := call(); !errors.Is(err, ErrKilled) {
			t.Errorf("%s after a kill: %v, want an error that matches ErrKilled", name, err)
		}
	}
	for name, call := range map[string]func(){
		"RunGC":      func() { m.RunGC() },
		"RunYoungGC": func() { m.RunYoungGC() },
	} {
		if p := panicked(call); p != brokenPanic {
			t.Errorf("%s after a kill panicked with %v, want %q", name, p, brokenPanic)
		}
	}
	// Free does not enter the engine of a VM that is good only for Close.
	v.Free()
	if h.Handles() == 0 {
		t.Fatal("the VM let go of the Go functions of its functions before Close")
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	if got := h.Handles(); got != 0 {
		t.Fatalf("%d Go functions held after Close", got)
	}
}

// testBuiltin is a Builtin that returns undefined.
var testBuiltin = NewBuiltin("testBuiltin", 0, func(a Args) (Value, error) { return UndefinedValue, nil })

// panicked runs f and returns what it panicked with, or nil.
func panicked(f func()) (r any) {
	defer func() { r = recover() }()
	f()
	return nil
}

// MemorySize and GCThreshold read one counter of the runtime and change
// nothing, so they answer on a killed VM as they do on one that an Interrupt
// stopped (the engine of lib never kills). The methods that collect, which
// need the engine in a state a kill does not leave, still panic.
func TestKilledVMReportsItsMemory(t *testing.T) {
	m := newTestVM(t)
	before := m.MemorySize()
	threshold := m.GCThreshold()
	if _, err := killedVMWith(t, m, nestedEntries["Eval"]); !errors.Is(err, ErrKilled) {
		t.Fatalf("the killed call got %v", err)
	}
	if p := panicked(func() { got := m.MemorySize(); _ = got }); p != nil {
		t.Fatalf("MemorySize after a kill panicked with %v", p)
	}
	if got := m.MemorySize(); got == 0 {
		t.Errorf("MemorySize after a kill is 0, it was %d before", before)
	}
	if p := panicked(func() { got := m.GCThreshold(); _ = got }); p != nil {
		t.Fatalf("GCThreshold after a kill panicked with %v", p)
	}
	if got := m.GCThreshold(); got != threshold {
		t.Errorf("GCThreshold after a kill is %d, want the %d it had", got, threshold)
	}
}
