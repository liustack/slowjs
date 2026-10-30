// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package slowjs // import "github.com/liustack/slowjs"

// How much of its shadow stack a VM uses at the deepest calls it allows.
// The shadow stack has a size fixed when the engine is generated
// (ShadowSize), and a script that recurses must meet the depth limit
// (SetMaxStackSize) before it meets the end of the stack.

import (
	"fmt"
	"strings"
	"testing"
	"unsafe"

	"github.com/liustack/slowjs/internal/cengine"
)

// shadowStackUsed returns how far up its shadow stack the VM has ever been,
// in bytes from the base. The stack is anonymous memory that starts as zeros
// and grows up, so the highest word that is not zero marks the deepest frame.
func shadowStackUsed(m *VM) uintptr {
	base, size := m.h.Stack()
	words := unsafe.Slice((*uint64)(addr(base)), size/8)
	for i := len(words) - 1; i >= 0; i-- {
		if words[i] != 0 {
			return uintptr(i+1) * 8
		}
	}
	return 0
}

// recursion is a script that recurses until it throws, with the depth it
// reached in `reached`, and the one run that catches the error.
type recursion struct {
	name string
	// js defines f and the way it recurses.
	js string
	// start is the call that begins it.
	start string
}

// catchesItself names the recursions in which the engine catches the
// RangeError and keeps it, as a promise does of the error of its executor.
var catchesItself = map[string]bool{"promise_executor": true}

func frameOf(locals int) string {
	var vars []string
	for i := range locals {
		vars = append(vars, fmt.Sprintf("v%d = %d", i, i))
	}
	if len(vars) == 0 {
		return ""
	}
	return "var " + strings.Join(vars, ", ") + ";"
}

func argsOf(n int) string {
	var a []string
	for i := range n {
		a = append(a, fmt.Sprintf("a%d", i))
	}
	return strings.Join(a, ", ")
}

var recursions = []recursion{
	{"plain", `function f() { reached++; return f() + 1 }`, `f()`},
	{"method", `const o = { m() { reached++; return this.m() + 1 } }`, `o.m()`},
	{"call", `function f() { reached++; return f.call(null) + 1 }`, `f()`},
	{"apply", `function f() { reached++; return f.apply(null, []) + 1 }`, `f()`},
	{"reflect_apply", `function f() { reached++; return Reflect.apply(f, null, []) + 1 }`, `f()`},
	{"bound", `function f() { reached++; return g() + 1 } const g = f.bind(null)`, `f()`},
	{"spread", `function f(...a) { reached++; return f(...a, 1) + 1 }`, `f()`},
	{"getter", `const o = { get x() { reached++; return this.x + 1 } }`, `o.x`},
	{"setter", `const o = { set x(v) { reached++; this.x = v + 1 } }`, `o.x = 1`},
	{"to_string", `const o = { toString() { reached++; return '' + this } }`, "`${o}`"},
	{"value_of", `const o = { valueOf() { reached++; return o + 1 } }`, `o + 1`},
	{"proxy_get", `const p = new Proxy({}, { get(t, k, r) { reached++; return r.x } })`, `p.x`},
	{"construct", `class C { constructor() { reached++; new C() } }`, `new C()`},
	{"derived", `class A { constructor() { reached++; new B() } } class B extends A { constructor() { super() } }`, `new A()`},
	{"eval", `function f() { reached++; return eval('f()') + 1 }`, `f()`},
	{"array_map", `function f() { reached++; return [1].map(f) }`, `f()`},
	{"array_sort", `function f() { reached++; return [2, 1].sort(f) }`, `f()`},
	{"json_to_json", `const o = { toJSON() { reached++; return JSON.stringify(o) } }`, `JSON.stringify(o)`},
	{"replace_callback", `function f() { reached++; return 'a'.replace(/a/, f) }`, `f()`},
	{"iterator", `const o = { [Symbol.iterator]() { reached++; return { next() { for (const x of o) {} } } } }`, `for (const x of o) {}`},
	{"promise_executor", `function f() { reached++; new Promise(f) }`, `f()`},
	{"generator", `function* g() { reached++; yield* g() }`, `g().next()`},
	{"tagged_template", `function t() { reached++; return t` + "`a`" + ` + 1 }`, `t()`},
	// Frames that carry more: the engine keeps the arguments, locals and
	// operand stack of a call on the shadow stack.
	{"locals_20", `function f() { reached++; ` + frameOf(20) + ` return f() + v0 }`, `f()`},
	{"locals_100", `function f() { reached++; ` + frameOf(100) + ` return f() + v0 }`, `f()`},
	{"locals_250", `function f() { reached++; ` + frameOf(250) + ` return f() + v0 }`, `f()`},
	{"args_20", `function f(` + argsOf(20) + `) { reached++; return f(` + argsOf(20) + `) + a0 }`, `f(` + strings.TrimSuffix(strings.Repeat("1,", 20), ",") + `)`},
	{"args_100", `function f(` + argsOf(100) + `) { reached++; return f(` + argsOf(100) + `) + a0 }`, `f(` + strings.TrimSuffix(strings.Repeat("1,", 100), ",") + `)`},
	{"sort_locals_100", `function f() { reached++; ` + frameOf(100) + ` return [2, 1].sort(f) }`, `f()`},
	{"operands_100", `function f() { reached++; return [` + strings.Repeat("1,", 100) + ` f()] }`, `f()`},
}

// shadowRun runs one recursion at a depth limit in a new VM.
func shadowRun(t *testing.T, r recursion, limit uintptr) (reached int, used uintptr, rangeError bool) {
	t.Helper()
	m, err := NewVM()
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	m.SetMaxStackSize(limit)
	js := `var reached = 0; ` + r.js + `;
		var caught = ''; try { ` + r.start + ` } catch (e) { caught = e.constructor.name + ': ' + e.message }
		reached + ' ' + caught`
	out, err := m.Eval(js, EvalGlobal)
	if err != nil {
		t.Fatalf("%s: %v", r.name, err)
	}
	s, _ := out.(string)
	var caught string
	if _, err := fmt.Sscanf(s, "%d", &reached); err != nil {
		t.Fatalf("%s: result %q", r.name, s)
	}
	caught = strings.TrimSpace(strings.TrimPrefix(s, fmt.Sprint(reached)))
	return reached, shadowStackUsed(m), strings.HasPrefix(caught, "RangeError")
}

// bigFrames names the recursions whose frames carry 100 or more values: at the
// depth limit of 10000 they come within a few MiB of the end of the stack, and
// more would end on the stack first (with a RangeError).
var bigFrames = map[string]bool{"locals_100": true, "locals_250": true, "args_100": true, "sort_locals_100": true}

// TestShadowStackAtDepth runs each recursion at depth limits of 5000 and 10000
// and logs how far up the shadow stack it went. The recursions of ordinary
// frames must end on the depth counters with at least a quarter of the stack
// left, and every recursion must end in a RangeError, not a fault.
func TestShadowStackAtDepth(t *testing.T) {
	size := uintptr(cengine.ShadowSize)
	for _, limit := range []uintptr{5000, 10000} {
		t.Logf("limit %d, shadow stack %d MiB", limit, size>>20)
		for _, r := range recursions {
			reached, used, rangeErr := shadowRun(t, r, limit)
			per := float64(used-uintptr(cengine.ScratchSize)) / float64(max(reached, 1))
			t.Logf("  %-18s reached %5d  used %9d bytes (%5.1f%% of the stack)  %7.1f bytes a level  RangeError %v",
				r.name, reached, used, 100*float64(used)/float64(size), per, rangeErr)
			if !rangeErr && !catchesItself[r.name] {
				t.Errorf("%s: the recursion did not end in a RangeError", r.name)
			}
			if !bigFrames[r.name] && used > size/4*3 {
				t.Errorf("%s: used %d%% of the shadow stack at a depth limit of %d", r.name, 100*used/size, limit)
			}
		}
	}
}

// TestShadowStackInternalDepth does the same for the recursions of C that make
// no call of javascript: the parser, JSON, the regexp parser, flat and
// toString of nested arrays. Their count is the internal depth, 2000 levels.
// They must end in an error, with room on the shadow stack to spare.
func TestShadowStackInternalDepth(t *testing.T) {
	size := uintptr(cengine.ShadowSize)
	for _, c := range []struct{ name, js string }{
		{"parse_array", `eval('['.repeat(100000))`},
		{"parse_paren", `eval('('.repeat(100000))`},
		{"parse_function", `eval('function f() {'.repeat(20000))`},
		{"parse_json", `JSON.parse('['.repeat(100000))`},
		{"parse_json_object", `JSON.parse('{"a":'.repeat(100000))`},
		{"parse_regexp", `new RegExp('('.repeat(100000))`},
		{"stringify", `let o = {}; for (let i = 0; i < 100000; i++) o = { a: o }; JSON.stringify(o)`},
		{"flat", `let a = []; for (let i = 0; i < 100000; i++) a = [a]; a.flat(Infinity)`},
		{"to_string_nested", `let a = []; for (let i = 0; i < 100000; i++) a = [a]; String(a)`},
		{"proxy_chain", `let p = {}; for (let i = 0; i < 100000; i++) p = new Proxy(p, {}); Object.keys(p)`},
		// The name of each bound function holds the names of all below it, so a long chain takes memory
		// by the square of its length.
		{"bound_chain", `let f = function () {}; for (let i = 0; i < 4000; i++) f = f.bind(null); f()`},
	} {
		m, err := NewVM()
		if err != nil {
			t.Fatal(err)
		}
		m.SetMaxStackSize(10000)
		m.SetMemoryLimit(512 << 20)
		_, err = m.Eval(c.js, EvalGlobal)
		used := shadowStackUsed(m)
		t.Logf("  %-18s used %9d bytes (%5.1f%% of the stack)  ended with %v", c.name, used, 100*float64(used)/float64(size), firstLine(err))
		if used > size/4*3 {
			t.Errorf("%s: used %d%% of the shadow stack", c.name, 100*used/size)
		}
		m.Close()
	}
}

func firstLine(err error) string {
	if err == nil {
		return "no error"
	}
	s, _, _ := strings.Cut(err.Error(), "\n")
	return s
}

// TestHasOwnCallDepth: the interpreter answers hasOwnProperty.call(obj, key)
// without the two calls when it can. A recursion that makes it must end where
// the two calls would end it, at the depth limit and at the end of the stack:
// a call made on an array, which the interpreter always calls, must reach as
// deep, with frames of every size.
func TestHasOwnCallDepth(t *testing.T) {
	for _, limit := range []uintptr{5000, 10000} {
		for _, locals := range []int{0, 50, 100, 150, 200, 250} {
			run := func(args string) (int, bool) {
				r := recursion{"has_own_call", `const has = Object.prototype.hasOwnProperty, o = { a: 1 }, arr = [1];
					function f() { ` + frameOf(locals) + ` if (!has.call(` + args + `)) throw 0; reached++; return f() + 1 }`, `f()`}
				reached, _, rangeErr := shadowRun(t, r, limit)
				return reached, rangeErr
			}
			answered, answeredErr := run(`o, 'a'`)
			called, calledErr := run(`arr, 0`)
			if answered != called || !answeredErr || !calledErr {
				t.Errorf("limit %d, %d locals: answered reached %d (RangeError %v), called reached %d (RangeError %v)",
					limit, locals, answered, answeredErr, called, calledErr)
			}
		}
	}
}

// cCallKinds are recursions that go through a C function, with what f does
// after its locals. The interpreter calls a plain C function itself
// (js_call_c_direct), without the frame of JS_CallInternal, and counts that
// frame as taken, so each must end at the depth, and with the RangeError,
// it gave when every call of a C function went through JS_CallInternal
// (docs/changes/c-engine.md). The tail ones return the call of the C
// function, a tail call (test_tail_c_calls of engine/test/unit-white.c),
// and abs calls one with no receiver on each level before going down.
var cCallKinds = map[string]string{
	"call":       "return f.call(null) + v0",
	"apply":      "return f.apply(null, []) + v0",
	"map":        "return [1].map(f)[0] + v0",
	"sort":       "[2, 1].sort(f); return v0",
	"max":        "return Math.max(f(), v0)",
	"abs":        "v0 = abs(-v0); return f() + v0",
	"tail_map":   "return [1].map(f)",
	"tail_apply": "return apply(f, null, [])",
}

// cCallRun runs the recursion of kind with locals in m, at its depth limit,
// and gives the depth it reached and the stack of the RangeError.
func cCallRun(t *testing.T, m *VM, kind string, locals int) string {
	t.Helper()
	out, err := m.Eval(`(() => { const abs = Math.abs, apply = Reflect.apply; let reached = 0;
		function f() { `+frameOf(locals+1)+` reached++; `+cCallKinds[kind]+` }
		let at = ''; try { f() } catch (e) { at = e instanceof RangeError ? 'RangeError ' + e.stack : String(e) }
		return reached + ' ' + at })()`, EvalGlobal)
	if err != nil {
		t.Fatalf("%s_%d: %v", kind, locals, err)
	}
	return out.(string)
}

// directCCalls turns the direct calls of C functions of m on or off.
func directCCalls(t *testing.T, m *VM, on bool) {
	t.Helper()
	var r uintptr
	m.outer(func() { r = cengine.JS_SetDirectCCalls(m.sp, m.ctx, uintptr(map[bool]int{false: 0, true: 1}[on])) })
	if int32(r) != 0 {
		t.Fatal("JS_SetDirectCCalls refused")
	}
}

// TestCCallDepth runs each recursion of cCallKinds at the depth limits of 5000
// and 10000, with frames from none to 300 locals, which end at the limit or
// on the shadow stack, with the direct calls of C functions on and off, and
// requires both to reach the same depth and give the same stack.
func TestCCallDepth(t *testing.T) {
	for _, limit := range []uintptr{5000, 10000} {
		for kind := range cCallKinds {
			for _, locals := range []int{0, 100, 200, 250, 300} {
				var got [2]string
				for i, on := range []bool{true, false} {
					m, err := NewVM()
					if err != nil {
						t.Fatal(err)
					}
					m.SetMaxStackSize(limit)
					directCCalls(t, m, on)
					got[i] = cCallRun(t, m, kind, locals)
					m.Close()
				}
				if got[0] != got[1] {
					t.Errorf("limit %d, %s_%d: direct %.200q, through JS_CallInternal %.200q", limit, kind, locals, got[0], got[1])
				}
			}
		}
	}
}

// TestCCallDepthAfterExits leaves nested direct calls of C functions by a
// RangeError, an Error that a catch takes, and an interrupt, in the same VM,
// and requires the next recursion to end as the first did: each way out gives
// back the depth and the stack it counted.
func TestCCallDepthAfterExits(t *testing.T) {
	m := newTestVM(t)
	m.SetMaxStackSize(10000)
	if err := m.RegisterFunc("stop", func() { m.Interrupt() }, false); err != nil {
		t.Fatal(err)
	}
	first := cCallRun(t, m, "map", 250)
	if !strings.Contains(first, "RangeError") {
		t.Fatalf("no RangeError: %.200q", first)
	}
	if got := cCallRun(t, m, "map", 250); got != first {
		t.Fatalf("after a RangeError: %.200q, want %.200q", got, first)
	}
	if got := evalString(t, m, `try { [1].map(() => [1].map(() => [2, 1].sort(() => { throw new Error('x') }))); 'no' } catch (e) { e.message }`); got != "x" {
		t.Fatalf("the Error: %s", got)
	}
	if got := cCallRun(t, m, "map", 250); got != first {
		t.Fatalf("after an Error: %.200q, want %.200q", got, first)
	}
	_, err := m.Eval(`[1].map(() => [1].map(() => [2, 1].sort(() => { stop(); for (;;); })))`, EvalGlobal)
	wantKind(t, err, interrupted)
	if got := cCallRun(t, m, "map", 250); got != first {
		t.Fatalf("after an interrupt: %.200q, want %.200q", got, first)
	}
	if engineFramesLeft(m) {
		t.Fatal("a call is still counted")
	}
}
