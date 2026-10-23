// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package libquickjs

import (
	"fmt"
	"testing"
	"unsafe"
)

// youngLists checks the GC objects of v outside a collection: each is on
// gc_obj_list or the young list, with a count above 0 and no mark, which
// js_weakref_free and the collectors rely on. It returns how many there are on
// each list.
func (v *shapeVM) youngLists() (old, young int) {
	v.t.Helper()
	for _, head := range []uintptr{v.rt + unsafe.Offsetof(TJSRuntime{}.Fgc_obj_list), youngHead(v.rt)} {
		n := 0
		for el := (*Tlist_head)(unsafe.Pointer(head)).Fnext; el != head; el = (*Tlist_head)(unsafe.Pointer(el)).Fnext {
			if c := *youngCount(v.tls, el); c <= 0 {
				v.t.Fatalf("a GC object on a list has a count of %d", c)
			}
			if youngMarked(v.tls, el) {
				v.t.Fatal("a GC object on a list has its mark set")
			}
			n++
		}
		if head == youngHead(v.rt) {
			young = n
		} else {
			old = n
		}
	}
	return old, young
}

// New GC objects are young, a young collection makes what it keeps old, and
// a full collection and JS_ComputeMemoryUsage see the young objects.
func TestYoungGCLists(t *testing.T) {
	v := newShapeVM(t, 0)
	defer v.close()
	XJS_SetGCThreshold(v.tls, v.rt, ^Tsize_t(0))
	XJS_RunGC(v.tls, v.rt)
	if _, young := v.youngLists(); young != 0 {
		t.Fatalf("%d young objects after a full collection", young)
	}
	v.run(`globalThis.kept = Array.from({length: 100}, (_, i) => ({i})); void 0`)
	old, young := v.youngLists()
	if young < 100 {
		t.Fatalf("%d young objects after making 100", young)
	}
	XJS_RunYoungGC(v.tls, v.rt)
	if old2, young2 := v.youngLists(); young2 != 0 || old2 < old+100 {
		t.Fatalf("after a young collection: %d old, %d young, before %d old, %d young", old2, young2, old, young)
	}
	v.run(`kept.push({}); void 0`)
	v.shapes() // JS_ComputeMemoryUsage
	if _, young := v.youngLists(); young != 0 {
		t.Fatalf("%d young objects after JS_ComputeMemoryUsage", young)
	}
}

// After a young collection, no object that only the garbage held is left on
// a list with a count of 0, where the weak pass of the next collection would
// free it while still on the list.
func TestYoungGCLeavesNoZeroCount(t *testing.T) {
	for _, observe := range []string{``, `wm.set(old, 1);`, `wr = new WeakRef(old);`, `fr.register(old, 1);`, `old.f = async function () {};`, `old.g = (function* () { yield 1 })(); old.g.next();`} {
		t.Run(observe, func(t *testing.T) {
			v := newShapeVM(t, 0)
			defer v.close()
			XJS_SetGCThreshold(v.tls, v.rt, ^Tsize_t(0))
			v.run(`globalThis.wm = new WeakMap(); globalThis.wr = null; globalThis.fr = new FinalizationRegistry(() => {});
				globalThis.old = {inner: {deep: [1, 2, 3]}}; ` + observe + ` void 0`)
			XJS_RunGC(v.tls, v.rt)
			v.run(`(() => { const a = {old}, b = {a}; a.b = b; old = null })(); void 0`)
			XJS_RunYoungGC(v.tls, v.rt)
			v.youngLists()
			XJS_RunYoungGC(v.tls, v.rt)
			XJS_RunGC(v.tls, v.rt)
			v.youngLists()
			if got := v.run(`String(wr === null || wr.deref() === undefined)`); got != "true" {
				t.Fatal(got)
			}
		})
	}
}

// Each case of TestEphemeron with young collections before and between the
// full ones: a young collection keeps every reachable value, and one full
// collection afterwards still frees the payload.
func TestYoungGCEphemeron(t *testing.T) {
	for _, tc := range ephemeronCases {
		t.Run(tc.name, func(t *testing.T) {
			v := newShapeVM(t, 0)
			defer v.close()
			XJS_SetGCThreshold(v.tls, v.rt, ^Tsize_t(0))
			v.run(`globalThis.payload='x'.repeat(2097152); ` + tc.setup + ` payload=null; void 0`)
			XJS_RunYoungGC(v.tls, v.rt)
			if got := v.run("String(" + tc.live + ")"); got != "true" {
				t.Fatalf("reachable value lost by the young collection: %s", got)
			}
			XJS_RunYoungGC(v.tls, v.rt)
			XJS_RunGC(v.tls, v.rt)
			if got := v.run("String(" + tc.live + ")"); got != "true" {
				t.Fatalf("reachable value lost: %s", got)
			}
			if tc.drop != "" {
				v.run(tc.drop + ` void 0`)
				XJS_RunYoungGC(v.tls, v.rt)
				XJS_RunGC(v.tls, v.rt)
			}
			n := int(unsafe.Sizeof(TJSMemoryUsage{}))
			p := v.tls.Alloc(n)
			defer v.tls.Free(n)
			XJS_ComputeMemoryUsage(v.tls, v.rt, p)
			u := (*TJSMemoryUsage)(unsafe.Pointer(p))
			if u.Fstr_size > 128<<10 {
				t.Errorf("one full collection after young ones kept %d string bytes", u.Fstr_size)
			}
		})
	}
}

// The random graphs of TestEphemeronMixedGraph: a young collection keeps
// every node the oracle says is reachable, and a full one afterwards keeps
// exactly those.
func TestYoungGCMixedGraph(t *testing.T) {
	for seed := uint64(1); seed <= 20; seed++ {
		t.Run(fmt.Sprint(seed), func(t *testing.T) {
			js, want := ephemeronGraph(seed)
			v := newShapeVM(t, 0)
			defer v.close()
			XJS_SetGCThreshold(v.tls, v.rt, ^Tsize_t(0))
			if got := v.run(js); got != "<undefined>" {
				t.Fatal(got)
			}
			XJS_RunYoungGC(v.tls, v.rt)
			got := v.run(`observers.map(r=>r.deref()===undefined?'0':'1').join('')`)
			for i := range want {
				if want[i] == '1' && got[i] != '1' {
					t.Fatalf("the young collection freed node %d: got %s want at least %s", i, got, want)
				}
			}
			XJS_RunGC(v.tls, v.rt)
			if got := v.run(`observers.map(r=>r.deref()===undefined?'0':'1').join('')`); got != want {
				t.Fatalf("reachability got %s want %s", got, want)
			}
		})
	}
}

// WeakRefs and FinalizationRegistry callbacks see what a young collection
// frees, and not what it keeps.
func TestYoungGCWeakObservers(t *testing.T) {
	v := newShapeVM(t, 0)
	defer v.close()
	XJS_SetGCThreshold(v.tls, v.rt, ^Tsize_t(0))
	v.run(`globalThis.calls=[]; globalThis.fr=new FinalizationRegistry(x=>calls.push(x)); globalThis.wm=new WeakMap(); globalThis.key={}; wm.set(key,{key}); globalThis.wr=new WeakRef(key); fr.register(key,'kept');
		globalThis.wr2 = (() => { const k = {}; k.self = k; fr.register(k, 'cycle'); return new WeakRef(k) })(); void 0`)
	XJS_RunYoungGC(v.tls, v.rt)
	if got := v.run(`String(wr.deref() === key && wr2.deref() === undefined && calls.length === 0)`); got != "true" {
		t.Fatal(got)
	}
	for XJS_IsJobPending(v.tls, v.rt) != 0 {
		if XJS_ExecutePendingJob(v.tls, v.rt, 0) < 0 {
			t.Fatal("cleanup job threw")
		}
	}
	if got := v.run(`calls.join()`); got != "cycle" {
		t.Fatal(got)
	}
	// The key's weak map value points back to it: the young collection
	// keeps it, the full one frees it.
	v.run(`key=null; void 0`)
	XJS_RunYoungGC(v.tls, v.rt)
	if got := v.run(`String(wr.deref() !== undefined)`); got != "true" {
		t.Fatal(got)
	}
	XJS_RunGC(v.tls, v.rt)
	if got := v.run(`String(wr.deref() === undefined)`); got != "true" {
		t.Fatal(got)
	}
	for XJS_IsJobPending(v.tls, v.rt) != 0 {
		if XJS_ExecutePendingJob(v.tls, v.rt, 0) < 0 {
			t.Fatal("cleanup job threw")
		}
	}
	if got := v.run(`calls.join()`); got != "cycle,kept" {
		t.Fatal(got)
	}
}
