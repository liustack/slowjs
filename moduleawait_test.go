// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package slowjs

import (
	"errors"
	"testing"
	"time"
)

// moduleBytecode compiles the module src in a VM of its own. A module made
// from a string has the same name as any other, so evaluating it in the VM
// that compiled it would find the compiled module, not a new one.
func moduleBytecode(t *testing.T, src string) []byte {
	t.Helper()
	c, err := NewVM()
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	code, err := c.Compile(src, EvalModule)
	if err != nil {
		t.Fatal(err)
	}

	return code
}

// wantEval checks that javascript evaluates to want.
func wantEval(t *testing.T, m *VM, javascript string, want any) {
	t.Helper()
	if v, err := m.Eval(javascript, EvalGlobal); err != nil || v != want {
		t.Fatalf("%s: got %v, %v, want %v", javascript, v, err, want)
	}
}

// throwingJob is javascript that queues a job that throws, catchable, while
// it runs: a promise whose species gives then a resolve function that
// throws, which the job calls with the result of the handler.
const throwingJob = `
class P extends Promise {
	static get [Symbol.species]() {
		return function(executor) { executor(() => { throw new TypeError('resolve throws') }, () => {}) }
	}
}
P.resolve(1).then(() => {});
`

// The tests here also run under the memgrind allocator (TestMemgrind2).

// TestModuleAwaitPending evaluates a module whose top-level await waits for
// a promise nothing resolves. The job queue runs dry with the module still
// pending: EvalBytecode returns errAwaitPending, and the VM still works.
func TestModuleAwaitPending(t *testing.T) {
	code := moduleBytecode(t, "await new Promise(() => {})\n")
	m := newTestVM(t)
	if _, err := m.EvalBytecode(code); !errors.Is(err, errAwaitPending) {
		t.Fatalf("got %v, want errAwaitPending", err)
	}

	wantEval(t, m, "1 + 1", 2)
}

// TestModuleAwaitGoResolves evaluates a module that waits for a promise a Go
// function returned. EvalBytecode returns errAwaitPending, and once Go
// resolves it, ExecutePendingJobs runs the rest of the module.
func TestModuleAwaitGoResolves(t *testing.T) {
	code := moduleBytecode(t, "globalThis.got = await later()\n")
	m := newTestVM(t)
	var pc *PromiseCapability
	if err := m.RegisterFunc("later", func() Value {
		var err error
		if pc, err = m.NewPromiseCapability(); err != nil {
			t.Fatal(err)
		}
		return pc.Promise
	}, false); err != nil {
		t.Fatal(err)
	}

	if _, err := m.EvalBytecode(code); !errors.Is(err, errAwaitPending) {
		t.Fatalf("got %v, want errAwaitPending", err)
	}

	if _, err := pc.Resolve.Call(UndefinedValue, "from Go"); err != nil {
		t.Fatal(err)
	}
	pc.Resolve.Free()
	pc.Reject.Free()
	if _, err := m.ExecutePendingJobs(); err != nil {
		t.Fatal(err)
	}

	wantEval(t, m, "got", "from Go")
}

// TestModuleAwaitRunsJobs evaluates a module that waits on jobs it queues
// itself: EvalBytecode runs them until the module is done.
func TestModuleAwaitRunsJobs(t *testing.T) {
	code := moduleBytecode(t, `
await null;
globalThis.done = await new Promise(r => Promise.resolve().then(() => r('done')));
`)
	m := newTestVM(t)
	if _, err := m.EvalBytecode(code); err != nil {
		t.Fatal(err)
	}

	wantEval(t, m, "done", "done")
}

// TestModuleAwaitRejects evaluates a module whose await throws: the
// rejection is the error.
func TestModuleAwaitRejects(t *testing.T) {
	code := moduleBytecode(t, "await null; throw new TypeError('late')\n")
	m := newTestVM(t)
	_, err := m.EvalBytecode(code)
	wantError(t, err, "TypeError", "late")
}

// TestModuleAwaitJobThrows evaluates a module next to a job that throws. The
// error does not stop the wait: the module runs to its end, and EvalBytecode
// returns the job's error, as ExecutePendingJobs would. No job is left.
func TestModuleAwaitJobThrows(t *testing.T) {
	code := moduleBytecode(t, throwingJob+"globalThis.steps = []; await null; steps.push('a'); await null; steps.push('b')\n")
	m := newTestVM(t)
	_, err := m.EvalBytecode(code)
	wantError(t, err, "TypeError", "resolve throws")
	wantKind(t, err, thrown)
	wantEval(t, m, "steps.join()", "a,b")
	if n, err := m.ExecutePendingJobs(); n != 0 || err != nil {
		t.Fatalf("jobs left: %d, %v", n, err)
	}
}

// TestModuleAwaitRejectionWins evaluates a module that rejects after a job
// threw: the module's own error is the one returned, as Node reports it.
func TestModuleAwaitRejectionWins(t *testing.T) {
	code := moduleBytecode(t, throwingJob+"await null; await null; throw new RangeError('module')\n")
	m := newTestVM(t)
	_, err := m.EvalBytecode(code)
	wantError(t, err, "RangeError", "module")
}

// TestModuleAwaitJobThrowsPending evaluates a module that is still pending
// when the queue runs dry, after a job threw: the error is both.
func TestModuleAwaitJobThrowsPending(t *testing.T) {
	code := moduleBytecode(t, throwingJob+"await new Promise(() => {})\n")
	m := newTestVM(t)
	_, err := m.EvalBytecode(code)
	if !errors.Is(err, errAwaitPending) {
		t.Fatalf("got %v, want errAwaitPending", err)
	}
	wantError(t, err, "TypeError", "resolve throws")
}

// TestModuleAwaitTimeout runs into the eval timeout after the module's first
// await. The job that runs the rest of the module is interrupted, and
// EvalBytecode returns the interrupt instead of printing it and going on.
func TestModuleAwaitTimeout(t *testing.T) {
	code := moduleBytecode(t, "await null; for (;;) {}\n")
	m := newTestVM(t)
	if err := m.SetEvalTimeout(100 * time.Millisecond); err != nil {
		t.Fatal(err)
	}

	_, err := m.EvalBytecode(code)
	wantKind(t, err, interrupted)
	wantEval(t, m, "1 + 1", 2)
}

// TestModuleAwaitInterrupt calls Interrupt from a Go function that a job of
// the module calls: EvalBytecode returns the interrupt.
func TestModuleAwaitInterrupt(t *testing.T) {
	code := moduleBytecode(t, "await null; stop(); for (;;) {}\n")
	m := newTestVM(t)
	mustHost(t, m, "stop", func([]any) (any, error) {
		m.Interrupt()
		return nil, nil
	})

	_, err := m.EvalBytecode(code)
	wantKind(t, err, interrupted)
	wantEval(t, m, "1 + 1", 2)
}

// TestModuleAwaitInterruptInBuiltin interrupts a job in a resolve function
// reading a 'then' getter. The resolve function only records the error, so
// the job ends as if it had not failed, with the interrupt pending: the wait
// ends there, and the rest of the module does not run.
func TestModuleAwaitInterruptInBuiltin(t *testing.T) {
	code := moduleBytecode(t, `
Promise.resolve().then(() => ({ get then() { for (;;) {} } }));
await null; await null; await null;
globalThis.after = true;
`)
	m := newTestVM(t)
	if err := m.SetEvalTimeout(100 * time.Millisecond); err != nil {
		t.Fatal(err)
	}

	_, err := m.EvalBytecode(code)
	wantKind(t, err, interrupted)
	wantNoPendingException(t, m)
	wantEval(t, m, "typeof after", "undefined")
}

// TestModuleAwaitPanic evaluates a module whose job calls a Go function that
// panics: the panic reaches the caller of EvalBytecode with its value, and
// the VM works afterwards.
func TestModuleAwaitPanic(t *testing.T) {
	code := moduleBytecode(t, "await null; boom()\n")
	m := newTestVM(t)
	mustHost(t, m, "boom", func([]any) (any, error) { panic("boom") })

	p := func() (p any) {
		defer func() { p = recover() }()
		m.EvalBytecode(code)
		return nil
	}()
	if p != "boom" {
		t.Fatalf("recovered %v, want the Go function's panic", p)
	}
	wantNoPendingException(t, m)
	wantEval(t, m, "1 + 1", 2)
	wantTimeoutWorks(t, m)
}

// TestModuleAwaitOutOfMemory runs out of memory in a job of the module.
func TestModuleAwaitOutOfMemory(t *testing.T) {
	code := moduleBytecode(t, "await null; fail()\n")
	m := newTestVM(t)
	if _, err := m.Eval(growForever, EvalGlobal); err != nil {
		t.Fatal(err)
	}

	m.SetMemoryLimit(uintptr(vmMemory(m).Fmalloc_size) + 1<<20)
	_, err := m.EvalBytecode(code)
	m.SetMemoryLimit(^uintptr(0))
	wantError(t, err, "InternalError", "out of memory")
	wantKind(t, err, outOfMemory)
	wantNoPendingException(t, m)
	wantEval(t, m, "1 + 1", 2)
}
