// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package slowjs

import (
	"encoding/binary"
	"unicode/utf8"
	"unsafe"
)

// The conversion of 16-bit units to UTF-8 as v0.30.6 made it, kept for the
// tests to compare the one of jsstring.go with, byte for byte: the users of
// the module see its output (a render's HTML, every string argument).

// utf16StringRef is utf16String of v0.30.6 for any u, in one go.
func utf16StringRef(u []uint16, wtf8 bool) string {
	b := make([]byte, 3*len(u))
	return string(b[:utf16WriteRef(b, u, wtf8)])
}

// utf16WriteRef writes the UTF-8 of u to out, which has room for 3 bytes a
// unit, as utf16String converts it, and returns how many bytes it wrote.
func utf16WriteRef(out []byte, u []uint16, wtf8 bool) int {
	if len(out) < 3*len(u) {
		panic("slowjs: no room for the UTF-8 of the units")
	}
	d := unsafe.Pointer(unsafe.SliceData(out))
	j := uintptr(0)
	for i := 0; i < len(u); i++ {
		// Pages are mostly ASCII markup around runs of text, so this takes
		// four units at a time while they are ASCII, or all from U+0800 and
		// none a surrogate.
		if i+4 <= len(u) {
			w := unitsAtRef(u, i)
			if w&laneNotASCII == 0 {
				*(*[4]byte)(unsafe.Add(d, j)) = [4]byte{byte(w), byte(w >> 16), byte(w >> 32), byte(w >> 48)}
				i += 3
				j += 4
				continue
			}
			if lanesFrom(w, 11) == laneTop && noSurrogate(w) {
				p := (*[12]byte)(unsafe.Add(d, j))
				for k := range 4 {
					c := uint16(w >> (16 * k))
					p[3*k], p[3*k+1], p[3*k+2] = 0xE0|byte(c>>12), 0x80|byte(c>>6)&0x3F, 0x80|byte(c)&0x3F
				}
				i += 3
				j += 12
				continue
			}
		}
		c := uint32(u[i])
		switch {
		case c < 0x80:
			*(*byte)(unsafe.Add(d, j)) = byte(c)
			j++
			continue
		case c < 0x800:
			*(*[2]byte)(unsafe.Add(d, j)) = [2]byte{0xC0 | byte(c>>6), 0x80 | byte(c)&0x3F}
			j += 2
			continue
		case c&0xF800 != 0xD800:
		case c&0xFC00 == 0xD800 && i+1 < len(u) && u[i+1]&0xFC00 == 0xDC00:
			c = 0x10000 + (c-0xD800)<<10 + (uint32(u[i+1]) - 0xDC00)
			i++
			*(*[4]byte)(unsafe.Add(d, j)) = [4]byte{0xF0 | byte(c>>18), 0x80 | byte(c>>12)&0x3F, 0x80 | byte(c>>6)&0x3F, 0x80 | byte(c)&0x3F}
			j += 4
			continue
		case !wtf8:
			c = utf8.RuneError
		}
		*(*[3]byte)(unsafe.Add(d, j)) = [3]byte{0xE0 | byte(c>>12), 0x80 | byte(c>>6)&0x3F, 0x80 | byte(c)&0x3F}
		j += 3
	}
	return int(j)
}

// unitsAtRef returns the four units of u from i, the first in the low bits. It
// reads them as bytes, which the compiler makes one load, as a pointer to a
// uint64 would not be aligned.
func unitsAtRef(u []uint16, i int) uint64 {
	b := unsafe.Slice((*byte)(unsafe.Pointer(&u[i])), 8)
	_ = u[i+3]
	return binary.LittleEndian.Uint64(b)
}
