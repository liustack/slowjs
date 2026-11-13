/*
 * Copyright 2020 National Library of Norway.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *       http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

// This file started from github.com/nlnwa/whatwg-url v0.6.2 (see
// LICENSE-WHATWG-URL) and was rewritten and changed by The liustack/quickjs
// Authors, who offer their changes under the MIT license (LICENSE-MIT at the
// root of this repository) as well. SOURCES.md says what remains of it.

// The host parser below follows the standard, and started from the one of
// github.com/nlnwa/whatwg-url (Copyright 2020 National Library of Norway,
// Apache-2.0, see LICENSE-WHATWG-URL).

package whatwgurl

import (
	"strconv"
	"strings"
	"unicode/utf8"
	"unsafe"
)

// parseHost is the host parser of the standard. It returns the serialized host.
func parseHost(input string, isOpaque bool) (string, error) {
	if strings.HasPrefix(input, "[") {
		if !strings.HasSuffix(input, "]") {
			return "", ErrFailure
		}
		return parseIPv6(input[1 : len(input)-1])
	}
	if isOpaque {
		return parseOpaqueHost(input)
	}
	decoded := percentDecode(input)
	// UTF-8 decode without BOM replaces what is not UTF-8 with U+FFFD, which
	// domain to ASCII refuses.
	if !utf8.Valid(decoded) {
		return "", ErrFailure
	}
	ascii, err := domainToASCII(string(decoded), false)
	if err != nil {
		return "", err
	}
	for i := 0; i < len(ascii); i++ {
		if isForbiddenDomainCodePoint(ascii[i]) {
			return "", ErrFailure
		}
	}
	if endsInANumber(ascii) {
		return parseIPv4(ascii)
	}
	return ascii, nil
}

func isForbiddenHostCodePoint(c byte) bool {
	switch c {
	case 0, '\t', '\n', '\r', ' ', '#', '/', ':', '<', '>', '?', '@', '[', '\\', ']', '^', '|':
		return true
	}
	return false
}

func isForbiddenDomainCodePoint(c byte) bool {
	return isForbiddenHostCodePoint(c) || c <= 0x1f || c == '%' || c == 0x7f
}

func parseOpaqueHost(input string) (string, error) {
	// What is written is the input and two bytes more for each byte that is
	// percent-encoded, a byte of a character that is not ASCII and C0 controls, so
	// the buffer is made once and the string is not copied from it.
	size := len(input)
	for i := 0; i < len(input); i++ {
		if isForbiddenHostCodePoint(input[i]) {
			return "", ErrFailure
		}
		if c := input[i]; c >= utf8.RuneSelf || c0Set[c] {
			size += 2
		}
	}
	if size == len(input) {
		return input, nil
	}
	out := make([]byte, 0, size)
	for _, c := range input {
		out = appendEncoded(out, c, c0Set)
	}
	return unsafe.String(unsafe.SliceData(out), len(out)), nil
}

// domainToASCII is the standard's domain to ASCII. A domain that is ASCII is
// not run through IDNA when beStrict is false.
func domainToASCII(domain string, beStrict bool) (string, error) {
	if !beStrict && isASCII(domain) {
		return strings.ToLower(domain), nil
	}
	if units(domain) > MaxDomainUnits {
		return "", ErrFailure
	}
	result, err := toASCII(domain, beStrict)
	if err != nil || result == "" {
		return "", ErrFailure
	}
	return result, nil
}

// parseIPv4Number returns the value of a part of an IPv4 address, saturated
// above 1<<40, and false when the standard's parser fails.
func parseIPv4Number(s string) (uint64, bool) {
	if s == "" {
		return 0, false
	}
	radix := uint64(10)
	if len(s) >= 2 && s[0] == '0' && (s[1] == 'x' || s[1] == 'X') {
		s, radix = s[2:], 16
	} else if len(s) >= 2 && s[0] == '0' {
		s, radix = s[1:], 8
	}
	var v uint64
	for i := 0; i < len(s); i++ {
		c := s[i]
		var d uint64
		switch {
		case '0' <= c && c <= '9':
			d = uint64(c - '0')
		case radix == 16 && 'a' <= c|0x20 && c|0x20 <= 'f':
			d = uint64(c|0x20-'a') + 10
		default:
			return 0, false
		}
		if d >= radix {
			return 0, false
		}
		if v < 1<<40 {
			v = v*radix + d
		}
	}
	return v, true
}

func endsInANumber(s string) bool {
	if strings.HasSuffix(s, ".") {
		s = s[:len(s)-1]
		if s == "" {
			return false
		}
	}
	last := s[strings.LastIndexByte(s, '.')+1:]
	if last != "" {
		allDigits := true
		for i := 0; i < len(last); i++ {
			if last[i] < '0' || last[i] > '9' {
				allDigits = false
				break
			}
		}
		if allDigits {
			return true
		}
	}
	_, ok := parseIPv4Number(last)
	return ok
}

func parseIPv4(s string) (string, error) {
	if strings.HasSuffix(s, ".") {
		s = s[:len(s)-1]
	}
	if strings.Count(s, ".") > 3 {
		return "", ErrFailure
	}
	parts := strings.Split(s, ".")
	var numbers [4]uint64
	for i, part := range parts {
		n, ok := parseIPv4Number(part)
		if !ok {
			return "", ErrFailure
		}
		numbers[i] = n
	}
	last := len(parts) - 1
	for i := 0; i < last; i++ {
		if numbers[i] > 255 {
			return "", ErrFailure
		}
	}
	if numbers[last] >= uint64(1)<<(8*uint(5-len(parts))) {
		return "", ErrFailure
	}
	ipv4 := numbers[last]
	for i := 0; i < last; i++ {
		ipv4 += numbers[i] << (8 * uint(3-i))
	}
	return strconv.Itoa(int(ipv4>>24&255)) + "." + strconv.Itoa(int(ipv4>>16&255)) + "." +
		strconv.Itoa(int(ipv4>>8&255)) + "." + strconv.Itoa(int(ipv4&255)), nil
}

func parseIPv6(in string) (string, error) {
	var addr [8]uint16
	piece := 0
	compress := -1
	p := 0
	at := func(i int) rune {
		if i < len(in) {
			return rune(in[i])
		}
		return eof
	}
	if at(p) == ':' {
		if at(p+1) != ':' {
			return "", ErrFailure
		}
		p += 2
		piece++
		compress = piece
	}
	for p < len(in) {
		if piece == 8 {
			return "", ErrFailure
		}
		if at(p) == ':' {
			if compress != -1 {
				return "", ErrFailure
			}
			p++
			piece++
			compress = piece
			continue
		}
		value, length := 0, 0
		for length < 4 && isASCIIHex(at(p)) {
			value = value*16 + int(hexVal(in[p]))
			p++
			length++
		}
		if at(p) == '.' {
			if length == 0 {
				return "", ErrFailure
			}
			p -= length
			if piece > 6 {
				return "", ErrFailure
			}
			seen := 0
			for p < len(in) {
				v4 := -1
				if seen > 0 {
					if at(p) == '.' && seen < 4 {
						p++
					} else {
						return "", ErrFailure
					}
				}
				if !isASCIIDigit(at(p)) {
					return "", ErrFailure
				}
				for isASCIIDigit(at(p)) {
					n := int(in[p] - '0')
					if v4 == -1 {
						v4 = n
					} else if v4 == 0 {
						return "", ErrFailure
					} else {
						v4 = v4*10 + n
					}
					if v4 > 255 {
						return "", ErrFailure
					}
					p++
				}
				addr[piece] = addr[piece]*0x100 + uint16(v4)
				seen++
				if seen == 2 || seen == 4 {
					piece++
				}
			}
			if seen != 4 {
				return "", ErrFailure
			}
			break
		} else if at(p) == ':' {
			p++
			if p >= len(in) {
				return "", ErrFailure
			}
		} else if p < len(in) {
			return "", ErrFailure
		}
		addr[piece] = uint16(value)
		piece++
	}
	if compress != -1 {
		swaps := piece - compress
		piece = 7
		for piece != 0 && swaps > 0 {
			addr[piece], addr[compress+swaps-1] = addr[compress+swaps-1], addr[piece]
			piece--
			swaps--
		}
	} else if piece != 8 {
		return "", ErrFailure
	}
	return "[" + serializeIPv6(addr) + "]", nil
}

func serializeIPv6(a [8]uint16) string {
	// The first longest run of two or more zero pieces is compressed.
	compress, best := -1, 1
	for i := 0; i < 8; {
		if a[i] != 0 {
			i++
			continue
		}
		j := i
		for j < 8 && a[j] == 0 {
			j++
		}
		if j-i > best {
			compress, best = i, j-i
		}
		i = j
	}
	var b strings.Builder
	ignore0 := false
	for i := 0; i < 8; i++ {
		if ignore0 && a[i] == 0 {
			continue
		}
		ignore0 = false
		if compress == i {
			if i == 0 {
				b.WriteString("::")
			} else {
				b.WriteByte(':')
			}
			ignore0 = true
			continue
		}
		b.WriteString(strconv.FormatUint(uint64(a[i]), 16))
		if i != 7 {
			b.WriteByte(':')
		}
	}
	return b.String()
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			return false
		}
	}
	return true
}
