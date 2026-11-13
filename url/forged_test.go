// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package url

import (
	"strings"
	"testing"

	"github.com/liustack/quickjs"
)

// newForgingVM returns a VM with URL whose set Builtin is reached through
// globalThis.forge: url.js calls it with the arguments it would hand to set,
// and set gets what it returns. No script of a page can do this (see
// TestStateIsUnreachable). It is how the tests hand set parts that are not
// those of a URL, as only a fault of url.js could.
func newForgingVM(t *testing.T) *quickjs.VM {
	t.Helper()
	vm, err := quickjs.NewVM()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { vm.Close() })
	code, err := bytecode()
	if err != nil {
		t.Fatal(err)
	}
	install, err := vm.EvalBytecodeValue(code)
	if err != nil {
		t.Fatal(err)
	}
	defer install.Free()
	wrap, err := vm.EvalValue(`(set) => {
		globalThis.forge = (args) => args;
		return function () { return set(...globalThis.forge([...arguments])); };
	}`, quickjs.EvalGlobal)
	if err != nil {
		t.Fatal(err)
	}
	defer wrap.Free()
	args := make([]any, 0, len(bridge))
	for _, b := range bridge {
		v, err := vm.NewBuiltinFunction(b)
		if err != nil {
			t.Fatal(err)
		}
		defer v.Free()
		if b == setBuiltin {
			if v, err = wrap.CallValue(quickjs.UndefinedValue, v); err != nil {
				t.Fatal(err)
			}
			defer v.Free()
		}
		args = append(args, v)
	}
	if _, err := install.Call(quickjs.UndefinedValue, args...); err != nil {
		t.Fatal(err)
	}
	return vm
}

// TestForgedRecord checks that set refuses, with a TypeError, parts that are
// not those of a URL, and that the URL is left as it was: flags that say other
// than the parts, a port that is not a port, a host that does not serialize
// to itself, parts longer in all than a URL may be, a part that is not a
// string. The arguments of set are part, value, flags, then protocol,
// username, password, hostname, port, pathname, search and hash, at 3 to 10.
func TestForgedRecord(t *testing.T) {
	vm := newForgingVM(t)
	evalString(t, vm, `var C = 65536, a = 'a'.repeat(C / 2), b = 'b'.repeat(C / 2);
		function at(i, v) { return (args) => { args[i] = v; return args; }; }
		function flags(f) { return (args) => { args[2] = f(args[2]); return args; }; }
		function attempt(href, forge, set) {
			const u = new URL(href);
			globalThis.forge = forge;
			let got;
			try {
				set(u);
				got = 'no error';
			} catch (e) {
				got = e.name;
			} finally {
				globalThis.forge = (args) => args;
			}
			return got + ' ' + (u.href === href ? 'unchanged' : 'changed to ' + u.href);
		}
		'ok'`)
	path := `(u) => { u.pathname = "/x" }`
	for _, c := range []struct{ name, href, forge, set string }{
		{"no host on a URL with one", "http://h/p", `flags((f) => f ^ 1)`, path},
		{"an opaque path on a URL with a host", "http://h/p", `flags((f) => f ^ 2)`, path},
		{"no query on a URL with one", "http://h/p?q", `flags((f) => f ^ 4)`, path},
		{"no fragment on a URL with one", "http://h/p#f", `flags((f) => f ^ 8)`, path},
		{"a host on a URL with none", "foo:x", `flags((f) => f | 1)`, `(u) => { u.hash = "y" }`},
		{"flags past the four bits", "http://h/p", `flags((f) => f | 16)`, path},
		{"negative flags", "http://h/p", `flags(() => -1)`, path},
		{"flags that are not an integer", "http://h/p", `flags((f) => f + 0.5)`, path},
		{"flags that are a string", "http://h/p", `flags((f) => String(f))`, path},
		{"no flags", "http://h/p", `flags(() => undefined)`, path},
		{"a port of letters", "http://h:8/p", `at(7, "x")`, path},
		{"a port past 65535", "http://h:8/p", `at(7, "65536")`, path},
		{"a port with a 0 in front", "http://h:8/p", `at(7, "08")`, path},
		{"the default port", "http://h:8/p", `at(7, "80")`, path},
		{"a port that is a number", "http://h:8/p", `at(7, 8)`, path},
		{"a host in capitals", "http://h/p", `at(6, "H")`, path},
		{"a host with a percent-encoded letter", "http://h/p", `at(6, "%41")`, path},
		{"an IPv4 host not as it is written", "http://h/p", `at(6, "0x7f.1")`, path},
		{"a host with a /", "http://h/p", `at(6, "h/x")`, path},
		{"a host that is not ASCII", "http://h/p", `at(6, "é")`, path},
		{"a protocol in capitals", "http://h/p", `at(3, "HTTP:")`, path},
		{"a protocol without :", "http://h/p", `at(3, "http")`, path},
		{"a path with a ?", "http://h/p", `at(8, "/a?b")`, `(u) => { u.hash = "y" }`},
		{"a search without ?", "http://h/p?q", `at(9, "q")`, path},
		{"a part that is missing", "http://h/p", `at(5, undefined)`, path},
		{"a part that is an object", "http://h/p", `at(5, {toString() { return "" }})`, path},
		{"parts longer in all than a URL", "http://h/p", `(args) => { args[8] = "/" + a; args[10] = "#" + b; return args; }`, `(u) => { u.search = "q" }`},
		{"a part that is not a setter's", "http://h/p", `(args) => { args[0] = 99; return args; }`, path},
		{"a part number that is not a number", "http://h/p", `(args) => { args[0] = "7"; return args; }`, path},
	} {
		js := `attempt(` + quote(c.href) + `, ` + c.forge + `, ` + c.set + `)`
		if got := evalString(t, vm, js); got != "TypeError unchanged" {
			t.Errorf("%s: %s", c.name, got)
		}
	}
	// The wrapper hands set what url.js gave it, and the setters work.
	if got := evalString(t, vm, `attempt("http://h/p", (args) => args, (u) => { u.pathname = "/x" })`); got != "no error changed to http://h/x" {
		t.Errorf("parts as url.js hands them: %s", got)
	}
	// Each part may be as long as a URL, but not all of them together.
	if got := evalString(t, vm, `attempt("http://h/p", (args) => args, (u) => { u.pathname = "/" + a; u.hash = b.slice(0, C / 2 - 20) })`); !strings.HasPrefix(got, "no error changed") {
		t.Errorf("a URL of two long parts: %s", got)
	}
}

func quote(s string) string { return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"` }
