// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package whatwgurl

import (
	"cmp"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"
)

// The fuzz targets say what the parser promises for any input of at most
// MaxUnits, which a server takes from anyone:
//
//   - it never panics;
//   - the serialization of a URL it made is ASCII and at most MaxUnits long, so
//     it can be given back as an Input, and it parses again to the same record
//     (FuzzParse);
//   - the URL a setter leaves is made again from its parts, and its
//     serialization is ASCII, at most MaxUnits long and a URL (FuzzSetters);
//   - a setter that would make the serialization longer leaves the URL as it
//     was (FuzzSetters);
//   - FromParts takes no parts but those of a URL, and does not panic on any
//     (FuzzFromParts);
//   - a call takes at most callBound, whatever the input (FuzzLimit).
//
// The seeds are the inputs of the web-platform-tests data. A failing input
// that a fuzz run found goes into testdata/fuzz/<target>/, which go test runs
// as a test from then on, with the fix.

// normalize makes a record comparable: an empty list of segments is the nil one.
func normalize(u *URL) *URL {
	c := *u
	if c.Path.n == 0 {
		c.Path.b = nil
	}
	return &c
}

// check says what is wrong with u, a URL that came out of the parser, or "".
func check(u *URL) string {
	if msg := checkSet(u); msg != "" {
		return msg
	}
	href := u.Href()
	again, err := parse(href, nil)
	if err != nil {
		return "the href does not parse again: " + href
	}
	if !reflect.DeepEqual(normalize(u), normalize(again)) {
		return "the href parses to another record: " + href + "\nbefore " + dump(u) + "\nafter  " + dump(again)
	}
	if again.Href() != href {
		return "the href serializes differently: " + again.Href()
	}
	return ""
}

// checkSet says what is wrong with u, a URL that a setter left, or "". Such a
// URL may be a record the parser does not make from its href (see Parts), so
// it is not parsed to itself: it is made again from its parts.
func checkSet(u *URL) string {
	href := u.Href()
	for i := 0; i < len(href); i++ {
		if href[i] >= utf8.RuneSelf {
			return "href is not ASCII: " + href
		}
	}
	if len(href) > MaxUnits {
		return "href of " + itoa(len(href)) + " bytes is longer than MaxUnits"
	}
	if len(href) != u.length() {
		return "length says " + itoa(u.length()) + " for an href of " + itoa(len(href))
	}
	p, h := u.Parts()
	if h != href {
		return "Parts gives another href: " + h
	}
	again, err := FromParts(p)
	if err != nil {
		return "FromParts refuses the parts of " + href + ": " + err.Error()
	}
	if !reflect.DeepEqual(normalize(u), normalize(again)) {
		return "the parts make another record: " + href + "\nbefore " + dump(u) + "\nafter  " + dump(again)
	}
	if _, err := parse(href, nil); err != nil {
		return "the href is not a URL: " + href
	}
	return ""
}

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}

func dump(u *URL) string {
	b, _ := json.Marshal(u)
	return string(b)
}

// seeds returns the inputs and bases of the WPT URL data.
func seeds(f *testing.F) (inputs [][2]string, values []string) {
	var raw []json.RawMessage
	b, err := os.ReadFile("testdata/urltestdata.json")
	if err != nil {
		f.Fatal(err)
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		f.Fatal(err)
	}
	for _, r := range raw {
		if r[0] == '"' {
			continue
		}
		var c struct {
			Input string
			Base  *string
		}
		json.Unmarshal(r, &c)
		base := ""
		if c.Base != nil {
			base = *c.Base
		}
		inputs = append(inputs, [2]string{c.Input, base})
	}
	var all map[string]json.RawMessage
	b, err = os.ReadFile("testdata/setters_tests.json")
	if err != nil {
		f.Fatal(err)
	}
	json.Unmarshal(b, &all)
	for _, list := range all {
		var cases []struct {
			Href     string
			NewValue string `json:"new_value"`
		}
		if json.Unmarshal(list, &cases) == nil {
			for _, c := range cases {
				inputs = append(inputs, [2]string{c.Href, ""})
				values = append(values, c.NewValue)
			}
		}
	}
	return
}

func FuzzParse(f *testing.F) {
	inputs, _ := seeds(f)
	for _, in := range inputs {
		f.Add(in[0], in[1])
	}
	f.Fuzz(func(t *testing.T, input, base string) {
		in, ok := NewInput(input)
		if !ok {
			if units(input) <= MaxUnits {
				t.Fatalf("NewInput refused %d units", units(input))
			}
			return
		}
		var b *URL
		if bi, ok := NewInput(base); ok && base != "" {
			if b, _ = Parse(bi, nil); b != nil {
				if msg := check(b); msg != "" {
					t.Fatalf("base %q: %s", base, msg)
				}
			}
		}
		u, err := Parse(in, b)
		switch err {
		case nil:
		case ErrFailure, ErrTooLong:
			return
		default:
			t.Fatalf("err = %v", err)
		}
		if msg := check(u); msg != "" {
			t.Fatalf("%q against %q: %s", input, base, msg)
		}
	})
}

var setterNames = []string{"protocol", "username", "password", "host", "hostname", "port", "pathname", "search", "hash"}

func FuzzSetters(f *testing.F) {
	inputs, values := seeds(f)
	for i, in := range inputs {
		if i < len(values) {
			f.Add(in[0], uint8(i%len(setterNames)), values[i])
		}
	}
	f.Fuzz(func(t *testing.T, href string, which uint8, value string) {
		in, ok := NewInput(href)
		if !ok {
			return
		}
		u, err := Parse(in, nil)
		if err != nil {
			return
		}
		if _, ok := NewInput(value); !ok {
			return
		}
		attr := setterNames[int(which)%len(setterNames)]
		before := normalize(u.Clone())
		switch _, err := setAttr(u, attr, value); err {
		case nil:
		case ErrTooLong:
			if !reflect.DeepEqual(normalize(u), before) {
				t.Fatalf("setter %s refused %q but changed %q to %q", attr, value, before.Href(), u.Href())
			}
		default:
			t.Fatalf("setter %s: %v", attr, err)
		}
		if msg := checkSet(u); msg != "" {
			t.Fatalf("%s = %q on %q: %s", attr, value, href, msg)
		}
	})
}

// FuzzFromParts forges parts: those of a URL with one replaced by any string
// and the bits flipped by any mask. FromParts never panics, and the URL it
// takes them for gives them back, and passes checkSet.
func FuzzFromParts(f *testing.F) {
	inputs, values := seeds(f)
	for i, in := range inputs {
		if i < len(values) {
			f.Add(in[0], uint8(i), values[i], uint8(i>>3))
		}
	}
	f.Fuzz(func(t *testing.T, href string, which uint8, value string, mask uint8) {
		in, ok := NewInput(href)
		if !ok {
			return
		}
		u, err := Parse(in, nil)
		if err != nil {
			return
		}
		v, ok := NewInput(value)
		if !ok {
			return
		}
		p, _ := u.Parts()
		parts := [...]*Input{&p.Protocol, &p.Username, &p.Password, &p.Hostname, &p.Port, &p.Pathname, &p.Search, &p.Hash}
		if i := int(which) % (len(parts) + 1); i < len(parts) {
			*parts[i] = v
		}
		p.HasHost = p.HasHost != (mask&1 != 0)
		p.Opaque = p.Opaque != (mask&2 != 0)
		p.HasQuery = p.HasQuery != (mask&4 != 0)
		p.HasFragment = p.HasFragment != (mask&8 != 0)
		got, err := FromParts(p)
		switch {
		case err == ErrNotParts || err == ErrTooLong:
			if got != nil {
				t.Fatalf("FromParts refused %+v but made a URL", p)
			}
			return
		case err != nil:
			t.Fatalf("FromParts(%+v): %v", p, err)
		}
		if again, _ := got.Parts(); again != p {
			t.Fatalf("FromParts took %+v and gives %+v back", p, again)
		}
		if msg := checkSet(got); msg != "" {
			t.Fatalf("FromParts took %+v: %s", p, msg)
		}
	})
}

// FuzzLimit checks the time of a call on an input of MaxUnits made of a
// prefix, a unit repeated and a suffix: a parse and its serialization, or a
// setter on a URL whose five long parts are each an eighth of the limit
// and the serialization, take at most callBound. The time that a call takes is in
// proportion to its input but for the IDNA of a host, which MaxDomainUnits
// bounds, so this is the check that nothing else is more than linear and that
// the constant is small enough.
func FuzzLimit(f *testing.F) {
	inputs, _ := seeds(f)
	// A few of the WPT inputs, each with a long run after it, so that the seeds
	// run in seconds. The fuzzer goes on from them.
	for i, in := range inputs {
		if i%50 == 0 {
			f.Add(in[0], "a/", "", uint8(i))
		}
	}
	for i, s := range shapeSeeds {
		f.Add(s[0], s[1], s[2], uint8(i))
	}
	big := "http://" + strings.Repeat("u", MaxUnits/8) + ":" + strings.Repeat("p", MaxUnits/8) + "@h/" + strings.Repeat("a", MaxUnits/8) + "?" + strings.Repeat("q", MaxUnits/8) + "#" + strings.Repeat("f", MaxUnits/8)
	f.Fuzz(func(t *testing.T, prefix, unit, suffix string, which uint8) {
		if raceEnabled {
			t.Skip("the times it checks mean nothing under the race detector")
		}
		long, ok := fill(prefix, unit, suffix, MaxUnits)
		if !ok {
			return
		}
		in := mustInput(long)
		attr := ""
		if which%2 == 1 {
			attr = setterNames[int(which/2)%len(setterNames)]
		}
		withinBound(t, fmt.Sprintf("%s of %.60q", cmp.Or(attr, "a parse"), long), func() {
			if attr != "" {
				u, _ := parse(big, nil)
				setAttr(u, attr, long)
				u.Serialize()
			} else if u, err := Parse(in, nil); err == nil {
				u.Serialize()
			}
		})
	})
}

// shapeSeeds are inputs with a long part, as prefix, unit and suffix: the
// shapes that make the loops of the parser long.
var shapeSeeds = [][3]string{
	{"http://h/", "a", ""}, {"http://h/", "/", ""}, {"http://h/", "a/", ""}, {"http://h/", "../", ""},
	{"http://h/", "\\", ""}, {"http://h/?", "a", ""}, {"http://h/#", "a", ""}, {"x:", "a", ""},
	{"x:a", " ", "?b"}, {"http://", ":", "@h/"}, {"http://", "a@", "h/"}, {"http://", "a", "/"},
	{"http://", "a.", "/"}, {"http://", "\u00e9", "/"}, {"http://a", "\u0315\u0300", "/"},
	{"http://", "%C3%A9", "/"}, {"http://", "xn--", "/"}, {"x://", "\u00e9", "/"}, {"http://[", "1:", "]/"},
	{"http://", "1.", "/"}, {"http://h/", "a\t", ""}, {"", " ", "http://h/"}, {"http://h/?", "%41", ""},
	{"http://h/?", "+=", ""}, {"http://h/?", "a&", ""}, {"http://h/?", "\u4e00", ""}, {"http://h/", "\U0001F600", ""},
	{"http://h/", "\u00e9", ""}, {"http://h/?", "\u00e9", "#\u00e9"}, {"x:", "\u00e9", ""}, {"http://", "\u00e9:", "@h/"}, {"x://h/", "%", ""},
	{"http://h/", "%2e", ""}, {"http://h/", "a/..", ""}, {"file:///", "a/", ""}, {"file://", "/", ""},
}
