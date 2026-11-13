// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package whatwgurl

import (
	"errors"
	"unicode/utf8"
)

// MaxUnits is the longest string, in UTF-16 units, that the package takes and
// the longest it makes: an Input, the serialization of a URL, a name or a value
// of a query string, decoded or encoded. Its work on one string is linear in
// the length but for the IDNA of a host, which MaxDomainUnits bounds, so the
// longest any call works is bounded by the length, and what it makes can be
// given back to it. package url refuses a longer string before it reads it (see
// docs/changes/url.md).
const MaxUnits = 1 << 16

// MaxDomainUnits is the longest a domain that is not ASCII may be, in UTF-16
// units after percent-decoding and before it is mapped, and once mapped, for
// domain to ASCII to take it: the IDNA of a unit, mapping, normalization, the
// checks and Punycode, costs up to 20 times what the rest of the parser does.
// A longer one is a failure of the host parser. A domain is the host of a
// special scheme. An ASCII domain, which every host the package makes is, and
// the opaque host of another scheme, which is only percent-encoded, do not go
// through IDNA and are not bounded but by MaxUnits.
const MaxDomainUnits = 1 << 10

// ErrFailure is the failure of the URL parser: the input is not a URL.
var ErrFailure = errors.New("whatwgurl: failure")

// ErrTooLong is returned when what the package would make is longer than
// MaxUnits: the serialization of a URL, or the encoding of a string. A URL a
// setter returned it for is not to be used.
var ErrTooLong = errors.New("whatwgurl: longer than MaxUnits")

// ErrNotParts is the error of FromParts for parts that are not those of a URL.
var ErrNotParts = errors.New("whatwgurl: parts that are not those of a URL")

// An Input is a string of at most MaxUnits UTF-16 units. Only NewInput makes
// one, so the length of everything the package works on has been checked once,
// where it came in.
type Input struct{ s string }

// NewInput returns s as an Input, and false when it is longer than MaxUnits
// UTF-16 units. A byte that is not part of valid UTF-8 counts as one unit, as
// the parser reads it as U+FFFD.
func NewInput(s string) (Input, bool) {
	// A unit takes at most 3 bytes (a pair of units 4), so this needs no count.
	if len(s) > 3*MaxUnits {
		return Input{}, false
	}
	if units(s) > MaxUnits {
		return Input{}, false
	}
	return Input{s}, true
}

// units is the length of s in UTF-16 units.
func units(s string) int {
	n := 0
	for i := 0; i < len(s); {
		c := s[i]
		if c < utf8.RuneSelf {
			i++
			n++
			continue
		}
		r, w := utf8.DecodeRuneInString(s[i:])
		i += w
		n++
		if r >= 0x10000 {
			n++
		}
	}
	return n
}
