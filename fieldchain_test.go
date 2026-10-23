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

// Reads by name of strings, numbers and arrays, which get_field and
// get_field2 look for on the prototype (see fixFieldChain), and of the
// values next to them, give what Node gives: with methods and accessors
// added to the prototypes later, read-only keys, a proxy or a frozen object
// up the chain, frozen, sealed, sparse and typed arrays, arguments, String
// objects, symbols, with and eval.
func TestFieldChain(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("testdata", "field_chain.js"))
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join("testdata", "field_chain.golden"))
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

// fieldChainFails are fail() of the uncatchable cases reached through a
// read by name of a string, a number or an array: a getter on their
// prototypes, or a method there.
var fieldChainFails = []struct{ name, js string }{
	{"string getter", `Object.defineProperty(String.prototype, 'boom', { get() { fail() }, configurable: true }); globalThis.go = () => 'abc'.boom`},
	{"number getter", `Object.defineProperty(Number.prototype, 'boom', { get() { 'use strict'; fail() }, configurable: true }); globalThis.go = () => (1.5).boom`},
	{"array getter", `Object.defineProperty(Array.prototype, 'boom', { get() { fail() }, configurable: true }); globalThis.go = () => [1].boom`},
	{"string method", `String.prototype.boom = function () { fail() }; globalThis.go = () => 'abc'.boom()`},
	{"array method", `Object.prototype.boom = function () { fail() }; globalThis.go = () => [1].boom()`},
}

// An interrupt or running out of memory in a getter or a method reached
// from a string, a number or an array ends the call: page code cannot catch
// it.
func TestFieldChainUncatchable(t *testing.T) {
	for _, c := range fieldChainFails {
		for _, kind := range []errorKind{interrupted, outOfMemory} {
			t.Run(c.name+" "+kind.String(), func(t *testing.T) {
				m := newTestVM(t)
				if err := m.RegisterFunc("stop", m.Interrupt, false); err != nil {
					t.Fatal(err)
				}
				fail := `globalThis.fail = () => { stop(); for (;;); }`
				if kind == outOfMemory {
					fail = growForever
				}
				if _, err := m.Eval(`globalThis.seen = []; `+fail+`; `+c.js, EvalGlobal); err != nil {
					t.Fatal(err)
				}
				if kind == outOfMemory {
					m.SetMemoryLimit(uintptr(vmMemory(m).Fmalloc_size) + 1<<20)
				}
				err, seen := runUncatchable(t, m, `try { go() } catch (e) { seen.push('catch') } finally { seen.push('finally') }`)
				wantKind(t, err, kind)
				if seen != "" {
					t.Fatalf("page code saw it: %s", seen)
				}
				wantNoPendingException(t, m)
			})
		}
	}
}
