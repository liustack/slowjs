// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build 386 || arm

package libquickjs // import "github.com/liustack/quickjs/lib"

// The JSValue macros backtrace.go needs, for the NaN-boxed JSValue of 32-bit
// targets. backtrace.go only asks for tags that are not floats, which are the
// high 32 bits.

func jsvTag(v TJSValue) int32 { return int32(v >> 32) }

func jsvPtr(v TJSValue) uintptr { return uintptr(uint32(v)) }

func jsvMkPtr(tag int32, p uintptr) TJSValue { return TJSValue(uint32(tag))<<32 | TJSValue(uint32(p)) }
