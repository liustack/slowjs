// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package quickjs

import (
	"fmt"
	"testing"
)

// youngSetup builds random graphs of objects, closures, arrays, maps and sets
// held from old objects, an old closure variable and an old WeakMap whose
// values point back to their keys, and checks what the roots reach.
const youngSetup = `
globalThis.R = {arr: [], map: new Map(), obj: {}, wm: new WeakMap(), set: new Set()};
{ let hidden = null; globalThis.setHidden = v => { hidden = v }; globalThis.getHidden = () => hidden; }
globalThis.rnd = seed => () => (seed = (seed * 1103515245 + 12345) & 0x7fffffff) / 0x7fffffff;
globalThis.round = seed => {
	const r = rnd(seed), N = 300, nodes = [];
	for (let i = 0; i < N; i++) {
		const kind = Math.floor(r() * 4);
		let n;
		if (kind === 0) n = {id: i, out: []};
		else if (kind === 1) { const out = []; n = {id: i, out, f: () => out}; }
		else if (kind === 2) { n = [i]; n.id = i; n.out = []; }
		else n = {id: i, out: [], m: new Map()};
		n.pay = 'p' + i + '-' + seed;
		nodes.push(n);
	}
	for (const n of nodes) {
		const k = Math.floor(r() * 3);
		for (let j = 0; j < k; j++) {
			const t = nodes[Math.floor(r() * N)];
			n.out.push(t);
			if (n.m) n.m.set(j, t);
		}
	}
	R.arr.length = 0; R.map.clear(); R.set.clear(); setHidden(null);
	for (const k of Object.keys(R.obj)) delete R.obj[k];
	for (let j = 0; j < 6; j++) {
		const n = nodes[Math.floor(r() * N)], w = Math.floor(r() * 5);
		if (w === 0) R.arr.push(n); else if (w === 1) R.map.set('k' + j, n);
		else if (w === 2) R.obj['k' + j] = n; else if (w === 3) R.set.add(n); else setHidden(n);
		if (j === 0) R.wm.set(n, {back: n, pay: 'wm' + seed});
	}
	return reach();
};
globalThis.reach = () => {
	const seen = new Set(), stack = [...R.arr, ...R.map.values(), ...Object.values(R.obj), ...R.set];
	if (getHidden()) stack.push(getHidden());
	while (stack.length) {
		const n = stack.pop();
		if (seen.has(n)) continue;
		seen.add(n);
		for (const t of n.out) stack.push(t);
		if (n.f && n.f() !== n.out) throw new Error('closure');
		if (n.m) for (const t of n.m.values()) stack.push(t);
	}
	const ids = [];
	for (const n of seen) {
		ids.push(n.id + ':' + n.pay);
		const v = R.wm.get(n);
		if (v) { if (v.back !== n) throw new Error('weak map value'); ids.push(v.pay); }
	}
	return ids.sort().join(',');
};
globalThis.garbage = n => { for (let i = 0; i < n; i++) { const a = {}, b = {a}; a.b = b; } };
`

func youngEval(t *testing.T, m *VM, js string) string {
	t.Helper()
	v, err := m.Eval(js, EvalGlobal)
	if err != nil {
		t.Fatalf("%s: %v", js, err)
	}
	s, ok := v.(string)
	if !ok {
		t.Fatalf("%s: %T %v", js, v, v)
	}
	return s
}

func youngVM(t *testing.T, setup string) *VM {
	t.Helper()
	m, err := NewVM()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { m.Close() })
	m.SetGCThreshold(^uintptr(0))
	if _, err := m.Eval(setup, EvalGlobal); err != nil {
		t.Fatal(err)
	}
	m.RunGC()
	return m
}

// youngRounds runs 400 rounds of random graphs, with a young GC after each
// round when young is set, a full GC every 7 rounds, and checks after each GC
// that what the roots reach is all there. The engine collects by itself past
// threshold. It returns what the VM holds after the roots are cleared and a
// full GC runs, and the bytes the young GCs freed.
func youngRounds(t *testing.T, young bool, threshold uintptr) (held, freed uintptr) {
	m := youngVM(t, youngSetup)
	m.SetGCThreshold(threshold)
	clear := "R.arr.length = 0; R.map.clear(); R.set.clear(); setHidden(null); for (const k of Object.keys(R.obj)) delete R.obj[k]; ''"
	youngEval(t, m, "round(1)")
	youngEval(t, m, clear)
	m.RunGC()
	for seed := 2; seed < 400; seed++ {
		want := youngEval(t, m, fmt.Sprintf("round(%d)", seed))
		if young {
			before := m.MemorySize()
			m.RunYoungGC()
			if after := m.MemorySize(); after < before {
				freed += before - after
			}
			if got := youngEval(t, m, "reach()"); got != want {
				t.Fatalf("seed %d: after the young GC\n got %.200s\nwant %.200s", seed, got, want)
			}
		}
		if seed%7 == 0 {
			m.RunGC()
			if got := youngEval(t, m, "reach()"); got != want {
				t.Fatalf("seed %d: after the full GC\n got %.200s\nwant %.200s", seed, got, want)
			}
		}
	}
	youngEval(t, m, clear)
	m.RunGC()
	return m.MemorySize(), freed
}

// A young collection never frees what old objects, closures, maps, sets and
// weak map values still reach, a full collection afterwards finds every count
// right, also when the engine collects by itself in between, and in the end
// the full collection frees as much as it does without young collections.
func TestYoungGCKeepsReachable(t *testing.T) {
	base, _ := youngRounds(t, false, ^uintptr(0))
	for _, tc := range []struct {
		name      string
		threshold uintptr
	}{{"no-engine-gc", ^uintptr(0)}, {"engine-gc", 4 << 20}} {
		t.Run(tc.name, func(t *testing.T) {
			held, freed := youngRounds(t, true, tc.threshold)
			t.Logf("held after the last full GC: %d bytes without young GCs, %d with, the young GCs freed %d", base, held, freed)
			if freed == 0 {
				t.Fatal("the young GCs freed nothing")
			}
			if held > base+base/50 {
				t.Fatalf("with young GCs the VM holds %d bytes after a full GC, %d without", held, base)
			}
		})
	}
}

// A young collection frees young cycles as the full one does, and keeps a
// suspended generator whose frame holds a cycle.
func TestYoungGCFreesCycles(t *testing.T) {
	m := youngVM(t, youngSetup)
	base := m.MemorySize()
	if _, err := m.Eval("R.g = (function*() { const o = {}; o.o = o; yield 1; yield o.o === o })(); R.g.next(); garbage(20000)", EvalGlobal); err != nil {
		t.Fatal(err)
	}
	full := m.MemorySize()
	m.RunYoungGC()
	after := m.MemorySize()
	if after > base+(full-base)/20 {
		t.Fatalf("young GC left %d bytes of %d", after-base, full-base)
	}
	if got := youngEval(t, m, "String(R.g.next().value)"); got != "true" {
		t.Fatalf("generator: %s", got)
	}
}

// An old object that only young garbage holds is freed by the young
// collection that frees the garbage, whether or not a weak map, a WeakRef or
// a FinalizationRegistry observes it, and its weak references go with it. One
// that is also in a cycle of old objects stays until the next full
// collection.
func TestYoungGCFreesWhatOnlyGarbageHeld(t *testing.T) {
	for _, tc := range []struct {
		name, observe, check string
		young                bool // the young collection frees the old object
	}{
		{"plain", ``, `true`, true},
		{"weak-map-key", `wm.set(old, {pay: 'v'.repeat(1 << 20)});`, `true`, true},
		{"weak-ref", `globalThis.wr = new WeakRef(old);`, `wr.deref() === undefined`, true},
		{"registry", `fr.register(old, 'gone');`, `calls.length === 0`, true},
		{"old-cycle", `old.self = old; globalThis.wr = new WeakRef(old);`, `wr.deref() !== undefined`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := youngVM(t, `globalThis.wm = new WeakMap(); globalThis.calls = []; globalThis.fr = new FinalizationRegistry(x => calls.push(x));
				globalThis.old = {pay: 'x'.repeat(2 << 20)}; `+tc.observe+` void 0`)
			base := m.MemorySize()
			// A young cycle that alone holds the old object.
			if _, err := m.Eval(`(() => { const a = {old}, b = {a}; a.b = b; old = null })(); void 0`, EvalGlobal); err != nil {
				t.Fatal(err)
			}
			m.RunYoungGC()
			after := m.MemorySize()
			t.Logf("before %d, after the young GC %d", base, after)
			if freed := after+2<<20 <= base; freed != tc.young {
				t.Fatalf("the young GC freed the old object: %v, want %v (%d bytes before, %d after)", freed, tc.young, base, after)
			}
			if got := youngEval(t, m, "String("+tc.check+")"); got != "true" {
				t.Fatalf("%s: %s", tc.check, got)
			}
			if _, err := m.ExecutePendingJobs(); err != nil {
				t.Fatal(err)
			}
			if tc.name == "registry" {
				if got := youngEval(t, m, "calls.join()"); got != "gone" {
					t.Fatalf("registry callbacks: %q", got)
				}
			}
			// The weak pass of a second young collection, and the full
			// collection, find no object freed twice.
			m.RunYoungGC()
			m.RunGC()
			full := m.MemorySize()
			if full+2<<20 > base {
				t.Fatalf("held %d after the full GC, %d before", full, base)
			}
			if tc.young && full > after+64<<10 {
				t.Fatalf("held %d after the full GC, %d after the young GC", full, after)
			}
		})
	}
}

// Young collections keep the weak map entries a full collection must judge:
// each case of TestRunGCEphemeron is freed by one full collection after a
// young one, and the young one alone frees an entry only when its key is
// garbage on its own.
func TestYoungGCEphemeron(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		young      bool // the young collection alone frees the payload
	}{
		{"no-back-reference", `let key = {}; wm.set(key, {payload});`, true},
		{"back-reference", `let key = {}; wm.set(key, {key, payload});`, false},
		{"key-cycle", `let key = {}; key.self = key; wm.set(key, {payload});`, true},
		{"chain", `let a = {}, b = {}, c = {}; a.self = a; wm.set(a, b); wm.set(b, c); wm.set(c, {a, payload});`, false},
		{"dead-map", `let key = {}; globalThis.kept = key; let map = new WeakMap(); map.set(key, {map, payload});`, true},
		{"young-map", `let m = new WeakMap(); globalThis.held = m; let key = {}; m.set(key, {key, payload});`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := youngVM(t, `globalThis.wm = new WeakMap(); void 0`)
			base := m.MemorySize()
			if _, err := m.Eval(`(() => { let payload = 'x'.repeat(2*1024*1024); `+tc.body+` })(); void 0`, EvalGlobal); err != nil {
				t.Fatal(err)
			}
			peak := m.MemorySize()
			m.RunYoungGC()
			young := m.MemorySize()
			m.RunYoungGC()
			m.RunGC()
			full := m.MemorySize()
			t.Logf("base=%d peak=%d after_young=%d after_full=%d", base, peak, young, full)
			if peak < base+2<<20 {
				t.Fatal("payload was not allocated")
			}
			if freed := young < base+1<<20; freed != tc.young {
				t.Errorf("the young collection freed the payload: %v, want %v", freed, tc.young)
			}
			if full > base+128<<10 {
				t.Errorf("one full collection after young ones retained %d bytes", full-base)
			}
		})
	}
}
