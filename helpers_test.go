// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package slowjs // import "github.com/liustack/slowjs"

// Helpers of the tests that both engines run.

import (
	"errors"
	"testing"
	"time"
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

// growForever is a fail() that runs out of memory.
const growForever = `globalThis.fail = () => { const a = []; for (;;) a.push('x'.repeat(256) + a.length) }`

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

// mustHost registers fn as a host function or fails the test.
func mustHost(t *testing.T, m *VM, name string, fn HostFunc) {
	t.Helper()
	if err := m.RegisterHostFunc(name, fn); err != nil {
		t.Fatalf("RegisterHostFunc(%q): %v", name, err)
	}
}

// cycles defines garbage(n), which leaves n pairs of objects that point at
// each other: reference counting never frees them, only the GC does.
const cycles = `globalThis.garbage = n => {
	for (let i = 0; i < n; i++) {
		const a = {}, b = {a};
		a.b = b;
	}
}`

func newGCVM(t *testing.T) *VM {
	t.Helper()
	m, err := NewVM()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { m.Close() })
	if _, err := m.Eval(cycles, EvalGlobal); err != nil {
		t.Fatal(err)
	}
	return m
}

// jsStringCases makes, in javascript, strings of every form the engine
// holds: empty, ASCII, 8-bit with characters up to U+00FF, 16-bit with
// characters of every UTF-8 length, surrogate pairs, lone surrogates at the
// start, the middle and the end, 16-bit strings whose characters are all
// below U+0100, ropes of each, and long strings, plus random ones.
const jsStringCases = `(() => {
  const out = ['', 'a', 'hello, world', 'a\0b', '\x7f\x80', 'é', 'café crème', '\xff'.repeat(9), 'ÿ\x00\x80',
    '中文', 'a中', '\u07ff\u0800', '\uffff', '\ufffd', '😀', 'x😀y', '\ud800', '\udc00', 'a\ud800', '\ud800a',
    '\udc00\ud800', '\ud800\ud800\udc00', '\udbff\udfff', '\ud83d' + '\ude00', 'ab\ud83d',
    '\u0100é'.slice(1), 'é\u0100'.slice(0, 1), 'abcdefghij'.repeat(1000), 'é'.repeat(70000), '中'.repeat(70000),
    '😀'.repeat(40000), 'a'.repeat(1 << 16) + '\ud800', 'é' + 'a'.repeat(7), 'é' + 'a'.repeat(8), 'é' + 'a'.repeat(9) + 'ÿ',
    'é' + 'abcdefghijklmnopq' + 'ÿ' + 'ab', 'café crème brûlé'.repeat(64), '中文字符串测试 abc'.repeat(30)]
  const ropes = []
  for (const s of out) ropes.push('r'.repeat(600) + s, s + 'é'.repeat(600), s + '中'.repeat(600) + s)
  let seed = 7
  const next = (n) => Math.floor((seed = (seed * 1103515245 + 12345) % 2147483648) / 65536) % n
  for (let k = 0; k < 2000; k++) {
    let s = ''
    const len = next(k < 1500 ? 40 : 3000)
    const kind = next(4)
    for (let i = 0; i < len; i++) {
      const r = next(10)
      s += String.fromCharCode(r < 4 ? 32 + next(95) : r < 6 || kind === 0 ? next(0x100) : r < 8 ? next(0x10000) : 0xd800 + next(0x800))
    }
    out.push(s)
  }
  return out.concat(ropes)
})()`
