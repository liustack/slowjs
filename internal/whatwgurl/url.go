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

// Package whatwgurl is the URL parser of the WHATWG URL standard
// (https://url.spec.whatwg.org/), with the setters of its API and the
// serializer, for package url.
//
// Its state machine follows the one of github.com/nlnwa/whatwg-url (Apache-2.0,
// see LICENSE-WHATWG-URL), which was taken in and changed: it reads the input
// as UTF-8 where it is, keeps no options and no list of validation errors,
// follows the standard of 2025 and 2026, and takes and makes no string longer
// than MaxUnits (see Input). The IDNA code is from golang.org/x/net and
// golang.org/x/text (BSD-3-Clause, see LICENSE-GO). SOURCES.md says what came
// from where.
//
// Nothing here keeps state between calls: a URL is a value the caller owns.
package whatwgurl

import (
	"bytes"
	"strconv"
	"strings"
)

// URL is a URL record of the standard.
type URL struct {
	Scheme   string
	Username string
	Password string
	// Host is the serialized host. HasHost is false for a null host, which
	// differs from the empty host of file: URLs.
	Host    string
	HasHost bool
	// Port is the port, or -1 for null.
	Port int
	// Path is the list of segments of a path that is not opaque. For an
	// opaque path Opaque is true and OpaquePath holds the whole path.
	Path       Path
	Opaque     bool
	OpaquePath string
	Query      string
	HasQuery   bool
	Fragment   string
	// HasFragment is false for a null fragment.
	HasFragment bool
}

// specialPort returns the default port of a special scheme, -1 for a special
// scheme with none (file), and -2 for a scheme that is not special.
func specialPort(scheme string) int {
	switch scheme {
	case "http", "ws":
		return 80
	case "https", "wss":
		return 443
	case "ftp":
		return 21
	case "file":
		return -1
	}
	return -2
}

// IsSpecial reports whether the scheme of u is special.
func (u *URL) IsSpecial() bool { return specialPort(u.Scheme) != -2 }

func isSpecialScheme(s string) bool { return specialPort(s) != -2 }

// Clone returns a copy that shares nothing with u that a change could reach.
func (u *URL) Clone() *URL {
	c := *u
	c.Path = u.Path.clone()
	return &c
}

func (u *URL) includesCredentials() bool { return u.Username != "" || u.Password != "" }

// cannotHaveUsernamePasswordPort is the standard's test for the setters of
// username, password and port.
func (u *URL) cannotHaveUsernamePasswordPort() bool {
	return !u.HasHost || u.Host == "" || u.Scheme == "file"
}

func (u *URL) shortenPath() {
	if u.Scheme == "file" && u.Path.n == 1 && isNormalizedWindowsDriveLetter(u.Path.First()) {
		return
	}
	u.Path.Pop()
}

// Components says where the parts of Href are in it, as offsets in the
// bytes of the string, which are also offsets in the UTF-16 units of the
// string a script sees: a serialized URL is ASCII. A part that is absent is
// an empty range, and SearchStart and HashStart are -1 when there is no query
// and no fragment. Href holds, in order:
//
//	scheme ":" [ "//" [ username [ ":" password ] "@" ] host [ ":" port ] ]
//	  [ "/." ] path [ "?" query ] [ "#" fragment ]
//
// ProtocolEnd is the offset after the ":". HostEnd is the end of the host
// name, and PortStart is after the ":" of the port. PathStart is after "/.".
type Components struct {
	ProtocolEnd            int
	UsernameStart          int
	UsernameEnd            int
	PasswordStart          int
	PasswordEnd            int
	HostStart, HostEnd     int
	PortStart, PortEnd     int
	PathStart              int
	SearchStart, HashStart int
}

// Serialize returns the URL serializer's output, and where the parts are in
// it.
func (u *URL) Serialize() (string, Components) {
	var c Components
	var b strings.Builder
	b.Grow(len(u.Scheme) + len(u.Username) + len(u.Password) + len(u.Host) + len(u.OpaquePath) + len(u.Path.b) + len(u.Query) + len(u.Fragment) + 16)
	b.WriteString(u.Scheme)
	b.WriteByte(':')
	c.ProtocolEnd = b.Len()
	if u.HasHost {
		b.WriteString("//")
		c.UsernameStart = b.Len()
		b.WriteString(u.Username)
		c.UsernameEnd = b.Len()
		c.PasswordStart = c.UsernameEnd
		if u.Password != "" {
			b.WriteByte(':')
			c.PasswordStart = b.Len()
			b.WriteString(u.Password)
		}
		c.PasswordEnd = b.Len()
		if u.includesCredentials() {
			b.WriteByte('@')
		}
		c.HostStart = b.Len()
		b.WriteString(u.Host)
		c.HostEnd = b.Len()
		c.PortStart = c.HostEnd
		if u.Port >= 0 {
			b.WriteByte(':')
			c.PortStart = b.Len()
			b.WriteString(strconv.Itoa(u.Port))
		}
		c.PortEnd = b.Len()
	} else {
		n := c.ProtocolEnd
		c.UsernameStart, c.UsernameEnd, c.PasswordStart, c.PasswordEnd = n, n, n, n
		c.HostStart, c.HostEnd, c.PortStart, c.PortEnd = n, n, n, n
		if !u.Opaque && u.Path.n > 1 && u.Path.b[1] == '/' {
			b.WriteString("/.")
		}
	}
	c.PathStart = b.Len()
	if u.Opaque {
		b.WriteString(u.OpaquePath)
	} else {
		b.Write(u.Path.b)
	}
	c.SearchStart, c.HashStart = -1, -1
	if u.HasQuery {
		c.SearchStart = b.Len()
		b.WriteByte('?')
		b.WriteString(u.Query)
	}
	if u.HasFragment {
		c.HashStart = b.Len()
		b.WriteByte('#')
		b.WriteString(u.Fragment)
	}
	return b.String(), c
}

// Parts are a URL as package url keeps it in javascript: what the getters of
// protocol, username, password, hostname, port, pathname, search and hash
// answer, and the four bits of the record they do not show. They are the
// record, and not its href, so a URL keeps a record that the parser would not
// make from its href, which a setter can make: a file URL whose host is
// localhost, which the parser makes the empty host, or whose first segment is
// a drive letter with a |, which the parser writes with a colon.
type Parts struct {
	Protocol, Username, Password, Hostname, Port, Pathname, Search, Hash Input
	HasHost, Opaque, HasQuery, HasFragment                               bool
}

// Parts returns the parts of u.
func (u *URL) Parts() (Parts, string) {
	href, c := u.Serialize()
	pathEnd, searchEnd := len(href), len(href)
	if c.HashStart >= 0 {
		pathEnd, searchEnd = c.HashStart, c.HashStart
	}
	if c.SearchStart >= 0 {
		pathEnd = c.SearchStart
	}
	p := Parts{
		Protocol:    Input{href[:c.ProtocolEnd]},
		Username:    Input{href[c.UsernameStart:c.UsernameEnd]},
		Password:    Input{href[c.PasswordStart:c.PasswordEnd]},
		Hostname:    Input{href[c.HostStart:c.HostEnd]},
		Port:        Input{href[c.PortStart:c.PortEnd]},
		Pathname:    Input{href[c.PathStart:pathEnd]},
		HasHost:     u.HasHost,
		Opaque:      u.Opaque,
		HasQuery:    u.HasQuery,
		HasFragment: u.HasFragment,
	}
	if c.SearchStart >= 0 && searchEnd-c.SearchStart > 1 {
		p.Search = Input{href[c.SearchStart:searchEnd]}
	}
	if c.HashStart >= 0 && len(href)-c.HashStart > 1 {
		p.Hash = Input{href[c.HashStart:]}
	}
	return p, href
}

// Text is the string of an Input.
func (in Input) Text() string { return in.s }

// FromParts is the URL whose parts are p, taken as the record and not parsed
// again. Parts that are not those of a URL are refused with ErrNotParts, and
// parts that sum to more than MaxUnits with ErrTooLong: what FromParts
// returns is a URL that Parts gives back as p, whose href is ASCII and parses
// to the same record, but for the two the parser does not make (see Parts).
// So no part, flag or sum the caller hands it can make a record the rest of
// the package does not expect.
func FromParts(p Parts) (*URL, error) {
	if p.Protocol.s == "" {
		return nil, ErrNotParts
	}
	n := 0
	for _, s := range [...]string{p.Protocol.s, p.Username.s, p.Password.s, p.Hostname.s, p.Port.s, p.Pathname.s, p.Search.s, p.Hash.s} {
		if !isASCII(s) {
			return nil, ErrNotParts
		}
		n += len(s)
	}
	if n > MaxUnits {
		return nil, ErrTooLong
	}
	port := -1
	if p.Port.s != "" {
		n, err := strconv.Atoi(p.Port.s)
		if err != nil || n < 0 || n > 65535 {
			return nil, ErrNotParts
		}
		port = n
	}
	u := &URL{
		Scheme:      p.Protocol.s[:len(p.Protocol.s)-1],
		Username:    p.Username.s,
		Password:    p.Password.s,
		Host:        p.Hostname.s,
		HasHost:     p.HasHost,
		Port:        port,
		Opaque:      p.Opaque,
		HasQuery:    p.HasQuery,
		HasFragment: p.HasFragment,
	}
	path := p.Pathname.s
	if p.Opaque {
		u.OpaquePath = path
	} else if path != "" {
		u.Path = Path{b: []byte(path), n: strings.Count(path, "/")}
	}
	if p.Search.s != "" {
		u.Query = p.Search.s[1:]
	}
	if p.Hash.s != "" {
		u.Fragment = p.Hash.s[1:]
	}
	if u.length() > MaxUnits {
		return nil, ErrTooLong
	}
	again, href := u.Parts()
	if again != p {
		// A part with a character that another part starts with, a flag that says
		// other than the parts, or a port not as the serializer writes it.
		return nil, ErrNotParts
	}
	q, err := Parse(Input{href}, nil)
	if err != nil || !q.sameRecord(u.asParsed()) {
		return nil, ErrNotParts
	}
	return u, nil
}

// asParsed is u as the parser makes it from its href: the two records of
// Parts that it makes into others.
func (u *URL) asParsed() *URL {
	if u.Scheme != "file" {
		return u
	}
	c := u.Clone()
	if c.Host == "localhost" {
		c.Host = ""
	}
	if c.Path.n > 0 && isWindowsDriveLetter(c.Path.First()) {
		c.Path.b[2] = ':'
	}
	return c
}

// sameRecord reports whether u and v are the same record.
func (u *URL) sameRecord(v *URL) bool {
	return u.Scheme == v.Scheme && u.Username == v.Username && u.Password == v.Password &&
		u.Host == v.Host && u.HasHost == v.HasHost && u.Port == v.Port &&
		u.Opaque == v.Opaque && u.OpaquePath == v.OpaquePath && u.Path.n == v.Path.n && bytes.Equal(u.Path.b, v.Path.b) &&
		u.Query == v.Query && u.HasQuery == v.HasQuery && u.Fragment == v.Fragment && u.HasFragment == v.HasFragment
}

// Href is the serialization of u.
func (u *URL) Href() string {
	s, _ := u.Serialize()
	return s
}

// PathString is the serialization of the path of u.
func (u *URL) PathString() string {
	if u.Opaque {
		return u.OpaquePath
	}
	return string(u.Path.b)
}

// Origin is the serialization of the origin of u, with "null" for an opaque
// origin. It is never longer than the serialization of u.
func (u *URL) Origin() string {
	switch u.Scheme {
	case "blob":
		// The path is a part of the serialization of u, which is at most MaxUnits
		// long, as every URL the parser returns.
		p, err := Parse(Input{u.PathString()}, nil)
		if err == nil && (p.Scheme == "http" || p.Scheme == "https") {
			return p.Origin()
		}
		return "null"
	case "ftp", "http", "https", "ws", "wss":
		s := u.Scheme + "://" + u.Host
		if u.Port >= 0 {
			s += ":" + strconv.Itoa(u.Port)
		}
		return s
	}
	return "null"
}

// length is the length of the serialization of u, which is ASCII, worked out
// without making it.
func (u *URL) length() int {
	n := len(u.Scheme) + 1
	if u.HasHost {
		n += 2 + len(u.Username) + len(u.Host)
		if u.Password != "" {
			n += 1 + len(u.Password)
		}
		if u.includesCredentials() {
			n++
		}
		if u.Port >= 0 {
			n += 1 + len(strconv.Itoa(u.Port))
		}
	} else if !u.Opaque && u.Path.n > 1 && u.Path.b[1] == '/' {
		n += 2
	}
	if u.Opaque {
		n += len(u.OpaquePath)
	} else {
		n += len(u.Path.b)
	}
	if u.HasQuery {
		n += 1 + len(u.Query)
	}
	if u.HasFragment {
		n += 1 + len(u.Fragment)
	}
	return n
}

// Path is the list of segments of a path that is not opaque, kept as its
// serialization: each segment written after a "/". A segment never has a "/"
// in it (the parser cuts at it, and a percent-encoded one is not one), so the
// list can be read back from the bytes, and a path of millions of empty
// segments takes a byte each and not the 16 of a string header.
type Path struct {
	b []byte
	n int
}

// Len is how many segments the list has.
func (p *Path) Len() int { return p.n }

// First is the first segment, which the list must have.
func (p *Path) First() string {
	rest := p.b[1:]
	if i := bytes.IndexByte(rest, '/'); i >= 0 {
		rest = rest[:i]
	}
	return string(rest)
}

// push adds a segment at the end.
func (p *Path) push(seg string) {
	p.b = append(p.b, '/')
	p.b = append(p.b, seg...)
	p.n++
}

// Pop takes the last segment off.
func (p *Path) Pop() {
	if p.n > 0 {
		p.b = p.b[:bytes.LastIndexByte(p.b, '/')]
		p.n--
	}
}

// String is the serialization of the path.
func (p *Path) String() string { return string(p.b) }

// clone copies the list.
func (p *Path) clone() Path {
	if p.n == 0 {
		return Path{}
	}
	return Path{b: bytes.Clone(p.b), n: p.n}
}

// pushBytes adds a segment at the end, from its bytes.
func (p *Path) pushBytes(seg []byte) {
	p.b = append(p.b, '/')
	p.b = append(p.b, seg...)
	p.n++
}
