// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT AND BSD-3-Clause
// The fork's code here is under the MIT license in LICENSE-MIT at the root of
// this repository. Parts follow QuickJS (Copyright (c) 2017-2021 Fabrice
// Bellard and Charlie Gordon, MIT, see LICENSE-LIBQUICKJS) as translated by
// modernc.org/libquickjs (The libquickjs-go Authors, BSD-3-Clause, see
// lib/LICENSE).

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
