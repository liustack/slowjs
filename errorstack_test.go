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
	"testing"
	"unsafe"

	lib "github.com/liustack/quickjs/lib"
)

var updateErrorStack = flag.Bool("update-error-stack", false, "rewrite testdata/error_stack.golden")

func TestErrorStack(t *testing.T) { testErrorStack(t) }

// testErrorStack compares the stack of errors made in many situations with
// testdata/error_stack.golden, which holds what the engine has always
// produced. It is also run under the memgrind allocator by TestMemgrind2, so
// it must create and Close its own VM.
func testErrorStack(t *testing.T) {
	m, err := NewVM()
	if err != nil {
		t.Fatal(err)
	}

	defer m.Close()

	if err := m.RegisterFunc("gc", func() { lib.XJS_RunGC(m.runtime.tls, m.runtime.cRuntime) }, false); err != nil {
		t.Fatal(err)
	}

	src, err := os.ReadFile(filepath.Join("testdata", "error_stack.js"))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := m.Eval(string(src), EvalGlobal); err != nil {
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

	got = b.String() + "\n"
	golden := filepath.Join("testdata", "error_stack.golden")
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
				t.Fatalf("line %d:\ngot  %q\nwant %q", i+1, g, w)
			}
		}
	}
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

	tls, rt := m.runtime.tls, m.runtime.cRuntime
	usage := func() (malloc, objects int64) {
		lib.XJS_RunGC(tls, rt)
		var u lib.TJSMemoryUsage
		lib.XJS_ComputeMemoryUsage(tls, rt, uintptr(unsafe.Pointer(&u)))
		return int64(u.Fmalloc_size), int64(u.Fobj_count)
	}

	if _, err := m.Eval(`
function deep(n, f) { return n == 0 ? f() : deep(n - 1, f) }
function make(n) {
	var kept = []
	for (var i = 0; i < n; i++) {
		kept.push(deep(20, function () {
			// eval makes a function whose bytecode only the error keeps
			var err = eval("(function inEval() { return new Error('e') })()")
			// the closure holds the error, and the error holds the closure's frame
			err.self = function () { return err }
			return err
		}))
	}
	return kept
}
var kept
`, EvalGlobal); err != nil {
		t.Fatal(err)
	}

	// One round first, so that what the first call leaves behind for good
	// (shapes, the eval cache) is part of the baseline.
	if _, err := m.Eval("kept = make(1); kept = undefined", EvalGlobal); err != nil {
		t.Fatal(err)
	}

	baseMalloc, baseObjects := usage()
	if _, err := m.Eval("kept = make(2000); kept.length", EvalGlobal); err != nil {
		t.Fatal(err)
	}

	heldMalloc, heldObjects := usage()
	if _, err := m.Eval("kept = undefined", EvalGlobal); err != nil {
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
