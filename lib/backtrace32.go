// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT AND BSD-3-Clause
// The fork's code here is under the MIT license in LICENSE-MIT at the root of
// this repository. Parts follow QuickJS (Copyright (c) 2017-2021 Fabrice
// Bellard and Charlie Gordon, MIT, see LICENSE-LIBQUICKJS) as translated by
// modernc.org/libquickjs (The libquickjs-go Authors, BSD-3-Clause, see
// lib/LICENSE).

//go:build 386 || arm

package libquickjs // import "github.com/liustack/quickjs/lib"

// The JSValue macros backtrace.go needs, for the NaN-boxed JSValue of 32-bit
// targets. backtrace.go only asks for tags that are not floats, which are the
// high 32 bits.

func jsvTag(v TJSValue) int32 { return int32(v >> 32) }

func jsvPtr(v TJSValue) uintptr { return uintptr(uint32(v)) }

func jsvMkPtr(tag int32, p uintptr) TJSValue { return TJSValue(uint32(tag))<<32 | TJSValue(uint32(p)) }
