// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT AND BSD-3-Clause
// The fork's code here is under the MIT license in LICENSE-MIT at the root of
// this repository. Parts follow QuickJS (Copyright (c) 2017-2021 Fabrice
// Bellard and Charlie Gordon, MIT, see LICENSE-LIBQUICKJS) as translated by
// modernc.org/libquickjs (The libquickjs-go Authors, BSD-3-Clause, see
// lib/LICENSE).

package libquickjs // import "github.com/liustack/quickjs/lib"

import (
	"unsafe"

	"modernc.org/libc"
)

// StringChars returns where the characters of v are, how many there are and
// whether they are 16-bit units, when v is a string the engine holds in one
// piece. ok is false for a rope and for any other value. The characters
// belong to v: they are valid while v is, and must not be changed.
//
// It reads the JSString as jsonQuoteRun does, with the constants
// internal/fixlib takes from the file's JS_ToQuotedString (see
// fixJSONStrings).
func StringChars(v TJSValue) (p uintptr, n int, wide bool, ok bool) {
	if jsvTag(v) != EJS_TAG_STRING {
		return 0, 0, false, false
	}
	s := jsvPtr(v)
	n = int(*(*uint32)(unsafe.Pointer(s + jsonStrLenOff)) & jsonStrLenMask >> jsonStrLenShift)
	wide = *(*uint32)(unsafe.Pointer(s + jsonStrWideOff))&jsonStrWideMask>>jsonStrWideShift != 0
	return s + jsonStrData, n, wide, true
}

// NewStringChars returns a new string of n characters, 16-bit units when
// wide is true, and where its characters are, for the caller to write all
// of them before the string is used. It allocates the string as
// js_new_string8_len and the string buffers do, with js_alloc_string, which
// throws the out of memory error when it fails: the value is then the
// exception and p is 0. An empty string is the atom of the empty string, as
// js_new_string8_len gives it, and p is 0. n must be at most
// MJS_STRING_LEN_MAX.
func NewStringChars(tls *libc.TLS, ctx uintptr, n int, wide bool) (v TJSValue, p uintptr) {
	if n == 0 {
		return XJS_AtomToString(tls, ctx, EJS_ATOM_empty_string), 0
	}
	if n < 0 || n > MJS_STRING_LEN_MAX {
		panic("quickjs: NewStringChars of a length the engine cannot hold")
	}
	w := int32(0)
	if wide {
		w = 1
	}
	s := _js_alloc_string(tls, ctx, int32(n), w)
	if s == 0 {
		return jsvMkPtr(EJS_TAG_EXCEPTION, 0), 0
	}
	if !wide {
		// An 8-bit string ends with a NUL the engine does not count.
		*(*byte)(unsafe.Pointer(s + jsonStrData + uintptr(n))) = 0
	}
	return jsvMkPtr(EJS_TAG_STRING, s), s + jsonStrData
}

// DupValue is JS_DupValue: it adds a reference to v when v has a reference
// count, and returns v. XDupValue does the same in a TLS frame, which it
// takes for its copy of v.
func DupValue(v TJSValue) TJSValue {
	if t := jsvTag(v); t < 0 && t >= EJS_TAG_FIRST {
		(*TJSMallocBlockHeader)(unsafe.Pointer(_js_rc(nil, jsvPtr(v)))).Fref_count++
	}
	return v
}
