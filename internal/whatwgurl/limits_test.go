// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package whatwgurl

import (
	"strings"
	"testing"

	"github.com/liustack/quickjs/internal/whatwgurl/idna"
)

// TestNewInput checks that an Input is at most MaxUnits UTF-16 units, as a
// script counts them: a character above U+FFFF is two, a byte that is not
// UTF-8 one (it is the one U+FFFD it stands for).
func TestNewInput(t *testing.T) {
	for _, c := range []struct {
		unit string
		size int // units of unit
	}{{"a", 1}, {"é", 1}, {"一", 1}, {"😀", 2}, {"\x80", 1}} {
		at := strings.Repeat(c.unit, MaxUnits/c.size)
		if _, ok := NewInput(at); !ok {
			t.Errorf("%q x %d, MaxUnits units, refused", c.unit, MaxUnits/c.size)
		}
		if _, ok := NewInput(at + c.unit); ok {
			t.Errorf("%q x %d, past MaxUnits, taken", c.unit, MaxUnits/c.size+1)
		}
	}
	if _, ok := NewInput(strings.Repeat("a", MaxUnits) + "é"); ok {
		t.Error("MaxUnits+1 units, the last not ASCII, taken")
	}
}

// TestSpacesCount checks the limit of an input counts the spaces, tabs and
// newlines the parser removes: an input is refused by its length as given,
// before any of its work (the seventh finding of the third review, written
// down as a limit in docs/changes/url.md).
func TestSpacesCount(t *testing.T) {
	url := "http://h/"
	at := strings.Repeat(" ", MaxUnits-len(url)) + url
	u, err := parse(at, nil)
	if err != nil || u.Href() != url {
		t.Fatalf("%d spaces then %s: %v %v", MaxUnits-len(url), url, u, err)
	}
	if _, ok := NewInput(" " + at); ok {
		t.Fatal("one space more taken")
	}
	tabs := url[:4] + strings.Repeat("\t", MaxUnits-len(url)) + url[4:]
	if u, err := parse(tabs, nil); err != nil || u.Href() != url {
		t.Fatalf("tabs: %v %v", u, err)
	}
	if _, ok := NewInput(tabs + "\n"); ok {
		t.Fatal("one newline more taken")
	}
}

// TestNothingLonger checks the parser and the setters make no URL whose
// serialization is longer than MaxUnits, whatever the input: such a URL could
// not be given back as an Input, and a script could not change it. They
// refuse it with ErrTooLong, and a setter leaves the URL as it was.
func TestNothingLonger(t *testing.T) {
	// é is one unit and becomes %C3%A9, six.
	long := strings.Repeat("é", MaxUnits/6+1)
	for _, in := range []string{"http://h/" + long, "http://h/?" + long, "http://h/#" + long, "x:" + long, "http://" + long + "@h/", "x://" + long + "/"} {
		if u, err := parse(in, nil); err != ErrTooLong {
			t.Errorf("%.20q: %v %v, want ErrTooLong", in, u, err)
		}
	}
	// A URL that fits exactly, and one past it.
	at := "http://h/" + strings.Repeat("a", MaxUnits-len("http://h/"))
	if u, err := parse(at, nil); err != nil || len(u.Href()) != MaxUnits {
		t.Fatalf("MaxUnits long: %v", err)
	}
	// Against a base, each fits, and the URL does not.
	base, err := parse("http://h/"+strings.Repeat("a", MaxUnits/2), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parse("?"+strings.Repeat("q", MaxUnits/2), base); err != ErrTooLong {
		t.Errorf("against a base: %v, want ErrTooLong", err)
	}

	half := strings.Repeat("a", MaxUnits/2)
	for _, attr := range setterNames {
		u, err := parse("http://u:p@h:1/"+half+"?"+"#", nil)
		if err != nil {
			t.Fatal(err)
		}
		before := normalize(u.Clone())
		_, err = setAttr(u, attr, long)
		switch attr {
		case "protocol", "host", "hostname", "port":
			// These fail on the value as the standard says and leave the URL.
			if err != nil {
				t.Errorf("%s: %v", attr, err)
			}
		default:
			if err != ErrTooLong {
				t.Errorf("%s: %v, want ErrTooLong", attr, err)
			}
		}
		if got := normalize(u); got.Href() != before.Href() || got.Path.n != before.Path.n {
			t.Errorf("%s changed the URL to %.40q", attr, got.Href())
		}
	}
	// The path setter cuts and adds segments in place, which the undo must not
	// see: the old path comes back as it was.
	u, _ := parse("http://h/a/b/"+half, nil)
	before := u.Href()
	if err := u.SetPathname(mustInput("../c/" + long)); err != ErrTooLong || u.Href() != before {
		t.Errorf("pathname: %v, %.30q, want ErrTooLong and %.30q", err, u.Href(), before)
	}
	u.Path.Pop()
	u.Path.push("d")
	if !strings.HasSuffix(u.Href(), "/a/b/d") {
		t.Errorf("the path was shared after the undo: %.30q", u.Href())
	}
}

// TestFormEncodeLimit checks FormEncode refuses an encoding longer than
// MaxUnits, and FormDecode never makes a string longer than its input.
func TestFormEncodeLimit(t *testing.T) {
	if s, err := FormEncode(mustInput(strings.Repeat("a", MaxUnits))); err != nil || len(s) != MaxUnits {
		t.Errorf("plain MaxUnits: %d %v", len(s), err)
	}
	if s, err := FormEncode(mustInput(strings.Repeat("%", MaxUnits/3))); err != nil || len(s) != MaxUnits/3*3 {
		t.Errorf("%% x MaxUnits/3: %d %v", len(s), err)
	}
	if _, err := FormEncode(mustInput(strings.Repeat("%", MaxUnits/3+1))); err != ErrTooLong {
		t.Errorf("%% x MaxUnits/3+1: %v, want ErrTooLong", err)
	}
	for _, s := range []string{strings.Repeat("%41", MaxUnits/3), strings.Repeat("\x80", MaxUnits), strings.Repeat("%80", MaxUnits/3), strings.Repeat("+", MaxUnits), strings.Repeat("😀", MaxUnits/2)} {
		if d := FormDecode(mustInput(s)); units(d) > units(s) {
			t.Errorf("FormDecode(%.12q) is %d units, longer than its input", s, units(d))
		}
	}
}

// TestDomainLimit checks the limit of a host that is not ASCII:
// MaxDomainUnits UTF-16 units after percent-decoding, whatever the mapping of
// IDNA makes of them, here a name and an IPv4 address that are short (the
// sixth finding of the third review, written down as a limit in
// docs/changes/url.md). A host that is ASCII is not run through IDNA and has
// no limit but that of the URL.
func TestDomainLimit(t *testing.T) {
	shy, zero := "\u00ad", "\uff10" // a soft hyphen, which the mapping deletes, and a fullwidth 0
	for _, c := range []struct {
		host func(n int) string // a host of n units
		want string
	}{
		{func(n int) string { return "a" + strings.Repeat(shy, n-5) + ".com" }, "http://a.com/"},
		{func(n int) string { return strings.Repeat(zero, n-1) + "\uff11" }, "http://0.0.0.1/"},
		{func(n int) string { return strings.Repeat("%C2%AD", n-5) + "a.com" }, "http://a.com/"},
	} {
		at := c.host(MaxDomainUnits)
		if u, err := parse("http://"+at+"/", nil); err != nil || u.Href() != c.want {
			t.Errorf("%.20q of MaxDomainUnits: %v %v, want %s", at, u, err, c.want)
		}
		past := c.host(MaxDomainUnits + 1)
		if u, err := parse("http://"+past+"/", nil); err != ErrFailure {
			t.Errorf("%.20q of MaxDomainUnits+1: %v %v, want failure", past, u, err)
		}
	}
	// MaxDomainUnits bounds the domain once mapped too: U+3300 maps to four
	// katakana, so a few hundred units make a domain at the limit, and one
	// unit more past it (the first finding of the fourth review).
	square := "㌀"
	for n, want := range map[int]error{MaxDomainUnits: nil, MaxDomainUnits + 1: ErrFailure} {
		host := strings.Repeat(square, n/4) + strings.Repeat("a", n%4)
		if _, err := parse("http://"+host+"/", nil); err != want {
			t.Errorf("a domain that maps to %d units: %v, want %v", n, err, want)
		}
	}
	ascii := strings.Repeat("a", MaxUnits-len("http:///"))
	if u, err := parse("http://"+ascii+"/", nil); err != nil || u.Host != ascii {
		t.Errorf("an ASCII host of %d: %v", len(ascii), err)
	}
	// Only a domain goes through IDNA: the host of a scheme that is not
	// special is opaque, percent-encoded and bounded only by MaxUnits.
	long := strings.Repeat("é", MaxDomainUnits+1)
	if u, err := parse("x://"+long, nil); err != nil || u.Host != strings.Repeat("%C3%A9", MaxDomainUnits+1) {
		t.Errorf("an opaque host of MaxDomainUnits+1: %v %v", u, err)
	}
	if u, err := parse("http://"+long, nil); err != ErrFailure {
		t.Errorf("a domain of MaxDomainUnits+1: %v %v, want failure", u, err)
	}
}

// TestWorstAtLimit checks the time of a parse of each shape of input that
// makes the loops of the parser long, at MaxUnits: FuzzLimit with its seeds.
func TestWorstAtLimit(t *testing.T) {
	if raceEnabled {
		t.Skip("the times it checks mean nothing under the race detector")
	}
	for _, s := range shapeSeeds {
		long, ok := fill(s[0], s[1], s[2], MaxUnits)
		if !ok {
			t.Fatalf("%q does not fit", s)
		}
		in := mustInput(long)
		withinBound(t, "a parse of "+s[0]+s[1]+"...", func() {
			if u, err := Parse(in, nil); err == nil {
				u.Serialize()
			}
		})
	}
	// The hosts of the code points the tables grow the most, at the limit of a
	// domain as given and once mapped.
	m, mn, d, _ := idna.Widest()
	for _, host := range []string{
		strings.Repeat(string(m), MaxDomainUnits),
		strings.Repeat(string(m), MaxDomainUnits/mn),
		strings.Repeat(string(d), MaxDomainUnits),
	} {
		in := mustInput("http://" + host + "/")
		withinBound(t, "a parse of a host of "+host[:4]+"...", func() {
			if u, err := Parse(in, nil); err == nil {
				u.Serialize()
			}
		})
	}
}

// TestInvalidHostEnds checks a host of percent-encoded bytes that are not
// UTF-8 fails.
func TestInvalidHostEnds(t *testing.T) {
	if _, err := parse("http://"+strings.Repeat("%80", MaxUnits/3-3)+"/", nil); err != ErrFailure {
		t.Errorf("err = %v, want failure", err)
	}
}
