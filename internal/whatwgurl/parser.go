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

// The state machine below is the basic URL parser of the standard. It started
// from the one of github.com/nlnwa/whatwg-url (Copyright 2020 National Library
// of Norway, Apache-2.0, see LICENSE-WHATWG-URL) and was rewritten to read the
// input as UTF-8 in place, to keep its output in the URL record, to follow the
// standard of 2026 and to do no work that is more than linear in its input.

package whatwgurl

import (
	"strings"
	"unicode/utf8"
	"unsafe"
)

type state int

const (
	noState state = iota
	stateSchemeStart
	stateScheme
	stateNoScheme
	stateSpecialRelativeOrAuthority
	statePathOrAuthority
	stateRelative
	stateRelativeSlash
	stateSpecialAuthoritySlashes
	stateSpecialAuthorityIgnoreSlashes
	stateAuthority
	stateHost
	stateHostname
	statePort
	stateFile
	stateFileSlash
	stateFileHost
	statePathStart
	statePath
	stateOpaquePath
	stateQuery
	stateFragment
)

const eof = rune(-1)

// Parse is the basic URL parser without a state override, and with base
// non-nil for a relative input. It returns ErrFailure when the input is not
// a URL, and ErrTooLong for one whose serialization is longer than MaxUnits.
func Parse(in Input, base *URL) (*URL, error) {
	u, err := basicParse(in.s, base, nil, noState)
	if err != nil {
		return nil, err
	}
	if u.length() > MaxUnits {
		return nil, ErrTooLong
	}
	return u, nil
}

func isC0OrSpace(c byte) bool { return c <= 0x20 }

func isTabOrNewline(c byte) bool { return c == '\t' || c == '\n' || c == '\r' }

// basicParse is the basic URL parser. With u non-nil it changes u in place,
// from the state override, as the setters do.
func basicParse(input string, base *URL, u *URL, override state) (*URL, error) {
	if u == nil {
		u = &URL{Port: -1}
		start, end := 0, len(input)
		for start < end && isC0OrSpace(input[start]) {
			start++
		}
		for end > start && isC0OrSpace(input[end-1]) {
			end--
		}
		input = input[start:end]
	}
	if strings.ContainsAny(input, "\t\n\r") {
		b := make([]byte, 0, len(input))
		for i := 0; i < len(input); i++ {
			if !isTabOrNewline(input[i]) {
				b = append(b, input[i])
			}
		}
		input = string(b)
	}

	p := parser{s: input, base: base, u: u, override: override}
	p.state = override
	if override == noState {
		p.state = stateSchemeStart
	}
	if err := p.run(); err != nil {
		return nil, err
	}
	return u, nil
}

type parser struct {
	s        string
	i        int // the pointer, as a byte offset, -1 before the first code point
	c        rune
	w        int // the width of c in bytes, 0 at the end of the input
	base     *URL
	u        *URL
	state    state
	override state
	buf      []byte

	atSignSeen        bool
	insideBrackets    bool
	passwordTokenSeen bool
	userinfoUser      []byte
	userinfoPass      []byte
	port              int
	portDigits        int
}

// decode sets c and w from the pointer.
func (p *parser) decode() {
	if p.i >= len(p.s) {
		p.c, p.w = eof, 0
		return
	}
	c := p.s[p.i]
	if c < utf8.RuneSelf {
		p.c, p.w = rune(c), 1
		return
	}
	r, w := utf8.DecodeRuneInString(p.s[p.i:])
	p.c, p.w = r, w
}

// back decreases the pointer by one code point.
func (p *parser) back() {
	if p.i <= 0 {
		p.i = -1
		return
	}
	if p.s[p.i-1] < utf8.RuneSelf {
		p.i--
		return
	}
	_, w := utf8.DecodeLastRuneInString(p.s[:p.i])
	p.i -= w
}

// remaining is the input after the current code point.
func (p *parser) remaining() string {
	if p.i+p.w > len(p.s) {
		return ""
	}
	return p.s[p.i+p.w:]
}

func (p *parser) special() bool { return isSpecialScheme(p.u.Scheme) }

// specialBackslash is "url is special and c is \".
func (p *parser) specialBackslash() bool { return p.c == '\\' && p.special() }

func (p *parser) overridden() bool { return p.override != noState }

func (p *parser) run() error {
	u := p.u
	for {
		p.decode()
		c := p.c
		switch p.state {
		case stateSchemeStart:
			if isASCIIAlpha(c) {
				p.buf = append(p.buf, byte(c|0x20))
				p.state = stateScheme
			} else if !p.overridden() {
				p.state = stateNoScheme
				p.back()
			} else {
				return ErrFailure
			}

		case stateScheme:
			if isASCIIAlpha(c) || isASCIIDigit(c) || c == '+' || c == '-' || c == '.' {
				if 'A' <= c && c <= 'Z' {
					c |= 0x20
				}
				p.buf = append(p.buf, byte(c))
			} else if c == ':' {
				scheme := string(p.buf)
				if p.overridden() {
					if isSpecialScheme(u.Scheme) != isSpecialScheme(scheme) {
						return nil
					}
					if (u.includesCredentials() || u.Port >= 0) && scheme == "file" {
						return nil
					}
					if u.Scheme == "file" && u.HasHost && u.Host == "" {
						return nil
					}
				}
				u.Scheme = scheme
				if p.overridden() {
					if u.Port == specialPort(u.Scheme) {
						u.Port = -1
					}
					return nil
				}
				p.buf = p.buf[:0]
				switch {
				case u.Scheme == "file":
					p.state = stateFile
				case u.IsSpecial() && p.base != nil && p.base.Scheme == u.Scheme:
					p.state = stateSpecialRelativeOrAuthority
				case u.IsSpecial():
					p.state = stateSpecialAuthoritySlashes
				case strings.HasPrefix(p.remaining(), "/"):
					p.state = statePathOrAuthority
					p.i++
				default:
					u.Opaque = true
					u.OpaquePath = ""
					p.state = stateOpaquePath
				}
			} else if !p.overridden() {
				p.buf = p.buf[:0]
				p.state = stateNoScheme
				p.i = -1
			} else {
				return ErrFailure
			}

		case stateNoScheme:
			switch {
			case p.base == nil || (p.base.Opaque && c != '#'):
				return ErrFailure
			case p.base.Opaque && c == '#':
				u.Scheme = p.base.Scheme
				u.Opaque, u.OpaquePath = true, p.base.OpaquePath
				u.Query, u.HasQuery = p.base.Query, p.base.HasQuery
				u.Fragment, u.HasFragment = "", true
				p.state = stateFragment
			case p.base.Scheme != "file":
				p.state = stateRelative
				p.back()
			default:
				p.state = stateFile
				p.back()
			}

		case stateSpecialRelativeOrAuthority:
			if c == '/' && strings.HasPrefix(p.remaining(), "/") {
				p.state = stateSpecialAuthorityIgnoreSlashes
				p.i++
			} else {
				p.state = stateRelative
				p.back()
			}

		case statePathOrAuthority:
			if c == '/' {
				p.state = stateAuthority
			} else {
				p.state = statePath
				p.back()
			}

		case stateRelative:
			b := p.base
			u.Scheme = b.Scheme
			if c == '/' {
				p.state = stateRelativeSlash
			} else if p.specialBackslash() {
				p.state = stateRelativeSlash
			} else {
				u.Username, u.Password = b.Username, b.Password
				u.Host, u.HasHost, u.Port = b.Host, b.HasHost, b.Port
				u.Path = b.Path.clone()
				u.Opaque, u.OpaquePath = b.Opaque, b.OpaquePath
				u.Query, u.HasQuery = b.Query, b.HasQuery
				if c == '?' {
					u.Query, u.HasQuery = "", true
					p.state = stateQuery
				} else if c == '#' {
					u.Fragment, u.HasFragment = "", true
					p.state = stateFragment
				} else if c != eof {
					u.Query, u.HasQuery = "", false
					u.shortenPath()
					p.state = statePath
					p.back()
				}
			}

		case stateRelativeSlash:
			if u.IsSpecial() && (c == '/' || c == '\\') {
				p.state = stateSpecialAuthorityIgnoreSlashes
			} else if c == '/' {
				p.state = stateAuthority
			} else {
				b := p.base
				u.Username, u.Password = b.Username, b.Password
				u.Host, u.HasHost, u.Port = b.Host, b.HasHost, b.Port
				p.state = statePath
				p.back()
			}

		case stateSpecialAuthoritySlashes:
			if c == '/' && strings.HasPrefix(p.remaining(), "/") {
				p.state = stateSpecialAuthorityIgnoreSlashes
				p.i++
			} else {
				p.state = stateSpecialAuthorityIgnoreSlashes
				p.back()
			}

		case stateSpecialAuthorityIgnoreSlashes:
			if c != '/' && c != '\\' {
				p.state = stateAuthority
				p.back()
			}

		case stateAuthority:
			if c == '@' {
				if p.atSignSeen {
					p.buf = append([]byte("%40"), p.buf...)
				}
				p.atSignSeen = true
				for j := 0; j < len(p.buf); {
					r, w := rune(p.buf[j]), 1
					if r >= utf8.RuneSelf {
						r, w = utf8.DecodeRune(p.buf[j:])
					}
					j += w
					if r == ':' && !p.passwordTokenSeen {
						p.passwordTokenSeen = true
						continue
					}
					if p.passwordTokenSeen {
						p.userinfoPass = appendEncoded(p.userinfoPass, r, userinfoSet)
					} else {
						p.userinfoUser = appendEncoded(p.userinfoUser, r, userinfoSet)
					}
				}
				p.buf = p.buf[:0]
			} else if c == eof || c == '/' || c == '?' || c == '#' || p.specialBackslash() {
				if p.atSignSeen && len(p.buf) == 0 {
					return ErrFailure
				}
				// Decrease the pointer by the code points of the buffer and one,
				// which the loop makes up for. The buffer holds the bytes of
				// the input just before c.
				p.i -= len(p.buf)
				p.back()
				p.buf = p.buf[:0]
				// What was written is not written again, so the strings are not copies.
				u.Username, u.Password = unsafeString(p.userinfoUser), unsafeString(p.userinfoPass)
				p.userinfoUser, p.userinfoPass = nil, nil
				p.state = stateHost
			} else {
				p.buf = append(p.buf, p.s[p.i:p.i+p.w]...)
			}

		case stateHost, stateHostname:
			if p.overridden() && u.Scheme == "file" {
				p.back()
				p.state = stateFileHost
			} else if c == ':' && !p.insideBrackets {
				if len(p.buf) == 0 {
					return ErrFailure
				}
				if p.override == stateHostname {
					return nil
				}
				host, err := parseHost(string(p.buf), !u.IsSpecial())
				if err != nil {
					return err
				}
				u.Host, u.HasHost = host, true
				p.buf = p.buf[:0]
				p.state = statePort
			} else if c == eof || c == '/' || c == '?' || c == '#' || p.specialBackslash() {
				p.back()
				if u.IsSpecial() && len(p.buf) == 0 {
					return ErrFailure
				} else if p.overridden() && len(p.buf) == 0 && (u.includesCredentials() || u.Port >= 0) {
					return nil
				}
				host, err := parseHost(string(p.buf), !u.IsSpecial())
				if err != nil {
					return err
				}
				u.Host, u.HasHost = host, true
				p.buf = p.buf[:0]
				p.state = statePathStart
				if p.overridden() {
					return nil
				}
			} else {
				if c == '[' {
					p.insideBrackets = true
				} else if c == ']' {
					p.insideBrackets = false
				}
				p.buf = append(p.buf, p.s[p.i:p.i+p.w]...)
			}

		case statePort:
			if isASCIIDigit(c) {
				p.buf = append(p.buf, byte(c))
				p.portDigits++
				if p.port <= 65535 {
					p.port = p.port*10 + int(c-'0')
				}
			} else if c == eof || c == '/' || c == '?' || c == '#' || p.specialBackslash() || p.overridden() {
				if p.portDigits > 0 {
					if p.port > 65535 {
						return ErrFailure
					}
					if p.port == specialPort(u.Scheme) {
						u.Port = -1
					} else {
						u.Port = p.port
					}
					p.buf = p.buf[:0]
					p.portDigits = 0
				}
				if p.overridden() {
					return nil
				}
				p.state = statePathStart
				p.back()
			} else {
				return ErrFailure
			}

		case stateFile:
			u.Scheme = "file"
			u.Host, u.HasHost = "", true
			if c == '/' || c == '\\' {
				p.state = stateFileSlash
			} else if p.base != nil && p.base.Scheme == "file" {
				b := p.base
				u.Host, u.HasHost = b.Host, b.HasHost
				u.Path = b.Path.clone()
				u.Query, u.HasQuery = b.Query, b.HasQuery
				if c == '?' {
					u.Query, u.HasQuery = "", true
					p.state = stateQuery
				} else if c == '#' {
					u.Fragment, u.HasFragment = "", true
					p.state = stateFragment
				} else if c != eof {
					u.Query, u.HasQuery = "", false
					if !startsWithWindowsDriveLetter(p.s[p.i:]) {
						u.shortenPath()
					} else {
						u.Path = Path{}
					}
					p.state = statePath
					p.back()
				}
			} else {
				p.state = statePath
				p.back()
			}

		case stateFileSlash:
			if c == '/' || c == '\\' {
				p.state = stateFileHost
			} else {
				if p.base != nil && p.base.Scheme == "file" {
					u.Host, u.HasHost = p.base.Host, p.base.HasHost
					if !startsWithWindowsDriveLetter(p.s[p.i:]) && p.base.Path.n > 0 && isNormalizedWindowsDriveLetter(p.base.Path.First()) {
						u.Path.push(p.base.Path.First())
					}
				}
				p.state = statePath
				p.back()
			}

		case stateFileHost:
			if c == eof || c == '/' || c == '\\' || c == '?' || c == '#' {
				p.back()
				if !p.overridden() && isWindowsDriveLetter(string(p.buf)) {
					p.state = statePath
				} else if len(p.buf) == 0 {
					u.Host, u.HasHost = "", true
					if p.overridden() {
						return nil
					}
					p.state = statePathStart
				} else {
					host, err := parseHost(string(p.buf), !u.IsSpecial())
					if err != nil {
						return err
					}
					if host == "localhost" {
						host = ""
					}
					u.Host, u.HasHost = host, true
					if p.overridden() {
						return nil
					}
					p.buf = p.buf[:0]
					p.state = statePathStart
				}
			} else {
				p.buf = append(p.buf, p.s[p.i:p.i+p.w]...)
			}

		case statePathStart:
			if u.IsSpecial() {
				p.state = statePath
				if c != '/' && c != '\\' {
					p.back()
				}
			} else if !p.overridden() && c == '?' {
				u.Query, u.HasQuery = "", true
				p.state = stateQuery
			} else if !p.overridden() && c == '#' {
				u.Fragment, u.HasFragment = "", true
				p.state = stateFragment
			} else if c != eof {
				p.state = statePath
				if c != '/' {
					p.back()
				}
			} else if p.overridden() && !u.HasHost {
				u.Path.push("")
			}

		case statePath:
			if c == eof || c == '/' || p.specialBackslash() || (!p.overridden() && (c == '?' || c == '#')) {
				// Only a segment of at most 6 bytes can be a dot segment or a drive
				// letter, so a long one is not made a string: it goes to the path as
				// it is.
				short := len(p.buf) <= 6
				var seg string
				if short {
					seg = string(p.buf)
				}
				slash := c == '/' || p.specialBackslash()
				if short && isDoubleDotSegment(seg) {
					u.shortenPath()
					if !slash {
						u.Path.push("")
					}
				} else if short && isSingleDotSegment(seg) && !slash {
					u.Path.push("")
				} else if !(short && isSingleDotSegment(seg)) {
					if short && u.Scheme == "file" && u.Path.n == 0 && isWindowsDriveLetter(seg) {
						seg = seg[:1] + ":"
					}
					if short {
						u.Path.push(seg)
					} else {
						u.Path.pushBytes(p.buf)
					}
				}
				p.buf = p.buf[:0]
				if c == '?' {
					u.Query, u.HasQuery = "", true
					p.state = stateQuery
				} else if c == '#' {
					u.Fragment, u.HasFragment = "", true
					p.state = stateFragment
				}
			} else {
				p.buf = appendEncoded(p.buf, c, pathSet)
			}

		case stateOpaquePath:
			if c == '?' {
				u.OpaquePath = p.take()
				u.Query, u.HasQuery = "", true
				p.state = stateQuery
			} else if c == '#' {
				u.OpaquePath = p.take()
				u.Fragment, u.HasFragment = "", true
				p.state = stateFragment
			} else if c == ' ' {
				if r := p.remaining(); strings.HasPrefix(r, "?") || strings.HasPrefix(r, "#") {
					p.buf = append(p.buf, "%20"...)
				} else {
					p.buf = append(p.buf, ' ')
				}
			} else if c != eof {
				p.buf = appendEncoded(p.buf, c, c0Set)
			} else {
				u.OpaquePath = p.take()
			}

		case stateQuery:
			if (!p.overridden() && c == '#') || c == eof {
				u.Query = p.take()
				if c == '#' {
					u.Fragment, u.HasFragment = "", true
					p.state = stateFragment
				}
			} else {
				set := querySet
				if u.IsSpecial() {
					set = specialQuerySet
				}
				p.buf = appendEncoded(p.buf, c, set)
			}

		case stateFragment:
			if c != eof {
				p.buf = appendEncoded(p.buf, c, fragmentSet)
			} else {
				u.Fragment = p.take()
			}
		}

		if p.i >= len(p.s) {
			return nil
		}
		if p.i < 0 {
			p.i = 0
		} else {
			// The pointer may have moved, so the width is read again.
			p.decode()
			p.i += p.w
		}
	}
}

func isSingleDotSegment(s string) bool {
	return s == "." || strings.EqualFold(s, "%2e")
}

func isDoubleDotSegment(s string) bool {
	switch len(s) {
	case 2:
		return s == ".."
	case 4:
		return strings.EqualFold(s, ".%2e") || strings.EqualFold(s, "%2e.")
	case 6:
		return strings.EqualFold(s, "%2e%2e")
	}
	return false
}

func isWindowsDriveLetter(s string) bool {
	return len(s) == 2 && isASCIIAlpha(rune(s[0])) && (s[1] == ':' || s[1] == '|')
}

func isNormalizedWindowsDriveLetter(s string) bool {
	return len(s) == 2 && isASCIIAlpha(rune(s[0])) && s[1] == ':'
}

func startsWithWindowsDriveLetter(s string) bool {
	if len(s) < 2 || !isWindowsDriveLetter(s[:2]) {
		return false
	}
	return len(s) == 2 || s[2] == '/' || s[2] == '\\' || s[2] == '?' || s[2] == '#'
}

// unsafeString is the string of the bytes b, without a copy: b is not written
// to again.
func unsafeString(b []byte) string {
	return unsafe.String(unsafe.SliceData(b), len(b))
}

// take is the string of the buffer, which the parser starts again on, without
// a copy of a long one.
func (p *parser) take() string {
	s := unsafeString(p.buf)
	p.buf = nil
	return s
}
