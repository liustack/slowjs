// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package quickjs // import "github.com/liustack/quickjs"

import (
	"testing"
)

// callKinds are the four ways the engine calls a function: a javascript
// function (JS_CallInternal), a native one with as many arguments as it
// declares and with fewer (js_call_c_function, which pads the missing ones
// with undefined), a bound function (js_call_bound_function) and a native
// function with data (js_c_function_data_call): a revoke function of
// Proxy.revocable, which declares no argument, and the functions
// Promise.all makes with a constructor of the script's, the executor of
// the capability, which declares two and gets none, then two, and the
// function that resolves an element, which declares one and gets none.
var callKinds = []struct {
	name, setup, call string
}{
	{"js", `function f(a) { return a }`, `f(i)`},
	{"native", ``, `Math.abs(i)`},
	{"native padded", ``, `Math.max(i)`},
	{"bound", `const g = function (a) { return a }.bind(null)`, `g(i)`},
	{"data function", `const r = Proxy.revocable({}, {})`, `r.revoke()`},
	{"data function padded", `function C(ex) { ex(); ex(() => {}, () => {}) }; C.resolve = v => ({ then(r) { r() } })`, `Promise.all.call(C, [i])`},
}

// newCallVM returns a VM with a global loop(n) that makes n calls of kind.
func newCallVM(tb testing.TB, setup, call string) *VM {
	tb.Helper()
	m, err := NewVM()
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(func() { m.Close() })
	if _, err := m.Eval(`globalThis.loop = (() => { `+setup+`; return function (n) { for (let i = 0; i < n; i++) `+call+` } })()`, EvalGlobal); err != nil {
		tb.Fatal(err)
	}
	return m
}

// BenchmarkCall times one call of each kind. ns/op and allocs/op are per
// call.
func BenchmarkCall(b *testing.B) {
	for _, k := range callKinds {
		b.Run(k.name, func(b *testing.B) {
			m := newCallVM(b, k.setup, k.call)
			b.ReportAllocs()
			b.ResetTimer()
			if _, err := m.Call("loop", b.N); err != nil {
				b.Fatal(err)
			}
		})
	}
}

// A call of any kind allocates no Go memory: its TLS frame and the room
// for its arguments and locals are libc memory.
func TestCallAllocs(t *testing.T) {
	for _, k := range callKinds {
		m := newCallVM(t, k.setup, k.call)
		run := func(n int) float64 {
			return testing.AllocsPerRun(20, func() {
				if _, err := m.Call("loop", n); err != nil {
					t.Fatal(err)
				}
			})
		}
		if per := (run(2000) - run(1000)) / 1000; per != 0 {
			t.Errorf("%s: %.2f Go allocations per call, want 0", k.name, per)
		}
	}
}
