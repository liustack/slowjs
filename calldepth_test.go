// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package quickjs // import "github.com/liustack/quickjs"

import (
	"fmt"
	"math/bits"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	goruntime "runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"testing"
	"time"
	"unsafe"

	lib "github.com/liustack/quickjs/lib"
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

// The room of a JS_CallInternal frame for its arguments, locals and stack
// goes into the frame up to allocaMax bytes (internal/fixlib/alloca.go),
// and the frame with that room stays in the power of two block the frame
// takes alone, the block linux's libc keeps per depth. So a recursion
// through functions with any number of locals near that limit leaves the
// TLS blocks as large as one through functions with none. The frame size
// and the limit are read from the generated code, and the number of
// locals runs from a little under the limit to twice it: a larger frame or
// a higher limit grows the blocks. On 32-bit targets the frame, 160 bytes,
// may grow from a 256 byte block to a 1 KB one. Smaller frames do grow
// with room under the limit: a level through a bound function, whose frame
// is 16 bytes on 64-bit targets, may take up to about 1 KB more.
func TestTLSStackResidentNearLimit(t *testing.T) {
	frame, limit := callInternalFrame(t)
	var v lib.TJSValue // a struct on 64-bit targets, an integer on 32-bit ones
	value := int(unsafe.Sizeof(v))
	wide := unsafe.Sizeof(uintptr(0)) == 8
	if block := 1 << bits.Len(uint(frame-1)); wide && frame+limit > block {
		t.Errorf("a frame of %d bytes with up to %d of room takes more than its block of %d", frame, limit, block)
	}
	recursion := func(n int) string {
		var b strings.Builder
		for i := range n {
			fmt.Fprintf(&b, "var v%d = %d; ", i, i)
		}
		return `function f(k) { ` + b.String() + `if (k) f(k - 1) } f(1000)`
	}
	small, blocks := tlsResident(t, recursion(0))
	want := small + 64<<10
	if !wide {
		want = blocks << 10
	}
	for n := limit/value - 8; n <= 2*limit/value; n++ {
		if size, _ := tlsResident(t, recursion(n)); size > want {
			t.Errorf("%d locals: %d bytes of TLS blocks after the recursion, want at most %d (%d without)", n, size, want, small)
		}
	}
}

// callInternalFrame returns the size of the TLS frame of JS_CallInternal
// and the largest room for its arguments, locals and stack that goes into
// it, from the generated code of this target. On linux the frame takes the
// room from the entry, as frame_size := N + room, and the block that took
// it again is a comment (internal/fixlib/oneframe.go): then the room the
// entry takes must be the limit of that block, and elsewhere the block
// must still run.
func callInternalFrame(t *testing.T) (frame, limit int) {
	t.Helper()
	fn := filepath.Join("lib", "ccgo_"+goruntime.GOOS+"_"+goruntime.GOARCH+".go")
	if goruntime.GOOS == "windows" {
		fn = filepath.Join("lib", "ccgo_windows.go")
	}
	b, err := os.ReadFile(fn)
	if err != nil {
		t.Fatal(err)
	}
	i := strings.Index(string(b), "\nfunc _JS_CallInternal(")
	if i < 0 {
		t.Fatalf("%s: no _JS_CallInternal", fn)
	}
	body := string(b[i:])
	body = body[:strings.Index(body, "\n}\n")]
	f := regexp.MustCompile(`\n\tframe_size := (\d+)(?: \+ (\d+))?\n`).FindStringSubmatch(body)
	l := regexp.MustCompile(`\n\t(//)?if alloca_size <= (\d+) \{\n`).FindStringSubmatch(body)
	if f == nil || l == nil {
		t.Fatalf("%s: _JS_CallInternal has no frame size or no limit for its room", fn)
	}
	frame, _ = strconv.Atoi(f[1])
	limit, _ = strconv.Atoi(l[2])
	switch room, once := f[2], l[1] != ""; {
	case room == "" && once:
		t.Fatalf("%s: _JS_CallInternal does not take the frame again, but its entry takes no room", fn)
	case room != "" && !once:
		t.Fatalf("%s: _JS_CallInternal takes %s bytes of room at its entry and the frame again", fn, room)
	case room != "" && room != l[2]:
		t.Fatalf("%s: _JS_CallInternal takes %s bytes of room at its entry, want the limit %d", fn, room, limit)
	}
	return frame, limit
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
