// Copyright 2025 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build 386 || arm

package quickjs // import "github.com/liustack/quickjs"

import (
	"math"
	"unsafe"

	lib "github.com/liustack/quickjs/lib"
)

const sizeofJsValue = unsafe.Sizeof(lib.TJSValue(0))

var (
	null      = uint64(lib.EJS_TAG_NULL << 32)
	undefined = uint64(lib.EJS_TAG_UNDEFINED << 32)
)

// #define JS_FLOAT64_TAG_ADDEND (0x7ff80000 - JS_TAG_FIRST + 1) /* quiet NaN encoding */
const _JS_FLOAT64_TAG_ADDEND = (0x7ff80000 - lib.EJS_TAG_FIRST + 1)

// #define JS_NAN (0x7ff8000000000000 - ((uint64_t)JS_FLOAT64_TAG_ADDEND << 32))
const _JS_NAN = (0x7ff8000000000000 - ((_JS_FLOAT64_TAG_ADDEND) << 32))

// static inline JSValue __JS_NewFloat64(JSContext *ctx, double d)
// {
//     union {
//         double d;
//         uint64_t u64;
//     } u;
//     JSValue v;
//     u.d = d;
//     /* normalize NaN */
//     if (js_unlikely((u.u64 & 0x7fffffffffffffff) > 0x7ff0000000000000))
//         v = JS_NAN;
//     else
//         v = u.u64 - ((uint64_t)JS_FLOAT64_TAG_ADDEND << 32);
//     return v;
// }

func jsvFromInt64(n int64) lib.TJSValue {
	return lib.TJSValue(n)
}

func newFloat(n float64) (r lib.TJSValue) {
	u64 := math.Float64bits(n)
	if u64&0x7fffffffffffffff > 0x7ff0000000000000 {
		return jsvFromInt64(_JS_NAN)
	}

	return u64 - ((uint64)(_JS_FLOAT64_TAG_ADDEND) << 32)
}

// #define JS_MKVAL(tag, val) (((uint64_t)(tag) << 32) | (uint32_t)(val))

// static js_force_inline JSValue JS_NewBool(JSContext *ctx, JS_BOOL val)
// {
//     return JS_MKVAL(JS_TAG_BOOL, (val != 0));
// }

func newBool(n bool) (r lib.TJSValue) {
	r = lib.EJS_TAG_BOOL << 32
	if n {
		jsvSetInt32(&r, 1)
	}
	return r
}

// #define JS_VALUE_GET_INT(v) (int)(v)

func jsvToInt32(v lib.TJSValue) int32 {
	return int32(v)
}

// jsvSetInt32 sets the int32 payload of 'v', keeping its tag.
func jsvSetInt32(v *lib.TJSValue, n int32) {
	*v = *v&0xffffffff00000000 | lib.TJSValue(uint32(n))
}

// #define JS_VALUE_GET_TAG(v) (int)((v) >> 32)

// static inline JS_BOOL JS_IsException(JSValueConst v)
// {
//     return js_unlikely(JS_VALUE_GET_TAG(v) == JS_TAG_EXCEPTION);
// }

func isException(v lib.TJSValue) bool {
	return tag(v) == lib.EJS_TAG_EXCEPTION
}

// #define JS_TAG_IS_FLOAT64(tag) ((unsigned)((tag) - JS_TAG_FIRST) >= (JS_TAG_FLOAT64 - JS_TAG_FIRST))
//
// /* same as JS_VALUE_GET_TAG, but return JS_TAG_FLOAT64 with NaN boxing */
// static inline int JS_VALUE_GET_NORM_TAG(JSValue v)
// {
//     uint32_t tag;
//     tag = JS_VALUE_GET_TAG(v);
//     if (JS_TAG_IS_FLOAT64(tag))
//         return JS_TAG_FLOAT64;
//     else
//         return tag;
// }

func u32FromInt32(n int32) uint32 {
	return uint32(n)
}

func tag(v lib.TJSValue) (r int32) {
	if r = int32(v >> 32); uint32(r)-u32FromInt32(lib.EJS_TAG_FIRST) >= lib.EJS_TAG_FLOAT64-lib.EJS_TAG_FIRST {
		r = lib.EJS_TAG_FLOAT64
	}
	return r
}

// static inline double JS_VALUE_GET_FLOAT64(JSValue v)
// {
//     union {
//         JSValue v;
//         double d;
//     } u;
//     u.v = v;
//     u.v += (uint64_t)JS_FLOAT64_TAG_ADDEND << 32;
//     return u.d;
// }

func jsvToFloat64(v lib.TJSValue) (r float64) {
	return math.Float64frombits(v + _JS_FLOAT64_TAG_ADDEND<<32)
}

// jsvToPtr extracts the JSModuleDef pointer from a JSValue.
func jsvToPtr(v lib.TJSValue) uintptr {
	return uintptr(v)
}
