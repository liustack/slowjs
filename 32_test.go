// Copyright 2025 The libquickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build 386 || arm

package libquickjs // import "modernc.org/libquickjs"

import (
	"math"
)

func tag(v TJSValue) (r int32) {
	if r = int32(v >> 32); uint32(r)-u32FromInt32(EJS_TAG_FIRST) >= EJS_TAG_FLOAT64-EJS_TAG_FIRST {
		r = EJS_TAG_FLOAT64
	}
	return r
}

func u32FromInt32(n int32) uint32 {
	return uint32(n)
}

// #define JS_FLOAT64_TAG_ADDEND (0x7ff80000 - JS_TAG_FIRST + 1) /* quiet NaN encoding */
const _JS_FLOAT64_TAG_ADDEND = (0x7ff80000 - EJS_TAG_FIRST + 1)

func jsvToFloat64(v TJSValue) (r float64) {
	return math.Float64frombits(v + _JS_FLOAT64_TAG_ADDEND<<32)
}

// jsvToInt32 extracts a JS_TAG_INT payload. On 32-bit (NaN-boxing) TJSValue is
// a uint64 with the int value in its low 32 bits (JS_VALUE_GET_INT is (int)v).
func jsvToInt32(v TJSValue) int32 {
	return int32(v)
}
