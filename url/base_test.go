// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package url

import (
	"strings"
	"testing"
)

// countBaseParses sets baseParseHook for the test and returns the count of
// bases parsed since.
func countBaseParses(t *testing.T) *int {
	t.Helper()
	n := new(int)
	baseParseHook = func() { *n++ }
	t.Cleanup(func() { baseParseHook = nil })
	return n
}

// TestBaseParsedOnce checks a VM parses the base it was last given once, and
// a base it was not last given, a long one and one that is not a URL each
// time.
func TestBaseParsedOnce(t *testing.T) {
	n := countBaseParses(t)
	vm := newVM(t)
	long := "http://h/" + strings.Repeat("a", maxBaseBytes)
	for _, c := range []struct {
		js   string
		want int
	}{
		{`for (var i = 0; i < 100; i++) new URL("/p" + i, "http://localhost"); ""`, 1},
		{`for (var i = 0; i < 100; i++) URL.canParse("/p" + i, "http://localhost"); ""`, 0},
		{`for (var i = 0; i < 10; i++) new URL("/p", i % 2 ? "http://a" : "http://b"); ""`, 10},
		{`for (var i = 0; i < 10; i++) new URL("/p", "` + long + `"); ""`, 10},
		// a string short enough whose href, percent-encoded, is not
		{`for (var i = 0; i < 10; i++) new URL("/p", "http://h/" + "\x01".repeat(40) + "/"); ""`, 10},
		{`for (var i = 0; i < 10; i++) try { new URL("/p", "no base"); } catch (e) {} ""`, 10},
		{`for (var i = 0; i < 10; i++) new URL("http://h/p", undefined); ""`, 0},
	} {
		*n = 0
		evalString(t, vm, c.js)
		if *n != c.want {
			t.Errorf("%s: %d bases parsed, want %d", c.js, *n, c.want)
		}
	}
	// A second VM keeps a base of its own.
	*n = 0
	other := newVM(t)
	evalString(t, other, `new URL("/p", "http://localhost"); ""`)
	evalString(t, vm, `new URL("/p", "http://localhost"); ""`)
	evalString(t, other, `new URL("/p", "http://localhost"); ""`)
	if *n != 2 {
		t.Errorf("two VMs: %d bases parsed, want 2", *n)
	}
}

// TestKeptBaseUnchanged checks that the URLs resolved against a kept base
// are those resolved against a base parsed for each, and that none changes the
// base: each input reaches a part of the base the parser copies (the path, the
// query, the host), and is resolved after the others in both orders.
func TestKeptBaseUnchanged(t *testing.T) {
	vm := newVM(t)
	got := evalString(t, vm, `(function () {
		var bases = ["http://u:p@h:8/a/b/c?q#f", "file:///C:/x/y", "sc:/p/q", "sc:opaque"];
		var inputs = ["", "?n", "#n", "d", "./d/", "../d", "../../../d", "/d", "//g/d", "C:/z", "\\\\d", "d?x#y"];
		var out = [];
		for (var b of bases) {
			for (var pass = 0; pass < 2; pass++) {
				var list = pass ? inputs.slice().reverse() : inputs;
				for (var s of list) {
					var u = URL.parse(s, b);
					out.push(b + " " + s + " " + (u === null ? "null" : u.href));
				}
			}
		}
		return out.join("\n");
	})()`)
	lines := strings.Split(got, "\n")
	byKey := map[string]string{}
	for _, l := range lines {
		i := strings.LastIndexByte(l, ' ')
		k, v := l[:i], l[i+1:]
		if w, ok := byKey[k]; ok && w != v {
			t.Errorf("%s: %s, then %s", k, w, v)
		}
		byKey[k] = v
	}
	for k, want := range map[string]string{
		"http://u:p@h:8/a/b/c?q#f ":           "http://u:p@h:8/a/b/c?q",
		"http://u:p@h:8/a/b/c?q#f ../../../d": "http://u:p@h:8/d",
		"http://u:p@h:8/a/b/c?q#f //g/d":      "http://g/d",
		"file:///C:/x/y /d":                   "file:///C:/d",
		"file:///C:/x/y ../d":                 "file:///C:/d",
		"sc:/p/q d?x#y":                       "sc:/p/d?x#y",
		"sc:opaque #n":                        "sc:opaque#n",
		"sc:opaque d":                         "null",
	} {
		if byKey[k] != want {
			t.Errorf("%s: %s, want %s", k, byKey[k], want)
		}
	}
}
