// Copyright (c) 2026 The liustack/tifajs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

package tifajs

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// newMurmur2VM returns a VM whose global h is NewMurmur2Function, and which
// has the functions of engine/test/render-test/murmur2.js, emotionHash among
// them, with __tifa_murmur2__ as h itself.
func newMurmur2VM(t *testing.T) *VM {
	t.Helper()
	m := newTestVM(t)
	f, err := m.NewMurmur2Function()
	if err != nil {
		t.Fatal(err)
	}
	defer f.Free()
	g := m.GlobalObject()
	defer g.Free()
	for _, name := range []string{"h", "__tifa_murmur2__"} {
		a, err := m.NewAtom(name)
		if err != nil {
			t.Fatal(err)
		}
		if err := m.SetPropertyValue(g, a, f); err != nil {
			t.Fatal(err)
		}
	}
	src, err := os.ReadFile(filepath.Join("engine", "test", "render-test", "murmur2.js"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Eval(string(src), EvalGlobal); err != nil {
		t.Fatal(err)
	}
	return m
}

// The hash is that of @emotion/hash for every string, 8-bit and 16-bit, with
// lone surrogates, long and joined from pieces, and undefined for anything
// but a string, whatever its toString.
func TestMurmur2(t *testing.T) {
	m := newMurmur2VM(t)
	for _, c := range []struct {
		js   string
		want any
	}{
		{`sameHashes()`, "2011 0"},
		{`['', 'a', 'color:red;', 'x'.repeat(4096 * 3 + 3)].every((s) => h(s) === emotionHash(s))`, true},
		{`var a = 'ab'.repeat(5000), b = 'é😀'.repeat(3000); h(a + b) === emotionHash(a + b) && h(b + a) === emotionHash(b + a)`, true},
		{`[undefined, null, 1, true, {}, ['a'], new String('abc'), {toString: () => 'abc'}, Symbol()].every((v) => h(v) === undefined) && h() === undefined`, true},
		{`h.name + ' ' + h.length + ' ' + typeof h.prototype`, "murmur2 1 undefined"},
	} {
		got, err := m.Eval(c.js, EvalGlobal)
		if err != nil {
			t.Fatalf("%s: %v", c.js, err)
		}
		if got != c.want {
			t.Errorf("%s: %v, want %v", c.js, got, c.want)
		}
	}
}

// The eval timeout stops the hash of one long string, 8-bit, 16-bit and a
// rope, and a loop of calls.
func TestMurmur2Stops(t *testing.T) {
	for _, c := range []struct{ name, setup, run string }{
		{"8-bit", `var s = 'a'.repeat(1 << 27)`, `h(s)`},
		{"16-bit", `var s = 'Ā'.repeat(1 << 26)`, `h(s)`},
		{"rope", `var s = 'a'.repeat(1 << 26) + 'Ā'.repeat(1 << 25)`, `h(s)`},
		{"short strings", `var s = 'display:flex;'.repeat(20)`, `for (;;) h(s)`},
	} {
		t.Run(c.name, func(t *testing.T) {
			m := newMurmur2VM(t)
			if _, err := m.Eval(c.setup+`; 0`, EvalGlobal); err != nil {
				t.Fatal(err)
			}
			m.SetEvalTimeout(10 * time.Millisecond)
			start := time.Now()
			_, err := m.Eval(c.run, EvalGlobal)
			checkStopped(t, m, err, time.Since(start), stopPolls, "")
		})
	}
}
