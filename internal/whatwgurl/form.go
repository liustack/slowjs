// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package whatwgurl

import (
	"strings"
	"unicode/utf8"
)

// The application/x-www-form-urlencoded parser and serializer of the standard
// (https://url.spec.whatwg.org/#application/x-www-form-urlencoded), for one
// name or value at a time: the caller splits a query string at & and =.

// FormDecode is what the parser makes of a name or a value: + becomes a space,
// it is percent-decoded, and the bytes are decoded as UTF-8 without BOM, each
// ill-formed byte sequence as the Encoding Standard's decoder makes it, one
// U+FFFD. It is never longer than in: a %XX is at most one unit, and the bytes
// of a character of in stay that character.
func FormDecode(in Input) string {
	s := in.s
	if strings.IndexByte(s, '+') < 0 && strings.IndexByte(s, '%') < 0 && utf8.ValidString(s) {
		return s
	}
	b := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c == '+':
			b = append(b, ' ')
		case c == '%' && i+2 < len(s) && isASCIIHex(rune(s[i+1])) && isASCIIHex(rune(s[i+2])):
			b = append(b, hexVal(s[i+1])<<4|hexVal(s[i+2]))
			i += 2
		default:
			b = append(b, c)
		}
	}
	if utf8.Valid(b) {
		return unsafeString(b)
	}
	return string(decodeUTF8(b))
}

// decodeUTF8 is the UTF-8 decoder of the Encoding Standard, with U+FFFD for
// the error of each maximal ill-formed subsequence.
func decodeUTF8(b []byte) []byte {
	out := make([]byte, 0, len(b)+8)
	for i := 0; i < len(b); {
		c := b[i]
		if c < 0x80 {
			out = append(out, c)
			i++
			continue
		}
		var need int
		var cp rune
		lower, upper := byte(0x80), byte(0xBF)
		switch {
		case 0xC2 <= c && c <= 0xDF:
			need, cp = 1, rune(c&0x1F)
		case 0xE0 <= c && c <= 0xEF:
			need, cp = 2, rune(c&0x0F)
			if c == 0xE0 {
				lower = 0xA0
			}
			if c == 0xED {
				upper = 0x9F
			}
		case 0xF0 <= c && c <= 0xF4:
			need, cp = 3, rune(c&0x07)
			if c == 0xF0 {
				lower = 0x90
			}
			if c == 0xF4 {
				upper = 0x8F
			}
		default:
			out = append(out, "�"...)
			i++
			continue
		}
		i++
		ok := true
		for k := 0; k < need; k++ {
			if i >= len(b) || b[i] < lower || b[i] > upper {
				ok = false
				break
			}
			lower, upper = 0x80, 0xBF
			cp = cp<<6 | rune(b[i]&0x3F)
			i++
		}
		if ok {
			out = utf8.AppendRune(out, cp)
		} else {
			out = append(out, "�"...)
		}
	}
	return out
}

// FormEncode is what the serializer writes of a name or a value: its UTF-8
// bytes, a space as +, the bytes that are not safe as %XX. It returns
// ErrTooLong when that is longer than MaxUnits, which it finds before it
// writes it.
func FormEncode(in Input) (string, error) {
	s := in.s
	size := len(s)
	plain := true
	for i := 0; i < len(s); i++ {
		if c := s[i]; c == ' ' {
			plain = false
		} else if !formSafe(c) {
			plain = false
			size += 2
		}
	}
	if size > MaxUnits {
		return "", ErrTooLong
	}
	if plain {
		return s, nil
	}
	b := make([]byte, 0, size)
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c == ' ':
			b = append(b, '+')
		case formSafe(c):
			b = append(b, c)
		default:
			b = append(b, '%', upperHex[c>>4], upperHex[c&15])
		}
	}
	return unsafeString(b), nil
}

// formSafe reports whether the serializer writes c as it is (a space, which
// it writes as a plus, too).
func formSafe(c byte) bool {
	return c == ' ' || c == '*' || c == '-' || c == '.' || c == '_' ||
		'0' <= c && c <= '9' || 'a' <= c|0x20 && c|0x20 <= 'z'
}
