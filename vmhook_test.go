// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package quickjs // import "github.com/liustack/quickjs"

import (
	"fmt"
	"runtime"
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
		setup func(m *VM) (stop func())
	}{
		{"timeout", func(m *VM) func() { m.SetEvalTimeout(50 * time.Millisecond); return func() {} }},
		{"interrupt", func(m *VM) func() { return interruptAfter(m, 50*time.Millisecond) }},
	} {
		t.Run(c.name, func(t *testing.T) {
			m := newTestVM(t)
			defineBuiltins(t, m, loopBuiltin)
			if _, err := m.Eval(`globalThis.seen = []; globalThis.fail = () => { seen.push(loop()) }`, EvalGlobal); err != nil {
				t.Fatal(err)
			}
			stop := c.setup(m)
			start := time.Now()
			err, seen := runUncatchableThen(t, m, `try { fail() } catch (e) { seen.push('catch') } finally { seen.push('finally') }`, stop)
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

// charsBuiltin returns how vmhook.StringLen and vmhook.Chars see its argument:
// "len,tag,latin1|utf16,chars", tag being "rope" when it is a rope, and the
// characters of the view decoded, which the test compares with the string.
var charsBuiltin = NewBuiltin("chars", 1, func(a Args) (Value, error) {
	n, ok := vmhook.StringLen(a, 0)
	if !ok {
		return a.VM().NewString("none")
	}
	kind := "flat"
	if tag(a.arg(0)) == tagStringRope {
		kind = "rope"
	}
	b, u, release, ok := vmhook.Chars(a, 0)
	if !ok {
		return a.VM().NewString("unread")
	}
	defer release()
	var s []rune
	view := "latin1"
	if u != nil {
		view = "utf16"
		for _, c := range u {
			s = append(s, rune(c))
		}
	} else {
		for _, c := range b {
			s = append(s, rune(c))
		}
	}
	got := len(b) + len(u)
	return a.VM().NewString(fmt.Sprintf("%d,%d,%s,%s,%s", n, got, kind, view, string(s)))
})

// vmhook.StringLen reads the length of a string argument, of a rope too,
// without joining it, and vmhook.Chars its characters, joining a rope first,
// so that a Go function reads a long argument in place.
func TestVMHookChars(t *testing.T) {
	m := newTestVM(t)
	defineBuiltins(t, m, charsBuiltin)
	for _, c := range []struct{ js, want string }{
		{`chars(1)`, "none"},
		{`chars()`, "none"},
		{`chars('')`, "0,0,flat,latin1,"},
		{`chars('abc')`, "3,3,flat,latin1,abc"},
		{`chars('éÿ')`, "2,2,flat,latin1,éÿ"},
		{`chars('aĀ')`, "2,2,flat,utf16,aĀ"},
		{`(() => { const s = 'x'.repeat(5000) + 'y'.repeat(5000); return chars(s).slice(0, 20) })()`, "10000,10000,rope,lat"},
		{`(() => { const s = 'x'.repeat(5000) + 'Ā'.repeat(5000); return chars(s).slice(0, 20) })()`, "10000,10000,rope,utf"},
	} {
		v, err := m.Eval(c.js, EvalGlobal)
		if err != nil {
			t.Fatalf("%s: %v", c.js, err)
		}
		if v != c.want {
			t.Errorf("%s = %q, want %q", c.js, v, c.want)
		}
	}
	// The characters of a rope, joined, are the string's.
	v, err := m.Eval(`(() => { const s = 'ab'.repeat(3000) + 'cĀ'.repeat(3000); return chars(s) === '12000,12000,rope,utf16,' + s })()`, EvalGlobal)
	if err != nil || v != true {
		t.Errorf("the joined rope: %v %v", v, err)
	}
}

// lenBuiltin returns Args.stringLen of its argument, "rope" or "flat", and
// what 1,000 calls of stringLen allocated in Go.
var lenBuiltin = NewBuiltin("strlen", 1, func(a Args) (Value, error) {
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	n, ok := 0, false
	for range 1000 {
		n, ok = a.stringLen(0)
	}
	runtime.ReadMemStats(&after)
	kind := "flat"
	if tag(a.arg(0)) == tagStringRope {
		kind = "rope"
	}
	return a.VM().NewString(fmt.Sprintf("%d,%v,%s,%d", n, ok, kind, after.TotalAlloc-before.TotalAlloc))
})

// Args.stringLen, which vmhook.StringLen calls, reads the length of the
// longest string javascript makes, a rope of 2^30-1 units, without joining
// it, and allocates nothing.
func TestStringLenRope(t *testing.T) {
	m := newTestVM(t)
	defineBuiltins(t, m, lenBuiltin)
	if _, err := m.Eval(`var r = 'x'.repeat((1 << 20) - 1); for (let i = 0; i < 10; i++) r = r + r; r = r + 'x'.repeat(1023); 0`, EvalGlobal); err != nil {
		t.Fatal(err)
	}
	before := m.MemorySize()
	v, err := m.Eval(`strlen(r)`, EvalGlobal)
	if err != nil {
		t.Fatal(err)
	}
	if want := fmt.Sprintf("%d,true,rope,0", 1<<30-1); v != want {
		t.Errorf("strlen(r) = %v, want %s", v, want)
	}
	if grown := int64(m.MemorySize()) - int64(before); grown > 1<<20 {
		t.Errorf("the VM took %d bytes more to read the length", grown)
	}
	if v, err := m.Eval(`strlen(1) + '|' + strlen('ab')`, EvalGlobal); err != nil || v != "0,false,flat,0|2,true,flat,0" {
		t.Errorf("strlen of a number and of 'ab': %v, %v", v, err)
	}
}
