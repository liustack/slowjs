// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package whatwgurl

import (
	"reflect"
	"strings"
	"testing"
)

// TestPartsKeepTheRecord checks the two records a setter makes that the parser
// does not make from their href: FromParts makes them again from their parts,
// where parsing the href makes another record.
func TestPartsKeepTheRecord(t *testing.T) {
	for _, c := range []struct{ href, protocol, host, path string }{
		// A file URL whose host is localhost: the parser makes it empty.
		{"http://localhost/a", "file", "localhost", "/a"},
		// A file URL whose first segment is a drive letter with a |: the parser
		// writes it with a colon.
		{"http://h/C|/x", "file", "h", "/C|/x"},
		// Both.
		{"http://localhost/C|/x", "file", "localhost", "/C|/x"},
	} {
		u, err := parse(c.href, nil)
		if err != nil {
			t.Fatal(err)
		}
		if err := u.SetProtocol(mustInput(c.protocol)); err != nil {
			t.Fatal(err)
		}
		if u.Host != c.host || u.PathString() != c.path {
			t.Fatalf("%s with protocol %s: host %q path %q, want %q %q", c.href, c.protocol, u.Host, u.PathString(), c.host, c.path)
		}
		p, href := u.Parts()
		if again, err := FromParts(p); err != nil {
			t.Errorf("%s: %v", href, err)
		} else if !reflect.DeepEqual(normalize(again), normalize(u)) {
			t.Errorf("%s: the parts make %s, want %s", href, dump(again), dump(u))
		}
		if reparsed, _ := parse(href, nil); reflect.DeepEqual(normalize(reparsed), normalize(u)) {
			t.Errorf("%s parses to the same record: the test no longer shows the difference", href)
		}
	}
}

// TestFromPartsRefuses checks that FromParts refuses, with an error and
// without a panic, parts that are not those of a URL: a script cannot hand
// them to it (url.js holds the parts where none reaches them), but package url
// does not count on that.
func TestFromPartsRefuses(t *testing.T) {
	long := Input{"#" + strings.Repeat("f", MaxUnits)}
	for _, c := range []struct {
		href string
		name string
		bad  func(p *Parts)
		want error
	}{
		// Parts that are not those of any URL.
		{"http://u:p@h:8/p?q#f", "no protocol", func(p *Parts) { p.Protocol = Input{} }, ErrNotParts},
		{"http://u:p@h:8/p?q#f", "a protocol without :", func(p *Parts) { p.Protocol = Input{"http"} }, ErrNotParts},
		{"http://u:p@h:8/p?q#f", "a protocol in capitals", func(p *Parts) { p.Protocol = Input{"HTTP:"} }, ErrNotParts},
		{"http://u:p@h:8/p?q#f", "a protocol with a space", func(p *Parts) { p.Protocol = Input{"ht tp:"} }, ErrNotParts},
		{"http://u:p@h:8/p?q#f", "a protocol with a /", func(p *Parts) { p.Protocol = Input{"a/b:"} }, ErrNotParts},
		{"http://u:p@h:8/p?q#f", "a protocol of a digit", func(p *Parts) { p.Protocol = Input{"1a:"} }, ErrNotParts},
		{"http://u:p@h:8/p?q#f", "a port of letters", func(p *Parts) { p.Port = Input{"x"} }, ErrNotParts},
		{"http://u:p@h:8/p?q#f", "a port too big", func(p *Parts) { p.Port = Input{"65536"} }, ErrNotParts},
		{"http://u:p@h:8/p?q#f", "a port with a 0 in front", func(p *Parts) { p.Port = Input{"08"} }, ErrNotParts},
		{"http://u:p@h:8/p?q#f", "a port with a sign", func(p *Parts) { p.Port = Input{"+8"} }, ErrNotParts},
		{"http://u:p@h:8/p?q#f", "the default port", func(p *Parts) { p.Port = Input{"80"} }, ErrNotParts},
		{"file:///p", "a port on a file URL", func(p *Parts) { p.Port = Input{"8"} }, ErrNotParts},
		{"http://u:p@h:8/p?q#f", "a host in capitals", func(p *Parts) { p.Hostname = Input{"H"} }, ErrNotParts},
		{"http://u:p@h:8/p?q#f", "a host with a /", func(p *Parts) { p.Hostname = Input{"h/x"} }, ErrNotParts},
		{"http://u:p@h:8/p?q#f", "a host with a :", func(p *Parts) { p.Hostname = Input{"h:9"} }, ErrNotParts},
		{"http://u:p@h:8/p?q#f", "a host with an @", func(p *Parts) { p.Hostname = Input{"a@h"} }, ErrNotParts},
		{"http://u:p@h:8/p?q#f", "a host with a percent-encoded letter", func(p *Parts) { p.Hostname = Input{"%41"} }, ErrNotParts},
		{"http://u:p@h:8/p?q#f", "an IPv4 host not as it is written", func(p *Parts) { p.Hostname = Input{"1.2.3"} }, ErrNotParts},
		{"http://u:p@h:8/p?q#f", "an IPv4 host in hex", func(p *Parts) { p.Hostname = Input{"0x7f.1"} }, ErrNotParts},
		{"http://u:p@h:8/p?q#f", "an IPv6 host not as it is written", func(p *Parts) { p.Hostname = Input{"[0:0::1]"} }, ErrNotParts},
		{"http://u:p@h:8/p?q#f", "an empty host on a special URL", func(p *Parts) { p.Hostname = Input{} }, ErrNotParts},
		{"http://u:p@h:8/p?q#f", "a host that is not ASCII", func(p *Parts) { p.Hostname = Input{"é"} }, ErrNotParts},
		{"foo://h/p", "an opaque host with a space", func(p *Parts) { p.Hostname = Input{"a b"} }, ErrNotParts},
		{"http://u:p@h:8/p?q#f", "a user with a :", func(p *Parts) { p.Username = Input{"a:b"} }, ErrNotParts},
		{"http://u:p@h:8/p?q#f", "a password with an @", func(p *Parts) { p.Password = Input{"a@b"} }, ErrNotParts},
		{"foo://u@h/p", "a user on no host", func(p *Parts) { p.HasHost, p.Hostname = false, Input{} }, ErrNotParts},
		{"http://u:p@h:8/p?q#f", "a path without /", func(p *Parts) { p.Pathname = Input{"p"} }, ErrNotParts},
		{"http://u:p@h:8/p?q#f", "a path with a ?", func(p *Parts) { p.Pathname = Input{"/a?b"} }, ErrNotParts},
		{"http://u:p@h:8/p?q#f", "a path with a #", func(p *Parts) { p.Pathname = Input{"/a#b"} }, ErrNotParts},
		{"http://u:p@h:8/p?q#f", "a path with ..", func(p *Parts) { p.Pathname = Input{"/a/../b"} }, ErrNotParts},
		{"http://u:p@h:8/p?q#f", "a path with a space", func(p *Parts) { p.Pathname = Input{"/a b"} }, ErrNotParts},
		{"http://u:p@h:8/p?q#f", "a path that is not ASCII", func(p *Parts) { p.Pathname = Input{"/é"} }, ErrNotParts},
		{"http://u:p@h:8/p?q#f", "a search without ?", func(p *Parts) { p.Search = Input{"q"} }, ErrNotParts},
		{"http://u:p@h:8/p?q#f", "a search with a #", func(p *Parts) { p.Search = Input{"?q#r"} }, ErrNotParts},
		{"http://u:p@h:8/p?q#f", "a hash without #", func(p *Parts) { p.Hash = Input{"f"} }, ErrNotParts},
		{"http://u:p@h:8/p?q#f", "a hash with a space", func(p *Parts) { p.Hash = Input{"#a b"} }, ErrNotParts},
		{"foo:x", "an opaque path with a space at its end", func(p *Parts) { p.Pathname = Input{"x "} }, ErrNotParts},
		{"foo:x", "an opaque path that starts with /", func(p *Parts) { p.Pathname = Input{"/x"} }, ErrNotParts},
		// Flags that say other than the parts.
		{"http://u:p@h:8/p?q#f", "a search, no query", func(p *Parts) { p.HasQuery = false }, ErrNotParts},
		{"http://u:p@h:8/p?q#f", "a hash, no fragment", func(p *Parts) { p.HasFragment = false }, ErrNotParts},
		{"http://u:p@h:8/p?q#f", "a host on no host", func(p *Parts) { p.HasHost = false }, ErrNotParts},
		{"http://u:p@h:8/p?q#f", "opaque with a host", func(p *Parts) { p.Opaque = true }, ErrNotParts},
		{"foo:x", "a host on an opaque path", func(p *Parts) { p.HasHost = true }, ErrNotParts},
		{"foo:x", "an opaque path said not to be", func(p *Parts) { p.Opaque = false }, ErrNotParts},
		{"http://h/p", "a special URL with no host", func(p *Parts) { p.HasHost, p.Hostname = false, Input{} }, ErrNotParts},
		// Parts longer, in all, than a URL may be.
		{"http://u:p@h:8/p?q#f", "longer than allowed", func(p *Parts) { p.Hash = long }, ErrTooLong},
	} {
		good, _ := mustParse(t, c.href).Parts()
		p := good
		c.bad(&p)
		u, err := FromParts(p)
		if err != c.want || u != nil {
			t.Errorf("%s, %s: %v, %v, want nil, %v", c.href, c.name, u, err, c.want)
		}
	}
}

// TestFromPartsTakes checks that FromParts takes the parts of URLs of every
// kind, the empty ones too.
func TestFromPartsTakes(t *testing.T) {
	for _, href := range []string{
		"http://u:p@h:8/p?q#f", "http://h/", "http://h/?#", "https://[::1]:1/", "http://1.2.3.4/",
		"http://xn--nxasmq6b/", "file:///", "file://h/C:/x", "file:///C:", "foo:", "foo:x?#", "foo://",
		"foo://h:0", "foo:/.//p", "foo:///", "foo:/p", "foo://%41/%zz?%#%", "sc://ñ.test/",
		"blob:http://h/", "data:,x", "javascript:a b",
	} {
		u := mustParse(t, href)
		p, _ := u.Parts()
		again, err := FromParts(p)
		if err != nil {
			t.Errorf("%s: %v", href, err)
			continue
		}
		if !reflect.DeepEqual(normalize(again), normalize(u)) {
			t.Errorf("%s: the parts make %s", href, dump(again))
		}
	}
}

func mustParse(t *testing.T, s string) *URL {
	t.Helper()
	u, err := parse(s, nil)
	if err != nil {
		t.Fatal(err)
	}
	return u
}
