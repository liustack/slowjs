// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package slowjs // import "github.com/liustack/slowjs"

import (
	"encoding/binary"
	"fmt"
	"math/bits"
	"sync"
	"unicode/utf8"
	"unsafe"
)

// argString returns the Go string of v, a lone surrogate as U+FFFD, or as
// the three bytes of its code point when wtf8 is true: the string every
// value javascript gives Go becomes, a result, the argument of a Builtin or of
// a Go function, the JSON of an object. A string the engine holds in one
// piece is read where it is, as one copy, where JS_ToCStringLen2 made a UTF-8
// copy of any string that is not ASCII in the VM's memory, which Go then
// copied again. A rope, a string kept as the pieces it was joined from, is
// joined first (JS_ToString), and any other value goes the way of
// toGoString. ok is false, and the exception pending, when that threw, as
// when there is no memory to join a rope.
func (m *VM) argString(v jsValue, wtf8 bool) (string, bool) {
	p, n, wide, ok := stringChars(v)
	if !ok {
		if tag(v) != tagStringRope {
			return m.toGoString(v, wtf8)
		}
		flat := m.jsToString(v)
		if isException(flat) {
			return "", false
		}
		s, ok := m.argString(flat, wtf8)
		m.jsFreeValue(flat)
		return s, ok
	}
	if n == 0 {
		return "", true
	}
	if !wide {
		return latin1String(unsafe.Slice((*byte)(unsafe.Pointer(p)), n)), true
	}
	return utf16String(unsafe.Slice((*uint16)(unsafe.Pointer(p)), n), wtf8), true
}

// asciiLen returns the length of the run of ASCII bytes b starts with.
func asciiLen(b []byte) int {
	i := 0
	for ; i+8 <= len(b); i += 8 {
		if binary.LittleEndian.Uint64(b[i:])&0x8080808080808080 != 0 {
			break
		}
	}
	for i < len(b) && b[i] < utf8.RuneSelf {
		i++
	}
	return i
}

// stringBytes returns the bytes of s, which must not be changed.
func stringBytes(s string) []byte {
	return unsafe.Slice(unsafe.StringData(s), len(s))
}

// latin1String returns the UTF-8 form of the 8-bit characters b, each a code
// point below U+0100, as JS_ToCStringLen2 writes them.
func latin1String(b []byte) string {
	i := asciiLen(b)
	if i == len(b) {
		return string(b)
	}
	src := b[i:]
	n := len(b)
	k := 0
	for ; k+8 <= len(src); k += 8 {
		n += bits.OnesCount64(binary.LittleEndian.Uint64(src[k:]) & 0x8080808080808080)
	}
	for _, c := range src[k:] {
		n += int(c >> 7)
	}
	out := make([]byte, n)
	copy(out, b[:i])
	j := i
	for _, c := range src {
		if c < utf8.RuneSelf {
			out[j] = c
			j++
			continue
		}
		out[j] = 0xC0 | c>>6
		out[j+1] = 0x80 | c&0x3F
		j += 2
	}
	return unsafe.String(unsafe.SliceData(out), n)
}

// utf16String returns the UTF-8 form of the 16-bit units u, a surrogate pair
// as its one character. A lone surrogate is U+FFFD, as wellFormed makes it,
// or with wtf8 the three bytes of its code point, as JS_ToCStringLen2 writes
// it.
func utf16String(u []uint16, wtf8 bool) string {
	// Each unit takes at most 3 bytes, a pair 4. The UTF-8 is written once,
	// into memory on the stack for a short string and kept for the next ones
	// otherwise, and copied into a string of its size, as JS_ToCStringLen2
	// wrote it in C and Go copied it, where counting the bytes first took a
	// second pass over u.
	if len(u) <= utf16Short {
		var b [3 * utf16Short]byte
		return string(b[:utf16Write(b[:], u, wtf8)])
	}
	p := utf16Scratch.Get().(*[]byte)
	if cap(*p) < 3*len(u) {
		*p = make([]byte, 3*len(u))
	}
	s := string((*p)[:utf16Write((*p)[:3*len(u)], u, wtf8)])
	utf16Scratch.Put(p)
	return s
}

// utf16Short is the length of the strings utf16String converts on the stack.
const utf16Short = 64

// utf16Scratch keeps the memory utf16String writes longer strings in.
var utf16Scratch = sync.Pool{New: func() any { return new([]byte) }}

// utf16Write writes the UTF-8 of u to out, which has room for 3 bytes a
// unit, as utf16String converts it, and returns how many bytes it wrote.
func utf16Write(out []byte, u []uint16, wtf8 bool) int {
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
			w := unitsAt(u, i)
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

// Four 16-bit units in a uint64: lanes repeats a unit in each, laneTop is the
// top bit of each, and laneNotASCII the bits of each from 0x80 up.
const (
	lanes        = 0x0001_0001_0001_0001
	laneTop      = 0x8000 * lanes
	laneNotASCII = 0xFF80 * lanes
)

// lanesFrom returns laneTop with the top bit of each unit of w from 1<<shift
// up, and none of the others: the bits of the unit from shift up, plus
// 0x7FFF, carry into the top bit unless they are all zero, and never out of
// the unit.
func lanesFrom(w uint64, shift uint) uint64 {
	return ((w>>shift)&((0xFFFF>>shift)*lanes) + 0x7FFF*lanes) & laneTop
}

// noSurrogate reports whether none of the units of w is from U+D800 to
// U+DFFF: each has a bit from 0x800 up that is not that of 0xD800.
func noSurrogate(w uint64) bool {
	return lanesFrom(w&(0xF800*lanes)^(0xD800*lanes), 11) == laneTop
}

// unitsAt returns the four units of u from i, the first in the low bits. It
// reads them as bytes, which the compiler makes one load, as a pointer to a
// uint64 would not be aligned.
func unitsAt(u []uint16, i int) uint64 {
	b := unsafe.Slice((*byte)(unsafe.Pointer(&u[i])), 8)
	_ = u[i+3]
	return binary.LittleEndian.Uint64(b)
}

// newStringFast makes the JS string of s in Go, as validUTF8 and
// JS_NewStringLen make it: each byte that does not start a valid UTF-8
// sequence becomes U+FFFD, a character above U+FFFF a surrogate pair, and
// the string has 8-bit characters when all are below U+0100 and 16-bit
// units otherwise. It writes the characters straight into the new string,
// where newString copied s to C memory and the engine decoded it there. ok
// is false for a string longer than the engine holds, which goes the way of
// the engine to fail as it does.
func (m *VM) newStringFast(s string) (v jsValue, ok bool) {
	if len(s) > stringLenMax {
		return v, false
	}
	b := stringBytes(s)
	i := asciiLen(b)
	if i == len(b) {
		v, p := m.newStringChars(len(b), false)
		if p != 0 {
			copy(unsafe.Slice((*byte)(unsafe.Pointer(p)), len(b)), b)
		}
		return v, true
	}
	units, latin1, valid := utf8Units(b[i:])
	if !valid {
		return m.newStringInvalid(s), true
	}
	units += i
	if latin1 {
		v, p := m.newStringChars(units, false)
		if p != 0 {
			out := unsafe.Slice((*byte)(unsafe.Pointer(p)), units)
			copy(out, b[:i])
			for j := i; i < len(b); j++ {
				c := b[i]
				if c < utf8.RuneSelf {
					out[j] = c
					i++
					continue
				}
				out[j] = c<<6 | b[i+1]&0x3F
				i += 2
			}
		}
		return v, true
	}
	v, p := m.newStringChars(units, true)
	if p != 0 {
		out := unsafe.Slice((*uint16)(unsafe.Pointer(p)), units)
		for j := range i {
			out[j] = uint16(b[j])
		}
		for j := i; i < len(b); {
			switch c := uint32(b[i]); {
			case c < 0x80:
				out[j] = uint16(c)
				i++
				j++
			case c < 0xE0:
				out[j] = uint16(c&0x1F<<6 | uint32(b[i+1])&0x3F)
				i += 2
				j++
			case c < 0xF0:
				out[j] = uint16(c&0x0F<<12 | uint32(b[i+1])&0x3F<<6 | uint32(b[i+2])&0x3F)
				i += 3
				j++
			default:
				r := (c&0x07<<18 | uint32(b[i+1])&0x3F<<12 | uint32(b[i+2])&0x3F<<6 | uint32(b[i+3])&0x3F) - 0x10000
				out[j] = uint16(0xD800 + r>>10)
				out[j+1] = uint16(0xDC00 + r&0x3FF)
				i += 4
				j += 2
			}
		}
	}
	return v, true
}

// utf8Units returns the number of UTF-16 units of b, whether all its
// characters are below U+0100, and whether b is valid UTF-8 as Go decodes
// it: no surrogate, nothing above U+10FFFF, no overlong form, and no
// sequence cut short.
func utf8Units(b []byte) (units int, latin1, valid bool) {
	if !utf8.Valid(b) {
		return 0, false, false
	}
	// In valid UTF-8 every byte but a continuation byte (0x80 to 0xBF)
	// starts a character, one from 0xF0 on starts a character of two
	// units, and one from 0xC4 on starts a character from U+0100 on.
	big := 0
	for _, c := range b {
		if c&0xC0 != 0x80 {
			units++
		}
		if c >= 0xF0 {
			units++
		}
		if c >= 0xC4 {
			big++
		}
	}
	return units, big == 0, true
}

// newStringInvalid is newStringFast for s that is not valid UTF-8: each
// byte that does not start a valid sequence is U+FFFD, as for range reads
// it.
func (m *VM) newStringInvalid(s string) jsValue {
	units, max := 0, rune(0)
	for _, r := range s {
		units++
		if r > 0xFFFF {
			units++
		}
		max = maxRune(max, r)
	}
	v, p := m.newStringChars(units, max >= 0x100)
	if p == 0 {
		return v
	}
	if max < 0x100 {
		out := unsafe.Slice((*byte)(unsafe.Pointer(p)), units)
		j := 0
		for _, r := range s {
			out[j] = byte(r)
			j++
		}
		return v
	}
	out := unsafe.Slice((*uint16)(unsafe.Pointer(p)), units)
	j := 0
	for _, r := range s {
		if r > 0xFFFF {
			r -= 0x10000
			out[j] = uint16(0xD800 + r>>10)
			out[j+1] = uint16(0xDC00 + r&0x3FF)
			j += 2
			continue
		}
		out[j] = uint16(r)
		j++
	}
	return v
}

func maxRune(a, b rune) rune {
	if b > a {
		return b
	}
	return a
}

// NewStringLatin1 returns a new string of the characters b, one byte each:
// a byte is the character of its value, U+0000 to U+00FF. Unlike
// [VM.NewString] it decodes no UTF-8, so a [Builtin] that read its argument
// with [Args.Latin1] returns what it made of it this way, as one copy.
func (m *VM) NewStringLatin1(b []byte) (Value, error) {
	if err := m.usable(); err != nil {
		return Value{}, err
	}
	if len(b) > stringLenMax {
		return Value{}, errStringTooLong(len(b))
	}
	v, p := m.newStringChars(len(b), false)
	if p != 0 {
		copy(unsafe.Slice((*byte)(unsafe.Pointer(p)), len(b)), b)
	}
	return Value{vm: m, v: v}, nil
}

// NewStringUTF16 returns a new string of the 16-bit units u, lone
// surrogates as they are. A [Builtin] that read its argument with
// [Args.UTF16] returns what it made of it this way, as one copy.
func (m *VM) NewStringUTF16(u []uint16) (Value, error) {
	if err := m.usable(); err != nil {
		return Value{}, err
	}
	if len(u) > stringLenMax {
		return Value{}, errStringTooLong(len(u))
	}
	// The engine holds a string whose characters are all below U+0100 one
	// byte each, as JS_NewStringLen and the string buffers make it.
	wide := false
	for _, c := range u {
		if c >= 0x100 {
			wide = true
			break
		}
	}
	v, p := m.newStringChars(len(u), wide)
	if p == 0 {
		return Value{vm: m, v: v}, nil
	}
	if wide {
		copy(unsafe.Slice((*uint16)(unsafe.Pointer(p)), len(u)), u)
	} else {
		out := unsafe.Slice((*byte)(unsafe.Pointer(p)), len(u))
		for i, c := range u {
			out[i] = byte(c)
		}
	}
	return Value{vm: m, v: v}, nil
}

func errStringTooLong(n int) error {
	return fmt.Errorf("slowjs: a string of %d characters is longer than javascript allows (%d)", n, stringLenMax)
}
