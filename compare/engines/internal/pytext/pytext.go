// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// Package pytext has the two text functions of Python's strings that the
// generated Markdown files were made with: what counts as white space and
// where a line ends. The archives of earlier runs were written that way, and
// a new one has to read the same.
package pytext

import "unicode"

// IsSpace is Python's str.isspace for one character.
func IsSpace(r rune) bool { return unicode.IsSpace(r) || (r >= 0x1c && r <= 0x1f) }

// SplitLines is Python's str.splitlines(): the lines without their ends. A
// line ends at \n, \r, \r\n, \v, \f, \x1c to \x1e, \x85, U+2028 and U+2029.
func SplitLines(s string) []string {
	var out []string
	rs := []rune(s)
	start := 0
	for i := 0; i < len(rs); i++ {
		switch rs[i] {
		case '\n', '\v', '\f', '\x1c', '\x1d', '\x1e', '\x85', ' ', ' ':
		case '\r':
			out = append(out, string(rs[start:i]))
			if i+1 < len(rs) && rs[i+1] == '\n' {
				i++
			}
			start = i + 1
			continue
		default:
			continue
		}
		out = append(out, string(rs[start:i]))
		start = i + 1
	}
	if start < len(rs) {
		out = append(out, string(rs[start:]))
	}
	return out
}
