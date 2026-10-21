// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package quickjs // import "github.com/liustack/quickjs"

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"testing"
	"unsafe"

	lib "github.com/liustack/quickjs/lib"
	"modernc.org/libc"
)

// enginePanic is what rawPanic panics with.
const enginePanic = "a panic in the engine"

// rawPanic is a native function that panics. It is not a Go function of
// RegisterFunc, whose panic the VM recovers before it leaves the engine,
// but called as the engine calls its own code, so its panic goes up through
// every engine function on the stack, as a bug in the engine would, for
// example a nil pointer.
func rawPanic(tls *libc.TLS, ctx uintptr, this lib.TJSValue, argc int32, argv uintptr, magic int32) lib.TJSValue {
	panic(enginePanic)
}

var rawPanicName = [...]byte{'b', 'o', 'o', 'm', 0}

// addRawPanic defines boom, which calls rawPanic, on the global object of m.
func addRawPanic(t *testing.T, m *VM) {
	t.Helper()
	addRaw(t, m, fp(rawPanic), &rawPanicName[0])
}

// rawLeak is a native function that takes a TLS frame and returns without
// freeing it, as an engine function would if fixNoDefer missed a Free.
func rawLeak(tls *libc.TLS, ctx uintptr, this lib.TJSValue, argc int32, argv uintptr, magic int32) lib.TJSValue {
	tls.Alloc(32)
	return undefined
}

var rawLeakName = [...]byte{'l', 'e', 'a', 'k', 0}

// addRaw defines the native function f as name on the global object of m.
func addRaw(t *testing.T, m *VM, f uintptr, name *byte) {
	t.Helper()
	tls := m.runtime.tls
	ctx := m.cContext
	v := lib.XJS_NewCFunction2(tls, ctx, f, uintptr(unsafe.Pointer(name)), 0, lib.EJS_CFUNC_generic_magic, 0)
	g := m.globalObject()
	defer lib.XFreeValue(tls, ctx, g)
	if lib.XJS_SetPropertyStr(tls, ctx, g, uintptr(unsafe.Pointer(name)), v) < 0 {
		t.Fatal("cannot define a native function")
	}
}

// enginePanicCases panic in the engine: right below Call, at the bottom of
// a deep recursion of javascript, which takes many TLS frames, and in
// javascript that a Go function evaluates, so that the VM recovers the
// panic in the middle of the engine.
var enginePanicCases = []struct {
	name string
	run  func(t *testing.T, m *VM)
}{
	{"call", func(t *testing.T, m *VM) { m.Call("boom") }},
	{"deep", func(t *testing.T, m *VM) {
		m.Eval(`function down(n) { return n ? [n].map(x => down(x - 1))[0] : boom() } down(300)`, EvalGlobal)
	}},
	{"nested", func(t *testing.T, m *VM) {
		if err := m.RegisterFunc("nested", func() { m.Eval(`(function down(n) { return n ? down(n - 1) : boom() })(50)`, EvalGlobal) }, false); err != nil {
			t.Fatal(err)
		}
		m.Eval(`[1, 2].map(() => nested())`, EvalGlobal)
	}},
}

// runEnginePanic runs c on m and checks that it panicked with enginePanic.
func runEnginePanic(t *testing.T, m *VM, run func(*testing.T, *VM)) {
	t.Helper()
	p := func() (p any) {
		defer func() { p = recover() }()
		run(t, m)
		return nil
	}()
	if p != enginePanic {
		t.Fatalf("panicked with %v, want %q", p, enginePanic)
	}
}

// A Go panic that goes up through the engine leaves the TLS stack of the VM
// where it was before the call, though the engine frees TLS frames on
// return only. The VM then closes, and another VM is not touched. A panic
// in the engine is a bug, and the engine's own state after it is not
// known: an embedder closes such a VM rather than using it again.
func TestEnginePanicTLS(t *testing.T) {
	for _, c := range enginePanicCases {
		t.Run(c.name, func(t *testing.T) {
			m, err := NewVM()
			if err != nil {
				t.Fatal(err)
			}
			other := newTestVM(t)
			addRawPanic(t, m)
			runEnginePanic(t, m, c.run)
			if got, want := markTLS(m.runtime.tls), m.runtime.base; got != want {
				t.Errorf("the TLS stack is at %+v after the panic, want %+v", got, want)
			}
			if got, err := other.Eval(`[1, 2, 3].map(x => x * 2).join()`, EvalGlobal); err != nil || got != "2,4,6" {
				t.Errorf("the other VM: %v, %v", got, err)
			}
			if err := m.Close(); err != nil {
				t.Fatal(err)
			}
			if got, err := other.Eval(`JSON.stringify({a: [1, {b: 2}]})`, EvalGlobal); err != nil || got != `{"a":[1,{"b":2}]}` {
				t.Errorf("the other VM after Close: %v, %v", got, err)
			}
		})
	}
}

// A call into javascript, or a Go function called from javascript, that
// returns with engine frames left on the TLS stack panics: a frame is not
// freed, or a Go function recovered a panic that went up through the engine.
// The VM is then good only for Close, as after a panic in the engine.
func TestEngineFramesLeft(t *testing.T) {
	for _, c := range []struct {
		name string
		run  func(t *testing.T, m *VM)
	}{
		{"call", func(t *testing.T, m *VM) { m.Call("leak") }},
		{"nested", func(t *testing.T, m *VM) {
			if err := m.RegisterFunc("nested", func() { m.Eval(`leak()`, EvalGlobal) }, false); err != nil {
				t.Fatal(err)
			}
			m.Eval(`[1, 2].map(() => nested())`, EvalGlobal)
		}},
		{"recovered", func(t *testing.T, m *VM) {
			if err := m.RegisterFunc("safe", func() string {
				defer func() { recover() }()
				m.Eval(`(function down(n) { return n ? down(n - 1) : boom() })(50)`, EvalGlobal)
				return "ok"
			}, false); err != nil {
				t.Fatal(err)
			}
			m.Eval(`[1, 2].map(() => safe())`, EvalGlobal)
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			m, err := NewVM()
			if err != nil {
				t.Fatal(err)
			}
			addRawPanic(t, m)
			addRaw(t, m, fp(rawLeak), &rawLeakName[0])
			// A frame not freed makes every later Free free the bytes
			// next to its own. Outside linux, where the TLS stack is
			// segments of about 4 KiB, that breaks the stack where such
			// a Free crosses the start of a segment, before the VM can
			// see it. So the frames of the test go on one segment of 1
			// MiB, as libc keeps a segment it allocated once.
			m.runtime.tls.Alloc(1 << 20)
			m.runtime.tls.Free(1 << 20)
			if p := panicOf(func() { c.run(t, m) }); p != framesLeftPanic {
				t.Fatalf("panicked with %v, want %q", p, framesLeftPanic)
			}
			checkBroken(t, m)
		})
	}
}

// A Go function that javascript called and that ends its goroutine
// (runtime.Goexit, as t.Fatal does) goes up through the engine as a panic
// does, and is not taken for a frame that is not freed.
func TestEngineGoexit(t *testing.T) {
	m, err := NewVM()
	if err != nil {
		t.Fatal(err)
	}
	if err := m.RegisterFunc("exit", func() { goruntime.Goexit() }, false); err != nil {
		t.Fatal(err)
	}
	done := make(chan any)
	go func() {
		defer func() { done <- recover() }()
		m.Eval(`function down(n) { return n ? [n].map(x => down(x - 1))[0] : exit() } down(20)`, EvalGlobal)
	}()
	if p := <-done; p != nil {
		t.Fatalf("panicked with %v", p)
	}
	checkBroken(t, m)
}

// checkBroken checks that the TLS stack of m is back where it was before
// any call, that every later call into javascript panics, and that m
// closes.
func checkBroken(t *testing.T, m *VM) {
	t.Helper()
	if got, want := markTLS(m.runtime.tls), m.runtime.base; got != want {
		t.Errorf("the TLS stack is at %+v, want %+v", got, want)
	}
	if p := panicOf(func() { m.Eval(`1 + 1`, EvalGlobal) }); p != brokenPanic {
		t.Errorf("a later Eval panicked with %v, want %q", p, brokenPanic)
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
}

func panicOf(f func()) (p any) {
	defer func() { p = recover() }()
	f()
	return nil
}

// The Go code that calls the engine frees its TLS frames before it
// returns, as the engine does, and not in a defer: a panic that goes up
// through the engine leaves the engine's frames on the stack, and a defer
// would free them in place of its own (see tlsMark).
func TestNoDeferredTLSFree(t *testing.T) {
	var files []string
	for _, pattern := range []string{"*.go", "lib/*.go"} {
		m, err := filepath.Glob(pattern)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, m...)
	}
	fset := token.NewFileSet()
	for _, fn := range files {
		if strings.HasSuffix(fn, "_test.go") || strings.HasPrefix(filepath.Base(fn), "ccgo_") {
			continue
		}
		f, err := parser.ParseFile(fset, fn, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			d, ok := n.(*ast.DeferStmt)
			if !ok {
				return true
			}
			ast.Inspect(d.Call, func(n ast.Node) bool {
				if sel, ok := n.(*ast.SelectorExpr); ok && sel.Sel.Name == "Free" {
					if id, ok := sel.X.(*ast.Ident); ok && id.Name == "tls" {
						t.Errorf("%s: a deferred tls.Free", fset.Position(sel.Pos()))
					}
				}
				return true
			})
			return true
		})
	}
}
