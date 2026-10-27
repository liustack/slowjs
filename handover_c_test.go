// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build slowjs_cengine && linux && amd64

package slowjs // import "github.com/liustack/slowjs"

import (
	"fmt"
	"runtime"
	"testing"
)

// The handover of the shadow stack to a Go function that javascript calls:
// the calls it makes into the engine start above the frames of the call, so
// that the frames below keep what they hold, and the VM's shadow pointer is
// put back when it ends, by a return, a panic or a Goexit.

// handoverJS is the javascript of one level: its locals hold sentinels, in
// the frame the interpreter takes from the shadow stack by a dynamic alloca,
// and burn(n) fills the stack above with frames of its own before reenter
// calls Go. Each level checks its sentinels after the calls.
func handoverJS(level, below int) string {
	return fmt.Sprintf(`(() => {
		const s1 = %[1]d * 1000003, s2 = 'level %[1]d', s3 = [%[1]d, %[1]d + 1], s4 = %[1]d + 0.5
		const r = burn(64) + reenter(%[2]d) + burn(64)
		if (s1 !== %[1]d * 1000003 || s2 !== 'level %[1]d' || s3[0] !== %[1]d || s3[1] !== %[1]d + 1 || s4 !== %[1]d + 0.5)
			throw new Error('level %[1]d lost its locals: ' + [s1, s2, s3, s4])
		return r
	})()`, level, below)
}

// burn is a recursion whose frames each hold a few locals.
const burnJS = `globalThis.burn = n => {
	const a = n * 7, b = 'x' + n, c = [n, a]
	const r = n > 0 ? burn(n - 1) : 0
	return r + c[0] + c[1] - a + b.length - String(n).length - 1 - n
}`

// handover is what the Go function of a test does at the innermost level.
var handover func(m *VM)

var reenterBuiltin = NewBuiltin("reenter", 1, func(a Args) (Value, error) {
	m := a.VM()
	if m.sp <= m.start || m.sp%16 != 0 || m.sp >= m.end {
		return Value{}, fmt.Errorf("a Go function got the shadow pointer %#x, the stack being %#x to %#x", m.sp, m.start, m.end)
	}
	n, _ := a.Number(0)
	sp := m.sp
	if n > 0 {
		// The level below, in the same VM, whose frames start above sp.
		v, err := m.EvalValue(handoverJS(int(n)+10, int(n)-1), EvalGlobal)
		if err != nil {
			return Value{}, err
		}
		v.Free()
	} else {
		runtime.GC()
		if handover != nil {
			handover(m)
		}
	}
	if m.sp != sp {
		return Value{}, fmt.Errorf("the shadow pointer of the Go function was %#x and is %#x after it called the engine", sp, m.sp)
	}
	return m.NewInt(0), nil
})

func newHandoverVM(t *testing.T) *VM {
	t.Helper()
	m := newTestVM(t)
	defineBuiltin(t, m, "reenter", reenterBuiltin)
	if _, err := m.Eval(burnJS, EvalGlobal); err != nil {
		t.Fatal(err)
	}
	return m
}

// wantIdle checks that m is out of the engine, with its shadow pointer back
// at its start.
func wantIdle(t *testing.T, m *VM) {
	t.Helper()
	if m.sp != m.start || m.entered != 0 || m.host != 0 {
		t.Fatalf("after the call: sp %#x (start %#x), entered %d, host %d", m.sp, m.start, m.entered, m.host)
	}
}

func TestHandover(t *testing.T) {
	m := newHandoverVM(t)
	defer func() { handover = nil }()

	// Two levels of the same VM below the outer one, with a GC of Go at
	// the innermost.
	if _, err := m.Eval(handoverJS(1, 2), EvalGlobal); err != nil {
		t.Fatal(err)
	}
	wantIdle(t, m)

	// A panic at the innermost level goes on where Go called javascript,
	// and the VM goes on.
	handover = func(*VM) { panic("handover panic") }
	if p := evalRecover(m, handoverJS(1, 2)); p != "handover panic" {
		t.Fatalf("panicked with %v", p)
	}
	wantIdle(t, m)
	handover = nil
	if _, err := m.Eval(handoverJS(1, 2), EvalGlobal); err != nil {
		t.Fatalf("after the panic: %v", err)
	}
	wantIdle(t, m)
}

// TestHandoverGoexit: runtime.Goexit at the innermost level goes up through
// the engine's frames: the shadow pointer is back at its start, and the VM is
// good only for Close.
func TestHandoverGoexit(t *testing.T) {
	m := newHandoverVM(t)
	defer func() { handover = nil }()
	handover = func(*VM) { runtime.Goexit() }
	done := make(chan struct{})
	go func() {
		defer close(done)
		m.Eval(handoverJS(1, 2), EvalGlobal)
		t.Error("Eval returned after a Goexit")
	}()
	<-done
	wantIdle(t, m)
	if p := evalRecover(m, `1`); p != brokenPanic {
		t.Fatalf("Eval after the Goexit panicked with %v, want %q", p, brokenPanic)
	}
}
