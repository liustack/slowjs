// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package quickjs // import "github.com/liustack/quickjs"

import (
	"fmt"
	"strings"
	"testing"
	"unsafe"

	lib "github.com/liustack/quickjs/lib"
	"modernc.org/libc"
)

// compileScripts use the parts of the compiler that move or rewrite code:
// loops, labels, scopes, closures, classes, regexps, templates, async code
// and eval.
var compileScripts = []string{
	`class A { #p = 1; get p() { return this.#p } static s = [1,2,3].map(x => x * 2) }; const a = new A(); JSON.stringify({p: a.p, s: A.s})`,
	`function f(a, {b, c = 2}, ...r) { let s = 0; for (const x of r) s += x; return a + b + c + s } f(1, {b: 2}, 3, 4, 5)`,
	`const re = /(\d+)-(\w+)/g; 'a 12-ab 34-cd'.replace(re, (m, d, w) => w + d) + new RegExp('x(y)+', 'i').test('XYY')`,
	"const t = (s, ...v) => s.raw.join('|') + v.join(); t`a${1}b${2}c` + `x${[1,2].join()}`",
	`(async () => { const r = await Promise.all([1, 2, Promise.resolve(3)]); return r.reduce((a, b) => a + b) })().then(v => globalThis.out = v)`,
	`const m = new Map([[1, 'a']]); const s = new Set([1, 2]); let o = {...{x: 1}, y: [...s]}; try { null.x } catch { o.z = m.get(1) } JSON.parse(JSON.stringify(o))`,
	`function* g() { let i = 0; while (i < 5) yield i++ } [...g()].map(String).join() + Array.from({length: 3}, (_, i) => i * i)`,
	`eval('var q = 1; q + 1') + new Function('a', 'b', 'return a * b')(3, 4)`,
	`with ({w: 1}) { w + 1 }`,
	`label: for (let i = 0; i < 3; i++) { for (let j = 0; j < 3; j++) { if (j == 1) continue label; if (i == 2) break label } } typeof undefinedThing`,
	`for (const k in {a: 1, b: 2}) { for (var i = 0, n = 0; i < 2; i++) n += i } try { undefinedThing } catch (e) { e.name }`,
	// Scopes in scopes, which grow the function's scope array.
	`function f() { ` + strings.Repeat(`{ let a = 1; for (let i = 0; i < 1; i++) { try { a++ } catch (e) { with ({}) { `, 8) + strings.Repeat(`} } } } `, 8) + `} f()`,
}

// Compiling with little memory left either works or fails with the out of
// memory error: the engine does not crash, does not report a syntax error
// the script does not have, and the VM works afterwards.
func TestCompileOutOfMemory(t *testing.T) {
	for i, js := range compileScripts {
		failed := 0
		for room := uintptr(0); room <= 48<<10; room += 32 {
			if compileWithRoom(t, i, js, room) != nil {
				failed++
			}
		}
		if failed == 0 {
			t.Fatalf("script %d never ran out of memory", i)
		}
	}
}

// compileFineScripts run out of memory where a sweep in steps of 32 bytes
// of memory left misses it.
var compileFineScripts = []string{
	// A class constructor with fields and a parameter with a default. When
	// the code of the fields failed, opening the scope of the parameters
	// failed without an exception.
	`class A { x = 1; constructor(a = 1) { this.a = a } }; new A().a`,
	// A derived class constructor with a parameter with a default. When
	// writing the constant pool index of the constructor failed, the parser
	// put it in place at the end of the code anyway, after the bytes the
	// buffer holds.
	`class B { constructor() {} }; class A extends B { constructor(a = 1) { super(); this.a = a } }; new A().a`,
	// When adding an atom to the atom table of the bytecode failed, the
	// writer wrote on and gave the bytecode, with the error left pending.
	`for (var [a = 1] of [[]]) ; a`,
}

// TestCompileOutOfMemoryFine is TestCompileOutOfMemory in steps of 4 bytes,
// from no memory left up to well past where the script first runs.
func TestCompileOutOfMemoryFine(t *testing.T) {
	for i, js := range compileFineScripts {
		failed, ran := 0, 0
		for room := uintptr(0); ran < 512; room += 4 {
			if compileWithRoom(t, i, js, room) != nil {
				failed++
				ran = 0
			} else {
				ran++
			}
		}
		if failed == 0 {
			t.Fatalf("script %d never ran out of memory", i)
		}
	}
}

// compileWithRoom evals, then compiles, script i, js, with room bytes of
// memory left, and checks that it fails only with the out of memory error,
// takes the error out of the context and leaves the VM working.
func compileWithRoom(t *testing.T, i int, js string, room uintptr) (err error) {
	t.Helper()
	m, err := NewVM()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if p := recover(); p != nil {
			t.Fatalf("script %d, room %d: panic: %v", i, room, p)
		}
	}()

	m.SetMemoryLimit(uintptr(vmMemory(m).Fmalloc_size) + room)
	_, err = m.Eval(js, EvalGlobal)
	if err == nil {
		_, err = m.ExecutePendingJobs()
	}
	if err == nil {
		_, err = m.Compile(js, EvalGlobal)
	}
	m.SetMemoryLimit(^uintptr(0))
	if err != nil {
		wantOutOfMemory(t, fmt.Sprintf("script %d, room %d", i, room), err)
	}

	wantNoPendingException(t, m)
	if v, err := m.Eval(`1 + 1`, EvalGlobal); err != nil || v != 2 {
		t.Fatalf("script %d, room %d: 1 + 1 afterwards: %v, %v", i, room, v, err)
	}
	m.Close()
	return err
}

// Writing the bytecode of a compiled script with little memory left either
// gives bytecode that runs or fails with the out of memory error. The writer
// used to fail without an exception, and could return bytecode cut short.
func TestWriteBytecodeOutOfMemory(t *testing.T) {
	var sb strings.Builder
	for i := range 3000 {
		fmt.Fprintf(&sb, "var v%s%d = 'str%d';\n", strings.Repeat("x", i%50), i, i)
	}
	js := sb.String()

	failed := 0
	for room := uintptr(0); room <= 1<<20; room += 2048 {
		m, err := NewVM()
		if err != nil {
			t.Fatal(err)
		}

		v, err := m.eval(js, EvalGlobal|lib.MJS_EVAL_FLAG_COMPILE_ONLY)
		if err != nil || isException(v) {
			t.Fatal(err)
		}
		m.SetMemoryLimit(uintptr(vmMemory(m).Fmalloc_size) + room)
		bc, err := m.writeBytecode(v)
		m.SetMemoryLimit(^uintptr(0))
		lib.XFreeValue(m.runtime.tls, m.cContext, v)
		if err != nil {
			failed++
			wantOutOfMemory(t, "writing bytecode", err)
		}
		wantNoPendingException(t, m)
		m.Close()

		if err == nil {
			m, err := NewVM()
			if err != nil {
				t.Fatal(err)
			}
			if _, err := m.EvalBytecode(bc); err != nil {
				t.Fatalf("room %d: the bytecode written fails: %v", room, err)
			}
			m.Close()
		}
	}

	if failed == 0 {
		t.Fatal("writing never ran out of memory")
	}
}

// failAt is the allocation that the failing allocator fails, counting from
// 1 since allocs was last set to 0, or 0 to fail none.
var failAt, allocs int

// failing reports whether the failing allocator fails the allocation it is
// about to make.
func failing() bool {
	allocs++
	return allocs == failAt
}

func failingMalloc(tls *libc.TLS, s uintptr, size lib.Tsize_t) uintptr {
	if failing() {
		return 0
	}
	return libc.Xmalloc(tls, libc.Tsize_t(size))
}

func failingFree(tls *libc.TLS, s, p uintptr) {
	libc.Xfree(tls, p)
}

func failingRealloc(tls *libc.TLS, s, p uintptr, size lib.Tsize_t) uintptr {
	if size == 0 {
		libc.Xfree(tls, p)
		return 0
	}
	if failing() {
		return 0
	}
	return libc.Xrealloc(tls, p, libc.Tsize_t(size))
}

func failingUsableSize(tls *libc.TLS, p uintptr) lib.Tsize_t {
	return lib.Tsize_t(libc.Xmalloc_usable_size(tls, p))
}

// compileFailing compiles js, what, in a new runtime whose allocation number fail
// of the compile fails, 0 for none, and runs the code compiled, with no
// allocation failing. It returns the result of the code as a string, or
// failed, and the number of allocations of the compile. A compile may only
// fail with the out of memory error.
func compileFailing(t *testing.T, what, js string, fail int) (result string, failed bool, n int) {
	t.Helper()
	tls := libc.NewTLS()
	defer tls.Close()
	mf := lib.TJSMallocFunctions{
		Fjs_malloc:             fp(failingMalloc),
		Fjs_free:               fp(failingFree),
		Fjs_realloc:            fp(failingRealloc),
		Fjs_malloc_usable_size: fp(failingUsableSize),
	}
	rt := lib.XJS_NewRuntime2(tls, uintptr(unsafe.Pointer(&mf)), 0)
	ctx := lib.XJS_NewContext(tls, rt)
	if rt == 0 || ctx == 0 {
		t.Fatal("cannot make a runtime")
	}
	defer lib.XJS_FreeRuntime(tls, rt)
	defer lib.XJS_FreeContext(tls, ctx)
	src, err := libc.CString(js)
	if err != nil {
		t.Fatal(err)
	}
	defer libc.Xfree(tls, src)

	func() {
		defer func() {
			failAt = 0
			if p := recover(); p != nil {
				t.Fatalf("%s, allocation %d failing: panic: %v", what, fail, p)
			}
		}()
		allocs, failAt = 0, fail
		v := lib.XJS_Eval(tls, ctx, src, libc.Tsize_t(len(js)), uintptr(unsafe.Pointer(&evalFN)), EvalGlobal|lib.MJS_EVAL_FLAG_COMPILE_ONLY)
		n = allocs
		if isException(v) {
			failed = true
			if lib.XJS_HasException(tls, ctx) == 0 || lib.XJS_IsUncatchableException(tls, ctx) == 0 {
				t.Fatalf("%s, allocation %d failing: the compile fails without the out of memory error", what, fail)
			}
			lib.XFreeValue(tls, ctx, lib.XJS_GetException(tls, ctx))
			return
		}
		if lib.XJS_HasException(tls, ctx) != 0 {
			t.Fatalf("%s, allocation %d failing: the compile works, but leaves an exception", what, fail)
		}
		failAt = 0
		v = lib.XJS_EvalFunction(tls, ctx, v)
		if isException(v) {
			lib.XFreeValue(tls, ctx, lib.XJS_GetException(tls, ctx))
			result = "exception"
			return
		}
		s := lib.XJS_ToCStringLen2(tls, ctx, 0, v, 0)
		result = libc.GoString(s)
		lib.XJS_FreeCString(tls, ctx, s)
		lib.XFreeValue(tls, ctx, v)
	}()
	return result, failed, n
}

// Compiling with one allocation failing, and the others working, fails
// with the out of memory error or gives code that works as it should. A
// failing allocation followed by working ones is what a large allocation
// failing under the memory limit, as small ones still fit, looks like.
func TestCompileOneFailure(t *testing.T) {
	scripts := append(append([]string{}, compileScripts...), compileFineScripts...)
	scripts = append(scripts,
		`function f() { let n = 0; `+strings.Repeat(`{ let a = 1; n += a } `, 400)+`return n } f()`,
		strings.Repeat(`{ let a = 1; `, 300)+strings.Repeat(`} `, 300)+`1`,
		`class A { #m() { return 1 } `+strings.Repeat(`x = 1; `, 200)+`y() { return this.#m() } }; new A().y()`,
		`switch (2) { `+strings.Repeat(`case 1: break; `, 300)+`default: 3 }`,
	)
	for i, js := range scripts {
		what := fmt.Sprintf("script %d", i)
		want, failed, n := compileFailing(t, what, js, 0)
		if failed {
			t.Fatalf("script %d does not compile", i)
		}
		failures := 0
		for fail := 1; fail <= n; fail++ {
			got, failed, _ := compileFailing(t, what, js, fail)
			if failed {
				failures++
			} else if got != want {
				t.Fatalf("script %d, allocation %d failing: the code compiled gives %q, want %q", i, fail, got, want)
			}
		}
		if failures == 0 {
			t.Fatalf("script %d: no compile of its %d allocations failed", i, n)
		}
	}
}
