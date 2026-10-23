// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package slowjs // import "github.com/liustack/slowjs"

import (
	"testing"
	"time"
	"unsafe"

	lib "github.com/liustack/slowjs/lib"
)

// evalRecover evaluates js and returns what it panicked with.
func evalRecover(m *VM, js string) (p any) {
	defer func() { p = recover() }()
	m.Eval(js, EvalGlobal)
	return nil
}

// wantTimeoutWorks checks that the eval timeout stops a script that runs
// forever.
func wantTimeoutWorks(t *testing.T, m *VM) {
	t.Helper()
	m.SetEvalTimeout(50 * time.Millisecond)
	defer m.SetEvalTimeout(0)
	done := make(chan error, 1)
	go func() {
		_, err := m.Eval(`for (;;);`, EvalGlobal)
		done <- err
	}()
	select {
	case err := <-done:
		wantError(t, err, "InternalError", "interrupted")
		wantKind(t, err, interrupted)
	case <-time.After(5 * time.Second):
		t.Fatal("the timeout did not stop the script")
	}
}

// A Go function that panics: the panic reaches the caller of Eval, page code
// does not see it on the way (no catch or finally runs), and the VM works
// afterwards, its eval timeout too. The panic used to unwind through the
// engine and leave it pointing at stack frames that were gone, and the next
// error the engine made walked them forever.
func TestHostPanic(t *testing.T) {
	for _, c := range uncatchableCases {
		t.Run(c.name, func(t *testing.T) {
			m := newTestVM(t)
			if err := m.RegisterHostFunc("boom", func([]any) (any, error) { panic("boom") }); err != nil {
				t.Fatal(err)
			}
			if _, err := m.Eval(`globalThis.seen = []; globalThis.fail = () => { boom(); seen.push('after boom') }`, EvalGlobal); err != nil {
				t.Fatal(err)
			}

			p := evalRecover(m, c.js)
			if p == nil {
				// The panic came in a job.
				func() {
					defer func() { p = recover() }()
					m.ExecutePendingJobs()
				}()
			}
			if p != "boom" {
				t.Fatalf("recovered %v, want the Go function's panic", p)
			}
			if f := (*lib.TJSRuntime)(unsafe.Pointer(m.runtime.cRuntime)).Fcurrent_stack_frame; f != 0 {
				t.Fatalf("the engine still has a stack frame: %#x", f)
			}
			wantNoPendingException(t, m)
			if seen, err := m.Eval(`seen.join()`, EvalGlobal); err != nil || seen != "" {
				t.Fatalf("page code saw the panic: %v, %v", seen, err)
			}
			wantTimeoutWorks(t, m)
		})
	}
}

// A panic in a Go function that javascript called from inside another Go
// function goes through the outer Go function, as a Go panic does, and the
// outer function can recover it. The javascript between them still ends.
func TestHostPanicNested(t *testing.T) {
	m := newTestVM(t)
	if err := m.RegisterHostFunc("boom", func([]any) (any, error) { panic("boom") }); err != nil {
		t.Fatal(err)
	}
	var inner any
	var again error
	if err := m.RegisterHostFunc("outer", func([]any) (any, error) {
		inner = evalRecover(m, `try { boom() } finally { globalThis.finished = true }`)
		// The javascript the panic ended is still ending: more fails at once.
		_, again = m.Eval(`globalThis.again = true`, EvalGlobal)
		return "outer done", nil
	}); err != nil {
		t.Fatal(err)
	}

	_, err := m.Eval(`try { outer() } catch (e) { globalThis.caught = true }`, EvalGlobal)
	if inner != "boom" {
		t.Fatalf("outer recovered %v, want the inner panic", inner)
	}
	wantError(t, again, "InternalError", "panic in Go function boom: boom")
	wantKind(t, again, goPanic)
	wantError(t, err, "InternalError", "panic in Go function boom: boom")
	wantKind(t, err, goPanic)
	if v, err := m.Eval(`typeof finished + typeof caught + typeof again`, EvalGlobal); err != nil || v != "undefinedundefinedundefined" {
		t.Fatalf("page code saw the panic: %v, %v", v, err)
	}
	wantNoPendingException(t, m)
	wantTimeoutWorks(t, m)
}

// A panic in the module loader reaches the caller the same way.
func TestModuleLoaderPanic(t *testing.T) {
	m := newTestVM(t)
	m.SetModuleLoader(func(*VM, string) (string, error) { panic("boom") }, nil)
	if p := evalRecover(m, `import('m'); undefined`); p != nil {
		t.Fatalf("Eval panicked with %v", p)
	}
	var p any
	func() {
		defer func() { p = recover() }()
		m.ExecutePendingJobs()
	}()
	if p != "boom" {
		t.Fatalf("recovered %v, want the loader's panic", p)
	}
	wantNoPendingException(t, m)
	wantTimeoutWorks(t, m)
}
