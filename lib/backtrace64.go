// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !(386 || arm)

package libquickjs // import "github.com/liustack/quickjs/lib"

import "unsafe"

// The JSValue macros backtrace.go needs, for the struct JSValue of 64-bit
// targets. The pointer fills the union, so this holds on big-endian s390x too.

func jsvTag(v TJSValue) int32 { return int32(v.Ftag) }

func jsvPtr(v TJSValue) uintptr { return *(*uintptr)(unsafe.Pointer(&v.Fu)) }

func jsvMkPtr(tag int32, p uintptr) (v TJSValue) {
	*(*uintptr)(unsafe.Pointer(&v.Fu)) = p
	v.Ftag = Tint64_t(tag)
	return v
}
