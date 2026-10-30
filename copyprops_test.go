// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package slowjs // import "github.com/liustack/slowjs"

import (
	"os"
	"strings"
	"testing"
)

// The function of RegisterCopyProps gives what the loops of React 19.3 give
// in createElement and cloneElement, for every kind of config and props. The
// script is the one the C tree runs (engine/test/compare/copy_props.js), and
// its text is the file next to it.
func TestCopyPropsLikeReact(t *testing.T) {
	script, err := os.ReadFile("engine/test/compare/copy_props.js")
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("engine/test/compare/copy_props.txt")
	if err != nil {
		t.Fatal(err)
	}
	m := newTestVM(t)
	if err := m.RegisterCopyProps("copyProps"); err != nil {
		t.Fatal(err)
	}
	got := evalString(t, m, string(script))
	if got != strings.TrimRight(string(want), "\n") {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
}

// The function is on the global object only for a host that registers it,
// under the name the host gives, as a built-in function.
func TestRegisterCopyProps(t *testing.T) {
	m := newTestVM(t)
	if got := evalString(t, m, `typeof __copy`); got != "undefined" {
		t.Fatalf("a VM that registered nothing has __copy: %s", got)
	}
	if err := m.RegisterCopyProps("__copy"); err != nil {
		t.Fatal(err)
	}
	got := evalString(t, m, `
	  var d = Object.getOwnPropertyDescriptor(globalThis, '__copy');
	  var hop = Object.prototype.hasOwnProperty, props = {};
	  JSON.stringify([d.writable, d.enumerable, d.configurable, __copy.name, __copy.length,
	    Object.hasOwn(__copy, 'prototype'), String(__copy), Object.getPrototypeOf(__copy) === Function.prototype,
	    (() => { try { new __copy({}, {}, hop, false); return 'made' } catch (e) { return e.name } })(),
	    Object.keys(globalThis).includes('__copy'),
	    __copy({ a: 1, key: 'k', b: [2] }, props, hop, false), Object.keys(props), JSON.stringify(props)])`)
	const want = `[true,false,true,"__copy",4,false,"function __copy() {\n    [native code]\n}",true,"TypeError",false,true,["a","b"],"{\"a\":1,\"b\":[2]}"]`
	if got != want {
		t.Fatalf("got %s\nwant %s", got, want)
	}

	// another VM is unaffected, and a second call replaces the function
	other := newTestVM(t)
	if got := evalString(t, other, `typeof __copy`); got != "undefined" {
		t.Fatalf("another VM has __copy: %s", got)
	}
	if _, err := m.Eval(`var first = __copy`, EvalGlobal); err != nil {
		t.Fatal(err)
	}
	if err := m.RegisterCopyProps("__copy"); err != nil {
		t.Fatal(err)
	}
	if got := evalString(t, m, `String(first !== __copy) + __copy({ a: 1 }, {}, Object.prototype.hasOwnProperty, false)`); got != "truetrue" {
		t.Fatal(got)
	}

	// it replaces what a script assigned, and keeps what cannot be replaced
	if _, err := m.Eval(`globalThis.assigned = 1; var declared = 2`, EvalGlobal); err != nil {
		t.Fatal(err)
	}
	if err := m.RegisterCopyProps("assigned"); err != nil {
		t.Fatal(err)
	}
	if got := evalString(t, m, `typeof assigned`); got != "function" {
		t.Fatal(got)
	}
	for _, name := range []string{"undefined", "NaN", "declared"} {
		if err := m.RegisterCopyProps(name); err == nil {
			t.Errorf("RegisterCopyProps(%q) returned no error", name)
		}
	}
	if got := evalString(t, m, `String(declared) + typeof undefined`); got != "2undefined" {
		t.Fatal(got)
	}
	if err := m.RegisterCopyProps(""); err == nil {
		t.Error("an empty name returned no error")
	}
	if err := m.RegisterCopyProps("a\x00b"); err == nil {
		t.Error("a name with a NUL byte returned no error")
	}
	// invalid UTF-8 in the name becomes U+FFFD as it does for RegisterFunc
	if err := m.RegisterCopyProps("bad\xc0name"); err != nil {
		t.Fatal(err)
	}
	if got := evalString(t, m, `typeof globalThis['bad�name']`); got != "function" {
		t.Fatal(got)
	}
}

// A VM closed after it registered the function, or never used it, leaves
// nothing behind, and the function of a VM keeps working across its collections.
func TestCopyPropsAcrossCollections(t *testing.T) {
	m := newTestVM(t)
	if err := m.RegisterCopyProps("copy"); err != nil {
		t.Fatal(err)
	}
	got := evalString(t, m, `
	  var hop = Object.prototype.hasOwnProperty, total = 0;
	  for (var round = 0; round < 50; round++) {
	    var list = [];
	    for (var i = 0; i < 200; i++) {
	      var props = {};
	      copy({ id: i, label: 'item ' + i, key: i, nested: { i: i } }, props, hop, false);
	      list.push(props);
	    }
	    total += list.length + list[199].nested.i;
	  }
	  total`)
	if got != "19950" {
		t.Fatal(got)
	}
}
