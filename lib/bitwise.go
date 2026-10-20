// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !(386 || arm)

package libquickjs // import "github.com/liustack/quickjs/lib"

import (
	"math"
	"unsafe"
)

// bitwiseNumbers does the bitwise operator op (&, |, ^, <<, >> or >>>) of
// the two values on top of the interpreter's stack, sp[-2] and sp[-1], when
// both are numbers, and stores the result in sp[-2], as js_binary_logic_slow
// and js_shr_slow do. It reports false, with nothing done, when one of them
// is not a number. The interpreter handles two ints itself and calls the
// slow functions for everything else, so a float, which any product past
// 2^31 becomes, took the whole slow path: a TLS frame, two conversions that
// check for BigInts and objects, and the generic ToInt32. The slow functions
// now call bitwiseNumbers at their entry (internal/fixlib puts the call
// there, see fixBitwise), on 64-bit targets.
//
// Numbers hold no reference, so nothing is freed, and no conversion of a
// number can throw or call a script, so the result is the same as on the
// slow path, which converts both with JS_ToNumericFree (nothing to do for a
// number) and JS_ToInt32Free, then does the same operation.
func bitwiseNumbers(sp uintptr, op int32) bool {
	a := (*TJSValue)(unsafe.Pointer(sp - 2*unsafe.Sizeof(TJSValue{})))
	b := (*TJSValue)(unsafe.Pointer(sp - unsafe.Sizeof(TJSValue{})))
	x, ok := numberToInt32(a)
	if !ok {
		return false
	}
	y, ok := numberToInt32(b)
	if !ok {
		return false
	}
	var r int32
	switch op {
	case EOP_shl:
		r = int32(uint32(x) << (uint32(y) & 0x1f))
	case EOP_sar:
		r = x >> (uint32(y) & 0x1f)
	case EOP_and:
		r = x & y
	case EOP_or:
		r = x | y
	case EOP_xor:
		r = x ^ y
	case EOP_shr:
		// JS_NewUint32: an int when it fits, else a float.
		u := uint32(x) >> (uint32(y) & 0x1f)
		if u > math.MaxInt32 {
			*(*float64)(unsafe.Pointer(&a.Fu)) = float64(u)
			a.Ftag = EJS_TAG_FLOAT64
			return true
		}
		r = int32(u)
	default:
		return false
	}
	// JS_NewInt32: the payload is the low 32 bits of the union, as a value,
	// so this holds on big-endian s390x too.
	*(*uint64)(unsafe.Pointer(&a.Fu)) = uint64(uint32(r))
	a.Ftag = EJS_TAG_INT
	return true
}

// numberToInt32 is JS_ToInt32Free for an int or a float, which frees
// nothing and cannot fail. ok is false for every other value.
func numberToInt32(v *TJSValue) (r int32, ok bool) {
	switch v.Ftag {
	case EJS_TAG_INT:
		return int32(*(*uint64)(unsafe.Pointer(&v.Fu))), true
	case EJS_TAG_FLOAT64:
		d := *(*float64)(unsafe.Pointer(&v.Fu))
		u := math.Float64bits(d)
		// As JS_ToInt32Free, without fmod.
		switch e := int32(u >> 52 & 0x7ff); {
		case e <= 1023+30:
			r = int32(d)
		case e <= 1023+30+53:
			m := (u&(1<<52-1) | 1<<52) << (e - 1023 - 52 + 32)
			r = int32(m >> 32)
			if u>>63 != 0 {
				r = -r
			}
		default:
			r = 0 // also NaN and the infinities
		}
		return r, true
	}
	return 0, false
}
