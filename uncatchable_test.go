// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package slowjs // import "github.com/liustack/slowjs"

import (
	"errors"
	"strings"
	"testing"
	"time"

	lib "github.com/liustack/slowjs/lib"
)

// uncatchableCases run fail(), which throws an error page code must not be
// able to catch, inside the constructs that catch an exception or run code
// on the way out of one. What page code saw is pushed onto globalThis.seen.
var uncatchableCases = []struct{ name, js string }{
	{"catch", `try { fail() } catch (e) { seen.push('catch') }`},
	{"finally", `try { fail() } finally { seen.push('finally') }`},
	{"catch and rethrow", `try { fail() } catch (e) { seen.push('catch'); throw e }`},
	{"error boundary", `
		function render() { try { return child() } catch (e) { seen.push('boundary'); return 'fallback' } }
		function child() { try { fail() } catch (x) { if (typeof x?.then === 'function') return 'suspended'; throw x } }
		render()`},
	{"generator", `
		function* g() { try { yield 1; fail() } finally { seen.push('generator finally') } }
		try { for (const x of g()) {} } catch (e) { seen.push('catch') }`},
	{"for of return", `
		const it = { [Symbol.iterator]() { return this }, next() { return { value: 1, done: false } }, return() { seen.push('return'); return {} } }
		for (const x of it) fail()`},
	{"Array.from return", `
		const it = { [Symbol.iterator]() { return this }, next() { return { value: 1, done: false } }, return() { seen.push('return'); return {} } }
		try { Array.from(it, () => fail()) } catch (e) { seen.push('catch') }`},
	{"async function", `(async () => { try { await 0; fail() } catch (e) { seen.push('catch') } finally { seen.push('finally') } })()`},
	{"async rejection", `(async () => { await 0; fail() })().catch(() => seen.push('rejection'))`},
	{"async first step", `(async () => { fail() })().catch(() => seen.push('rejection'))`},
	{"then", `Promise.resolve().then(() => fail()).catch(() => seen.push('rejection'))`},
	{"Promise constructor", `new Promise(() => fail()).catch(() => seen.push('rejection'))`},
	{"Promise.try", `Promise.try(fail).catch(() => seen.push('rejection'))`},
	{"thenable job", `Promise.resolve({ then() { fail() } }).catch(() => seen.push('rejection'))`},
	{"thenable getter", `new Promise(r => r({ get then() { fail() } })).catch(() => seen.push('rejection'))`},
	{"Promise.all", `Promise.all({ [Symbol.iterator]() { fail() } }).catch(() => seen.push('rejection'))`},
	{"async generator", `
		(async function* () { try { await 0; fail() } finally { seen.push('finally') } })().next().catch(() => seen.push('rejection'))`},
	{"async from sync iterator", `
		(async () => { try { for await (const x of { [Symbol.iterator]() { return { next() { fail() } } } }) {} } catch (e) { seen.push('catch') } })()`},
}

// runUncatchable evaluates js and then runs the jobs it queued, and returns
// the first error, with what page code pushed onto seen.
func runUncatchable(t *testing.T, m *VM, js string) (error, string) {
	t.Helper()
	_, err := m.Eval(js, EvalGlobal)
	if err == nil {
		_, err = m.ExecutePendingJobs()
	}
	m.SetMemoryLimit(^uintptr(0))
	m.SetEvalTimeout(0)
	// Nothing page code queued may run later either.
	if _, err := m.ExecutePendingJobs(); err != nil {
		t.Logf("later jobs: %v", err)
	}
	seen, err2 := m.Eval(`seen.join()`, EvalGlobal)
	if err2 != nil {
		t.Fatalf("reading seen: %v", err2)
	}
	return err, seen.(string)
}

// What ended a call, as the Uncatchable, OutOfMemory and Interrupted fields
// of its *Error report it.
type errorKind int

const (
	thrown      errorKind = iota // page code threw it
	outOfMemory                  // the VM ran out of memory
	interrupted                  // Interrupt or the eval timeout
	goPanic                      // a Go function javascript called panicked
)

func (k errorKind) String() string {
	return [...]string{"thrown", "out of memory", "interrupted", "Go panic"}[k]
}

// wantKind checks the fields of err that say what ended the call.
func wantKind(t *testing.T, err error, kind errorKind) {
	t.Helper()
	var e *Error
	if !errors.As(err, &e) {
		t.Fatalf("got error %v (%T), want an *Error", err, err)
	}
	want := Error{Uncatchable: kind != thrown, OutOfMemory: kind == outOfMemory, Interrupted: kind == interrupted}
	if e.Uncatchable != want.Uncatchable || e.OutOfMemory != want.OutOfMemory || e.Interrupted != want.Interrupted {
		t.Fatalf("%v: got Uncatchable %v, OutOfMemory %v, Interrupted %v, want %v", err, e.Uncatchable, e.OutOfMemory, e.Interrupted, kind)
	}
}

func wantError(t *testing.T, err error, name, message string) {
	t.Helper()
	var e *Error
	if !errors.As(err, &e) || e.Name != name || e.Message != message {
		t.Fatalf("got error %v, want %s: %s", err, name, message)
	}
}

// An interrupt, from Interrupt or from the eval timeout, ends the call that
// was running however page code is written: no catch, finally, iterator
// return or rejection handler sees it, and the call returns it.
func TestInterruptUncatchable(t *testing.T) {
	for _, c := range uncatchableCases {
		t.Run(c.name, func(t *testing.T) {
			m := newTestVM(t)
			if err := m.RegisterFunc("stop", m.Interrupt, false); err != nil {
				t.Fatal(err)
			}

			if _, err := m.Eval(`globalThis.seen = []; globalThis.fail = () => { stop(); for (;;); }`, EvalGlobal); err != nil {
				t.Fatal(err)
			}

			err, seen := runUncatchable(t, m, c.js)
			wantError(t, err, "InternalError", "interrupted")
			wantKind(t, err, interrupted)
			if seen != "" {
				t.Fatalf("page code saw the interrupt: %s", seen)
			}
		})
	}
}

// growForever is a fail() that runs out of memory.
const growForever = `globalThis.fail = () => { const a = []; for (;;) a.push('x'.repeat(256) + a.length) }`

// Running out of memory ends the call like an interrupt: page code cannot
// catch it, and the call returns the engine's out of memory error.
func TestOutOfMemoryUncatchable(t *testing.T) {
	for _, c := range uncatchableCases {
		t.Run(c.name, func(t *testing.T) {
			m := newTestVM(t)
			if _, err := m.Eval(`globalThis.seen = []; `+growForever, EvalGlobal); err != nil {
				t.Fatal(err)
			}

			m.SetMemoryLimit(uintptr(vmMemory(m).Fmalloc_size) + 1<<20)
			err, seen := runUncatchable(t, m, c.js)
			wantError(t, err, "InternalError", "out of memory")
			wantKind(t, err, outOfMemory)
			if seen != "" {
				t.Fatalf("page code saw the out of memory error: %s", seen)
			}
			wantNoPendingException(t, m)
		})
	}
}

// The engine throws null, uncatchable, when it runs out of memory building
// an error (lib/outofmemory_test.go), and the VM reports it as the out of
// memory error.
func TestUncatchableNullIsOutOfMemory(t *testing.T) {
	m := newTestVM(t)
	tls, ctx := m.runtime.tls, m.cContext
	lib.XJS_Throw(tls, ctx, null)
	lib.XJS_SetUncatchableException(tls, ctx, 1)
	err := m.errFromException()
	wantError(t, err, "InternalError", "out of memory")
	wantKind(t, err, outOfMemory)
	if err.Error() != "InternalError: out of memory" {
		t.Fatalf("got %q", err.Error())
	}
	wantNoPendingException(t, m)

	// A script throwing null is not running out of memory.
	_, err = m.Eval(`throw null`, EvalGlobal)
	if err == nil || err.Error() != "null" {
		t.Fatalf("throw null: got %v", err)
	}
	wantKind(t, err, thrown)

	// The same inside a Go function: javascript it evaluates fails at once
	// with the null, and so does the call that got there.
	var nestedErr error
	mustHost(t, m, "nested", func([]any) (any, error) {
		lib.XJS_Throw(tls, ctx, null)
		lib.XJS_SetUncatchableException(tls, ctx, 1)
		_, nestedErr = m.Eval(`globalThis.ran = true`, EvalGlobal)
		return nil, nil
	})
	_, err = m.Eval(`try { nested() } catch (e) { globalThis.caught = true }`, EvalGlobal)
	wantError(t, nestedErr, "InternalError", "out of memory")
	wantKind(t, nestedErr, outOfMemory)
	wantError(t, err, "InternalError", "out of memory")
	wantKind(t, err, outOfMemory)
	if v, err := m.Eval(`typeof ran + typeof caught`, EvalGlobal); err != nil || v != "undefinedundefined" {
		t.Fatalf("page code ran on: %v, %v", v, err)
	}
	wantNoPendingException(t, m)
}

// The regexp engine running out of memory, compiling a pattern or running
// it, is the out of memory error.
func TestRegexpOutOfMemory(t *testing.T) {
	for _, c := range []struct{ name, js string }{
		{"exec", `/(a)+$/.exec(s)`},
		{"compile", `new RegExp(s)`},
	} {
		t.Run(c.name, func(t *testing.T) {
			m := newTestVM(t)
			if _, err := m.Eval(`globalThis.seen = []; globalThis.s = 'a'.repeat(1e6) + 'x'`, EvalGlobal); err != nil {
				t.Fatal(err)
			}

			m.SetMemoryLimit(uintptr(vmMemory(m).Fmalloc_size) + 64<<10)
			err, seen := runUncatchable(t, m, `try { `+c.js+` } catch (e) { seen.push(e.message) }`)
			wantError(t, err, "InternalError", "out of memory")
			wantKind(t, err, outOfMemory)
			if seen != "" {
				t.Fatalf("page code saw the out of memory error: %s", seen)
			}
		})
	}
}

// What page code throws is never uncatchable, even when it copies the name
// and message of an error the engine ends a call with: from Eval, from Call,
// and from javascript a Go function evaluates.
func TestThrownIsNotUncatchable(t *testing.T) {
	for _, thrownJS := range []string{
		`new InternalError('out of memory')`,
		`new InternalError('interrupted')`,
		`new InternalError('panic in Go function boom: boom')`,
		`({ name: 'InternalError', message: 'out of memory' })`,
		`({ name: 'InternalError', message: 'interrupted' })`,
		`'InternalError: interrupted'`,
		`null`,
	} {
		for _, entry := range []string{"eval", "call", "nested"} {
			t.Run(thrownJS+"/"+entry, func(t *testing.T) {
				m := newTestVM(t)
				var nestedErr error
				mustHost(t, m, "nested", func([]any) (any, error) {
					_, nestedErr = m.Eval(`throw `+thrownJS, EvalGlobal)
					return nil, nestedErr
				})
				if _, err := m.Eval(`globalThis.fail = () => { throw `+thrownJS+` }`, EvalGlobal); err != nil {
					t.Fatal(err)
				}

				var err error
				switch entry {
				case "eval":
					_, err = m.Eval(`fail()`, EvalGlobal)
				case "call":
					_, err = m.Call("fail")
				case "nested":
					_, err = m.Eval(`nested()`, EvalGlobal)
					wantKind(t, nestedErr, thrown)
				}
				wantKind(t, err, thrown)
				wantNoPendingException(t, m)
			})
		}
	}
}

// Taking an uncatchable error out of the context runs no page code. Page
// code that ran there, an Error.prototype.toString or a name getter it
// installed, could rewrite the message the error is told apart by, and turn
// a timeout into running out of memory or the other way round, in the
// javascript a Go function evaluates and in the call around it.
func TestUncatchableRunsNoPageCode(t *testing.T) {
	const timeout, oom = "timeout", "out of memory"
	for _, c := range []struct {
		name, hook, cause string
		nested            bool
	}{
		{"toString", `Error.prototype.toString = function () { ran = true; this.message = 'out of memory'; return 'x' }`, timeout, false},
		{"name getter", `Object.defineProperty(InternalError.prototype, 'name', { get() { ran = true; this.message = 'out of memory'; return 'InternalError' } })`, timeout, false},
		{"name getter out of memory", `Object.defineProperty(InternalError.prototype, 'name', { get() { ran = true; this.message = 'nothing'; return 'InternalError' } })`, oom, false},
		{"toString out of memory", `Error.prototype.toString = function () { ran = true; this.message = 'interrupted'; return 'x' }`, oom, false},
		{"toString nested", `Error.prototype.toString = function () { ran = true; this.message = 'out of memory'; return 'x' }`, timeout, true},
		{"toString out of memory nested", `Error.prototype.toString = function () { ran = true; this.message = 'interrupted'; return 'x' }`, oom, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			m := newTestVM(t)
			run := `for (;;);`
			kind, message := interrupted, "interrupted"
			if c.cause == oom {
				run = `(() => { const a = []; for (;;) a.push('x'.repeat(256) + a.length) })()`
				kind, message = outOfMemory, "out of memory"
			}
			var nestedErr error
			mustHost(t, m, "nested", func([]any) (any, error) {
				_, nestedErr = m.Eval(run, EvalGlobal)
				return nil, nestedErr
			})
			if _, err := m.Eval(`globalThis.ran = false; `+c.hook, EvalGlobal); err != nil {
				t.Fatal(err)
			}

			if c.cause == oom {
				m.SetMemoryLimit(uintptr(vmMemory(m).Fmalloc_size) + 1<<20)
			} else {
				m.SetEvalTimeout(50 * time.Millisecond)
			}
			js := run
			if c.nested {
				js = `nested()`
			}
			_, err := m.Eval(js, EvalGlobal)
			m.SetMemoryLimit(^uintptr(0))
			m.SetEvalTimeout(0)
			errs := []error{err}
			if c.nested {
				errs = append(errs, nestedErr)
			}
			for _, err := range errs {
				wantError(t, err, "InternalError", message)
				wantKind(t, err, kind)
				if got, want := err.Error(), "InternalError: "+message; got != want {
					t.Fatalf("got %q, want %q", got, want)
				}
				if e := err.(*Error); kind == interrupted && !strings.Contains(e.Stack, "    at ") {
					t.Fatalf("got stack %q, want the frames", e.Stack)
				}
			}
			if ran, err := m.Eval(`ran`, EvalGlobal); err != nil || ran != false {
				t.Fatalf("page code ran while the error was taken: %v, %v", ran, err)
			}
			wantNoPendingException(t, m)
		})
	}
}
