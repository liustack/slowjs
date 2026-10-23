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

// get_field and get_field2 of the interpreter (o.x and o.x(...)) look for
// the key on the object and then on its prototypes, and hand everything
// else to JS_GetPropertyInternal: a value that is not an object, an object
// with exotic behavior (an array, a String object, a proxy, ...), a getter.
// Two of those cases are frequent and need nothing that JS_GetPropertyInternal
// adds: a method of a string or a number (s.charCodeAt(i), n.toFixed(2)),
// which the CSS-in-JS code of MUI runs hundreds of thousands of times per
// page, and a method of an array (a.push(x), a.map(f)). internal/fixlib makes
// the interpreter call fieldProto and fieldGoesOn there (see
// fixFieldChain), on 64-bit targets, and both answer as JS_GetPropertyInternal
// would.

// fieldProto returns the object where get_field and get_field2 go on
// looking for atom when the value v is not an object: the prototype of a
// string, when atom is no index and not length, and the prototype of a
// number. A string has those two kinds of keys of its own and no other, a
// number none, so JS_GetPropertyInternal looks for any other key from the
// object JS_GetPrototypePrimitive gives, with v as this. The interpreter
// keeps v as the value it hands to JS_GetPropertyInternal, for a getter or
// an exotic object on the way. It returns 0 for every other value or key,
// which the interpreter hands to JS_GetPropertyInternal as before.
func fieldProto(tls *libc.TLS, ctx uintptr, v TJSValue, atom TJSAtom) uintptr {
	var class uintptr
	switch jsvTag(v) {
	case EJS_TAG_STRING, EJS_TAG_STRING_ROPE:
		if ___JS_AtomIsTaggedInt(tls, atom) != 0 || atom == EJS_ATOM_length {
			return 0
		}
		class = EJS_CLASS_STRING
	case EJS_TAG_INT, EJS_TAG_FLOAT64:
		class = EJS_CLASS_NUMBER
	default:
		return 0
	}
	return jsvPtr(*(*TJSValue)(unsafe.Pointer((*TJSContext)(unsafe.Pointer(ctx)).Fclass_proto + class*unsafe.Sizeof(TJSValue{}))))
}

// fieldGoesOn reports whether get_field and get_field2 go on to the
// prototype of p, an object with exotic behavior that has no property atom
// of its own, rather than hand it to JS_GetPropertyInternal. That is so for
// a fast array that is not a typed array, and a key that is no index: for
// such an object JS_GetPropertyInternal looks at its elements only for an
// index, and goes on to its prototype for any other key. A typed array
// answers undefined for a key that is a number in canonical form, such as
// Infinity, so it stays with JS_GetPropertyInternal.
//
// fieldFastArrayOff and fieldFastArrayMask are the fast_array bit of a
// JSObject, as the file's own JS_GetPropertyInternal reads it (see
// fixFieldChain).
func fieldGoesOn(tls *libc.TLS, p uintptr, atom TJSAtom) bool {
	class := int32((*TJSObject)(unsafe.Pointer(p)).Fclass_id)
	return *(*uint8)(unsafe.Pointer(p + fieldFastArrayOff))&fieldFastArrayMask != 0 &&
		___JS_AtomIsTaggedInt(tls, atom) == 0 &&
		(class < EJS_CLASS_UINT8C_ARRAY || class > EJS_CLASS_FLOAT64_ARRAY)
}
