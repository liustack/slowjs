// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

package slowjs // import "github.com/liustack/slowjs"

import "testing"

// newTestVM returns a new VM that the end of the test closes.
func newTestVM(t *testing.T) *VM {
	t.Helper()
	m, err := NewVM()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { m.Close() })
	return m
}

// newPC returns a new promise capability of m that the end of the test
// frees.
func newPC(t *testing.T, m *VM) *PromiseCapability {
	t.Helper()
	pc, err := m.NewPromiseCapability()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pc.Free)
	return pc
}

// settleFromGo makes the promise of a new capability the global p, has a
// script attach handlers that write promiseResult, settles it with settle,
// runs the jobs and returns promiseResult.
func settleFromGo(t *testing.T, settle func(pc *PromiseCapability) error) any {
	t.Helper()
	m := newTestVM(t)
	pc := newPC(t, m)
	g := m.GlobalObject()
	defer g.Free()
	if err := m.SetPropertyValue(g, atomOrFatal(t, m, "p"), pc.Promise); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Eval(`p.then(v => { globalThis.promiseResult = 'ok: ' + v }, e => { globalThis.promiseResult = 'err: ' + e })`, EvalGlobal); err != nil {
		t.Fatal(err)
	}
	if err := settle(pc); err != nil {
		t.Fatal(err)
	}
	if _, err := m.ExecutePendingJobs(); err != nil {
		t.Fatal(err)
	}
	r, err := m.Eval(`promiseResult`, EvalGlobal)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestNewPromiseCapability(t *testing.T) {
	got := settleFromGo(t, func(pc *PromiseCapability) error {
		_, err := pc.Resolve.Call(UndefinedValue, "from Go")
		return err
	})
	if got != "ok: from Go" {
		t.Fatalf("got %v", got)
	}
}

func TestNewPromiseCapability_Reject(t *testing.T) {
	got := settleFromGo(t, func(pc *PromiseCapability) error {
		_, err := pc.Reject.Call(UndefinedValue, "fail")
		return err
	})
	if got != "err: fail" {
		t.Fatalf("got %v", got)
	}
}

// Dup gives three references of their own: the copy outlives the Free of
// the first.
func TestPromiseCapabilityDup(t *testing.T) {
	m := newTestVM(t)
	pc, err := m.NewPromiseCapability()
	if err != nil {
		t.Fatal(err)
	}
	dup := pc.Dup()
	pc.Free()
	if pc.Promise != (Value{}) || pc.Resolve != (Value{}) || pc.Reject != (Value{}) {
		t.Fatal("Free left a value set")
	}
	defer dup.Free()
	got := ""
	r, err := dup.Promise.Then(func(v Value) { s, _ := v.Any(); got = s.(string) }, nil)
	if err != nil {
		t.Fatal(err)
	}
	r.Free()
	if _, err := dup.Resolve.Call(UndefinedValue, "kept"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.ExecutePendingJobs(); err != nil {
		t.Fatal(err)
	}
	if got != "kept" {
		t.Fatalf("got %q", got)
	}
}
