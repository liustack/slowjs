// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package quickjs // import "github.com/liustack/quickjs"

import (
	"fmt"
	"testing"
	"time"

	"github.com/liustack/quickjs/internal/vmhook"
)

// loopBuiltin is a Builtin that works in Go until vmhook.Stop says to stop,
// or for 10 s.
var loopBuiltin = NewBuiltin("loop", 0, func(a Args) (Value, error) {
	end := time.Now().Add(10 * time.Second)
	for time.Now().Before(end) {
		for range 1000 {
		}
		if vmhook.Stop(a.VM()) {
			return a.VM().NewString("stopped")
		}
	}
	return UndefinedValue, nil
})

// roomBuiltin returns vmhook.Room.
var roomBuiltin = NewBuiltin("room", 0, func(a Args) (Value, error) {
	return a.VM().NewFloat64(float64(vmhook.Room(a.VM()))), nil
})

// oomBuiltin throws vmhook.OutOfMemory and returns a string, which the call
// drops.
var oomBuiltin = NewBuiltin("oom", 0, func(a Args) (Value, error) {
	vmhook.OutOfMemory(a.VM())
	return a.VM().NewString("dropped")
})

func defineBuiltins(t *testing.T, m *VM, bs ...*Builtin) {
	t.Helper()
	for _, b := range bs {
		defineBuiltin(t, m, b.name, b)
	}
}

// A Go function that calls vmhook.Stop ends the call at the eval timeout and
// at Interrupt, with the uncatchable interrupt the engine's polls throw: no
// catch or finally of the page runs, and its result is dropped.
func TestVMHookStop(t *testing.T) {
	for _, c := range []struct {
		name  string
		setup func(m *VM)
	}{
		{"timeout", func(m *VM) { m.SetEvalTimeout(50 * time.Millisecond) }},
		{"interrupt", func(m *VM) { time.AfterFunc(50*time.Millisecond, m.Interrupt) }},
	} {
		t.Run(c.name, func(t *testing.T) {
			m := newTestVM(t)
			defineBuiltins(t, m, loopBuiltin)
			if _, err := m.Eval(`globalThis.seen = []; globalThis.fail = () => { seen.push(loop()) }`, EvalGlobal); err != nil {
				t.Fatal(err)
			}
			c.setup(m)
			start := time.Now()
			err, seen := runUncatchable(t, m, `try { fail() } catch (e) { seen.push('catch') } finally { seen.push('finally') }`)
			if d := time.Since(start); d > 2*time.Second {
				t.Errorf("the call took %v", d)
			}
			wantError(t, err, "InternalError", "interrupted")
			wantKind(t, err, interrupted)
			if seen != "" {
				t.Errorf("page code saw %q", seen)
			}
			wantTimeoutWorks(t, m)
		})
	}
}

// vmhook.Stop is false while nothing asked the call to stop, and the
// Builtin's result goes back.
func TestVMHookStopFalse(t *testing.T) {
	m := newTestVM(t)
	defineBuiltins(t, m, NewBuiltin("check", 0, func(a Args) (Value, error) {
		return a.VM().NewInt(map[bool]int{false: 0, true: 1}[vmhook.Stop(a.VM())]), nil
	}))
	m.SetEvalTimeout(time.Hour)
	if v, err := m.Eval(`check()`, EvalGlobal); err != nil || v != 0 {
		t.Fatalf("got %v, %v, want 0", v, err)
	}
}

// vmhook.Room is what the memory limit leaves, -1 for none.
func TestVMHookRoom(t *testing.T) {
	m := newTestVM(t)
	defineBuiltins(t, m, roomBuiltin)
	if v, err := m.Eval(`room()`, EvalGlobal); err != nil || fmt.Sprint(v) != "-1" {
		t.Fatalf("without a limit: got %v, %v, want -1", v, err)
	}
	const limit = 16 << 20
	m.SetMemoryLimit(limit)
	v, err := m.Eval(`room()`, EvalGlobal)
	if err != nil {
		t.Fatal(err)
	}
	if n, ok := toFloat(v); !ok || n <= 0 || n >= limit || n < float64(limit-int(m.MemorySize())-1<<20) {
		t.Fatalf("with a limit of %d and %d taken: got %v", limit, m.MemorySize(), v)
	}
	m.SetMemoryLimit(^uintptr(0))
	if v, err := m.Eval(`room()`, EvalGlobal); err != nil || fmt.Sprint(v) != "-1" {
		t.Fatalf("with the limit taken off: got %v, %v, want -1", v, err)
	}
}

// vmhook.OutOfMemory ends the call as running out of memory in the engine
// does.
func TestVMHookOutOfMemory(t *testing.T) {
	m := newTestVM(t)
	defineBuiltins(t, m, oomBuiltin)
	if _, err := m.Eval(`globalThis.seen = []; globalThis.fail = () => { seen.push(oom()) }`, EvalGlobal); err != nil {
		t.Fatal(err)
	}
	err, seen := runUncatchable(t, m, `try { fail() } catch (e) { seen.push('catch') } finally { seen.push('finally') }`)
	wantError(t, err, "InternalError", "out of memory")
	wantKind(t, err, outOfMemory)
	if seen != "" {
		t.Errorf("page code saw %q", seen)
	}
}

// The functions of vmhook are for a Go function javascript called.
func TestVMHookOutsideCall(t *testing.T) {
	m := newTestVM(t)
	defer func() {
		if recover() == nil {
			t.Fatal("vmhook.Stop outside a call did not panic")
		}
	}()
	vmhook.Stop(m)
}

// toFloat is v, a number javascript returned, as a float64.
func toFloat(v any) (float64, bool) {
	switch v := v.(type) {
	case int:
		return float64(v), true
	case float64:
		return v, true
	}
	return 0, false
}
