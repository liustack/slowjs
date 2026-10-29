// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package slowjs // import "github.com/liustack/slowjs"

// How deep calls nest and what MemorySize counts. A kill is in
// killed_test.go.

import (
	"testing"
)

// SetMaxStackSize counts calls: each call into javascript takes one level, a getter or a Proxy trap
// as a plain call, and a call through a built-in such as Array.prototype.map
// takes two. The default is 5000 levels, at most 10000, and 0 is the
// default again.
func TestCallDepthInLevels(t *testing.T) {
	cases := []struct{ name, js string }{
		{"call", `function f() { n++; f() } f()`},
		{"getter", `const o = { get x() { n++; return this.x } }; o.x`},
		{"map", `function f() { n++; return [1].map(f) } f()`},
	}
	for _, c := range []struct {
		limit uintptr
		want  [3]int
	}{
		{0, [3]int{4998, 4998, 2499}},
		{100, [3]int{98, 98, 49}},
		{10000, [3]int{9998, 9998, 4999}},
		{20000, [3]int{9998, 9998, 4999}},
	} {
		for i, d := range cases {
			m := newTestVM(t)
			m.SetMaxStackSize(c.limit)
			v, err := m.Eval(`let n = 0; try { `+d.js+` } catch (e) { if (!(e instanceof RangeError)) throw e }; n`, EvalGlobal)
			if err != nil {
				t.Fatalf("%s at %d: %v", d.name, c.limit, err)
			}
			if v != c.want[i] {
				t.Errorf("%s at %d levels: depth %v, want %d", d.name, c.limit, v, c.want[i])
			}
		}
	}
	m := newTestVM(t)
	m.SetMaxStackSize(100)
	m.SetMaxStackSize(0)
	if v, err := m.Eval(`let n = 0; try { function f() { n++; f() } f() } catch (e) {}; n`, EvalGlobal); err != nil || v != 4998 {
		t.Fatalf("depth after SetMaxStackSize(0): %v, %v, want 4998", v, err)
	}
}

// MemorySize counts what the allocator holds for the blocks in use. It counts
// less than the ccgo engine did (libc's malloc) for the same objects.
// The sizes of that engine are those of linux/amd64 after a full collection.
func TestMemorySizeBelowPrevious(t *testing.T) {
	for _, c := range []struct {
		js       string
		previous uintptr
	}{
		{``, 244064},
		{`globalThis.a = Array.from({length: 1e4}, (_, i) => ({i, s: 'x' + i}))`, 1967208},
		{`globalThis.a = Array.from({length: 1e4}, (_, i) => 'some string ' + i)`, 912480},
		{`globalThis.a = Array.from({length: 1e3}, (_, i) => ({i, a: [i, i + 1], f() { return i }}))`, 711896},
	} {
		m := newTestVM(t)
		if _, err := m.Eval(c.js, EvalGlobal); err != nil {
			t.Fatal(err)
		}
		m.RunGC()
		if got := m.MemorySize(); got > c.previous*95/100 {
			t.Errorf("%q: MemorySize %d, want below 0.95 of the %d of the previous engine", c.js, got, c.previous)
		}
	}
}
