// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package url

import "testing"

// The conversions of the arguments of URLSearchParams, which a page can make
// run its own code: a toString that changes the URL, a Symbol key, an iterator
// that is null.

func TestConversionReentry(t *testing.T) {
	vm := newVM(t)
	for js, want := range map[string]string{
		`(function () {
			const u = new URL("http://h/?old=1"), p = u.searchParams;
			p.append({toString() { u.search = "?new=2"; return "x"; }}, "3");
			return p.toString();
		})()`: "new=2&x=3",
		`(function () {
			const u = new URL("http://h/?old=1"), p = u.searchParams;
			p.set({toString() { u.search = "?new=2"; return "x"; }}, "3");
			return p.toString();
		})()`: "new=2&x=3",
		`(function () {
			const u = new URL("http://h/?a=1&b=2"), p = u.searchParams;
			p.delete({toString() { u.search = "?a=9&c=8"; return "a"; }});
			return p.toString() + "|" + u.search;
		})()`: "c=8|?c=8",
		`(function () {
			const u = new URL("http://h/?a=1"), p = u.searchParams;
			return String(p.get({toString() { u.search = "?a=2"; return "a"; }}));
		})()`: "2",
		`(function () {
			const u = new URL("http://h/?a=1"), p = u.searchParams;
			return p.getAll({toString() { u.search = "?a=2&a=3"; return "a"; }}).join();
		})()`: "2,3",
		`(function () {
			const u = new URL("http://h/?a=1"), p = u.searchParams;
			return String(p.has({toString() { u.search = "?b=2"; return "b"; }}));
		})()`: "true",
	} {
		if got := evalString(t, vm, js); got != want {
			t.Errorf("%s\n= %q, want %q", js, got, want)
		}
	}
}

func TestConversionRecordSymbolKey(t *testing.T) {
	vm := newVM(t)
	got := evalString(t, vm, `(function () {
		try { return "no error: " + new URLSearchParams({[Symbol("k")]: "v"}); } catch (e) { return e.name; }
	})()`)
	if got != "TypeError" {
		t.Errorf("got %q, want TypeError", got)
	}
	// A symbol key that is not enumerable is skipped.
	got = evalString(t, vm, `String(new URLSearchParams(Object.defineProperty({a: 1}, Symbol("k"), {value: "v"})))`)
	if got != "a=1" {
		t.Errorf("non-enumerable symbol: %q", got)
	}
}

func TestConversionNullIterator(t *testing.T) {
	vm := newVM(t)
	got := evalString(t, vm, `(function () {
		try {
			return String(new URLSearchParams(Object.defineProperty({a: 1}, Symbol.iterator, {value: null})));
		} catch (e) { return e.name; }
	})()`)
	if got != "a=1" {
		t.Errorf("got %q, want a=1", got)
	}
	got = evalString(t, vm, `(function () {
		try { new URLSearchParams(Object.defineProperty({a: 1}, Symbol.iterator, {value: 1})); return "no error"; } catch (e) { return e.name; }
	})()`)
	if got != "TypeError" {
		t.Errorf("a non-callable iterator: %q, want TypeError", got)
	}
}
