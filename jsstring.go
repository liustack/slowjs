// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package quickjs // import "github.com/liustack/quickjs"

import (
	"encoding/binary"
	"fmt"
	"math/bits"
	"sync"
	"unicode/utf8"
	"unsafe"

	"github.com/liustack/quickjs/internal/hostmem"
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
		return latin1String(m, unsafe.Slice((*byte)(addr(p)), n))
	}
	return utf16String(m, unsafe.Slice((*uint16)(addr(p)), n), wtf8)
}

// argBlock is argString for a Go function of a door (vmhook.LongWTF8) that
// uses the string only during the call. A string longer than stringStep is
// written into a block (hostmem.Take) of its length when that is BlockMin
// bytes or more, and give gives the block back: nothing may keep the string,
// or a part of it, after that. A Go allocation of the string would be
// cleared in one go, with no read of whether to stop, while the OS hands
// over its pages: 30 ms for 16 MiB on the virtual M1 of GitHub's macos-15
// runner under Rosetta. The pages of a block come as the pieces are written,
// with a read between two. ok is false as for argString, and give does
// nothing then.
func (m *VM) argBlock(v jsValue, wtf8 bool) (s string, give func(), ok bool) {
	give = func() {}
	p, n, wide, isFlat := stringChars(v)
	if !isFlat && tag(v) == tagStringRope {
		flat := m.jsToString(v)
		if isException(flat) {
			return "", give, false
		}
		s, give, ok = m.argBlock(flat, wtf8)
		m.jsFreeValue(flat)
		return s, give, ok
	}
	if !isFlat || n <= stringStep {
		s, ok = m.argString(v, wtf8)
		return s, give, ok
	}
	var held func()
	mk := func(size int) ([]byte, bool) {
		b, g, err := hostmem.Take[byte](size)
		if err != nil {
			m.goOutOfMemory()
			return nil, false
		}
		held = g
		return b, true
	}
	if wide {
		s, ok = utf16Steps(m, unsafe.Slice((*uint16)(addr(p)), n), wtf8, mk)
	} else {
		s, ok = latin1Steps(m, unsafe.Slice((*byte)(addr(p)), n), mk)
	}
	if held != nil {
		if !ok {
			held()
			return "", give, false
		}
		give = held
	}
	return s, give, ok
}

// stringStep is how many characters, or bytes of UTF-8, a conversion of a
// string between Go and the engine takes between two reads of whether the
// call is to stop (convStop). No poll runs while Go converts, so a string a
// script made long, up to stringLenMax characters, would hold off Interrupt
// and the eval timeout until it is done (a string of 16 Mi CJK characters
// took 55 ms to read and 145 ms to make on an M-series Mac). A piece takes
// at most 1.6 ms on an amd64 server (EPYC, t2d) and 2.7 ms on an Apple M4,
// far below the grace of 50 ms after which a VM is killed
// (gohost.DefaultKillGrace). The allocation of the Go string a read returns
// cannot be cut: Go zeroes it and may have the goroutine help the GC, up to
// 14 ms (amd64) and 2.7 ms (M4) for 16 Mi characters, 52 and 55 ms for 64
// Mi (docs/changes/go-api.md). A string of at most stringStep is converted
// as before, with no read. Tests make it small.
var stringStep = 1 << 18

// stringStepHook, when a test sets it, runs before each read of convStop.
var stringStepHook func()

// stringLenLimit is the most characters newString makes a string of:
// stringLenMax, the most the engine holds. Tests make it small. Only a
// string longer than stringStep, far fewer characters, can be too long, so
// only newStringSteps checks it.
var stringLenLimit = stringLenMax

// stringTooLong throws the error JS_NewStringLen throws for a string of more
// characters than the engine holds, and returns the exception.
func (m *VM) stringTooLong() jsValue {
	return m.jsThrowError(errorKindInternal, "string too long")
}

// atomOf returns the atom of the Go string s, as JS_NewAtomLen makes it of
// validUTF8(s), or atomNull and the error. Go would read a long s whole
// before the engine got it, so a long s is made a string first, a piece at
// a time (newStringFast), and is then an atom.
func (m *VM) atomOf(s string) (uint32, error) {
	if len(s) <= stringStep {
		a, err := m.jsNewAtom(validUTF8(s))
		if err == nil && a == atomNull {
			err = m.errFromException()
		}
		return a, err
	}
	v := m.newStringFast(s)
	if isException(v) {
		return atomNull, m.errFromException()
	}
	a := m.jsValueToAtom(v)
	m.jsFreeValue(v)
	if a == atomNull {
		return atomNull, m.errFromException()
	}
	return a, nil
}

// convStop reports whether a conversion of a long string is to stop, and
// then leaves the uncatchable "interrupted" pending, as NewArray does
// (goStop): only in a Go function that javascript called, at Interrupt,
// the eval timeout or an uncatchable error of the call. m is nil for a
// conversion that never stops.
func (m *VM) convStop() bool {
	if stringStepHook != nil {
		stringStepHook()
	}
	return m != nil && m.host > 0 && m.goStop()
}

// elemStop counts an element or a property that the conversion of a result
// of a Go function makes, and reads whether the call is to stop every
// newArrayStep of them, as NewArray does, however the values nest: each
// takes a call into the engine, about 50 ns or more, that reads no interrupt
// flag, and how many there are is up to what the Go function got from the
// script. The count goes on from one conversion to the next. (An argument of
// Call that is not a string or a number goes through JSON, which the engine
// parses with its own reads.)
func (m *VM) elemStop() bool {
	m.elems++
	return m.elems&(newArrayStep-1) == 0 && m.convStop()
}

// cut16 returns where the piece of u from lo ends: lo + stringStep, or one
// more when a surrogate pair would be cut there, or len(u).
func cut16(u []uint16, lo int) int {
	hi := lo + stringStep
	if hi >= len(u) {
		return len(u)
	}
	if u[hi-1]&0xFC00 == 0xD800 && u[hi]&0xFC00 == 0xDC00 {
		hi++
	}
	return hi
}

// cutUTF8 returns where the piece of b from lo ends, about stringStep bytes
// on, at a place no character of b crosses as Go decodes it (for range,
// utf8.Valid), the nearest at or before lo + stringStep, or else the next
// after it. Or len(b).
func cutUTF8(b []byte, lo int) int {
	hi := lo + stringStep
	if hi >= len(b) {
		return len(b)
	}
	for e := hi; e > lo; e-- {
		if utf8Cut(b, e) {
			return e
		}
	}
	for e := hi + 1; e < len(b); e++ {
		if utf8Cut(b, e) {
			return e
		}
	}
	return len(b)
}

// utf8Cut reports whether no character of b crosses e: b[e] is not a
// continuation byte, or the three bytes before it are, so that none of them
// starts a character that could reach e. One of any four places in a row is
// such a place.
func utf8Cut(b []byte, e int) bool {
	if b[e]&0xC0 != 0x80 {
		return true
	}
	for k := e - 1; k >= e-3 && k >= 0; k-- {
		if b[k]&0xC0 != 0x80 {
			return false
		}
	}
	return true
}

// An outFunc makes the n bytes a long conversion writes its string into, or
// returns false, with the error that ends the call pending, when it cannot.
// goOut is a Go allocation, for a string Go keeps (argString). A Go function
// of a door that uses its string only during the call takes a block instead
// (argBlock).
type outFunc func(n int) ([]byte, bool)

func goOut(n int) ([]byte, bool) { return make([]byte, n), true }

// copySteps copies src into a new string, a piece at a time for a long one.
// ok is false when m stopped it (convStop).
func copySteps(m *VM, src []byte, mk outFunc) (string, bool) {
	if len(src) <= stringStep {
		return string(src), true
	}
	if m.convStop() {
		return "", false
	}
	out, ok := mk(len(src))
	if !ok {
		return "", false
	}
	for lo := 0; lo < len(src); lo += stringStep {
		if lo > 0 && m.convStop() {
			return "", false
		}
		copy(out[lo:], src[lo:min(lo+stringStep, len(src))])
	}
	return unsafe.String(unsafe.SliceData(out), len(out)), true
}

// asciiSteps is asciiLen a piece at a time for a long b. It is -1 when m
// stopped it.
func asciiSteps(m *VM, b []byte) int {
	if len(b) <= stringStep {
		return asciiLen(b)
	}
	for lo := 0; lo < len(b); lo += stringStep {
		if lo > 0 && m.convStop() {
			return -1
		}
		hi := min(lo+stringStep, len(b))
		if n := asciiLen(b[lo:hi]); n < hi-lo {
			return lo + n
		}
	}
	return len(b)
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
// point below U+0100, as JS_ToCStringLen2 writes them. ok is false when m
// stopped it (convStop), for a long b.
func latin1String(m *VM, b []byte) (string, bool) {
	if len(b) > stringStep {
		return latin1Steps(m, b, goOut)
	}
	i := asciiLen(b)
	if i == len(b) {
		return string(b), true
	}
	src := b[i:]
	n := len(b) + latin1Extra(src)
	out := make([]byte, n)
	copy(out, b[:i])
	latin1Write(out, i, src)
	return unsafe.String(unsafe.SliceData(out), n), true
}

// latin1Steps is latin1String for a b longer than stringStep, a piece at a
// time, with a read of whether to stop between two (convStop). ok is false
// when m stopped it.
func latin1Steps(m *VM, b []byte, mk outFunc) (string, bool) {
	i := asciiSteps(m, b)
	if i < 0 {
		return "", false
	}
	if i == len(b) {
		return copySteps(m, b, mk)
	}
	src := b[i:]
	n := len(b)
	for lo := 0; lo < len(src); lo += stringStep {
		if lo > 0 && m.convStop() {
			return "", false
		}
		n += latin1Extra(src[lo:min(lo+stringStep, len(src))])
	}
	if len(src) > stringStep && m.convStop() {
		return "", false
	}
	out, ok := mk(n)
	if !ok {
		return "", false
	}
	// The ASCII before the first other character is copied in steps too:
	// its pages come from the OS as it is written (hostmem.Take).
	for lo := 0; lo < i; lo += stringStep {
		if lo > 0 && m.convStop() {
			return "", false
		}
		copy(out[lo:], b[lo:min(lo+stringStep, i)])
	}
	if i > stringStep && m.convStop() {
		return "", false
	}
	j := i
	for lo := 0; lo < len(src); lo += stringStep {
		if lo > 0 && m.convStop() {
			return "", false
		}
		j = latin1Write(out, j, src[lo:min(lo+stringStep, len(src))])
	}
	return unsafe.String(unsafe.SliceData(out), n), true
}

// latin1Extra returns how many more bytes than characters the UTF-8 of the
// 8-bit characters b takes: one for each from U+0080.
func latin1Extra(b []byte) int {
	n, k := 0, 0
	for ; k+8 <= len(b); k += 8 {
		n += bits.OnesCount64(binary.LittleEndian.Uint64(b[k:]) & 0x8080808080808080)
	}
	for _, c := range b[k:] {
		n += int(c >> 7)
	}
	return n
}

// latin1Write writes the UTF-8 of the 8-bit characters b to out from j, and
// returns where it ended.
func latin1Write(out []byte, j int, b []byte) int {
	for _, c := range b {
		if c < utf8.RuneSelf {
			out[j] = c
			j++
			continue
		}
		out[j] = 0xC0 | c>>6
		out[j+1] = 0x80 | c&0x3F
		j += 2
	}
	return j
}

// utf16String returns the UTF-8 form of the 16-bit units u, a surrogate pair
// as its one character. A lone surrogate is U+FFFD, as wellFormed makes it,
// or with wtf8 the three bytes of its code point, as JS_ToCStringLen2 writes
// it. ok is false when m stopped it (convStop), for a long u.
func utf16String(m *VM, u []uint16, wtf8 bool) (string, bool) {
	// Each unit takes at most 3 bytes, a pair 4. The UTF-8 is written once,
	// into memory on the stack for a short string and kept for the next ones
	// otherwise, and copied into a string of its size, as JS_ToCStringLen2
	// wrote it in C and Go copied it, where counting the bytes first took a
	// second pass over u.
	if len(u) <= utf16Short {
		var b [3 * utf16Short]byte
		return string(b[:utf16Write(b[:], u, wtf8)]), true
	}
	if len(u) > stringStep {
		return utf16Steps(m, u, wtf8, goOut)
	}
	p := utf16Scratch.Get().(*[]byte)
	if cap(*p) < 3*len(u) {
		*p = make([]byte, 3*len(u))
	}
	s := string((*p)[:utf16Write((*p)[:3*len(u)], u, wtf8)])
	utf16Scratch.Put(p)
	return s, true
}

// utf16Steps is utf16String for a u longer than stringStep, a piece at a
// time, with a read of whether to stop between two (convStop): it counts the
// bytes of the UTF-8, makes the string of that size, and writes each piece
// into the scratch memory of one piece and copies it from there. ok is
// false when m stopped it.
func utf16Steps(m *VM, u []uint16, wtf8 bool, mk outFunc) (string, bool) {
	n := 0
	for lo := 0; lo < len(u); {
		if lo > 0 && m.convStop() {
			return "", false
		}
		hi := cut16(u, lo)
		n += utf16Len(u[lo:hi])
		lo = hi
	}
	if m.convStop() {
		return "", false
	}
	out, ok := mk(n)
	if !ok {
		return "", false
	}
	// Each piece is written through a buffer of stepScratch units, which
	// utf16Write needs room of 3 bytes a unit in: one of a whole piece,
	// 768 KiB, came from utf16Scratch, which Go empties at a collection and
	// keeps for each P apart, so a call often made it again.
	buf := make([]byte, 3*(stepScratch+1))
	j := 0
	for lo := 0; lo < len(u); {
		if lo > 0 && m.convStop() {
			return "", false
		}
		hi := cut16(u, lo)
		for a := lo; a < hi; {
			b := min(hi, a+stepScratch)
			if b < hi && u[b-1]&0xFC00 == 0xD800 && u[b]&0xFC00 == 0xDC00 {
				b++
			}
			j += copy(out[j:], buf[:utf16Write(buf[:3*(b-a)], u[a:b], wtf8)])
			a = b
		}
		lo = hi
	}
	if j != n {
		panic("quickjs: the UTF-8 of the units is not the length counted")
	}
	return unsafe.String(unsafe.SliceData(out), n), true
}

// utf16Len returns how many bytes utf16Write writes for u: 1, 2 or 3 for a
// unit, as its value is below 0x80, below 0x800 or not, and 4 for a
// surrogate pair. A lone surrogate is 3 bytes, of U+FFFD or of itself.
func utf16Len(u []uint16) int {
	n := 0
	for i := 0; i < len(u); i++ {
		switch c := u[i]; {
		case c < 0x80:
			n++
		case c < 0x800:
			n += 2
		case c&0xFC00 == 0xD800 && i+1 < len(u) && u[i+1]&0xFC00 == 0xDC00:
			n += 4
			i++
		default:
			n += 3
		}
	}
	return n
}

// stepScratch is how many units utf16Steps writes through its buffer at a
// time: a buffer of 48 KiB.
const stepScratch = 1 << 14

// utf16Short is the length of the strings utf16String converts on the stack.
const utf16Short = 64

// utf16Scratch keeps the memory utf16String writes longer strings in.
var utf16Scratch = sync.Pool{New: func() any { return new([]byte) }}

// utf16Write writes the UTF-8 of u to out, which has room for 3 bytes a
// unit, as utf16String converts it, and returns how many bytes it wrote.
func utf16Write(out []byte, u []uint16, wtf8 bool) int {
	if len(out) < 3*len(u) {
		panic("quickjs: no room for the UTF-8 of the units")
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
// where newString copied s to C memory and the engine decoded it there. A
// string of more characters than the engine holds throws the error the
// engine throws for it. A long s is read and written a piece at a time, and
// v is the exception when m stopped it there (convStop, newStringSteps).
func (m *VM) newStringFast(s string) (v jsValue) {
	if len(s) > stringStep {
		return m.newStringSteps(s)
	}
	b := stringBytes(s)
	i := asciiLen(b)
	if i == len(b) {
		v, p := m.newStringChars(len(b), false)
		if p != 0 {
			copy(unsafe.Slice((*byte)(addr(p)), len(b)), b)
		}
		return v
	}
	units, latin1, valid := utf8Units(b[i:])
	if !valid {
		return m.newStringInvalid(s)
	}
	units += i
	if latin1 {
		v, p := m.newStringChars(units, false)
		if p != 0 {
			out := unsafe.Slice((*byte)(addr(p)), units)
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
		return v
	}
	v, p := m.newStringChars(units, true)
	if p != 0 {
		out := unsafe.Slice((*uint16)(addr(p)), units)
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
	return v
}

// newStringSteps is newStringFast for an s longer than stringStep, which it
// reads and writes a piece at a time, with a read of whether to stop between
// two (convStop). The pieces end where no character crosses (cutUTF8), so
// they make what the whole makes. It is the exception when m stopped it.
func (m *VM) newStringSteps(s string) (v jsValue) {
	b := stringBytes(s)
	i := asciiSteps(m, b)
	if i < 0 {
		return exception
	}
	if i == len(b) {
		if len(b) > stringLenLimit {
			return m.stringTooLong()
		}
		v, p := m.newStringChars(len(b), false)
		if p != 0 {
			out := unsafe.Slice((*byte)(addr(p)), len(b))
			for lo := 0; lo < len(b); lo += stringStep {
				if lo > 0 && m.convStop() {
					m.jsFreeValue(v)
					return exception
				}
				copy(out[lo:], b[lo:min(lo+stringStep, len(b))])
			}
		}
		return v
	}
	units, latin1, valid := 0, true, true
	for lo := i; lo < len(b) && valid; {
		if lo > i && m.convStop() {
			return exception
		}
		hi := cutUTF8(b, lo)
		n, l, ok := utf8Units(b[lo:hi])
		units, latin1, valid = units+n, latin1 && l, ok
		lo = hi
	}
	if !valid {
		return m.newStringInvalid(s)
	}
	units += i
	if units > stringLenLimit {
		return m.stringTooLong()
	}
	if latin1 {
		v, p := m.newStringChars(units, false)
		if p != 0 {
			out := unsafe.Slice((*byte)(addr(p)), units)
			copy(out, b[:i])
			for j, lo := i, i; lo < len(b); {
				if lo > i && m.convStop() {
					m.jsFreeValue(v)
					return exception
				}
				hi := cutUTF8(b, lo)
				j = utf8Latin1Write(out, j, b[lo:hi])
				lo = hi
			}
		}
		return v
	}
	v, p := m.newStringChars(units, true)
	if p != 0 {
		out := unsafe.Slice((*uint16)(addr(p)), units)
		for j := range i {
			out[j] = uint16(b[j])
		}
		for j, lo := i, i; lo < len(b); {
			if lo > i && m.convStop() {
				m.jsFreeValue(v)
				return exception
			}
			hi := cutUTF8(b, lo)
			j = utf8UnitsWrite(out, j, b[lo:hi])
			lo = hi
		}
	}
	return v
}

// utf8Latin1Write writes the characters of the valid UTF-8 b, all below
// U+0100, to out from j, one byte each, and returns where it ended.
func utf8Latin1Write(out []byte, j int, b []byte) int {
	for i := 0; i < len(b); j++ {
		c := b[i]
		if c < utf8.RuneSelf {
			out[j] = c
			i++
			continue
		}
		out[j] = c<<6 | b[i+1]&0x3F
		i += 2
	}
	return j
}

// utf8UnitsWrite writes the UTF-16 units of the valid UTF-8 b to out from
// j, and returns where it ended.
func utf8UnitsWrite(out []uint16, j int, b []byte) int {
	for i := 0; i < len(b); {
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
	return j
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
	for lo := 0; lo < len(s); {
		if lo > 0 && m.convStop() {
			return exception
		}
		hi := cutUTF8(stringBytes(s), lo)
		for _, r := range s[lo:hi] {
			units++
			if r > 0xFFFF {
				units++
			}
			max = maxRune(max, r)
		}
		lo = hi
	}
	if units > stringLenLimit {
		return m.stringTooLong()
	}
	v, p := m.newStringChars(units, max >= 0x100)
	if p == 0 {
		return v
	}
	j := 0
	for lo := 0; lo < len(s); {
		if lo > 0 && m.convStop() {
			m.jsFreeValue(v)
			return exception
		}
		hi := cutUTF8(stringBytes(s), lo)
		if max < 0x100 {
			j = runesLatin1Write(unsafe.Slice((*byte)(addr(p)), units), j, s[lo:hi])
		} else {
			j = runesUnitsWrite(unsafe.Slice((*uint16)(addr(p)), units), j, s[lo:hi])
		}
		lo = hi
	}
	return v
}

// runesLatin1Write writes the characters of s as for range reads them, all
// below U+0100, to out from j, one byte each, and returns where it ended.
func runesLatin1Write(out []byte, j int, s string) int {
	for _, r := range s {
		out[j] = byte(r)
		j++
	}
	return j
}

// runesUnitsWrite writes the UTF-16 units of the characters of s as for
// range reads them to out from j, and returns where it ended.
func runesUnitsWrite(out []uint16, j int, s string) int {
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
	return j
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
// with [Args.Latin1] returns what it made of it this way, as one copy. It
// ends at the limits of the call as NewString does.
func (m *VM) NewStringLatin1(b []byte) (Value, error) {
	if err := m.usable(); err != nil {
		return Value{}, err
	}
	if len(b) > stringLenMax {
		return Value{}, errStringTooLong(len(b))
	}
	v, p := m.newStringChars(len(b), false)
	if isException(v) {
		return Value{}, m.errFromException()
	}
	if p != 0 {
		out := unsafe.Slice((*byte)(addr(p)), len(b))
		for lo := 0; lo < len(b); lo += stringStep {
			if lo > 0 && m.convStop() {
				m.jsFreeValue(v)
				return Value{}, m.errFromException()
			}
			copy(out[lo:], b[lo:min(lo+stringStep, len(b))])
		}
	}
	return Value{vm: m, v: v}, nil
}

// NewStringUTF16 returns a new string of the 16-bit units u, lone
// surrogates as they are. A [Builtin] that read its argument with
// [Args.UTF16] returns what it made of it this way, as one copy. It ends at
// the limits of the call as NewString does.
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
	for lo := 0; lo < len(u) && !wide; lo += stringStep {
		if lo > 0 && m.convStop() {
			return Value{}, m.errFromException()
		}
		for _, c := range u[lo:min(lo+stringStep, len(u))] {
			if c >= 0x100 {
				wide = true
				break
			}
		}
	}
	v, p := m.newStringChars(len(u), wide)
	if isException(v) {
		return Value{}, m.errFromException()
	}
	if p == 0 {
		return Value{vm: m, v: v}, nil
	}
	for lo := 0; lo < len(u); lo += stringStep {
		if lo > 0 && m.convStop() {
			m.jsFreeValue(v)
			return Value{}, m.errFromException()
		}
		hi := min(lo+stringStep, len(u))
		if wide {
			copy(unsafe.Slice((*uint16)(addr(p)), len(u))[lo:], u[lo:hi])
			continue
		}
		out := unsafe.Slice((*byte)(addr(p)), len(u))
		for i, c := range u[lo:hi] {
			out[lo+i] = byte(c)
		}
	}
	return Value{vm: m, v: v}, nil
}

func errStringTooLong(n int) error {
	return fmt.Errorf("quickjs: a string of %d characters is longer than javascript allows (%d)", n, stringLenMax)
}
