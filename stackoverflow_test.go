// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package slowjs // import "github.com/liustack/slowjs"

import (
	"runtime/debug"
	"testing"
)

// stackOverflow is the error for running out of stack, worded as in V8.
const stackOverflow = "RangeError: Maximum call stack size exceeded"

// recursionCases recurse without end through the ways javascript calls
// javascript.
var recursionCases = []string{
	`function f() { return f() } f()`,
	`const o = { get x() { return this.x } }; o.x`,
	`const p = new Proxy({}, { get(t, k, r) { return r[k] } }); p.x`,
	`const o = { toString() { return '' + this } }; '' + o`,
	`const o = { toJSON() { return JSON.stringify(this) } }; JSON.stringify(o)`,
	`function f() { return [1].map(f) } f()`,
	`function F() { new F() } new F()`,
	`function f() { return f.call(null) } f()`,
	`function f() { return Reflect.apply(f, null, []) } f()`,
	`function* g() { yield* g() } [...g()]`,
	`class A { constructor() { new A() } }; new A()`,
	`const g = function () { return h() }; const h = g.bind(null); h()`,
	`const r = /a/; r[Symbol.replace] = function (s) { return s.replace(this) }; 'a'.replace(r)`,
	`let a = []; for (let i = 0; i < 1e5; i++) a = [a]; JSON.stringify(a)`,
	`let a = []; for (let i = 0; i < 1e5; i++) a = [a]; String(a)`,
}

// With the default settings, recursing without end throws a RangeError a
// script can catch, as in a browser, well before the goroutine runs out of
// stack, and the VM works afterwards. The default limit used to be a
// million TLS stack slots: the engine sets its default stack size in bytes,
// and the Go port reads it as slots, so the goroutine ran out of stack
// first and the process died. The Go stack is capped below its default
// here, near what a 32-bit platform has, so that a default too high fails
// quickly.
func TestDefaultStackLimit(t *testing.T) {
	defer debug.SetMaxStack(debug.SetMaxStack(256 << 20))

	m := newTestVM(t)
	for _, js := range recursionCases {
		v, err := m.Eval(`try { `+js+`; 'no error' } catch (e) { (e instanceof RangeError) + ' ' + e }`, EvalGlobal)
		if err != nil || v != "true "+stackOverflow {
			t.Fatalf("%s: got %v, %v, want %s", js, v, err, stackOverflow)
		}
	}

	// The parser and JSON.parse report nesting too deep for the stack with
	// the same RangeError, as V8 does for the parser. V8 parses JSON
	// without recursing, so it has no such limit there.
	for _, js := range []string{`eval('['.repeat(1e6))`, `eval('('.repeat(1e6))`, `JSON.parse('['.repeat(1e6))`} {
		v, err := m.Eval(`try { `+js+`; 'no error' } catch (e) { (e instanceof RangeError) + ' ' + e }`, EvalGlobal)
		if err != nil || v != "true "+stackOverflow {
			t.Fatalf("%s: got %v, %v, want %s", js, v, err, stackOverflow)
		}
	}

	// The regexp compiler still reports it as its own syntax error.
	v, err := m.Eval(`try { new RegExp('(?:'.repeat(1e6)); 'no error' } catch (e) { String(e) }`, EvalGlobal)
	if err != nil || v != "SyntaxError: stack overflow" {
		t.Fatalf("a regexp nested too deep: got %v, %v, want the syntax error", v, err)
	}

	// Deep recursion that ends still works: React renders nest deep.
	v, err = m.Eval(`function d(n) { return n ? d(n - 1) + 1 : 0 } d(5000)`, EvalGlobal)
	if err != nil || v != 5000 {
		t.Fatalf("recursing 5000 deep: %v, %v", v, err)
	}
}
