// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package quickjs // import "github.com/liustack/quickjs"

import (
	"fmt"
	"strings"
	"testing"

	lib "github.com/liustack/quickjs/lib"
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
