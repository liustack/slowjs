// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package quickjs // import "github.com/liustack/quickjs"

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Keys that are atoms take atomKey in JS_ValueToAtom, the others the
// generic path: both give what Node gives, for reads, writes, in,
// hasOwnProperty, descriptors, computed keys, delete and the order of keys.
func TestAtomKeys(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("testdata", "atom_keys.js"))
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join("testdata", "atom_keys.golden"))
	if err != nil {
		t.Fatal(err)
	}
	m, err := NewVM()
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
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

// atomKeyChurn makes keys at run time, uses each through every way a key
// reaches JS_ValueToAtom once it is an atom, and drops them.
const atomKeyChurn = `
function churn(round) {
	const o = {}
	for (let i = 0; i < 2000; i++) {
		const k = "churn" + round + "_" + i
		o[k] = i
		if (o[k] !== i || !(k in o) || !o.hasOwnProperty(k) || ({[k]: 1})[k] !== 1) throw new Error(k)
		delete o[k]
	}
}
`

// An atom string used as a key gets one reference per use and gives it
// back: once the keys are dropped, the runtime has the atoms it had. The
// shape cache is off, as a kept shape holds the atoms of its keys.
func TestAtomKeysCounts(t *testing.T) {
	m, err := NewVM()
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	if err := m.SetShapeCache(0); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Eval(atomKeyChurn, EvalGlobal); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Call("churn", 0); err != nil {
		t.Fatal(err)
	}
	before := vmMemory(m).Fatom_count
	for round := 1; round <= 5; round++ {
		if _, err := m.Call("churn", round); err != nil {
			t.Fatal(err)
		}
		if got := vmMemory(m).Fatom_count; got != before {
			t.Fatalf("round %d: %d atoms, %d before", round, got, before)
		}
	}
}
