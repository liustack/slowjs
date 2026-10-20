// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package quickjs // import "github.com/liustack/quickjs"

import (
	"fmt"
	"testing"
)

// uniqueKeys builds and drops objects whose shapes no later object has,
// each holding a key no other object has.
const uniqueKeys = `
function unique(round) {
	for (let i = 0; i < 3000; i++) {
		const o = {id: i}
		o["u" + round + "_" + i] = i
	}
}
`

// A new VM keeps the shapes of plain objects, at most DefaultShapeCache of
// them: shapes no later object reuses do not build up, and after
// SetShapeCache(0) the memory they took is free again.
func TestShapeCache(t *testing.T) {
	m, err := NewVM()
	if err != nil {
		t.Fatal(err)
	}

	defer m.Close()

	if _, err := m.Eval(uniqueKeys, EvalGlobal); err != nil {
		t.Fatal(err)
	}

	var settled int64
	for n := range 12 {
		if _, err := m.Call("unique", n); err != nil {
			t.Fatal(err)
		}

		used := int64(vmMemory(m).Fmalloc_size)
		switch {
		case n == 4:
			settled = used
		case n > 4 && used > settled:
			t.Fatalf("round %d: %d bytes after GC, %d after round 4", n, used, settled)
		}
	}

	shapes := vmMemory(m).Fshape_count
	if err := m.SetShapeCache(0); err != nil {
		t.Fatal(err)
	}
	if got := vmMemory(m).Fshape_count; shapes-got < DefaultShapeCache {
		t.Errorf("%d shapes with the cache, %d without, want %d fewer", shapes, got, DefaultShapeCache)
	}
	if err := m.SetShapeCache(-1); err == nil {
		t.Error("SetShapeCache(-1) took a cache of -1 shapes")
	}
	if err := m.SetShapeCache(16); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Call("unique", 99); err != nil {
		t.Fatal(err)
	}
}

// fillShapes leaves the ring full of shapes of 32 keys: each object dies
// with a shape no other object has.
const fillShapes = `
function fill(n) {
	for (let r = 0; r < n; r++) {
		const o = {}
		for (let k = 0; k < 32; k++) o["r" + r + "_key_" + k] = k
	}
}
function big(n) { const a = []; for (let i = 0; i < n; i++) a.push({v: i}); return a.length }
`

// Kept shapes count against the memory limit of the VM, as the rest of
// its memory does, and they hold the atoms of their keys: with the ring
// full of shapes of 32 keys no other object has they take more than 4 MiB,
// and an allocation that fits with the cache off fails with it on.
// SetShapeCache(0) lets the shapes and their atoms go, but the engine's
// table of atoms keeps the size it grew to.
func TestShapeCacheMemoryLimit(t *testing.T) {
	vm := func(n int) *VM {
		m, err := NewVM()
		if err != nil {
			t.Fatal(err)
		}
		if err := m.SetShapeCache(n); err != nil {
			t.Fatal(err)
		}
		if _, err := m.Eval(fillShapes, EvalGlobal); err != nil {
			t.Fatal(err)
		}
		if _, err := m.Call("fill", DefaultShapeCache); err != nil {
			t.Fatal(err)
		}
		return m
	}
	off := vm(0)
	defer off.Close()
	on := vm(DefaultShapeCache)
	defer on.Close()
	offUse, onUse := vmMemory(off), vmMemory(on)
	if onUse.Fmalloc_size-offUse.Fmalloc_size < 4<<20 {
		t.Fatalf("with the ring full of shapes of 32 keys: %d bytes, %d without the cache", onUse.Fmalloc_size, offUse.Fmalloc_size)
	}

	// The smallest limit, in steps of 64 KiB, at which big fits with the
	// cache off.
	fits := func(m *VM, limit uintptr) bool {
		vmMemory(m)
		m.SetMemoryLimit(limit)
		defer m.SetMemoryLimit(^uintptr(0))
		_, err := m.Call("big", 20000)
		return err == nil
	}
	var limit uintptr
	for l := uintptr(offUse.Fmalloc_size); l < uintptr(offUse.Fmalloc_size)+8<<20; l += 64 << 10 {
		if fits(off, l) {
			limit = l
			break
		}
	}
	if limit == 0 {
		t.Fatal("big does not fit in 8 MiB more")
	}
	if fits(on, limit) {
		t.Errorf("big fits in %d bytes with the cache on", limit)
	}

	if err := on.SetShapeCache(0); err != nil {
		t.Fatal(err)
	}
	got := vmMemory(on)
	if got.Fshape_count != offUse.Fshape_count || got.Fatom_count != offUse.Fatom_count {
		t.Errorf("after SetShapeCache(0): %d shapes and %d atoms, %d and %d without the cache", got.Fshape_count, got.Fatom_count, offUse.Fshape_count, offUse.Fatom_count)
	}
	if freed, kept := onUse.Fmalloc_size-got.Fmalloc_size, onUse.Fmalloc_size-offUse.Fmalloc_size; freed < kept/2 {
		t.Errorf("SetShapeCache(0) gave back %d of the %d bytes the cache took", freed, kept)
	}
}

// shapeWork builds objects in many ways from a seed and dumps their keys and
// values: literals, keys one at a time, keys no other object has, symbols,
// deletes, prototypes changed, getters, frozen objects, cycles, objects
// kept alive and objects of a class.
const shapeWork = `
function rnd(s) { return () => (s = (s * 1103515245 + 12345) & 0x7fffffff) }
function dump(o) { return Object.keys(o).join() + "=" + Object.values(o).map(v => typeof v == "object" ? "o" : String(v)).join() }
function mixed(seed) {
	const r = rnd(seed), out = [], keep = []
	for (let i = 0; i < 1000; i++) {
		const n = r() % 40, o = {}
		for (let k = 0; k < n; k++) o["k" + (r() % 12)] = k
		switch (r() % 12) {
		case 0: delete o["k" + (r() % 12)]; break
		case 1: Object.setPrototypeOf(o, null); break
		case 2: Object.defineProperty(o, "g", {get() { return 7 }, enumerable: true}); break
		case 3: Object.freeze(o); break
		case 4: keep.push(o); if (keep.length > 50) keep.shift(); break
		case 5: out.push(Object.keys({...o, z: 1}).length); break
		case 6: { const a = {x: o}; o.back = a; break }
		case 7: o.k0 = {k1: 1, k2: 2}; break
		case 8: { const j = JSON.parse(JSON.stringify(o)); j.extra = 1; out.push(Object.keys(j).join()); break }
		case 9: Object.assign(o, {k3: 3, k4: 4}); break
		case 10: { const s = Symbol("s" + i); o[s] = i; out.push(o[s]); break }
		case 11: { class C { a = 1; b = 2 }; const c = new C(); c.d = 1; out.push(Object.keys(c).join()); break }
		}
		out.push(dump(o))
	}
	for (const o of keep) out.push(dump(o))
	return out.join("|")
}
function oneByOne(seed) {
	const r = rnd(seed), out = [], keep = []
	for (let i = 0; i < 800; i++) {
		const o = {}, n = r() % 36, uniq = r() % 3 == 0
		for (let k = 0; k < n; k++) o[uniq ? "u" + i + "_" + k : "c" + k] = i + k
		if (r() % 4 == 0) keep.push(o)
		out.push(dump(o))
	}
	for (const o of keep) out.push(dump(o))
	return out.join("|")
}
function interleaved(seed) {
	const r = rnd(seed), out = [], keep = []
	for (let i = 0; i < 150; i++) {
		const m = 2 + r() % 5, objs = Array.from({length: m}, () => ({})), n = r() % 40
		for (let k = 0; k < n; k++)
			for (let j = 0; j < m; j++) {
				objs[j][j % 2 == 0 ? "s" + k : "t" + ((k * (j + 1)) % 50)] = k * 10 + j
				if (r() % 17 == 0) delete objs[j]["s" + (r() % (k + 1))]
			}
		for (const o of objs) { out.push(dump(o)); if (r() % 3 == 0) keep.push(o) }
	}
	for (const o of keep) out.push(dump(o))
	return out.join("|")
}
function delAdd(seed) {
	const r = rnd(seed), out = [], keep = []
	for (let i = 0; i < 800; i++) {
		const o = {}, n = 1 + r() % 30
		for (let k = 0; k < n; k++) o["d" + k] = k
		const del = "d" + (r() % n)
		delete o[del]
		for (let k = 0; k < r() % 10; k++) o["e" + k] = -k
		if (r() % 2) o[del] = "back"
		const o2 = {}
		for (let k = 0; k < n; k++) o2["d" + k] = k
		if (r() % 5 == 0) keep.push(o, o2)
		out.push(dump(o), dump(o2))
	}
	for (const o of keep) out.push(dump(o))
	return out.join("|")
}
`

// Objects built in many ways read the same with the cache on, at any size,
// as with it off, across GCs.
func TestShapeCacheSameResults(t *testing.T) {
	run := func(n int, fn string, seed int) string {
		m, err := NewVM()
		if err != nil {
			t.Fatal(err)
		}
		defer m.Close()
		if err := m.SetShapeCache(n); err != nil {
			t.Fatal(err)
		}
		if _, err := m.Eval(shapeWork, EvalGlobal); err != nil {
			t.Fatal(err)
		}
		var s string
		for round := range 2 {
			v, err := m.Call(fn, seed+round)
			if err != nil {
				t.Fatal(err)
			}
			s += fmt.Sprint(v)
			vmMemory(m)
		}
		return s
	}
	for _, fn := range []string{"mixed", "oneByOne", "interleaved", "delAdd"} {
		for seed := 1; seed <= 3; seed++ {
			want := run(0, fn, seed)
			for _, n := range []int{3, DefaultShapeCache} {
				if got := run(n, fn, seed); got != want {
					t.Errorf("%s, seed %d, cache of %d: the objects read otherwise than without the cache", fn, seed, n)
				}
			}
		}
	}
}
