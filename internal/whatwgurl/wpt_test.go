// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package whatwgurl

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
)

// These tests run the URL data of web-platform-tests (testdata/, see
// testdata/SOURCES.md) on the parser and the setters, with the getters of the
// URL API written on Components.

func loadJSON(t testing.TB, name string, v any) {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, v); err != nil {
		t.Fatal(err)
	}
}

// getters are the attributes of the URL interface, read from the serialization.
func getters(u *URL) map[string]string {
	href, c := u.Serialize()
	m := map[string]string{"href": href}
	m["protocol"] = href[:c.ProtocolEnd]
	m["username"] = href[c.UsernameStart:c.UsernameEnd]
	m["password"] = href[c.PasswordStart:c.PasswordEnd]
	m["hostname"] = href[c.HostStart:c.HostEnd]
	m["host"] = href[c.HostStart:c.PortEnd]
	m["port"] = href[c.PortStart:c.PortEnd]
	pathEnd := len(href)
	if c.HashStart >= 0 {
		pathEnd = c.HashStart
	}
	searchEnd := pathEnd
	if c.SearchStart >= 0 {
		pathEnd = c.SearchStart
	}
	m["pathname"] = href[c.PathStart:pathEnd]
	m["search"] = ""
	if c.SearchStart >= 0 && searchEnd-c.SearchStart > 1 {
		m["search"] = href[c.SearchStart:searchEnd]
	}
	m["hash"] = ""
	if c.HashStart >= 0 && len(href)-c.HashStart > 1 {
		m["hash"] = href[c.HashStart:]
	}
	m["origin"] = u.Origin()
	return m
}

var attrKeys = []string{"href", "protocol", "username", "password", "host", "hostname", "port", "pathname", "search", "hash", "origin"}

func setAttr(u *URL, attr, v string) (*URL, error) {
	switch attr {
	case "href":
		n, err := parse(v, nil)
		if err != nil {
			return u, nil
		}
		return n, nil
	case "protocol":
		return u, u.SetProtocol(mustInput(v))
	case "username":
		return u, u.SetUsername(mustInput(v))
	case "password":
		return u, u.SetPassword(mustInput(v))
	case "host":
		return u, u.SetHost(mustInput(v))
	case "hostname":
		return u, u.SetHostname(mustInput(v))
	case "port":
		return u, u.SetPort(mustInput(v))
	case "pathname":
		return u, u.SetPathname(mustInput(v))
	case "search":
		return u, u.SetSearch(mustInput(v))
	case "hash":
		return u, u.SetHash(mustInput(v))
	default:
		return nil, fmt.Errorf("no attribute %q", attr)
	}
}

// knownFailures reads testdata/known-failures.txt: one line per case that the
// parser is known to get wrong, as "<file> <case id>  # reason".
func knownFailures(t testing.TB) map[string]bool {
	b, err := os.ReadFile("testdata/known-failures.txt")
	if err != nil {
		t.Fatal(err)
	}
	m := map[string]bool{}
	for _, l := range strings.Split(string(b), "\n") {
		if l == "" || l[0] == '#' {
			continue
		}
		if i := strings.Index(l, "  # "); i >= 0 {
			l = l[:i]
		}
		m[l] = true
	}
	return m
}

type known struct {
	t    *testing.T
	m    map[string]bool
	used map[string]bool
}

func newKnown(t *testing.T) *known {
	return &known{t: t, m: knownFailures(t), used: map[string]bool{}}
}

// check reports a failure of case id unless it is known, and a known case
// that passes.
func (k *known) check(id string, bad string) {
	k.t.Helper()
	key := strconv.Quote(id)
	if k.m[key] {
		k.used[key] = true
		if bad == "" {
			k.t.Errorf("%s passes but is in known-failures.txt", key)
		}
		return
	}
	if bad != "" {
		k.t.Errorf("%s: %s", key, bad)
	}
}

func TestWPTParse(t *testing.T) {
	var raw []json.RawMessage
	loadJSON(t, "urltestdata.json", &raw)
	var raw2 []json.RawMessage
	loadJSON(t, "urltestdata-javascript-only.json", &raw2)
	raw = append(raw, raw2...)
	k := newKnown(t)
	n := 0
	for _, r := range raw {
		if r[0] == '"' {
			continue
		}
		var c map[string]any
		json.Unmarshal(r, &c)
		input := c["input"].(string)
		var base *URL
		var bad string
		id := "urltestdata " + input
		var err error
		if b, ok := c["base"].(string); ok {
			id += " base " + b
			base, err = parse(b, nil)
			if err != nil {
				bad = "base does not parse"
				if c["failure"] != true {
					k.check(id, bad)
				} else {
					k.check(id, "")
				}
				continue
			}
		}
		u, err := parse(input, base)
		if c["failure"] == true {
			if err == nil {
				bad = "parsed, want failure: " + u.Href()
			}
		} else if err != nil {
			bad = "failed: " + err.Error()
		} else {
			g := getters(u)
			for _, key := range attrKeys {
				if w, ok := c[key].(string); ok && g[key] != w {
					bad += fmt.Sprintf(" %s=%q want %q", key, g[key], w)
				}
			}
		}
		n++
		k.check(id, bad)
	}
	for key := range k.m {
		if strings.HasPrefix(key, `"urltestdata `) && !k.used[key] {
			t.Errorf("%s is in known-failures.txt but is not a case", key)
		}
	}
	t.Logf("%d cases", n)
}

func TestWPTSetters(t *testing.T) {
	var all map[string]json.RawMessage
	loadJSON(t, "setters_tests.json", &all)
	k := newKnown(t)
	for attr, list := range all {
		if attr == "comment" {
			continue
		}
		var cases []struct {
			Comment  string
			Href     string
			NewValue string `json:"new_value"`
			Expected map[string]string
		}
		if err := json.Unmarshal(list, &cases); err != nil {
			t.Fatal(err)
		}
		for _, c := range cases {
			id := fmt.Sprintf("setters %s %s on %s", attr, strconv.Quote(c.NewValue), c.Href)
			u, err := parse(c.Href, nil)
			if err != nil {
				k.check(id, "href does not parse")
				continue
			}
			u, err = setAttr(u, attr, c.NewValue)
			if err != nil {
				t.Fatal(err)
			}
			g := getters(u)
			bad := ""
			for key, w := range c.Expected {
				if g[key] != w {
					bad += fmt.Sprintf(" %s=%q want %q", key, g[key], w)
				}
			}
			k.check(id, bad)
		}
	}
}

func TestWPTToASCII(t *testing.T) {
	for _, file := range []string{"toascii.json", "IdnaTestV2.json", "IdnaTestV2-removed.json"} {
		var raw []json.RawMessage
		loadJSON(t, file, &raw)
		k := newKnown(t)
		for _, r := range raw {
			if r[0] == '"' {
				continue
			}
			var c struct {
				Input  string
				Output *string
			}
			json.Unmarshal(r, &c)
			if c.Input == "" && file == "IdnaTestV2.json" {
				continue
			}
			id := file + " " + c.Input
			bad := ""
			u, err := parse("https://"+c.Input+"/x", nil)
			if c.Output == nil {
				if err == nil {
					bad = "parsed, want failure: " + u.Href()
				}
			} else if err != nil {
				bad = "failed: " + err.Error()
			} else if g := getters(u); g["host"] != *c.Output || g["hostname"] != *c.Output || g["pathname"] != "/x" || g["href"] != "https://"+*c.Output+"/x" {
				bad = fmt.Sprintf("got %q want %q", g["href"], *c.Output)
			}
			k.check(id, bad)
			if file != "toascii.json" {
				continue
			}
			// The host and hostname setters on https://x/x.
			for _, attr := range []string{"host", "hostname"} {
				u, _ := parse("https://x/x", nil)
				setAttr(u, attr, c.Input)
				want := "x"
				if c.Output != nil {
					want = *c.Output
				}
				bad := ""
				if got := getters(u)[attr]; got != want {
					bad = fmt.Sprintf("%s=%q want %q", attr, got, want)
				}
				k.check(file+" "+attr+" "+c.Input, bad)
			}
		}
	}
}

// TestWPTPercentEncoding runs the cases of percent-encoding.json for UTF-8,
// the only encoding of the URL API.
func TestWPTPercentEncoding(t *testing.T) {
	var raw []json.RawMessage
	loadJSON(t, "percent-encoding.json", &raw)
	for _, r := range raw {
		if r[0] == '"' {
			continue
		}
		var c struct {
			Input  string
			Output map[string]string
		}
		json.Unmarshal(r, &c)
		want := c.Output["utf-8"]
		u, err := parse("https://example.org/?"+c.Input+"#"+c.Input, nil)
		if err != nil {
			t.Fatal(err)
		}
		g := getters(u)
		if g["search"] != "?"+want || g["hash"] != "#"+want {
			t.Errorf("%q: search %q hash %q, want %q", c.Input, g["search"], g["hash"], want)
		}
	}
}
