// Copyright (c) 2026 The liustack/beastjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

package beastjs // import "github.com/liustack/beastjs"

// The Go functions javascript calls are held by the host of their VM, each
// under a handle, until the engine frees the function object that calls it.

import (
	"strings"
	"testing"
)

// A Go function of RegisterFunc or RegisterHostFunc that javascript can no
// longer reach is let go: the engine releases its handle when it frees the
// function, at the drop of its last reference or, in a cycle, in a
// collection. Its name stays registered.
func TestRegisteredFuncReleased(t *testing.T) {
	m := newTestVM(t)
	n := heldFuncs(m)
	if err := m.RegisterFunc("f", func() int { return 1 }, false); err != nil {
		t.Fatal(err)
	}
	if err := m.RegisterHostFunc("h", func([]any) (any, error) { return 2, nil }); err != nil {
		t.Fatal(err)
	}
	if got := heldFuncs(m); got != n+2 {
		t.Fatalf("%d Go functions held after two were registered, want %d", got, n+2)
	}
	wantEval(t, m, "f() + h()", 3)
	if _, err := m.Eval("delete globalThis.f", EvalGlobal); err != nil {
		t.Fatal(err)
	}
	if got := heldFuncs(m); got != n+1 {
		t.Fatalf("%d Go functions held once f was deleted, want %d", got, n+1)
	}
	if _, err := m.Eval("h.self = h; delete globalThis.h", EvalGlobal); err != nil {
		t.Fatal(err)
	}
	if got := heldFuncs(m); got != n+1 {
		t.Fatalf("%d Go functions held before the collection of a cycle, want %d", got, n+1)
	}
	m.RunGC()
	if got := heldFuncs(m); got != n {
		t.Fatalf("%d Go functions held after the collection of a cycle, want %d", got, n)
	}
	if err := m.RegisterFunc("f", func() int { return 3 }, false); err == nil || !strings.Contains(err.Error(), "func already registered") {
		t.Fatalf("RegisterFunc of a name deleted: got %v, want func already registered", err)
	}
}

// Close lets go of every Go function the VM held, whether javascript could
// still reach it or not: of RegisterFunc, and the handlers of Then on a
// promise that never settles.
func TestCloseReleasesFuncs(t *testing.T) {
	m, err := NewVM()
	if err != nil {
		t.Fatal(err)
	}
	h := m.h
	for _, name := range []string{"a", "b", "c"} {
		if err := m.RegisterFunc(name, func() int { return 1 }, false); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := m.Eval("globalThis.kept = [a, b]; delete globalThis.c", EvalGlobal); err != nil {
		t.Fatal(err)
	}
	p, err := m.EvalValue("globalThis.p = new Promise(() => {})", EvalGlobal)
	if err != nil {
		t.Fatal(err)
	}
	res, err := p.Then(func(Value) { t.Error("onFulfilled ran") }, func(Value) { t.Error("onRejected ran") })
	if err != nil {
		t.Fatal(err)
	}
	res.Free()
	p.Free()
	if got := h.Handles(); got != 4 {
		t.Fatalf("%d Go functions held, want 4", got)
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	if got := h.Handles(); got != 0 {
		t.Fatalf("%d Go functions held after Close", got)
	}
}

// The Go functions that only the properties, the elements and the closures
// of other objects hold are let go of when those objects are freed: at the
// drop of the last reference, with a WeakRef on one of them, in a cycle that
// a full or a young collection frees, and after a script ran out of memory
// with them held.
func TestFuncsReleasedWithTheirHolders(t *testing.T) {
	m := newTestVM(t)
	n := heldFuncs(m)
	names := []string{"f0", "f1", "f2", "f3", "f4", "f5", "f6", "f7"}
	for i, name := range names {
		if err := m.RegisterFunc(name, func() int { return i }, false); err != nil {
			t.Fatal(err)
		}
	}
	held := func(want int, when string) {
		t.Helper()
		if got := heldFuncs(m) - n; got != want {
			t.Fatalf("%s: %d Go functions held, want %d", when, got, want)
		}
	}
	held(len(names), "registered")
	if _, err := m.Eval(`
globalThis.t = { a: f0, b: { c: f1, d: [f2, { e: f2 }] }, s: 'x' + f0(), i: 1,
	cl: (() => { const g = f3; return () => g })() };
globalThis.u = { k: f4, o: { p: f4 } };
globalThis.w = new WeakRef(f4);
globalThis.cy = { h: f5, q: { r: f5 } }; cy.self = cy; cy.q.back = cy;
globalThis.keep6 = { f6 };
globalThis.big = { k: f7 };
for (const name of ['f0', 'f1', 'f2', 'f3', 'f4', 'f5', 'f6', 'f7']) delete globalThis[name];
`, EvalGlobal); err != nil {
		t.Fatal(err)
	}
	held(len(names), "held by objects")
	wantEval(t, m, "t = null; 0", 0)
	held(4, "the holder dropped")
	// the job of the WeakRef ended with the last call
	wantEval(t, m, "u = null; String(w.deref())", "undefined")
	held(3, "the holder of a WeakRef target dropped")
	wantEval(t, m, "cy = null; 0", 0)
	held(3, "a cycle dropped")
	m.RunGC()
	held(2, "the cycle collected")
	// a young cycle that holds the function, which only an old object held before
	wantEval(t, m, "globalThis.ycy = { h: keep6.f6 }; ycy.self = [ycy, { x: keep6.f6 }]; keep6 = ycy = null; 0", 0)
	held(2, "a young cycle dropped")
	m.RunYoungGC()
	held(1, "the young cycle collected")
	m.SetMemoryLimit(m.MemorySize() + 256<<10)
	if _, err := m.Eval("globalThis.fill = []; for (;;) fill.push({ k: big.k, s: 'v' + fill.length })", EvalGlobal); err == nil {
		t.Fatal("the fill did not run out of memory")
	}
	m.SetMemoryLimit(^uintptr(0))
	wantEval(t, m, "big = null; 0", 0)
	held(1, "the holder dropped after running out of memory")
	wantEval(t, m, "fill = null; 0", 0)
	held(0, "the objects of the fill dropped")
}
