// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build slowjs_cengine && linux && amd64

package slowjs // import "github.com/liustack/slowjs"

// The Go functions javascript calls on the C engine are held by the host of
// their VM, each under a handle, until the engine frees the function object
// that calls it: on the engine of lib, those of RegisterFunc stay until the VM
// closes.

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
