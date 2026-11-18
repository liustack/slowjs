// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package slowjs // import "github.com/liustack/slowjs"

// What docs/api.md says a host may call on a VM after a call ended with an
// uncatchable error: after a stop (the eval timeout, Interrupt) or running
// out of memory every method works and Close frees what the VM holds, while
// async work that was in flight may never settle. After a kill only Close,
// Interrupt, MemorySize, MemoryFree and GCThreshold.

import (
	"errors"
	"fmt"
	"testing"
	"time"
)

// afterStopSetup leaves async work in flight: two requests on an async
// generator whose body runs in a job, and a closure that keeps a value of
// its frame.
const afterStopSetup = `
globalThis.g = (async function* () {
  let keep = { big: new Array(1000).fill(1) };
  globalThis.held = () => keep;
  await 0;
  %s
})();
globalThis.p1 = g.next();
globalThis.p2 = g.next();
`

// wholeAfterStop calls what docs/api.md says works after a stop or running
// out of memory, then closes m.
func wholeAfterStop(t *testing.T, m *VM, what string) {
	t.Helper()
	v, err := m.EvalValue(`({ a: 1 })`, EvalGlobal)
	if err != nil {
		t.Fatalf("%s: EvalValue after the stop: %v", what, err)
	}
	if got, err := v.Any(); err != nil || got == nil {
		t.Errorf("%s: Value.Any after the stop: %v, %v", what, got, err)
	}
	v.Free()
	if u := m.MemoryUsage(); u.MemoryUsedSize <= 0 {
		t.Errorf("%s: MemoryUsage after the stop has %d bytes in use", what, u.MemoryUsedSize)
	}
	if m.MemorySize() == 0 || m.MemoryFree() > m.MemorySize() {
		t.Errorf("%s: MemorySize %d, MemoryFree %d after the stop", what, m.MemorySize(), m.MemoryFree())
	}
	m.RunGC()
	m.RunYoungGC()
	if _, err := m.ExecutePendingJobs(); err != nil {
		t.Errorf("%s: ExecutePendingJobs after the stop: %v", what, err)
	}
	if got, err := m.Eval(`typeof g.next() + 2`, EvalGlobal); err != nil || got != "object2" {
		t.Errorf("%s: Eval after the stop: %v, %v", what, got, err)
	}
	h := m.h
	if err := m.Close(); err != nil {
		t.Fatalf("%s: Close: %v", what, err)
	}
	if n := h.Handles(); n != 0 {
		t.Errorf("%s: %d Go functions held after Close", what, n)
	}
}

func TestAfterStopWhatIsSafe(t *testing.T) {
	t.Run("stop", func(t *testing.T) {
		m, err := NewVM()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := m.Eval(fmt.Sprintf(afterStopSetup, `for (;;) {}`), EvalGlobal); err != nil {
			t.Fatal(err)
		}
		if err := m.SetEvalTimeout(20 * time.Millisecond); err != nil {
			t.Fatal(err)
		}
		_, err = m.ExecutePendingJobs()
		var e *Error
		if !errors.As(err, &e) || !e.Uncatchable || !e.Interrupted || errors.Is(err, ErrKilled) {
			t.Fatalf("the jobs ended with %v, want a stop that is no kill", err)
		}
		if err := m.SetEvalTimeout(0); err != nil {
			t.Fatal(err)
		}
		wholeAfterStop(t, m, "stop")
	})
	t.Run("out of memory", func(t *testing.T) {
		m, err := NewVM()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := m.Eval(fmt.Sprintf(afterStopSetup, `const a = []; for (;;) a.push(new Array(1000).fill(2));`), EvalGlobal); err != nil {
			t.Fatal(err)
		}
		m.SetMemoryLimit(m.MemorySize() + 8<<20)
		_, err = m.ExecutePendingJobs()
		var e *Error
		if !errors.As(err, &e) || !e.Uncatchable || !e.OutOfMemory {
			t.Fatalf("the jobs ended with %v, want out of memory", err)
		}
		wholeAfterStop(t, m, "out of memory")
	})
	t.Run("kill", func(t *testing.T) {
		m, err := NewVM()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := m.Eval(fmt.Sprintf(afterStopSetup, `1`), EvalGlobal); err != nil {
			t.Fatal(err)
		}
		if _, err := killedVMWith(t, m, nestedEntries["Eval"]); !errors.Is(err, ErrKilled) {
			t.Fatalf("the killed call got %v", err)
		}
		for name, call := range map[string]func(){
			"Interrupt":   func() { m.Interrupt() },
			"MemorySize":  func() { _ = m.MemorySize() },
			"MemoryFree":  func() { _ = m.MemoryFree() },
			"GCThreshold": func() { _ = m.GCThreshold() },
		} {
			if p := panicked(call); p != nil {
				t.Errorf("%s after a kill panicked with %v", name, p)
			}
		}
		if _, err := m.Eval(`1`, EvalGlobal); !errors.Is(err, ErrKilled) {
			t.Errorf("Eval after a kill: %v, want an error that matches ErrKilled", err)
		}
		if p := panicked(func() { m.MemoryUsage() }); p != brokenPanic {
			t.Errorf("MemoryUsage after a kill panicked with %v, want %q", p, brokenPanic)
		}
		h := m.h
		if err := m.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
		if n := h.Handles(); n != 0 {
			t.Errorf("%d Go functions held after Close", n)
		}
	})
}
