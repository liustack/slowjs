// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package slowjs // import "github.com/liustack/slowjs"

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Every way of listing the own keys of an object, which lib/ownkeys.go
// takes over for ordinary objects (see fixOwnKeys), gives what the engine
// gave before it. testdata/own_keys.golden is that output. Node gives the
// same but for what the engine did differently before: functions have no
// own arguments and caller, the stack of an error comes after its message
// as a data property, for in over a proxy asks the traps in another order,
// a spread whose getter deletes a key it has not copied yet still copies
// that key, and the spread of a string primitive copies nothing.
func TestOwnKeys(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("testdata", "own_keys.js"))
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join("testdata", "own_keys.golden"))
	if err != nil {
		t.Fatal(err)
	}
	m := newTestVM(t)
	got, err := m.Eval(string(src), EvalGlobal)
	if err != nil {
		t.Fatal(err)
	}
	gl, wl := strings.Split(fmt.Sprint(got), "\n"), strings.Split(strings.TrimSuffix(string(want), "\n"), "\n")
	for i := range max(len(gl), len(wl)) {
		var g, w string
		if i < len(gl) {
			g = gl[i]
		}
		if i < len(wl) {
			w = wl[i]
		}
		if g != w {
			t.Errorf("line %d:\ngot  %s\nwant %s", i+1, g, w)
		}
	}
}

// The keys of a module namespace are bindings of the module. Listing them
// with their values, or only the enumerable ones, throws for a binding not
// yet initialized, and the names alone do not: lib/ownkeys.go leaves the
// first to the engine.
func TestOwnKeysModuleNamespace(t *testing.T) {
	m := newTestVM(t)
	m.SetModuleLoader(func(m *VM, name string) (string, error) {
		if name != "self" {
			return "", fmt.Errorf("no module %s", name)
		}
		return `
			import * as me from "self"
			const r = (f) => { try { return JSON.stringify(f()) } catch (e) { return e.name } }
			const forIn = () => { const ks = []; for (const k in me) ks.push(k); return ks }
			globalThis.early = [
				r(() => Object.getOwnPropertyNames(me)), r(() => Reflect.ownKeys(me).map(String)),
				r(() => Object.keys(me)), r(() => Object.values(me)), r(() => Object.entries(me)),
				r(forIn), r(() => ({ ...me })), r(() => Object.assign({}, me)),
			].join(' ')
			export let late = 2
			export const b = 1
			export function f() {}
			globalThis.after = [
				r(() => Object.getOwnPropertyNames(me)), r(() => Object.keys(me)), r(() => Object.values(me).map(String)),
				r(() => Object.entries(me).map(String)), r(forIn), r(() => Object.keys({ ...me })), r(() => Object.keys(Object.assign({}, me))),
			].join(' ')
		`, nil
	}, func(m *VM, base, name string) (string, error) { return name, nil })
	if _, err := m.Eval(`import "self"`, EvalModule); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ name, want string }{
		{"early", `["b","f","late"] ["b","f","late","Symbol(Symbol.toStringTag)"] ReferenceError ReferenceError ReferenceError ReferenceError ReferenceError ReferenceError`},
		{"after", `["b","f","late"] ["b","f","late"] ["1","function f() {}","2"] ["b,1","f,function f() {}","late,2"] ["b","f","late"] ["b","f","late"] ["b","f","late"]`},
	} {
		got, err := m.Eval(c.name, EvalGlobal)
		if err != nil {
			t.Fatal(err)
		}
		if fmt.Sprint(got) != c.want {
			t.Errorf("%s:\ngot  %s\nwant %s", c.name, got, c.want)
		}
	}
}
