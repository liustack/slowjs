// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package libquickjs // import "github.com/liustack/quickjs/lib"

import (
	"testing"
	"unsafe"

	"modernc.org/libc"
)

// testdata/funcshape.js lists the own properties of new functions of every
// kind, made many times, and of functions changed after they were made.
// testdata/funcshape.txt is what Node gives, and the engine before
// fixFuncShape gives the same.
func TestFuncShapeSemantics(t *testing.T) {
	matchTranscript(t, "funcshape")
}

// shapeRefs returns the count of the shape sh.
func shapeRefs(tls *libc.TLS, sh uintptr) int32 {
	return (*TJSMallocBlockHeader)(unsafe.Pointer(_js_rc(tls, sh))).Fref_count
}

// funcShapeOf returns the shape of the object v.
func funcShapeOf(v TJSValue) uintptr {
	return (*TJSObject)(unsafe.Pointer(jsvPtr(v))).Fshape
}

// funcShape remembers the shapes of the four prototypes of new functions,
// and puts the functions made after onto the shape it remembers, which it
// holds, and which is the one add_property gave the first. Constructors
// and generators get a prototype property after that, which moves them on:
// the arrow and the async function show the shape.
func TestFuncShapeRemembers(t *testing.T) {
	tls := libc.NewTLS()
	defer tls.Close()
	rt := XJS_NewRuntime(tls)
	defer XJS_FreeRuntime(tls, rt)
	ctx := XJS_NewContext(tls, rt)
	defer XJS_FreeContext(tls, ctx)

	fs := &forkOf(rt).fn
	for i, e := range fs {
		if e.empty != 0 && i != 0 {
			t.Fatalf("slot %d is set before any function of its kind", i)
		}
	}
	for i, c := range []struct {
		js        string
		generator bool // or constructor
	}{
		{"((a) => a)", false},
		{"(async function f(a) {})", false},
		{"(function* f(a) {})", true},
		{"(async function* f(a) {})", true},
	} {
		first := eval0(t, tls, ctx, c.js)
		e := fs[i]
		if e.ctx != ctx || e.empty == 0 || e.both == 0 {
			t.Fatalf("%s: slot %d is %+v", c.js, i, e)
		}
		if got := funcShapeOf(first); got != e.both && !c.generator {
			t.Errorf("%s: the first function has shape %#x, the slot %#x", c.js, got, e.both)
		}
		empty, both := shapeRefs(tls, e.empty), shapeRefs(tls, e.both)
		next := eval0(t, tls, ctx, c.js)
		if fs[i] != e {
			t.Errorf("%s: the slot changed for a function of the same kind", c.js)
		}
		if got := shapeRefs(tls, e.empty); got != empty {
			t.Errorf("%s: the empty shape has %d references after one more function, want %d", c.js, got, empty)
		}
		if c.generator {
			if got := shapeRefs(tls, e.both); got != both {
				t.Errorf("%s: the shape has %d references after one more generator, want %d", c.js, got, both)
			}
		} else {
			if got := funcShapeOf(next); got != e.both {
				t.Errorf("%s: the next function has shape %#x, the slot %#x", c.js, got, e.both)
			}
			if got := shapeRefs(tls, e.both); got != both+1 {
				t.Errorf("%s: the shape has %d references after one more function, want %d", c.js, got, both+1)
			}
		}
		XFreeValue(tls, ctx, next)
		XFreeValue(tls, ctx, first)
	}
	// The cycle collector sees the references for the context, and frees
	// nothing of a live one.
	refs := [4][2]int32{}
	for i, e := range fs {
		refs[i] = [2]int32{shapeRefs(tls, e.empty), shapeRefs(tls, e.both)}
	}
	XJS_RunGC(tls, rt)
	for i, e := range fs {
		if got := [2]int32{shapeRefs(tls, e.empty), shapeRefs(tls, e.both)}; got != refs[i] || e.ctx != ctx {
			t.Errorf("slot %d after the cycle collector: references %v, want %v", i, got, refs[i])
		}
	}
	if got := eval1(t, tls, ctx, "var h = function (a, b, c) {}; [h.length, h.name, Object.getOwnPropertyNames(h)].join()"); got != "3,h,length,name,prototype" {
		t.Errorf("a function after the cycle collector: %s", got)
	}
}

// funcShape leaves what is not an ordinary extensible object, or has no
// empty shape of a prototype of new functions, to js_function_set_properties.
func TestFuncShapeRefuses(t *testing.T) {
	tls := libc.NewTLS()
	defer tls.Close()
	rt := XJS_NewRuntime(tls)
	defer XJS_FreeRuntime(tls, rt)
	ctx := XJS_NewContext(tls, rt)
	defer XJS_FreeContext(tls, ctx)

	if funcShape(tls, ctx, jsvMkPtr(EJS_TAG_INT, 1), EJS_ATOM_empty_string, 0) {
		t.Error("an int taken")
	}
	for _, js := range []string{
		"({})",                            // the prototype is Object.prototype
		"Object.setPrototypeOf({}, null)", // no prototype
		"Object.preventExtensions(Object.create(Function.prototype))", // not extensible
		"Object.setPrototypeOf([], Function.prototype)",               // a fast array
		"new Proxy(function () {}, {})",                               // exotic
		"var o = Object.create(Function.prototype); o.x = 1; o",       // a key already
		"globalThis",
	} {
		obj := eval0(t, tls, ctx, js)
		sh := funcShapeOf(obj)
		if funcShape(tls, ctx, obj, EJS_ATOM_empty_string, 0) {
			t.Errorf("%s taken", js)
		}
		if funcShapeOf(obj) != sh {
			t.Errorf("%s: the shape changed", js)
		}
		XFreeValue(tls, ctx, obj)
	}
	for i, e := range forkOf(rt).fn {
		if i != 0 && e.empty != 0 {
			t.Errorf("slot %d set by an object refused", i)
		}
	}
}

// The shapes held for a context hold its Function.prototype, and with it
// the context. Once the context is freed, the cycle collector frees it, and
// JS_FreeContext lets go of the shapes. funcShape starts again in the next
// context.
func TestFuncShapesFree(t *testing.T) {
	tls := libc.NewTLS()
	defer tls.Close()
	rt := XJS_NewRuntime(tls)
	defer XJS_FreeRuntime(tls, rt)
	ctx1 := XJS_NewContext(tls, rt)
	defer XJS_FreeContext(tls, ctx1)
	fs := &forkOf(rt).fn

	XFreeValue(tls, ctx1, eval0(t, tls, ctx1, "(function () {}); (function () {})"))
	first := fs[0]
	if first.ctx != ctx1 {
		t.Fatal("no shape held for the first context")
	}
	ctx2 := XJS_NewContext(tls, rt)
	f := eval0(t, tls, ctx2, "(async function () {}); ((a, b) => a)")
	if fs[0].ctx != ctx2 || fs[0].both != funcShapeOf(f) || fs[1].ctx != ctx2 {
		t.Error("the second context does not take the slots")
	}
	XFreeValue(tls, ctx2, f)
	XJS_FreeContext(tls, ctx2)
	XJS_RunGC(tls, rt)
	for i, e := range fs {
		if e != (struct{ ctx, empty, both uintptr }{}) {
			t.Errorf("slot %d kept after the second context was freed: %+v", i, e)
		}
	}
	g := eval0(t, tls, ctx1, "((a) => a)")
	if fs[0].ctx != ctx1 || fs[0].both != funcShapeOf(g) {
		t.Error("the first context does not take the slot again")
	}
	if got := eval1(t, tls, ctx1, "var h = function (a, b, c) {}; [h.length, h.name, Object.getOwnPropertyNames(h)].join()"); got != "3,h,length,name,prototype" {
		t.Errorf("a function after the slot was let go: %s", got)
	}
	XFreeValue(tls, ctx1, g)
}
