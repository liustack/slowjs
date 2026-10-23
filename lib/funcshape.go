// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT AND BSD-3-Clause
// The fork's code here is under the MIT license in LICENSE-MIT at the root of
// this repository. Parts follow QuickJS (Copyright (c) 2017-2021 Fabrice
// Bellard and Charlie Gordon, MIT, see LICENSE-LIBQUICKJS) as translated by
// modernc.org/libquickjs (The libquickjs-go Authors, BSD-3-Clause, see
// lib/LICENSE).

package libquickjs // import "github.com/liustack/slowjs/lib"

import (
	"unsafe"

	"modernc.org/libc"
)

// Every new function gets two properties, length and name, from
// js_function_set_properties, each through JS_DefinePropertyValue and
// add_property. A new function starts with the empty shape of its
// prototype, and the two keys move it, through a shape with length only, to
// the shape with both. Nothing else holds the empty shape or the shape with
// length only for long, so they are made again for most functions and freed
// again as the function moves on, and each step looks up the shape hash
// table.
//
// funcShape remembers, for each of the four prototypes of new functions
// (Function.prototype and the prototypes of async, generator and async
// generator functions), the empty shape of the prototype and the shape with
// both keys, and puts a new function with the empty shape straight onto the
// shape with both. internal/fixlib makes js_function_set_properties call it
// first (see fixFuncShape).
//
// A shape is shared by every object that has it and is never changed while
// more than one holds it: add_property, js_shape_prepare_update and the
// others copy a shared shape before they change it. funcShape holds each of
// the shapes it remembers, so they stay as they are, and the empty shape
// cannot be freed and its memory given to another shape: a pointer equal to
// a remembered empty shape is that shape. The shape with both keys is the
// one add_property finds after the empty shape, with the same prototype, the
// same keys in the same order and the same flags, and the same size of the
// property array, so the function is as add_property leaves it.
//
// The shapes are held for the context whose new function funcShape saw
// first, as the context holds the shapes of its arrays and arguments
// objects: JS_MarkContext calls funcShapesMark, which shows the cycle
// collector the references to them, and JS_FreeContext lets them go with
// the shapes of the context, through funcShapesFree. So a context the
// shapes hold, through their prototypes, is freed as it would be without
// them.

// TfuncShapes is what funcShape remembers in a runtime (see runtime.go):
// for each prototype, the empty shape and the shape with length and name,
// and the context that holds them, or zeros.
type TfuncShapes [4]struct{ ctx, empty, both uintptr }

// funcShape gives the function obj, made just now, the properties length
// and name, as js_function_set_properties does, and reports whether it did.
// When it did not, nothing was done, and js_function_set_properties goes on
// as before.
func funcShape(tls *libc.TLS, ctx uintptr, obj TJSValue, name TJSAtom, length int32) bool {
	// What objlitDefine tests: the two keys are added to an ordinary
	// object.
	if jsvTag(obj) != EJS_TAG_OBJECT {
		return false
	}
	p := jsvPtr(obj)
	o := (*TJSObject)(unsafe.Pointer(p))
	if *(*uint8)(unsafe.Pointer(p + objlitFlagsOff))&(objlitExtensible|objlitExotic|objlitFastArray) != objlitExtensible ||
		o.Fclass_id == EJS_CLASS_GLOBAL_OBJECT {
		return false
	}
	c := (*TJSContext)(unsafe.Pointer(ctx))
	fs := &forkOf(c.Frt).fn
	sh := o.Fshape
	for i := range fs {
		if fs[i].empty != sh {
			continue
		}
		both := fs[i].both
		// Made before the object changes: it can allocate and run the
		// cycle collector, which must find the object as it is.
		nameValue := XJS_AtomToString(tls, ctx, name)
		// add_property for the second key, which finds the shape with
		// both in the shape hash table. The property array keeps its
		// size (see funcShapeMade).
		shapeFound(tls, ctx)
		o.Fshape = _js_dup_shape(tls, both)
		_js_free_shape(tls, c.Frt, sh)
		*(*TJSValue)(unsafe.Pointer(o.Fprop)) = jsvMkPtr(EJS_TAG_INT, uintptr(uint32(length)))
		*(*TJSValue)(unsafe.Pointer(o.Fprop + unsafe.Sizeof(TJSProperty{}))) = nameValue
		return true
	}
	i := funcShapeSlot(c, sh)
	if i < 0 {
		return false
	}
	// Held while add_property moves the object on, so that the empty shape
	// stays as it is and is not freed: add_property copies a shape held by
	// more than one rather than grow it.
	empty := _js_dup_shape(tls, sh)
	// What js_function_set_properties does.
	XJS_DefinePropertyValue(tls, ctx, obj, EJS_ATOM_length, jsvMkPtr(EJS_TAG_INT, uintptr(uint32(length))), MJS_PROP_CONFIGURABLE)
	XJS_DefinePropertyValue(tls, ctx, obj, EJS_ATOM_name, XJS_AtomToString(tls, ctx, name), MJS_PROP_CONFIGURABLE)
	both := o.Fshape
	if !funcShapeMade(tls, empty, both) {
		_js_free_shape(tls, c.Frt, empty)
		return true
	}
	old := fs[i]
	fs[i].ctx, fs[i].empty, fs[i].both = ctx, empty, _js_dup_shape(tls, both)
	if old.empty != 0 {
		_js_free_shape(tls, c.Frt, old.empty)
		_js_free_shape(tls, c.Frt, old.both)
	}
	return true
}

// funcShapeSlot returns the slot of TfuncShapes for the shape sh of a new
// function of ctx, or -1 when sh is not the empty shape of one of the four
// prototypes of new functions of ctx. The context holds those prototypes,
// so the shapes funcShape holds hold nothing that would be freed without
// them.
func funcShapeSlot(c *TJSContext, sh uintptr) int {
	s := (*TJSShape)(unsafe.Pointer(sh))
	if s.Fis_hashed == 0 || s.Fprop_count != 0 || s.Fdeleted_prop_count != 0 || s.Fproto == 0 {
		return -1
	}
	if jsvTag(c.Ffunction_proto) == EJS_TAG_OBJECT && jsvPtr(c.Ffunction_proto) == s.Fproto {
		return 0
	}
	var proto TJSValue // a struct on 64-bit targets and an integer on 32-bit ones
	for i, class := range [...]uintptr{EJS_CLASS_ASYNC_FUNCTION, EJS_CLASS_GENERATOR_FUNCTION, EJS_CLASS_ASYNC_GENERATOR_FUNCTION} {
		proto = *(*TJSValue)(unsafe.Pointer(c.Fclass_proto + class*unsafe.Sizeof(proto)))
		if jsvTag(proto) == EJS_TAG_OBJECT && jsvPtr(proto) == s.Fproto {
			return 1 + i
		}
	}
	return -1
}

// funcShapeMade reports whether both, the shape js_function_set_properties
// left a new function with, is the shape with length and name after the
// empty shape empty: a shape in the shape hash table other than empty, with
// its prototype, the two keys and nothing else, and the size of the property
// array of empty. The flags of the keys are those of
// js_function_set_properties: add_property gave them to the two keys it
// added to empty, which had none.
func funcShapeMade(tls *libc.TLS, empty, both uintptr) bool {
	e, b := (*TJSShape)(unsafe.Pointer(empty)), (*TJSShape)(unsafe.Pointer(both))
	if both == empty || b.Fis_hashed == 0 || b.Fproto != e.Fproto || b.Fprop_size != e.Fprop_size ||
		b.Fprop_count != 2 || b.Fdeleted_prop_count != 0 {
		return false
	}
	pr := _get_shape_prop(tls, both)
	return (*TJSShapeProperty)(unsafe.Pointer(pr)).Fatom == EJS_ATOM_length &&
		(*TJSShapeProperty)(unsafe.Pointer(pr+unsafe.Sizeof(TJSShapeProperty{}))).Fatom == EJS_ATOM_name
}

// funcShapesMark marks the shapes held for ctx with markFunc, the function
// JS_MarkContext marks the shapes of ctx with.
func funcShapesMark(tls *libc.TLS, rt, ctx, markFunc uintptr) {
	mark := *(*func(*libc.TLS, uintptr, uintptr))(unsafe.Pointer(&struct{ uintptr }{markFunc}))
	for _, e := range forkOf(rt).fn {
		if e.ctx == ctx {
			mark(tls, rt, e.empty)
			mark(tls, rt, e.both)
		}
	}
}

// funcShapesFree lets go of the shapes held for ctx, which is being freed.
func funcShapesFree(tls *libc.TLS, ctx uintptr) {
	rt := (*TJSContext)(unsafe.Pointer(ctx)).Frt
	fs := &forkOf(rt).fn
	for i := range fs {
		if e := fs[i]; e.ctx == ctx {
			fs[i] = struct{ ctx, empty, both uintptr }{}
			_js_free_shape(tls, rt, e.empty)
			_js_free_shape(tls, rt, e.both)
		}
	}
}
