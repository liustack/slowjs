// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package url

import (
	"strings"
	"testing"
)

// countFromParts sets fromPartsHook for the test and returns the count of
// records made again from their parts since.
func countFromParts(t *testing.T) *int {
	t.Helper()
	n := new(int)
	fromPartsHook = func() { *n++ }
	t.Cleanup(func() { fromPartsHook = nil })
	return n
}

// TestSetTakesTheLastRecord checks that a setter on the URL whose record the
// VM made last takes that record, and that a setter on any other URL makes
// the record again from its parts, with the same results.
func TestSetTakesTheLastRecord(t *testing.T) {
	n := countFromParts(t)
	vm := newVM(t)
	long := strings.Repeat("a", maxKeptBytes)
	for _, c := range []struct {
		js   string
		want int
	}{
		// a router's link: a parse, then a setter or two on it
		{`var u = new URL("/p?q#h", "http://localhost"); u.pathname = "/zh" + u.pathname; u.search = "?r"; u.href`, 0},
		// a setter on a URL made before the last one
		{`var a = new URL("http://a/x"), b = new URL("http://b/y"); a.pathname = "/z"; a.href + " " + b.href`, 1},
		// then on b, whose record a's setter did not make
		{`b.hash = "#k"; b.href`, 1},
		// a setter that fails leaves the URL and the kept record as they were
		{`var c = new URL("http://c/"); c.port = "x"; c.protocol = "a b"; c.port = "81"; c.href`, 0},
		// a record whose href is longer than what is kept
		{`var d = new URL("http://d/` + long + `"); d.hash = "#1"; d.hash = "#2"; d.hash`, 2},
		// a record a setter made from parts the parser does not make
		{`var f = new URL("http://localhost/C|/x"); f.protocol = "file"; f.search = "?s"; f.host + " " + f.pathname + " " + f.href`, 0},
	} {
		*n = 0
		evalString(t, vm, c.js)
		if *n != c.want {
			t.Errorf("%s: %d records made from parts, want %d", c.js, *n, c.want)
		}
	}
	for js, want := range map[string]string{
		`u.href`:                "http://localhost/zh/p?r#h",
		`a.href + " " + b.href`: "http://a/z http://b/y#k",
		`c.href`:                "http://c:81/",
		`d.hash`:                "#2",
		`f.host + " " + f.pathname + " " + f.href`: "localhost /C|/x file://localhost/C|/x?s",
	} {
		if got := evalString(t, vm, js); got != want {
			t.Errorf("%s: %q, want %q", js, got, want)
		}
	}
}
