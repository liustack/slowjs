// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package quickjs // import "github.com/liustack/quickjs"

import (
	"testing"
	"time"
)

// A built-in that only records an error (an async function that fails before
// its first await, or in a job) returns as if it had not failed, while the
// out of memory error stays pending in the context. Each way into javascript
// has to notice it and return the error itself, not a result, and leave
// nothing pending, so that the limit still holds on the next call.
func TestOutOfMemoryPendingAfterSuccess(t *testing.T) {
	const setup = growForever + `; globalThis.seen = []; globalThis.run = async () => { try { fail() } catch (e) { seen.push('catch') } finally { seen.push('finally') } }`
	calls := []struct {
		name string
		do   func(t *testing.T, m *VM) error
	}{
		{"Eval", func(t *testing.T, m *VM) error {
			_, err := m.Eval(`run()`, EvalGlobal)
			return err
		}},
		{"EvalValue", func(t *testing.T, m *VM) error {
			v, err := m.EvalValue(`run()`, EvalGlobal)
			if err == nil {
				v.Free()
			}
			return err
		}},
		{"EvalThis", func(t *testing.T, m *VM) error {
			this, err := m.EvalValue(`({})`, EvalGlobal)
			if err != nil {
				t.Fatal(err)
			}
			_, err = m.EvalThis(this, `run()`, EvalGlobal)
			return err
		}},
		{"EvalBytecode", func(t *testing.T, m *VM) error {
			code, err := m.Compile(`run()`, EvalGlobal)
			if err != nil {
				t.Fatal(err)
			}
			m.SetMemoryLimit(uintptr(vmMemory(m).Fmalloc_size) + 1<<20)
			_, err = m.EvalBytecode(code)
			return err
		}},
		{"Call", func(t *testing.T, m *VM) error {
			_, err := m.Call(`run`)
			return err
		}},
		{"CallValue", func(t *testing.T, m *VM) error {
			v, err := m.CallValue(`run`)
			if err == nil {
				v.Free()
			}
			return err
		}},
		{"Value.Call", func(t *testing.T, m *VM) error {
			f, err := m.EvalValue(`run`, EvalGlobal)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Free()
			m.SetMemoryLimit(uintptr(vmMemory(m).Fmalloc_size) + 1<<20)
			_, err = f.Call(Value{})
			return err
		}},
		{"job after await", func(t *testing.T, m *VM) error {
			if _, err := m.Eval(`(async () => { await 0; run() })()`, EvalGlobal); err != nil {
				t.Fatal(err)
			}
			m.SetMemoryLimit(uintptr(vmMemory(m).Fmalloc_size) + 1<<20)
			_, err := m.ExecutePendingJobs()
			return err
		}},
		{"job of then", func(t *testing.T, m *VM) error {
			if _, err := m.Eval(`Promise.resolve().then(run)`, EvalGlobal); err != nil {
				t.Fatal(err)
			}
			m.SetMemoryLimit(uintptr(vmMemory(m).Fmalloc_size) + 1<<20)
			_, err := m.ExecutePendingJobs()
			return err
		}},
	}
	for _, c := range calls {
		t.Run(c.name, func(t *testing.T) {
			m := newTestVM(t)
			if _, err := m.Eval(setup, EvalGlobal); err != nil {
				t.Fatal(err)
			}
			// Twice: the second run shows that the first left nothing
			// behind that changes what running out of memory does.
			for i := 1; i <= 2; i++ {
				m.SetMemoryLimit(uintptr(vmMemory(m).Fmalloc_size) + 1<<20)
				err := c.do(t, m)
				wantError(t, err, "InternalError", "out of memory")
				wantKind(t, err, outOfMemory)
				wantNoPendingException(t, m)

				m.SetMemoryLimit(^uintptr(0))
				if _, err := m.ExecutePendingJobs(); err != nil {
					t.Fatalf("run %d: later jobs: %v", i, err)
				}
				v, err := m.Eval(`seen.join() + ':' + (1 + 1)`, EvalGlobal)
				if err != nil || v != ":2" {
					t.Fatalf("run %d: after the error got %v, %v: page code saw the error or the VM is not usable", i, v, err)
				}
			}
		})
	}
}

// A built-in that records an uncatchable error and returns, such as a promise
// resolve function whose thenable's then getter runs out of memory, leaves
// the error pending while the script goes on. A loop with no call in it then
// ends at its next backward jump with that error, with no timeout set: the
// memory limit holds, and a Go function that panicked ends the call.
func TestUncatchablePendingEndsLoop(t *testing.T) {
	const bound = 5 * time.Second
	cases := []struct{ name, js string }{
		{"resolve with a thenable getter", `let res; new Promise(r => { res = r }); res({ get then() { fail() } }); for (;;) {}`},
		{"async first step", `(async () => { fail() })(); for (;;) {}`},
		{"async generator", `(async function* () { fail() })().next(); for (;;) {}`},
		{"dynamic import", `import({ toString() { fail() } }); for (;;) {}`},
	}
	for _, c := range cases {
		for _, cause := range []string{"out of memory", "Go panic"} {
			t.Run(c.name+"/"+cause, func(t *testing.T) {
				m := newTestVM(t)
				fail := growForever
				if cause == "Go panic" {
					mustHost(t, m, "boom", func([]any) (any, error) { panic("boom") })
					fail = `globalThis.fail = () => boom()`
				}
				if _, err := m.Eval(fail, EvalGlobal); err != nil {
					t.Fatal(err)
				}

				// The watchdog has returned before the VM is used again, so its
				// Interrupt cannot land in a later call.
				done, stopped := make(chan struct{}), make(chan bool, 1)
				go func() {
					select {
					case <-done:
						stopped <- false
					case <-time.After(bound):
						m.Interrupt()
						stopped <- true
					}
				}()
				var err error
				var p any
				if cause == "Go panic" {
					p = evalRecover(m, c.js)
				} else {
					m.SetMemoryLimit(uintptr(vmMemory(m).Fmalloc_size) + 1<<20)
					_, err = m.Eval(c.js, EvalGlobal)
					m.SetMemoryLimit(^uintptr(0))
				}
				close(done)
				if <-stopped {
					t.Fatalf("the loop ran on for %v, until Interrupt", bound)
				}
				if cause == "Go panic" {
					if p != "boom" {
						t.Fatalf("got panic %v, want boom", p)
					}
				} else {
					wantError(t, err, "InternalError", "out of memory")
					wantKind(t, err, outOfMemory)
				}
				wantNoPendingException(t, m)
				if v, err := m.Eval(`let n = 0; for (let k = 0; k < 1000; k++) n += k; String(n)`, EvalGlobal); err != nil || v != "499500" {
					t.Fatalf("a loop after the error: %v, %v", v, err)
				}
			})
		}
	}
}
