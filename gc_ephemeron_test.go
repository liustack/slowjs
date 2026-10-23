// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package slowjs

import "testing"

func TestRunGCEphemeron(t *testing.T) {
	for _, tc := range []struct{ name, body string }{
		{"no-back-reference", `let key = {}; wm.set(key, {payload});`},
		{"back-reference", `let key = {}; wm.set(key, {key, payload});`},
		{"key-cycle", `let key = {}; key.self = key; wm.set(key, {payload});`},
		{"chain", `let a = {}, b = {}, c = {}; a.self = a; wm.set(a, b); wm.set(b, c); wm.set(c, {a, payload});`},
		{"dead-map", `let key = {}; globalThis.kept = key; let map = new WeakMap(); map.set(key, {map, payload});`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := newGCVM(t)
			m.SetGCThreshold(^uintptr(0))
			if _, err := m.Eval(`globalThis.wm = new WeakMap(); void 0`, EvalGlobal); err != nil {
				t.Fatal(err)
			}
			m.RunGC()
			base := m.MemorySize()
			if _, err := m.Eval(`(() => { let payload = 'x'.repeat(2*1024*1024); `+tc.body+` })(); void 0`, EvalGlobal); err != nil {
				t.Fatal(err)
			}
			peak := m.MemorySize()
			m.RunGC()
			got := m.MemorySize()
			t.Logf("base=%d peak=%d after_one_gc=%d", base, peak, got)
			if peak < base+2<<20 {
				t.Fatal("payload was not allocated")
			}
			if got > base+128<<10 {
				t.Errorf("one full collection retained %d bytes", got-base)
			}
		})
	}
}
