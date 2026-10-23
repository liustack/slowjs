// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT AND BSD-3-Clause
// The fork's code here is under the MIT license in LICENSE-MIT at the root of
// this repository. Parts follow QuickJS (Copyright (c) 2017-2021 Fabrice
// Bellard and Charlie Gordon, MIT, see LICENSE-LIBQUICKJS) as translated by
// modernc.org/libquickjs (The libquickjs-go Authors, BSD-3-Clause, see
// lib/LICENSE).

package libquickjs // import "github.com/liustack/quickjs/lib"

import (
	"bytes"
	"unsafe"
)

// stringFindRoom is the longest string stringFind looks for with the
// search of package bytes when the two strings differ in width: it is
// widened or narrowed into an array of this length on the stack.
const stringFindRoom = 64

// stringFind is the start of each turn of the loops that look for the
// string p2 in the string p1 by comparing all of p2 with string_cmp at one
// index after the other: those of String.prototype.includes, startsWith,
// endsWith, indexOf and lastIndexOf (internal/fixlib puts the call there,
// see fixStringSearch). From the index i toward stop, both included, by
// step inc (1 or -1), it returns the first index where p1 has the
// characters of p2, and the loop compares there, finds them and ends. It
// returns -1 when there is none, and the loop ends without a match. When i
// is stop, it returns i and leaves the one comparison to the loop.
//
// The characters of p1 from the lower of i and stop to the higher plus the
// length of p2 must be in p1, as the loops make sure.
//
// When both strings have 8-bit characters, or p1 has 16-bit characters,
// it uses the search of package bytes (16-bit characters as pairs of bytes,
// at even offsets only), and otherwise compares the characters one by one,
// as string_cmp does. A string of 16-bit characters may hold only
// characters below 0x100, so p2 may be found in p1 whatever their widths.
//
// It counts a step for each 8 indexes it looks through toward the
// interrupt counter of ctx, as the string buffer counts the characters it
// makes room for (see lib/charge.go), so that a loop of searches in a long
// string stops on time.
//
// The jsonStrLen and jsonStrWide constants are where the length and the
// 16-bit flag of a JSString are, and jsonStrData the offset of its
// characters (see fixJSONStrings).
func stringFind(ctx, p1, p2 uintptr, i, stop, inc int32) int32 {
	// Small enough for Go to inline, so that startsWith and endsWith, which
	// compare at one index, pay no call.
	if i == stop {
		return i
	}
	return stringFindRange(ctx, p1, p2, i, stop, inc)
}

// stringFindRange is stringFind from i to a stop other than i.
func stringFindRange(ctx, p1, p2 uintptr, i, stop, inc int32) int32 {
	lo, hi := i, stop
	if inc < 0 {
		lo, hi = stop, i
	}
	if lo > hi {
		return -1
	}
	n1, n2 := stringLen(p1), stringLen(p2)
	if lo < 0 || hi+n2 > n1 {
		panic("stringFind: the range is not in the string")
	}
	if n2 == 0 {
		return i
	}
	wide1, wide2 := stringWide(p1), stringWide(p2)
	data1, data2 := p1+jsonStrData, p2+jsonStrData
	if !wide1 {
		hay := unsafe.Slice((*byte)(unsafe.Pointer(data1+uintptr(lo))), hi-lo+n2)
		var needle []byte
		if !wide2 {
			needle = unsafe.Slice((*byte)(unsafe.Pointer(data2)), n2)
		} else if n2 <= stringFindRoom {
			var room [stringFindRoom]byte
			for k, c := range unsafe.Slice((*uint16)(unsafe.Pointer(data2)), n2) {
				if c > 0xff {
					return -1
				}
				room[k] = byte(c)
			}
			needle = room[:n2]
		} else {
			return stringCounted(ctx, i, stringFindSlow(p1, p2, i, stop, inc), stop, inc)
		}
		var k int
		if inc > 0 {
			k = bytes.Index(hay, needle)
		} else {
			k = bytes.LastIndex(hay, needle)
		}
		if k < 0 {
			return stringCounted(ctx, i, -1, stop, inc)
		}
		return stringCounted(ctx, i, lo+int32(k), stop, inc)
	}
	var needle []byte
	if wide2 {
		needle = unsafe.Slice((*byte)(unsafe.Pointer(data2)), 2*int(n2))
	} else if n2 <= stringFindRoom {
		var room [stringFindRoom]uint16
		for k, c := range unsafe.Slice((*byte)(unsafe.Pointer(data2)), n2) {
			room[k] = uint16(c)
		}
		needle = unsafe.Slice((*byte)(unsafe.Pointer(&room[0])), 2*int(n2))
	} else {
		return stringCounted(ctx, i, stringFindSlow(p1, p2, i, stop, inc), stop, inc)
	}
	hay := unsafe.Slice((*byte)(unsafe.Pointer(data1+2*uintptr(lo))), 2*int(hi-lo+n2))
	if inc > 0 {
		for from := 0; ; {
			k := bytes.Index(hay[from:], needle)
			if k < 0 {
				return stringCounted(ctx, i, -1, stop, inc)
			}
			if k += from; k&1 == 0 {
				return stringCounted(ctx, i, lo+int32(k/2), stop, inc)
			}
			from = k + 1
		}
	}
	for end := len(hay); ; {
		k := bytes.LastIndex(hay[:end], needle)
		if k < 0 {
			return stringCounted(ctx, i, -1, stop, inc)
		}
		if k&1 == 0 {
			return stringCounted(ctx, i, lo+int32(k/2), stop, inc)
		}
		end = k + len(needle) - 1
	}
}

// stringCounted returns k, the index a search from i toward stop by step
// inc found, or -1, and counts a step for each 8 indexes it looked through
// toward the interrupt counter of ctx.
func stringCounted(ctx uintptr, i, k, stop, inc int32) int32 {
	end := k
	if k < 0 {
		end = stop
	}
	_js_charge_interrupts(ctx, Tsize_t(uint32((end-i)*inc)>>3))
	return k
}

// stringFindSlow is stringFind for a string of 8-bit characters longer
// than stringFindRoom looked for in one of 16-bit characters, or the other
// way round: it compares the characters one by one, as string_cmp does.
func stringFindSlow(p1, p2 uintptr, i, stop, inc int32) int32 {
	n2 := stringLen(p2)
	for {
		k := int32(0)
		for k < n2 && stringChar(p1, i+k) == stringChar(p2, k) {
			k++
		}
		if k == n2 {
			return i
		}
		if i == stop {
			return -1
		}
		i += inc
	}
}

func stringLen(p uintptr) int32 {
	return int32(*(*uint32)(unsafe.Pointer(p + jsonStrLenOff)) & jsonStrLenMask >> jsonStrLenShift)
}

func stringWide(p uintptr) bool {
	return *(*uint32)(unsafe.Pointer(p + jsonStrWideOff))&jsonStrWideMask>>jsonStrWideShift != 0
}

// stringChar is the character at index k of the string p, as string_get
// reads it.
func stringChar(p uintptr, k int32) uint16 {
	if stringWide(p) {
		return *(*uint16)(unsafe.Pointer(p + jsonStrData + 2*uintptr(k)))
	}
	return uint16(*(*byte)(unsafe.Pointer(p + jsonStrData + uintptr(k))))
}
