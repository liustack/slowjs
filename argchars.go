// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build ((linux || darwin) && (amd64 || arm64)) || (windows && amd64)

package quickjs // import "github.com/liustack/quickjs"

import (
	"unsafe"

	"github.com/liustack/quickjs/internal/cengine"
)

// stringLen is vmhook.StringLen: the length of argument i, in UTF-16 code
// units as javascript counts it, when it is a string, without reading the
// string. A rope, a string the engine keeps as the pieces it was joined
// from, is not joined. It allocates nothing.
func (a Args) stringLen(i int) (n int, ok bool) {
	// The first uint32 of a string holds its length, under the bit of its
	// width, and that of a rope its length (JSStringRope.len):
	// TestVMHookChars checks both.
	switch v := a.arg(i); tag(v) {
	case tagString, tagStringRope:
		return int(*(*uint32)(addr(v.u)) & cengine.StringLenMask), true
	}
	return 0, false
}

// chars is vmhook.Chars.
func (a Args) chars(i int) (latin1 []byte, utf16 []uint16, release func(), ok bool) {
	v := a.arg(i)
	release = func() {}
	switch tag(v) {
	case tagString:
	case tagStringRope:
		m := a.vm
		flat := m.jsToString(v)
		if isException(flat) {
			return nil, nil, nil, false
		}
		v, release = flat, func() { m.jsFreeValue(flat) }
	default:
		return nil, nil, nil, false
	}
	p, n, wide, _ := stringChars(v)
	if wide {
		return nil, unsafe.Slice((*uint16)(addr(p)), n), release, true
	}
	return unsafe.Slice((*byte)(addr(p)), n), nil, release, true
}
