// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package slowjs

import (
	"testing"
)

func newTestVM(t *testing.T) *VM {
	t.Helper()
	m, err := NewVM()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { m.Close() })
	return m
}

func newPC(t *testing.T, m *VM) *PromiseCapability {
	t.Helper()
	pc, err := m.NewPromiseCapability()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pc.Free() })
	return pc
}

func TestNewPromiseCapability(t *testing.T) {
	m := newTestVM(t)
	pc := newPC(t, m)

	g := m.GlobalObject()
	pa, _ := m.NewAtom("p")
	m.SetPropertyValue(g, pa, pc.Promise)
	g.Free()

	m.Eval(`
		globalThis.promiseResult = 'pending';
		p.then(v => { globalThis.promiseResult = 'ok: ' + v; });
	`, EvalGlobal)

	pc.Resolve.Call(UndefinedValue, "from Go")
	m.ExecutePendingJobs()

	v, _ := m.Eval("globalThis.promiseResult;", EvalGlobal)
	if v != "ok: from Go" {
		t.Fatalf("got %q, want %q", v, "ok: from Go")
	}
}

func TestNewPromiseCapability_Reject(t *testing.T) {
	m := newTestVM(t)
	pc := newPC(t, m)

	g := m.GlobalObject()
	pa, _ := m.NewAtom("p")
	m.SetPropertyValue(g, pa, pc.Promise)
	g.Free()

	m.Eval(`
		globalThis.promiseResult = 'pending';
		p.then(
			v => { globalThis.promiseResult = 'ok: ' + v; },
			e => { globalThis.promiseResult = 'err: ' + (e.message || e); }
		);
	`, EvalGlobal)

	pc.Reject.Call(UndefinedValue, "fail")
	m.ExecutePendingJobs()

	v, _ := m.Eval("globalThis.promiseResult;", EvalGlobal)
	if v != "err: fail" {
		t.Fatalf("got %q, want %q", v, "err: fail")
	}
}
