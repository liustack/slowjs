// Copyright 2025 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !(386 || arm)

package quickjs // import "github.com/liustack/quickjs"

import (
	"unsafe"

	lib "github.com/liustack/quickjs/lib"
)

const sizeofJsValue = unsafe.Sizeof(lib.TJSValue{})

var (
	null      = lib.TJSValue{Ftag: lib.EJS_TAG_NULL}
	undefined = lib.TJSValue{Ftag: lib.EJS_TAG_UNDEFINED}
)

func newFloat(n float64) (r lib.TJSValue) {
	*(*float64)(unsafe.Pointer(&r)) = n
	r.Ftag = lib.EJS_TAG_FLOAT64
	return r
}

func newBool(n bool) (r lib.TJSValue) {
	switch {
	case n:
		jsvSetInt32(&r, 1)
	default:
		jsvSetInt32(&r, 0)
	}
	r.Ftag = lib.EJS_TAG_BOOL
	return r
}

func isException(v lib.TJSValue) bool {
	return v.Ftag == lib.EJS_TAG_EXCEPTION
}

// #define JS_VALUE_GET_TAG(v) ((int32_t)(v).tag)

func tag(v lib.TJSValue) int32 {
	return int32(v.Ftag)
}

func jsvToFloat64(v lib.TJSValue) (r float64) {
	return *(*float64)(unsafe.Pointer(&v))
}

// #define JS_VALUE_GET_INT(v) ((v).u.int32)
//
// The JSValue payload is the eight bytes wide C union JSValueUnion, which
// the lib package (ccgo) represents as its widest member, an uint64. The
// narrower members of that union are accessed in the value domain, not by
// address: JS_MKVAL(tag, val) zero extends 'val' into the low 32 bits of the
// uint64 and JS_VALUE_GET_INT truncates the uint64 back to 32 bits. Reading or
// writing the payload as the first four bytes of the union instead is
// equivalent on a little endian target only - on s390x those are the high, for
// an int32 or a boolean always zero, bits.
//
// jsvToInt32 must be used for the int32 payload of a JS_TAG_INT and for the
// boolean payload of a JS_TAG_BOOL. The float64 and pointer payloads are as
// wide as the union itself, so for those the two domains coincide and no
// conversion is needed.
func jsvToInt32(v lib.TJSValue) int32 {
	return int32(*(*uint64)(unsafe.Pointer(&v)))
}

// jsvSetInt32 sets the int32 payload of 'v', zero extended over the whole
// union the way JS_MKVAL does. See jsvToInt32 for why the payload is not
// written as the first four bytes of the union.
func jsvSetInt32(v *lib.TJSValue, n int32) {
	*(*uint64)(unsafe.Pointer(v)) = uint64(uint32(n))
}

// jsvToPtr extracts the JSModuleDef pointer from a JSValue.
func jsvToPtr(v lib.TJSValue) uintptr {
	return *(*uintptr)(unsafe.Pointer(&v))
}
