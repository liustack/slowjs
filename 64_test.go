// Copyright 2025 The libquickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !(386 || arm)

package libquickjs // import "modernc.org/libquickjs"

import (
	"unsafe"
)

func tag(v TJSValue) int32 {
	return int32(v.Ftag)
}

func jsvToFloat64(v TJSValue) (r float64) {
	return *(*float64)(unsafe.Pointer(&v))
}

// jsvToInt32 extracts a JS_TAG_INT payload. On every 64-bit target the value
// union is the first 8 bytes of TJSValue and QuickJS keeps the int in the low
// 32 value-bits (JS_VALUE_GET_INT reads u.int32). Read the whole union as a
// native uint64 and truncate: this reconstructs the stored value on both little-
// and big-endian (e.g. s390x) without naming the ccgo-generated union field,
// whose name is not portable -- ccgo backs the union with a uint64 word
// (Fuint641) on most targets but an int32+pad (Fint321) on windows.
func jsvToInt32(v TJSValue) int32 {
	return int32(*(*uint64)(unsafe.Pointer(&v)))
}
