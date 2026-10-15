// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package quickjs // import "github.com/liustack/quickjs"

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"unsafe"

	lib "github.com/liustack/quickjs/lib"
)

var updateErrorStack = flag.Bool("update-error-stack", false, "rewrite the testdata/error_stack*.golden files")

func TestErrorStack(t *testing.T) { testErrorStack(t) }

// testErrorStack compares the stack of errors made in many situations with
// the golden files in testdata, which hold what the engine has always
// produced. testdata/error_stack.js runs once as a script, and once as the
// body of a function called from Go, so that no top-level code is on the
// stack. It is also run under the memgrind allocator by TestMemgrind2, so it
// must create and Close its own VMs.
func testErrorStack(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("testdata", "error_stack.js"))
	if err != nil {
		t.Fatal(err)
	}

	for _, mode := range []struct {
		golden string
		call   bool
	}{
		{"error_stack.golden", false},
		{"error_stack_call.golden", true},
	} {
		got := errorStackText(t, string(src), mode.call)
		golden := filepath.Join("testdata", mode.golden)
		if *updateErrorStack {
			if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
				t.Fatal(err)
			}
		}

		want, err := os.ReadFile(golden)
		if err != nil {
			t.Fatal(err)
		}

		if got != string(want) {
			gl, wl := strings.Split(got, "\n"), strings.Split(string(want), "\n")
			for i := 0; i < len(gl) || i < len(wl); i++ {
				var g, w string
				if i < len(gl) {
					g = gl[i]
				}
				if i < len(wl) {
					w = wl[i]
				}
				if g != w {
					t.Fatalf("%s line %d:\ngot  %q\nwant %q", mode.golden, i+1, g, w)
				}
			}
		}
	}
}

// errorStackText runs src in a new VM, as a script or as the body of a
// function called from Go, and returns the stacks it collects, followed by
// stacks read through the Go API.
func errorStackText(t *testing.T, src string, call bool) string {
	m, err := NewVM()
	if err != nil {
		t.Fatal(err)
	}

	defer m.Close()

	if err := m.RegisterFunc("gc", func() { lib.XJS_RunGC(m.runtime.tls, m.runtime.cRuntime) }, false); err != nil {
		t.Fatal(err)
	}

	if call {
		if _, err := m.Eval("function run() {\n"+src+"\n}", EvalGlobal); err != nil {
			t.Fatal(err)
		}

		if _, err := m.Call("run"); err != nil {
			t.Fatal(err)
		}
	} else if _, err := m.Eval(src, EvalGlobal); err != nil {
		t.Fatal(err)
	}

	if _, err := m.ExecutePendingJobs(); err != nil {
		t.Fatal(err)
	}

	result, err := m.Eval("globalThis.result", EvalGlobal)
	if err != nil {
		t.Fatal(err)
	}

	got, ok := result.(string)
	if !ok {
		t.Fatalf("result = %T %v, want a string", result, result)
	}

	var b strings.Builder
	b.WriteString(got)
	section := func(label string, err error) {
		var e *Error
		if !errors.As(err, &e) {
			t.Fatalf("%s: want *Error, got %T %v", label, err, err)
		}
		fmt.Fprintf(&b, "\n== go: %s\n%s: %s\n%s", label, e.Name, e.Message, e.Stack)
	}

	// The stack read by ErrorFromValue when Eval and Call fail.
	_, err = m.Eval("function thrower(m) { throw new TypeError(m) }\nfunction outer() { thrower('eval') }\nouter()", EvalGlobal)
	section("eval", err)
	_, err = m.Call("thrower", "call")
	section("call", err)
	_, err = m.Eval("undefined.x", EvalGlobal)
	section("engine", err)

	// A host function that fails is a native frame.
	if err := m.RegisterHostFunc("hostFail", func(args []any) (any, error) { return nil, errors.New("host failed") }); err != nil {
		t.Fatal(err)
	}

	_, err = m.Eval("function callsHost() { hostFail() }\ncallsHost()", EvalGlobal)
	section("host function", err)
	_, err = m.Call("callsHost")
	section("host function called", err)
	v, err := m.Eval("(function () { try { callsHost() } catch (e) { return e.stack } })()", EvalGlobal)
	if err != nil {
		t.Fatal(err)
	}

	fmt.Fprintf(&b, "\n== go: host function caught\n%v", v)

	// A rejected promise reached through ExecutePendingJobs.
	if _, err := m.Eval("Promise.resolve().then(function job() { null.y })", EvalGlobal); err != nil {
		t.Fatal(err)
	}

	if _, err := m.ExecutePendingJobs(); err != nil {
		t.Fatal(err)
	}

	return b.String() + "\n"
}

// vmMemory runs the GC and returns the memory usage of the runtime of m.
func vmMemory(m *VM) lib.TJSMemoryUsage {
	tls, rt := m.runtime.tls, m.runtime.cRuntime
	lib.XJS_RunGC(tls, rt)
	// The engine writes the result, so it goes in C memory.
	n := int(unsafe.Sizeof(lib.TJSMemoryUsage{}))
	p := tls.Alloc(n)
	defer tls.Free(n)
	lib.XJS_ComputeMemoryUsage(tls, rt, p)
	return *(*lib.TJSMemoryUsage)(unsafe.Pointer(p))
}

// TestErrorStackMemory checks that errors whose stack is never read are freed,
// with the functions in their stack, including when an error and a closure
// from its own stack reference each other.
func TestErrorStackMemory(t *testing.T) {
	m, err := NewVM()
	if err != nil {
		t.Fatal(err)
	}

	defer m.Close()

	usage := func() (malloc, objects int64) {
		u := vmMemory(m)
		return int64(u.Fmalloc_size), int64(u.Fobj_count)
	}

	// make is called from Go, so no top-level code is on the stack of the
	// errors, and they keep the functions on their stack until freed.
	if _, err := m.Eval(`
function deep(n, f) { return n == 0 ? f() : deep(n - 1, f) }
var kept
function make(n) {
	kept = []
	for (var i = 0; i < n; i++) {
		kept.push(deep(20, function () {
			// a function whose bytecode only the error keeps
			var err = new Function("return new Error('e')")()
			// the closure holds the error, and the error holds the closure's frame
			err.self = function () { return err }
			return err
		}))
	}
	return kept.length
}
function drop() { kept = undefined }
`, EvalGlobal); err != nil {
		t.Fatal(err)
	}

	// One round first, so that what the first call leaves behind for good
	// (shapes, caches) is part of the baseline.
	for _, call := range []struct {
		name string
		args []any
	}{{"make", []any{1}}, {"drop", nil}} {
		if _, err := m.Call(call.name, call.args...); err != nil {
			t.Fatal(err)
		}
	}

	baseMalloc, baseObjects := usage()
	if _, err := m.Call("make", 2000); err != nil {
		t.Fatal(err)
	}

	heldMalloc, heldObjects := usage()
	if _, err := m.Call("drop"); err != nil {
		t.Fatal(err)
	}

	freedMalloc, freedObjects := usage()
	t.Logf("2000 errors: %d bytes and %d objects held, %d bytes and %d objects left after freeing (baseline %d, %d)",
		heldMalloc-baseMalloc, heldObjects-baseObjects, freedMalloc-baseMalloc, freedObjects-baseObjects, baseMalloc, baseObjects)
	if heldObjects-baseObjects < 2000 {
		t.Fatalf("the errors were not kept: %d objects", heldObjects-baseObjects)
	}

	if freedObjects != baseObjects {
		t.Errorf("%d objects left after freeing the errors", freedObjects-baseObjects)
	}

	// The allocator may keep a little memory for reuse.
	if d := freedMalloc - baseMalloc; d > 64<<10 {
		t.Errorf("%d bytes left after freeing the errors", d)
	}
}

// TestErrorStackTopLevel checks that an error made while a script runs, and
// kept after it ends, does not keep the code of the script alive. Libraries
// make such errors when they load (React keeps one to signal suspense).
func TestErrorStackTopLevel(t *testing.T) {
	var funcs strings.Builder
	for i := range 200 {
		fmt.Fprintf(&funcs, "\tfunction f%d(x) { return [x, %d, 'padding to make the code larger'] }\n", i, i)
	}

	// The size of the functions kept after a script keeps the value of make.
	kept := func(make string) int64 {
		m, err := NewVM()
		if err != nil {
			t.Fatal(err)
		}

		defer m.Close()

		before := vmMemory(m)
		script := "(function load() {\n" + funcs.String() + "\tfunction make() { return " + make + " }\n\tglobalThis.sentinel = make()\n})()\n"
		if _, err := m.Eval(script, EvalGlobal); err != nil {
			t.Fatal(err)
		}

		after := vmMemory(m)
		return after.Fjs_func_size + after.Fjs_func_code_size + after.Fjs_func_pc2line_size -
			before.Fjs_func_size - before.Fjs_func_code_size - before.Fjs_func_pc2line_size
	}

	object, keptErr := kept(`{ message: "sentinel" }`), kept(`new Error("sentinel")`)
	t.Logf("functions kept: %d bytes after keeping an object, %d after keeping an error", object, keptErr)
	if keptErr != object {
		t.Fatalf("keeping an error made by the script keeps %d bytes of functions, want %d", keptErr, object)
	}
}

func TestErrorStackOutOfMemory(t *testing.T) { testErrorStackOutOfMemory(t) }

// testErrorStackOutOfMemory reads the stack of an error when the VM has no
// memory left. Reading it may fail with an out of memory error, after which
// the VM works as before once memory is free again, and the stack can be
// read. It is also run under the memgrind allocator by TestMemgrind2.
func testErrorStackOutOfMemory(t *testing.T) {
	m, err := NewVM()
	if err != nil {
		t.Fatal(err)
	}

	defer m.Close()

	m.SetMemoryLimit(4 << 20)
	// The error is made in a call from Go, so its stack is not formatted yet.
	if _, err := m.Eval(`
var e = []
function deep(n) { return n ? deep(n - 1) : new Error("deep") }
function make() { for (var i = 0; i < 2; i++) e.push(deep(40)) }
`, EvalGlobal); err != nil {
		t.Fatal(err)
	}

	if _, err := m.Call("make"); err != nil {
		t.Fatal(err)
	}

	v, err := m.Eval(`
var fill = []
function exhaust(size) {
	try {
		for (;;) fill.push("x".repeat(size) + fill.length)
	} catch (x) {}
}
exhaust(4096)
exhaust(64)
exhaust(1)
var read, caught
try {
	read = typeof e[0].stack
} catch (x) {
	caught = x
}
fill = null
caught === undefined ? read : "threw " + caught.message
`, EvalGlobal)
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("reading the stack with no memory left: %v", v)
	if v != "string" && v != "threw out of memory" {
		t.Fatalf("got %v", v)
	}

	// The stack reads as the stack of the same error made the same way.
	v, err = m.Eval(`e[0].stack === e[1].stack && e[0].stack.split("\n").length`, EvalGlobal)
	if err != nil {
		t.Fatal(err)
	}

	if v != 43 {
		t.Fatalf("reading the stack again gives %v", v)
	}

	// The VM works again, and so do new stacks.
	v, err = m.Eval(`(function f() { return new Error("after").stack })()`, EvalGlobal)
	if err != nil {
		t.Fatal(err)
	}

	if g, w := v, "    at f (<eval>:1:33)\n    at <eval> (<eval>:1:51)\n"; g != w {
		t.Fatalf("got %q, want %q", g, w)
	}
}

// TestErrorStackConcurrent makes and reads stacks in several VMs at once, one
// goroutine each, and checks that every VM gets the text of its own frames.
func TestErrorStackConcurrent(t *testing.T) {
	const vms, rounds = 8, 200
	var wg sync.WaitGroup
	errs := make(chan error, vms)
	for i := range vms {
		wg.Add(1)
		go func() {
			defer wg.Done()
			m, err := NewVM()
			if err != nil {
				errs <- err
				return
			}

			defer m.Close()

			// Each VM has its own function names, so a frame recorded by
			// another VM would show up in the text.
			if _, err := m.Eval(fmt.Sprintf(`
var kept = []
function vm%[1]d_inner(n) { return new Error("e" + n) }
function vm%[1]d_outer(n) { kept.push(vm%[1]d_inner(n)); if (kept.length > 50) kept.shift(); return kept[kept.length - 1].stack }
`, i), EvalGlobal); err != nil {
				errs <- err
				return
			}

			want := fmt.Sprintf("    at vm%[1]d_inner (<eval>:3:41)\n    at vm%[1]d_outer (<eval>:4:44)\n", i)
			for n := range rounds {
				v, err := m.Call(fmt.Sprintf("vm%d_outer", i), n)
				if err != nil {
					errs <- err
					return
				}

				if v != want {
					errs <- fmt.Errorf("vm %d round %d: got %q, want %q", i, n, v, want)
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

// TestErrorStackFirstUseRace has one VM make the first pending stack of the
// process while other VMs read the 'stack' property of plain objects, which
// checks for a pending stack. Under -race it fails if the first use sets up
// anything those reads look at. Run it on its own (-run) so that no other
// test has made a stack first.
func TestErrorStackFirstUseRace(t *testing.T) {
	const vms = 8
	ms := make([]*VM, vms)
	for i := range ms {
		m, err := NewVM()
		if err != nil {
			t.Fatal(err)
		}

		defer m.Close()

		if _, err := m.Eval(`
var plain = { stack: 17 }
function read() { let s = 0; for (let i = 0; i < 1000; i++) s += plain.stack; return s }
function create() { return new Error("first") }
`, EvalGlobal); err != nil {
			t.Fatal(err)
		}

		ms[i] = m
	}
	start := make(chan struct{})
	var wg sync.WaitGroup
	errs := make(chan error, vms)
	for i, m := range ms {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if i == 0 {
				v, err := m.CallValue("create")
				if err != nil {
					errs <- err
					return
				}

				v.Free()
				return
			}

			for range 50 {
				if _, err := m.Call("read"); err != nil {
					errs <- err
					return
				}
			}
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}
