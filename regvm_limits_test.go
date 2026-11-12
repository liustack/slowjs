// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package quickjs // import "github.com/liustack/quickjs"

import (
	"strings"
	"sync"
	"testing"
	"time"
)

// regvmSpins are functions go() that never end, each in a shape the register
// loop runs: a loop, a call tree with no loop, a loop around a catch or a
// finally, a loop of calls of a closure, a method of a class, a callback of
// a C function, a loop of a generator, and a call into a Go function that
// calls back into javascript. spin is called by the Go function back.
var regvmSpins = []struct{ name, js string }{
	{"loop", `function go() { var i = 0; for (;;) i++; }`},
	{"call tree", `function t(n) { return n === 0 ? 0 : t(n - 1) + t(n - 1); } function go() { for (var k = 0; k < 1000; k++) t(40); }`},
	{"no loop at all", `function t(n) { return n === 0 ? 0 : t(n - 1) + t(n - 1) + 1; } function go() { return t(60); }`},
	{"catch", `function f() { throw 1; } function go() { for (;;) { try { f(); } catch (e) {} } }`},
	{"finally", `function go() { var i = 0; for (;;) { try { i++; } finally { i--; } } }`},
	{"closure", `function go() { var x = 0; var g = () => x++; for (;;) g(); }`},
	{"method", `class A { m() { for (;;) {} } } function go() { new A().m(); }`},
	{"callback", `function go() { [1].forEach(function () { for (;;) {} }); }`},
	{"generator", `function* gen() { for (;;) yield 1; } function go() { for (const v of gen()) {} }`},
	{"Go callback", `function spin() { for (;;) {} } function go() { back(); }`},
}

// regvmRun runs go() of js on a new VM with the register loop on, with stop
// to end it, and fails if it does not end within 10 seconds. It returns the
// error of the call, and checks that the call ran in the register loop and
// left the VM usable.
func regvmRun(t *testing.T, js string, stop func(m *VM)) error {
	t.Helper()
	m := newTestVM(t)
	mustHost(t, m, "back", func([]any) (any, error) { return m.Call("spin") })
	if _, err := m.Eval(js, EvalGlobal); err != nil {
		t.Fatal(err)
	}
	stop(m)
	done := make(chan error, 1)
	go func() {
		_, err := m.Call("go")
		done <- err
	}()
	var err error
	select {
	case err = <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("the call did not end")
	}
	m.SetEvalTimeout(0)
	m.SetMemoryLimit(^uintptr(0))
	if n := regvmStat(m, 0); n < 1 {
		t.Errorf("%d functions converted: the call did not run in the register loop", n)
	}
	wantNoPendingException(t, m)
	if engineFramesLeft(m) {
		t.Error("the engine still has a stack frame")
	}
	if v, err := m.Eval(`1 + 1`, EvalGlobal); err != nil || v != 2 {
		t.Errorf("the VM after the call: %v, %v", v, err)
	}
	return err
}

// The eval timeout and Interrupt end every call that runs in the register
// loop, whatever its shape, with the same error as in the old loop.
func TestRegvmLimitsInterrupt(t *testing.T) {
	for _, c := range regvmSpins {
		t.Run("timeout "+c.name, func(t *testing.T) {
			err := regvmRun(t, c.js, func(m *VM) { m.SetEvalTimeout(50 * time.Millisecond) })
			wantError(t, err, "InternalError", "interrupted")
			wantKind(t, err, interrupted)
		})
		t.Run("Interrupt "+c.name, func(t *testing.T) {
			err := regvmRun(t, c.js, func(m *VM) {
				go func() {
					time.Sleep(30 * time.Millisecond)
					m.Interrupt()
				}()
			})
			wantError(t, err, "InternalError", "interrupted")
			wantKind(t, err, interrupted)
		})
	}
}

// Running out of memory inside the register loop ends the call, also in a
// loop that catches every error.
func TestRegvmLimitsMemory(t *testing.T) {
	for _, c := range []struct{ name, js string }{
		{"push", `function go() { var a = []; for (;;) a.push('x'.repeat(256) + a.length); }`},
		{"catch", `function go() { var a = []; for (;;) { try { a.push('x'.repeat(256) + a.length); } catch (e) {} } }`},
		{"objects", `function go() { var a = []; for (var i = 0; ; i++) a.push({ i: i, s: [i, i + 1, i + 2] }); }`},
		{"recursion", `function r(a) { a.push(new Array(64).fill(a.length)); return r(a) + 1; } function go() { for (;;) { try { r([]); } catch (e) {} } }`},
	} {
		t.Run(c.name, func(t *testing.T) {
			err := regvmRun(t, c.js, func(m *VM) {
				m.SetMemoryLimit(uintptr(vmMemory(m).Fmalloc_size) + 4<<20)
			})
			wantError(t, err, "InternalError", "out of memory")
			wantKind(t, err, outOfMemory)
		})
	}
}

// A Go function called from the register loop that panics: the panic goes
// up through the frames of the register loop to the caller of Call.
func TestRegvmGoPanic(t *testing.T) {
	m := newTestVM(t)
	mustHost(t, m, "boom", func([]any) (any, error) { panic("boom") })
	if _, err := m.Eval(`function inner(n) { if (n === 0) boom(); return inner(n - 1) + 1; }
		function go() { var s = 0; for (var i = 0; i < 10; i++) s += i; try { return inner(50); } finally { s = 0; } }`, EvalGlobal); err != nil {
		t.Fatal(err)
	}
	p := func() (p any) {
		defer func() { p = recover() }()
		m.Call("go")
		return nil
	}()
	if p != "boom" {
		t.Fatalf("recovered %v, want the Go function's panic", p)
	}
	if regvmStat(m, 0) < 2 {
		t.Errorf("%d functions converted", regvmStat(m, 0))
	}
	if engineFramesLeft(m) {
		t.Fatal("the engine still has a stack frame")
	}
	wantNoPendingException(t, m)
	wantTimeoutWorks(t, m)
}

// Many renders on one VM with the register loop on, as tifa pools VMs:
// closures, classes, exceptions and Go calls each time, the same result
// every time, and Close frees everything.
func TestRegvmPooledRenders(t *testing.T) {
	m, err := NewVM()
	if err != nil {
		t.Fatal(err)
	}
	if err := m.RegisterHostFunc("upper", func(a []any) (any, error) { return strings.ToUpper(a[0].(string)), nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Eval(`
		class Node { constructor(tag, kids) { this.tag = tag; this.kids = kids; } html() { return '<' + this.tag + '>' + this.kids.map(k => typeof k === 'string' ? upper(k) : k.html()).join('') + '</' + this.tag + '>'; } }
		function tree(n) { if (n === 0) return 'leaf'; var kids = []; for (var i = 0; i < 3; i++) kids.push(tree(n - 1)); return new Node(n % 2 ? 'div' : 'span', kids); }
		function render(k) { try { if (k % 7 === 3) throw new Error('skip ' + k); return tree(4).html().length + k; } catch (e) { return e.message; } }`, EvalGlobal); err != nil {
		t.Fatal(err)
	}
	var first []any
	for round := range 3 {
		for k := range 100 {
			v, err := m.Call("render", k)
			if err != nil {
				t.Fatal(err)
			}
			if round == 0 {
				first = append(first, v)
			} else if v != first[k] {
				t.Fatalf("round %d, render %d: %v, want %v", round, k, v, first[k])
			}
		}
	}
	if regvmStat(m, 0) < 3 {
		t.Errorf("%d functions converted", regvmStat(m, 0))
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
}

// The same bytecode in several VMs at once, each with the register loop on:
// the register code of a function is its VM's own.
func TestRegvmSharedBytecodeVMs(t *testing.T) {
	m0 := newTestVM(t)
	code, err := m0.Compile(`function fib(n) { return n < 2 ? n : fib(n - 1) + fib(n - 2); }
		var counter = (function () { var c = 0; return function () { return ++c; }; })();
		function work(k) { var s = 0; for (var i = 0; i < 200; i++) s += fib(10) + counter() * k; return s; }`, EvalGlobal)
	if err != nil {
		t.Fatal(err)
	}
	shared := newSharedBytecode(t, code)
	defer shared.Close()
	var wg sync.WaitGroup
	results := make([][]any, 6)
	for g := range 6 {
		wg.Go(func() {
			m, err := NewVM()
			if err != nil {
				t.Error(err)
				return
			}
			defer m.Close()
			if g%2 == 0 {
				_, err = m.EvalSharedBytecode(shared)
			} else {
				_, err = m.EvalBytecode(code)
			}
			if err != nil {
				t.Error(err)
				return
			}
			for k := range 20 {
				v, err := m.Call("work", k)
				if err != nil {
					t.Error(err)
					return
				}
				results[g] = append(results[g], v)
			}
			if regvmStat(m, 0) < 2 {
				t.Errorf("VM %d: %d functions converted", g, regvmStat(m, 0))
			}
		})
	}
	wg.Wait()
	for g := 1; g < len(results); g++ {
		for k := range results[0] {
			if k >= len(results[g]) || results[g][k] != results[0][k] {
				t.Fatalf("VM %d differs from VM 0 at %d", g, k)
			}
		}
	}
}
