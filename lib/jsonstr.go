// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT AND BSD-3-Clause
// The fork's code here is under the MIT license in LICENSE-MIT at the root of
// this repository. Parts follow QuickJS (Copyright (c) 2017-2021 Fabrice
// Bellard and Charlie Gordon, MIT, see LICENSE-LIBQUICKJS) as translated by
// modernc.org/libquickjs (The libquickjs-go Authors, BSD-3-Clause, see
// lib/LICENSE).

package libquickjs // import "github.com/liustack/slowjs/lib"

import (
	"unsafe"

	"modernc.org/libc"
)

// jsonQuoteRun is the start of each turn of the loop of JS_ToQuotedString
// over the string p, from the index at pi (internal/fixlib puts the call
// there, see fixJSONStrings). The loop writes the characters JSON.stringify
// keeps as they are with one call of string_buffer_putc each, and escapes
// the others. jsonQuoteRun writes the run of kept characters from there into
// the StringBuffer b at once, with jsonWrite8 or jsonWrite16, and moves the
// index past them. The loop then escapes the character that
// ended the run, or ends.
//
// A kept character is one the loop puts as it is: not '"', '\\' or below
// 0x20, which it escapes, and, in a string of 16-bit units, no surrogate.
// A surrogate pair goes as it is and a lone surrogate as \uXXXX, which the
// loop decides, so a run stops at either.
//
// It returns -1 when b could not grow, as string_buffer_putc does, and 0
// otherwise.
//
// The jsonStrLen and jsonStrWide constants are where the length and the
// 16-bit flag of a JSString are, and jsonStrData the offset of its
// characters, which fixJSONStrings reads from the file's JS_ToQuotedString
// and string_get.
func jsonQuoteRun(tls *libc.TLS, b uintptr, p uintptr, pi uintptr) int32 {
	n := int32(*(*uint32)(unsafe.Pointer(p + jsonStrLenOff)) & jsonStrLenMask >> jsonStrLenShift)
	i := *(*int32)(unsafe.Pointer(pi))
	j := i
	data := p + jsonStrData
	if *(*uint32)(unsafe.Pointer(p + jsonStrWideOff))&jsonStrWideMask>>jsonStrWideShift == 0 {
		s := unsafe.Slice((*byte)(unsafe.Pointer(data)), n)
		for j < n {
			if c := s[j]; c < 0x20 || c == '"' || c == '\\' {
				break
			}
			j++
		}
		if j > i && jsonWrite8(tls, b, s[i:j]) != 0 {
			return -1
		}
	} else {
		s := unsafe.Slice((*uint16)(unsafe.Pointer(data)), n)
		for j < n {
			if c := s[j]; c < 0x20 || c == '"' || c == '\\' || c&0xF800 == 0xD800 {
				break
			}
			j++
		}
		if j > i && jsonWrite16(tls, b, s[i:j]) != 0 {
			return -1
		}
	}
	*(*int32)(unsafe.Pointer(pi)) = j
	return 0
}

// jsonStringRun is the start of each turn of the loop of json_parse_string
// over the UTF-8 text from p to end (internal/fixlib puts the call there,
// see fixJSONStrings). The loop puts each character of a JSON string into
// the StringBuffer b with one string_buffer_putc, decoding the bytes of one
// that is not ASCII with unicode_from_utf8. jsonStringRun puts the run of
// characters from p that the loop puts as they are, and returns where the
// run ends: at sep, '\\', a byte below 0x20, end, or a byte that does not
// start a character of 2, 3 or 4 bytes of at most U+10FFFF as
// unicode_from_utf8 reads it. The loop then does what it does for that
// byte: end the string, read an escape, or throw the same error at the same
// place. ASCII goes straight into the room b has, or in with jsonWrite8,
// a part with other characters through jsonStringWide.
//
// It returns 0 when b could not grow, which the loop takes as the failure
// of string_buffer_putc.
func jsonStringRun(tls *libc.TLS, b uintptr, p uintptr, end uintptr, sep int32) uintptr {
	sb := (*TStringBuffer)(unsafe.Pointer(b))
	for {
		// While b is 8-bit, copy plain ASCII straight into the room it has,
		// as string_buffer_putc would, byte by byte: most strings of JSON
		// are short. What does not fit goes through jsonWrite8 below.
		if sb.Fis_wide_char == 0 {
			lim := end
			if r := p + uintptr(sb.Fsize-sb.Flen1); r < lim {
				lim = r
			}
			dst := sb.Fstr + jsonStrData + uintptr(sb.Flen1) - p
			q := p
			for q < lim {
				c := *(*byte)(unsafe.Pointer(q))
				if c < 0x20 || c >= 0x80 || int32(c) == sep || c == '\\' {
					break
				}
				*(*byte)(unsafe.Pointer(dst + q)) = c
				q++
			}
			sb.Flen1 += int32(q - p)
			p = q
		}
		q, lim := p, jsonRunLimit(p, end)
		for q < lim {
			if c := *(*byte)(unsafe.Pointer(q)); c < 0x20 || c >= 0x80 || int32(c) == sep || c == '\\' {
				break
			}
			q++
		}
		if q > p {
			if jsonWrite8(tls, b, unsafe.Slice((*byte)(unsafe.Pointer(p)), q-p)) != 0 {
				return 0
			}
			p = q
		}
		if p == end || *(*byte)(unsafe.Pointer(p)) < 0x80 {
			return p
		}
		next := jsonStringWide(tls, b, p, end, sep)
		if next == 0 || next == p {
			return next
		}
		p = next
	}
}

// jsonRunLimit is where the ASCII run of jsonStringRun from p stops at the
// latest: end, or MJS_STRING_LEN_MAX+1 bytes on, so that the length of the
// run fits an int32 and jsonWrite8 of it makes string_buffer_realloc throw
// "string too long", as the next string_buffer_putc did before. The text
// of JS_ParseJSON can be longer than any string.
func jsonRunLimit(p, end uintptr) uintptr {
	if end-p > MJS_STRING_LEN_MAX+1 {
		return p + MJS_STRING_LEN_MAX + 1
	}
	return end
}

// jsonStringWide puts the characters from p, which starts with a byte of
// 0x80 or more, into b, as jsonStringRun does, up to 64 code units at a
// time, and returns where it stopped: past the last one it put, which is
// p when the first byte does not start a character it takes, or 0 when b
// could not grow.
func jsonStringWide(tls *libc.TLS, b uintptr, p uintptr, end uintptr, sep int32) uintptr {
	var units [64]uint16
	n := 0
	for p < end && n <= len(units)-2 {
		c := *(*byte)(unsafe.Pointer(p))
		if c < 0x80 {
			if c < 0x20 || int32(c) == sep || c == '\\' {
				break
			}
			units[n] = uint16(c)
			n++
			p++
			continue
		}
		r, size := jsonUTF8(p)
		if size == 0 {
			break
		}
		if r >= 0x10000 {
			units[n] = uint16(0xD800 + (r-0x10000)>>10)
			units[n+1] = uint16(0xDC00 + r&0x3FF)
			n += 2
		} else {
			units[n] = uint16(r)
			n++
		}
		p += size
	}
	if n > 0 && jsonWrite16(tls, b, units[:n]) != 0 {
		return 0
	}
	return p
}

// jsonUTF8 decodes the character of 2, 3 or 4 bytes at p as
// unicode_from_utf8 does: a lead byte, continuation bytes 0x80 to 0xBF and
// no shorter form than needed. Like unicode_from_utf8 it takes the three
// bytes of a surrogate. It returns size 0 for a byte unicode_from_utf8
// rejects, for one it reads as a character of 5 or 6 bytes and for a
// character above U+10FFFF, which json_parse_string then rejects itself.
// The text ends with a 0 byte, as the parser's buffer does, so it never
// reads past it.
func jsonUTF8(p uintptr) (r rune, size uintptr) {
	c := *(*byte)(unsafe.Pointer(p))
	var l uintptr
	var min rune
	switch {
	case c >= 0xC0 && c <= 0xDF:
		l, min, r = 1, 0x80, rune(c&0x1F)
	case c >= 0xE0 && c <= 0xEF:
		l, min, r = 2, 0x800, rune(c&0x0F)
	case c >= 0xF0 && c <= 0xF7:
		l, min, r = 3, 0x10000, rune(c&0x07)
	default:
		return 0, 0
	}
	for i := uintptr(1); i <= l; i++ {
		b := *(*byte)(unsafe.Pointer(p + i))
		if b < 0x80 || b >= 0xC0 {
			return 0, 0
		}
		r = r<<6 | rune(b&0x3F)
	}
	if r < min || r > 0x10FFFF {
		return 0, 0
	}
	return r, l + 1
}

// jsonWrite8 is string_buffer_write8 with a Go copy in place of the call
// of memcpy: it grows b as string_buffer_write8 does, then copies the
// bytes in, as bytes or, when b holds 16-bit units, as units. It returns -1
// when b could not grow. src holds at most MJS_STRING_LEN_MAX+1 bytes (see
// jsonRunLimit), so its length fits an int32.
func jsonWrite8(tls *libc.TLS, s uintptr, src []byte) int32 {
	sb := (*TStringBuffer)(unsafe.Pointer(s))
	n := int32(len(src))
	if sb.Flen1+n > sb.Fsize {
		if _string_buffer_realloc(tls, s, sb.Flen1+n, 0) != 0 {
			return -1
		}
	}
	data := sb.Fstr + jsonStrData
	if sb.Fis_wide_char != 0 {
		dst := unsafe.Slice((*uint16)(unsafe.Pointer(data+uintptr(sb.Flen1)*2)), n)
		for i, c := range src {
			dst[i] = uint16(c)
		}
	} else {
		copy(unsafe.Slice((*byte)(unsafe.Pointer(data+uintptr(sb.Flen1))), n), src)
	}
	sb.Flen1 += n
	return 0
}

// jsonWrite16 is string_buffer_write16 with Go copies: it grows or widens
// b as string_buffer_write16 does, then copies the units in, as 16-bit
// units or, when b holds 8-bit characters and every unit fits, as bytes.
// It returns -1 when b could not grow.
func jsonWrite16(tls *libc.TLS, s uintptr, units []uint16) int32 {
	sb := (*TStringBuffer)(unsafe.Pointer(s))
	var c uint16
	for _, u := range units {
		c |= u
	}
	n := int32(len(units))
	if sb.Flen1+n > sb.Fsize {
		if _string_buffer_realloc(tls, s, sb.Flen1+n, int32(c)) != 0 {
			return -1
		}
	} else if sb.Fis_wide_char == 0 && c >= 0x100 {
		if _string_buffer_widen(tls, s, sb.Fsize) != 0 {
			return -1
		}
	}
	data := sb.Fstr + jsonStrData
	if sb.Fis_wide_char != 0 {
		copy(unsafe.Slice((*uint16)(unsafe.Pointer(data+uintptr(sb.Flen1)*2)), n), units)
	} else {
		dst := unsafe.Slice((*byte)(unsafe.Pointer(data+uintptr(sb.Flen1))), n)
		for i, u := range units {
			dst[i] = byte(u)
		}
	}
	sb.Flen1 += n
	return 0
}
