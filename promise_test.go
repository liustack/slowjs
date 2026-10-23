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

func TestGoFuncReturnsPromise(t *testing.T) {
	t.Run("eval_value_then", func(t *testing.T) {
		m := newTestVM(t)

		var pc *PromiseCapability
		m.RegisterFunc("goDelayed", func() Value {
			var err error
			pc, err = m.NewPromiseCapability()
			if err != nil {
				t.Fatalf("NewPromiseCapability failed: %v", err)
			}
			return pc.Promise
		}, false)

		v, err := m.EvalValue(`goDelayed().then(v => "ok: " + v)`, EvalGlobal)
		if err != nil {
			t.Fatalf("EvalValue failed: %v", err)
		}
		defer v.Free()

		var got string
		res, err := v.Then(func(v Value) {
			a, _ := v.Any()
			got = a.(string)
		}, nil)
		if err != nil {
			t.Fatalf("Then failed: %v", err)
		}
		defer res.Free()

		if _, err := pc.Resolve.Call(UndefinedValue, "from Go func"); err != nil {
			t.Fatalf("Resolve failed: %v", err)
		}
		pc.Resolve.Free()
		pc.Reject.Free()

		if _, err := m.ExecutePendingJobs(); err != nil {
			t.Fatalf("ExecutePendingJobs failed: %v", err)
		}

		if got != "ok: from Go func" {
			t.Fatalf("got %q, want %q", got, "ok: from Go func")
		}
	})

	t.Run("immediate", func(t *testing.T) {
		m := newTestVM(t)

		m.RegisterFunc("goImmediate", func() Value {
			pc, err := m.NewPromiseCapability()
			if err != nil {
				t.Fatalf("NewPromiseCapability failed: %v", err)
			}
			pc.Resolve.Call(UndefinedValue, "immediate")
			p := pc.Promise
			pc.Resolve.Free()
			pc.Reject.Free()
			return p
		}, false)

		m.Eval(`
			globalThis.out = 'pending';
			goImmediate().then(v => { globalThis.out = 'ok: ' + v; });
		`, EvalGlobal)

		m.ExecutePendingJobs()

		v, _ := m.Eval("globalThis.out;", EvalGlobal)
		if v != "ok: immediate" {
			t.Fatalf("got %q, want %q", v, "ok: immediate")
		}
	})

	t.Run("await", func(t *testing.T) {
		m := newTestVM(t)
		pc := newPC(t, m)

		m.RegisterFunc("goAsyncFunc", func() Value { return pc.Promise.Dup() }, false)

		m.Eval(`
			globalThis.out = 'pending';
			(async () => {
				var v = await goAsyncFunc();
				globalThis.out = 'ok: ' + v;
			})();
		`, EvalGlobal)

		pc.Resolve.Call(UndefinedValue, "awaited")
		m.ExecutePendingJobs()

		v, _ := m.Eval("globalThis.out;", EvalGlobal)
		if v != "ok: awaited" {
			t.Fatalf("got %q, want %q", v, "ok: awaited")
		}
	})

	t.Run("multiple", func(t *testing.T) {
		m := newTestVM(t)

		var resolveVals, rejectVals []Value
		m.RegisterFunc("goMulti", func() Value {
			pc, err := m.NewPromiseCapability()
			if err != nil {
				t.Fatalf("NewPromiseCapability failed: %v", err)
			}
			resolveVals = append(resolveVals, pc.Resolve)
			rejectVals = append(rejectVals, pc.Reject)
			return pc.Promise
		}, false)

		m.Eval(`
			globalThis.results = [];
			goMulti().then(v => globalThis.results.push('p1:' + v));
			goMulti().then(v => globalThis.results.push('p2:' + v));
			goMulti().then(v => globalThis.results.push('p3:' + v));
		`, EvalGlobal)

		for i := range 3 {
			resolveVals[i].Call(UndefinedValue, i)
			resolveVals[i].Free()
			rejectVals[i].Free()
		}

		m.ExecutePendingJobs()

		v, _ := m.Eval("globalThis.results;", EvalGlobal)
		var results []string
		v.(*Object).Into(&results)

		want := []string{"p1:0", "p2:1", "p3:2"}
		for i, w := range want {
			if results[i] != w {
				t.Fatalf("result[%d] = %q, want %q", i, results[i], w)
			}
		}
	})
}

func TestThen(t *testing.T) {
	t.Run("fulfill_and_bypass_reject", func(t *testing.T) {
		m := newTestVM(t)
		pc := newPC(t, m)

		var fulfillStr, rejectStr string
		res, err := pc.Promise.Then(
			func(v Value) { s, _ := v.Any(); fulfillStr = s.(string) },
			func(v Value) { s, _ := v.Any(); rejectStr = s.(string) },
		)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Free()

		pc.Resolve.Call(UndefinedValue, "good")
		if _, err := m.ExecutePendingJobs(); err != nil {
			t.Fatal(err)
		}

		if fulfillStr != "good" || rejectStr != "" {
			t.Fatalf("fulfill=%q reject=%q, want fulfill=%q reject=\"\"", fulfillStr, rejectStr, "good")
		}
	})

	t.Run("reject", func(t *testing.T) {
		m := newTestVM(t)
		pc := newPC(t, m)

		var got string
		res, err := pc.Promise.Then(nil, func(v Value) {
			s, _ := v.Any()
			got = "err: " + s.(string)
		})
		if err != nil {
			t.Fatal(err)
		}
		defer res.Free()

		pc.Reject.Call(UndefinedValue, "fail")
		m.ExecutePendingJobs()

		if got != "err: fail" {
			t.Fatalf("got %q, want %q", got, "err: fail")
		}
	})

	t.Run("nil_callbacks", func(t *testing.T) {
		m := newTestVM(t)
		pc := newPC(t, m)

		res, err := pc.Promise.Then(nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Free()

		pc.Resolve.Call(UndefinedValue, "any")
		m.ExecutePendingJobs()
	})
}

func TestThenLeak(t *testing.T) {
	m := newTestVM(t)

	// Each Then call allocates 2 magics (one per handler). The old
	// release-on-fire approach leaked the unfired sibling handler's magic,
	// so 2*N magics would pile up and exhaust the pool (MaxInt16=32767).
	// With the finalizer approach, both magics are freed when the promise
	// settles, so we can run many more iterations than the pool allows.
	//
	// math.MaxInt16 = 32767. Each iteration creates 2 handlers. Without
	// the fix, iteration 16384 exhausts the pool. Run 40000 iterations to
	// definitively prove the fix works.
	const n = 40000
	for i := range n {
		pc, err := m.NewPromiseCapability()
		if err != nil {
			t.Fatal(err)
		}

		res, err := pc.Promise.Then(func(Value) {}, func(Value) {})
		if err != nil {
			t.Fatalf("Then failed at iteration %d: %v", i, err)
		}
		res.Free()

		if _, err := pc.Resolve.Call(UndefinedValue, "ok"); err != nil {
			t.Fatalf("Resolve failed at iteration %d: %v", i, err)
		}
		if _, err := m.ExecutePendingJobs(); err != nil {
			t.Fatalf("ExecutePendingJobs failed at iteration %d: %v", i, err)
		}
		pc.Free()
	}
}
