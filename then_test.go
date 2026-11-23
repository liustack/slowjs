// Copyright (c) 2026 The liustack/beastjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

package beastjs // import "github.com/liustack/beastjs"

import (
	"slices"
	"testing"
)

// A Go function returns the promise of a capability it made, and settles it
// later from Go.
func TestGoFuncReturnsPromise(t *testing.T) {
	t.Run("eval_value_then", func(t *testing.T) {
		m := newTestVM(t)
		var pc *PromiseCapability
		if err := m.RegisterFunc("goDelayed", func() Value {
			var err error
			if pc, err = m.NewPromiseCapability(); err != nil {
				t.Fatal(err)
			}
			return pc.Promise
		}, false); err != nil {
			t.Fatal(err)
		}
		p, err := m.EvalValue(`goDelayed().then(v => 'ok: ' + v)`, EvalGlobal)
		if err != nil {
			t.Fatal(err)
		}
		defer p.Free()
		var got []string
		r, err := p.Then(func(v Value) {
			s, err := v.Any()
			if err != nil {
				t.Error(err)
			}
			got = append(got, s.(string))
		}, nil)
		if err != nil {
			t.Fatal(err)
		}
		r.Free()
		if _, err := pc.Resolve.Call(UndefinedValue, "from Go func"); err != nil {
			t.Fatal(err)
		}
		pc.Resolve.Free()
		pc.Reject.Free()
		if _, err := m.ExecutePendingJobs(); err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(got, []string{"ok: from Go func"}) {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("immediate", func(t *testing.T) {
		m := newTestVM(t)
		if err := m.RegisterFunc("goNow", func() Value {
			pc, err := m.NewPromiseCapability()
			if err != nil {
				t.Fatal(err)
			}
			if _, err := pc.Resolve.Call(UndefinedValue, "immediate"); err != nil {
				t.Fatal(err)
			}
			pc.Resolve.Free()
			pc.Reject.Free()
			return pc.Promise
		}, false); err != nil {
			t.Fatal(err)
		}
		if _, err := m.Eval(`goNow().then(v => { globalThis.out = 'ok: ' + v })`, EvalGlobal); err != nil {
			t.Fatal(err)
		}
		if _, err := m.ExecutePendingJobs(); err != nil {
			t.Fatal(err)
		}
		wantEval(t, m, `out`, "ok: immediate")
	})

	t.Run("await", func(t *testing.T) {
		m := newTestVM(t)
		var pc *PromiseCapability
		if err := m.RegisterFunc("goAwaited", func() Value {
			var err error
			if pc, err = m.NewPromiseCapability(); err != nil {
				t.Fatal(err)
			}
			// The function keeps the promise, and so frees it with the rest.
			return pc.Promise.Dup()
		}, false); err != nil {
			t.Fatal(err)
		}
		if _, err := m.Eval(`(async () => { globalThis.out = 'ok: ' + await goAwaited() })()`, EvalGlobal); err != nil {
			t.Fatal(err)
		}
		if _, err := pc.Resolve.Call(UndefinedValue, "awaited"); err != nil {
			t.Fatal(err)
		}
		pc.Free()
		if _, err := m.ExecutePendingJobs(); err != nil {
			t.Fatal(err)
		}
		wantEval(t, m, `out`, "ok: awaited")
	})

	t.Run("multiple", func(t *testing.T) {
		m := newTestVM(t)
		var pcs []*PromiseCapability
		if err := m.RegisterFunc("goNext", func() Value {
			pc, err := m.NewPromiseCapability()
			if err != nil {
				t.Fatal(err)
			}
			pcs = append(pcs, pc)
			return pc.Promise
		}, false); err != nil {
			t.Fatal(err)
		}
		if _, err := m.Eval(`globalThis.results = [];
			goNext().then(v => results.push('p1:' + v));
			goNext().then(v => results.push('p2:' + v));
			goNext().then(v => results.push('p3:' + v));`, EvalGlobal); err != nil {
			t.Fatal(err)
		}
		for i, pc := range pcs {
			if _, err := pc.Resolve.Call(UndefinedValue, i); err != nil {
				t.Fatal(err)
			}
			pc.Resolve.Free()
			pc.Reject.Free()
		}
		if _, err := m.ExecutePendingJobs(); err != nil {
			t.Fatal(err)
		}
		r, err := m.Eval(`results`, EvalGlobal)
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		if err := r.(*Object).Into(&got); err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(got, []string{"p1:0", "p2:1", "p3:2"}) {
			t.Fatalf("got %q", got)
		}
	})
}

func TestThen(t *testing.T) {
	collect := func(out *[]string, tag string) func(Value) {
		return func(v Value) {
			s, err := v.Any()
			if err != nil {
				panic(err)
			}
			*out = append(*out, tag+":"+s.(string))
		}
	}
	for _, c := range []struct {
		name   string
		settle func(pc *PromiseCapability) (any, error)
		want   []string
	}{
		{"fulfilled", func(pc *PromiseCapability) (any, error) { return pc.Resolve.Call(UndefinedValue, "good") }, []string{"fulfilled:good"}},
		{"rejected", func(pc *PromiseCapability) (any, error) { return pc.Reject.Call(UndefinedValue, "fail") }, []string{"rejected:fail"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			m := newTestVM(t)
			pc := newPC(t, m)
			var got []string
			r, err := pc.Promise.Then(collect(&got, "fulfilled"), collect(&got, "rejected"))
			if err != nil {
				t.Fatal(err)
			}
			defer r.Free()
			if _, err := c.settle(pc); err != nil {
				t.Fatal(err)
			}
			if _, err := m.ExecutePendingJobs(); err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(got, c.want) {
				t.Fatalf("got %q, want %q", got, c.want)
			}
		})
	}
	t.Run("no handlers", func(t *testing.T) {
		m := newTestVM(t)
		pc := newPC(t, m)
		r, err := pc.Promise.Then(nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer r.Free()
		if _, err := pc.Resolve.Call(UndefinedValue, 1); err != nil {
			t.Fatal(err)
		}
		if _, err := m.ExecutePendingJobs(); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("no VM", func(t *testing.T) {
		if _, err := UndefinedValue.Then(nil, nil); err == nil {
			t.Fatal("Then of a Value of no VM returned no error")
		}
	})
}

// The handler that never runs is let go of with the one that does. When only
// the one that ran was freed, 16,384 calls used up the 32,767 handles of a
// VM.
func TestThenLeak(t *testing.T) {
	m := newTestVM(t)
	for i := range 40000 {
		pc, err := m.NewPromiseCapability()
		if err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
		r, err := pc.Promise.Then(func(Value) {}, func(Value) {})
		if err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
		r.Free()
		if _, err := pc.Resolve.Call(UndefinedValue, i); err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
		if _, err := m.ExecutePendingJobs(); err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
		pc.Free()
	}
}

// The Go functions of the handlers are held while the engine holds the
// functions that call them, and no longer: after the promise settles and its
// jobs ran, or once a promise that never settles is freed.
func TestThenReleasesHandlers(t *testing.T) {
	t.Run("settled", func(t *testing.T) {
		m := newTestVM(t)
		pc := newPC(t, m)
		n := heldFuncs(m)
		fulfilled, rejected := 0, 0
		r, err := pc.Promise.Then(func(Value) { fulfilled++ }, func(Value) { rejected++ })
		if err != nil {
			t.Fatal(err)
		}
		r.Free()
		if got := heldFuncs(m); got != n+2 {
			t.Fatalf("after Then %d Go functions are held, want %d", got, n+2)
		}
		if _, err := pc.Resolve.Call(UndefinedValue, 1); err != nil {
			t.Fatal(err)
		}
		if _, err := m.ExecutePendingJobs(); err != nil {
			t.Fatal(err)
		}
		if got := heldFuncs(m); got != n {
			t.Fatalf("after the jobs %d Go functions are held, want %d", got, n)
		}
		if fulfilled != 1 || rejected != 0 {
			t.Fatalf("onFulfilled ran %d times and onRejected %d, want 1 and 0", fulfilled, rejected)
		}
	})
	t.Run("pending", func(t *testing.T) {
		m := newTestVM(t)
		pc, err := m.NewPromiseCapability()
		if err != nil {
			t.Fatal(err)
		}
		n := heldFuncs(m)
		r, err := pc.Promise.Then(func(Value) {}, nil)
		if err != nil {
			t.Fatal(err)
		}
		r.Free()
		if got := heldFuncs(m); got != n+1 {
			t.Fatalf("after Then %d Go functions are held, want %d", got, n+1)
		}
		pc.Free()
		if got := heldFuncs(m); got != n {
			t.Fatalf("after the promise was freed %d Go functions are held, want %d", got, n)
		}
	})
}

// A then getter that throws is the error of Then, and leaves nothing pending.
func TestThenGetterThrows(t *testing.T) {
	m := newTestVM(t)
	p := evalValueOrFatal(t, m, `(() => {
		const p = Promise.resolve(1);
		Object.defineProperty(p, 'then', { get() { throw new RangeError('original') } });
		return p;
	})()`)
	defer p.Free()
	_, err := p.Then(func(Value) {}, nil)
	wantError(t, err, "RangeError", "original")
	wantNoPendingException(t, m)
}
