// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package quickjs // import "github.com/liustack/quickjs"

import (
	"fmt"
	"reflect"
	"runtime/debug"
	"strings"
	"testing"
	"time"
	"unsafe"
)

// depthCases recurse without end, each in another way javascript calls
// javascript, counting the calls in n until the stack runs out. The
// depths at the default limit and at 300 slots are those of the engine
// before its call frames changed: a change to how many TLS stack slots a
// call takes moves them, and with them how deep a page can render.
var depthCases = []struct {
	name, js     string
	limit, at300 int
}{
	{"plain", `function f() { n++; f() } f()`, 9994, 294},
	{"method", `const o = { m() { n++; this.m() } }; o.m()`, 9994, 294},
	{"map callback", `function f() { n++; [0].map(f) } f()`, 2498, 73},
	{"bound", `const g = function () { n++; h() }; const h = g.bind(null); h()`, 3331, 98},
	{"getter", `const o = { get x() { n++; return this.x } }; o.x`, 3331, 98},
	{"valueOf", `const o = { valueOf() { n++; return +this } }; +o`, 1998, 58},
	{"proxy get", `const p = new Proxy({}, { get(t, k, r) { n++; return r[k] } }); p.x`, 1999, 59},
	{"class new", `class A { constructor() { n++; new A() } }; new A()`, 4997, 147},
	{"call", `function f() { n++; f.call(null) } f()`, 3332, 98},
	{"Reflect.apply", `function f() { n++; Reflect.apply(f, null, []) } f()`, 2499, 74},
	{"padded native", `function f() { n++; f.call() } f()`, 3332, 98},
	{"yield*", `function* g() { n++; yield* g() } g().next()`, 2498, 73},
	{"async", `async function f() { n++; await f() } f()`, 3331, 98},
}

// Every way of recursing reaches the same depth as before, at the default
// stack limit and at a lower one.
func TestCallDepth(t *testing.T) {
	if unsafe.Sizeof(uintptr(0)) != 8 {
		t.Skip("the depths are those of the 64-bit engine")
	}
	defer debug.SetMaxStack(debug.SetMaxStack(512 << 20))

	for _, limit := range []uintptr{0, 300} {
		for _, c := range depthCases {
			m, err := NewVM()
			if err != nil {
				t.Fatal(err)
			}
			m.SetMaxStackSize(limit)
			// darwin's libc keeps errno in a TLS stack slot, linux's
			// does not: count from one slot in use on both.
			tls := m.runtime.tls
			pad := 1 - tls.StackSlots()
			if pad < 0 {
				t.Fatalf("%d TLS stack slots in use in a new VM", tls.StackSlots())
			}
			for range pad {
				tls.Alloc(16)
			}
			v, err := m.Eval(`let n = 0; try { `+c.js+` } catch (e) { if (!(e instanceof RangeError)) throw e }; n`, EvalGlobal)
			for range pad {
				tls.Free(16)
			}
			if err != nil {
				t.Fatalf("%s at %d: %v", c.name, limit, err)
			}
			want := c.limit
			if limit != 0 {
				want = c.at300
			}
			if v != want {
				t.Errorf("%s at %d slots: depth %v, want %d", c.name, limit, v, want)
			}
			m.Close()
		}
	}
}

// Every call into javascript leaves the TLS stack as it found it: an
// exception thrown through many frames, running out of stack, an
// interrupt deep in a recursion, and generators and async functions left
// suspended all give back every slot their frames took.
func TestTLSStackBalance(t *testing.T) {
	defer debug.SetMaxStack(debug.SetMaxStack(512 << 20))

	m := newTestVM(t)
	tls := m.runtime.tls
	base := tls.StackSlots()
	check := func(what string) {
		t.Helper()
		if n := tls.StackSlots(); n != base {
			t.Fatalf("%s: %d TLS stack slots in use, want %d", what, n, base)
		}
	}

	for _, js := range []string{
		`function f(k) { if (k == 0) throw new Error('deep'); return f(k - 1) } try { f(500) } catch (e) {}`,
		`function f(k) { if (k == 0) throw new Error('deep'); return [k - 1].map(f)[0] } try { f(200) } catch (e) {}`,
		`function f(k) { if (k == 0) throw new Error('deep'); return g.call(null, k - 1) } const g = f.bind(null); try { f(200) } catch (e) {}`,
		`function f(k) { if (k == 0) throw new Error('deep'); return f.call() } try { f(1) } catch (e) {}`,
		`function* g() { try { yield 1; yield 2 } finally { } } const it = g(); it.next()`,
		`function* g(k) { if (k) yield* g(k - 1); yield k } const it = g(50); it.next(); it.next()`,
		`async function f(k) { if (k) await f(k - 1); await new Promise(() => {}) } f(50)`,
		`new Promise(r => r(1)).then(v => { throw v })`,
	} {
		if _, err := m.Eval(`(() => { `+js+` })()`, EvalGlobal); err != nil {
			t.Fatalf("%s: %v", js, err)
		}
		check(js)
		if _, err := m.ExecutePendingJobs(); err != nil {
			t.Fatalf("%s: jobs: %v", js, err)
		}
		check(js + " (jobs)")
	}

	// An exception no script catches, from deep down.
	if _, err := m.Eval(`(() => { function f(k) { if (k == 0) throw new Error('deep'); return f(k - 1) } f(500) })()`, EvalGlobal); err == nil {
		t.Fatal("no error")
	}
	check("uncaught exception")

	// Running out of stack, caught and uncaught.
	for _, c := range depthCases {
		if _, err := m.Eval(`(() => { let n = 0; try { `+c.js+` } catch (e) { if (!(e instanceof RangeError)) throw e } })()`, EvalGlobal); err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		check(c.name)
	}
	if _, err := m.Eval(`(function f() { f() })()`, EvalGlobal); err == nil {
		t.Fatal("no stack overflow")
	}
	check("uncaught stack overflow")

	// An interrupt deep in a recursion.
	if err := m.SetEvalTimeout(20 * time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Eval(`(() => { function f(k) { if (k == 0) for (;;); return [k - 1].map(f) } f(300) })()`, EvalGlobal); !isInterrupt(err) {
		t.Fatalf("got %v, want the interrupt", err)
	}
	check("interrupt")
}

// The TLS stack of linux's libc keeps the block of every depth it has
// reached, grown to the largest frame used at that depth, until the VM
// closes, and JS_ComputeMemoryUsage does not count it. A deep recursion
// through functions with hundreds of locals, or with hundreds of bound
// arguments, leaves it about as large as one through functions with none:
// room that large stays out of the TLS frame. The 64 KB of slack are what
// compiling and calling the larger functions take at the bottom of the
// stack. On 32-bit targets a frame of JS_CallInternal may take up to 1 KB
// with smaller room, against 256 bytes without.
func TestTLSStackResident(t *testing.T) {
	vars := func(n int) string {
		var b strings.Builder
		for i := range n {
			fmt.Fprintf(&b, "var v%d = %d; ", i, i)
		}
		return b.String()
	}
	args := strings.Repeat("0, ", 200)
	for _, c := range []struct{ name, small, large string }{
		{"locals",
			`function f(k) { if (k) f(k - 1) } f(2000)`,
			`function f(k) { ` + vars(500) + `if (k) f(k - 1) } f(2000)`},
		{"bound arguments",
			`function f(k) { if (k) g(k - 1) } const g = f.bind(null); g(1000)`,
			`function f(...a) { const k = a[a.length - 1]; if (k) g(k - 1) } const g = f.bind(null, ` + args + `); g(1000)`},
	} {
		small, blocks := tlsResident(t, c.small)
		large, _ := tlsResident(t, c.large)
		limit := small + 64<<10
		if unsafe.Sizeof(uintptr(0)) == 4 {
			limit = blocks << 10
		}
		if large > limit {
			t.Errorf("%s: %d bytes of TLS blocks after the recursion, want at most %d (%d without)", c.name, large, limit, small)
		}
	}
}

// tlsResident returns the size of the TLS blocks linux's libc keeps after
// a fresh VM has run js, and how many there are.
func tlsResident(t *testing.T, js string) (size, blocks int) {
	t.Helper()
	m := newTestVM(t)
	if _, err := m.Eval(`(() => { `+js+` })()`, EvalGlobal); err != nil {
		t.Fatalf("%s: %v", js, err)
	}
	stack := reflect.ValueOf(m.runtime.tls).Elem().FieldByName("stack")
	if stack.Kind() != reflect.Slice {
		t.Skip("this libc frees the pages of its TLS stack")
	}
	stack = reflect.NewAt(stack.Type(), unsafe.Pointer(stack.UnsafeAddr())).Elem()
	for i := range stack.Len() {
		size += int(stack.Index(i).FieldByName("sz").Uint())
	}
	return size, stack.Len()
}

func isInterrupt(err error) bool {
	e, ok := err.(*Error)
	return ok && e.Interrupted
}
