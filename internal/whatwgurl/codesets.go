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

package whatwgurl

import (
	"strings"
	"unicode/utf8"
)

// encodeSet is a percent-encode set of the standard: which ASCII code points
// it holds. Every code point above U+007E is in every set, and so is every
// C0 control.
type encodeSet [128]bool

func newSet(base *encodeSet, extra string) *encodeSet {
	var s encodeSet
	if base != nil {
		s = *base
	}
	for i := 0; i < len(extra); i++ {
		s[extra[i]] = true
	}
	return &s
}

var (
	c0Set = func() *encodeSet {
		var s encodeSet
		for i := 0; i < 0x20; i++ {
			s[i] = true
		}
		s[0x7f] = true
		return &s
	}()
	fragmentSet     = newSet(c0Set, " \"<>`")
	querySet        = newSet(c0Set, " \"#<>")
	specialQuerySet = newSet(querySet, "'")
	pathSet         = newSet(querySet, "?^`{}")
	userinfoSet     = newSet(pathSet, "/:;=@[\\]^|")
	componentSet    = newSet(userinfoSet, "$%&+,")
	formSet         = newSet(componentSet, "!'()~")
)

const upperHex = "0123456789ABCDEF"

// appendEncoded appends the UTF-8 percent-encoding of c under set.
func appendEncoded(dst []byte, c rune, set *encodeSet) []byte {
	if c < 0x80 {
		if !set[c] {
			return append(dst, byte(c))
		}
		return append(dst, '%', upperHex[c>>4], upperHex[c&15])
	}
	var buf [4]byte
	n := utf8.EncodeRune(buf[:], c)
	for _, b := range buf[:n] {
		dst = append(dst, '%', upperHex[b>>4], upperHex[b&15])
	}
	return dst
}

func isASCIIAlpha(c rune) bool { return 'a' <= c|0x20 && c|0x20 <= 'z' }
func isASCIIDigit(c rune) bool { return '0' <= c && c <= '9' }
func isASCIIHex(c rune) bool {
	return isASCIIDigit(c) || ('a' <= c|0x20 && c|0x20 <= 'f')
}

func hexVal(c byte) byte {
	switch {
	case c <= '9':
		return c - '0'
	case c >= 'a':
		return c - 'a' + 10
	}
	return c - 'A' + 10
}

// percentDecode is the standard's percent-decode of a string, as bytes.
func percentDecode(s string) []byte {
	i := strings.IndexByte(s, '%')
	if i < 0 {
		i = len(s)
	}
	out := make([]byte, 0, len(s))
	out = append(out, s[:i]...)
	for ; i < len(s); i++ {
		if s[i] == '%' && i+2 < len(s) && isASCIIHex(rune(s[i+1])) && isASCIIHex(rune(s[i+2])) {
			out = append(out, hexVal(s[i+1])<<4|hexVal(s[i+2]))
			i += 2
		} else {
			out = append(out, s[i])
		}
	}
	return out
}
