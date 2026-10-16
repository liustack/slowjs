// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package quickjs // import "github.com/liustack/quickjs"

import (
	"errors"
	"testing"
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
			if seen != "" {
				t.Fatalf("page code saw the interrupt: %s", seen)
			}
		})
	}
}
