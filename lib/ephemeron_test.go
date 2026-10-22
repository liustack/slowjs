// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package libquickjs

import (
	"fmt"
	"strings"
	"testing"
	"unsafe"
)

func TestEphemeron(t *testing.T) {
	for _, tc := range []struct{ name, setup, live, drop string }{
		{"prepass-cascade", `globalThis.wmA=new WeakMap(); globalThis.wmB=new WeakMap(); (()=>{let a={},b={};wmA.set(a,{payload});wmB.set(b,a)})();`, `true`, ``},
		{"prepass-cascade-reverse", `globalThis.wmB=new WeakMap(); globalThis.wmA=new WeakMap(); (()=>{let a={},b={};wmA.set(a,{payload});wmB.set(b,a)})();`, `true`, ``},
		{"cycle", `globalThis.wm = new WeakMap(); (() => {let k = {}; wm.set(k, {k, payload});})();`, `true`, ``},
		{"plain-key", `globalThis.wm = new WeakMap(); (() => {let k = {}; wm.set(k, {payload});})();`, `true`, ``},
		{"no-backref", `globalThis.wm = new WeakMap(); (() => {let k = {}; k.self=k; wm.set(k, {payload});})();`, `true`, ``},
		{"chain", `globalThis.wm = new WeakMap(); (() => {let a={}, b={}, c={}; wm.set(a,b); wm.set(b,c); wm.set(c,{a,payload});})();`, `true`, ``},
		{"live-key-chain", `globalThis.wm = new WeakMap(); globalThis.key = {}; (() => {let b={}, c={}; wm.set(key,b); wm.set(b,c); wm.set(c,{key,payload});})();`, `wm.get(wm.get(wm.get(key))).payload.length === 2097152`, `key=null;`},
		{"weakset", `globalThis.ws = new WeakSet(); (() => {let k={payload}; k.self=k; ws.add(k);})();`, `true`, ``},
		{"weakset-live", `globalThis.ws = new WeakSet(); globalThis.key={payload}; key.self=key; ws.add(key);`, `ws.has(key) && key.payload.length === 2097152`, `key=null;`},
		{"symbol-cycle", `globalThis.wm = new WeakMap(); (()=>{let k=Symbol(); wm.set(k,{k,payload});})();`, `true`, ``},
		{"symbol-property-cycle", `globalThis.wm = new WeakMap(); (()=>{let k=Symbol(); wm.set(k,{[k]:payload});})();`, `true`, ``},
		{"symbol", `globalThis.wm = new WeakMap(); globalThis.key = Symbol(); wm.set(key,{payload});`, `wm.get(key).payload.length === 2097152`, `key=null;`},
		{"dead-map", `globalThis.key={}; (() => {let m=new WeakMap(); m.set(key,{m,payload});})();`, `true`, ``},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := newShapeVM(t, 0)
			defer v.close()
			XJS_SetGCThreshold(v.tls, v.rt, ^Tsize_t(0))
			v.run(`globalThis.payload='x'.repeat(2097152); ` + tc.setup + ` payload=null; void 0`)
			XJS_RunGC(v.tls, v.rt)
			if got := v.run("String(" + tc.live + ")"); got != "true" {
				t.Fatalf("reachable value lost: %s", got)
			}
			if tc.drop != "" {
				v.run(tc.drop + ` void 0`)
				XJS_RunGC(v.tls, v.rt)
			}
			n := int(unsafe.Sizeof(TJSMemoryUsage{}))
			p := v.tls.Alloc(n)
			defer v.tls.Free(n)
			XJS_ComputeMemoryUsage(v.tls, v.rt, p)
			u := (*TJSMemoryUsage)(unsafe.Pointer(p))
			if u.Fstr_size > 128<<10 {
				t.Errorf("one full collection kept %d string bytes", u.Fstr_size)
			}
		})
	}
}

func TestEphemeronWeakObservers(t *testing.T) {
	v := newShapeVM(t, 0)
	defer v.close()
	XJS_SetGCThreshold(v.tls, v.rt, ^Tsize_t(0))
	v.run(`globalThis.calls=[]; globalThis.fr=new FinalizationRegistry(x=>calls.push(x)); globalThis.wm=new WeakMap(); globalThis.key={}; wm.set(key,{key}); globalThis.wr=new WeakRef(key); fr.register(key,'done'); void 0`)
	XJS_RunGC(v.tls, v.rt)
	if got := v.run(`String(wr.deref() === key && calls.length === 0)`); got != "true" {
		t.Fatal(got)
	}
	v.run(`key=null; void 0`)
	XJS_RunGC(v.tls, v.rt)
	if got := v.run(`String(wr.deref() === undefined && calls.length === 0)`); got != "true" {
		t.Fatal("deref or synchronous callback: ", got)
	}
	for XJS_IsJobPending(v.tls, v.rt) != 0 {
		if XJS_ExecutePendingJob(v.tls, v.rt, 0) < 0 {
			t.Fatal("cleanup job threw")
		}
	}
	if got := v.run(`String(calls.length === 1 && calls[0] === 'done')`); got != "true" {
		t.Fatal(got)
	}
	XJS_RunGC(v.tls, v.rt)
	if XJS_IsJobPending(v.tls, v.rt) != 0 {
		t.Fatal("cleanup callback queued twice")
	}
}

func TestEphemeronLiveSymbols(t *testing.T) {
	for _, root := range []string{`globalThis.key=s`, `globalThis.key={[s]:true}`, `globalThis.key=Object(s)`, `globalThis.key=new Map([[s,42]])`, `globalThis.key=[s]`} {
		t.Run(root, func(t *testing.T) {
			v := newShapeVM(t, 0)
			defer v.close()
			XJS_SetGCThreshold(v.tls, v.rt, ^Tsize_t(0))
			v.run(`globalThis.wm=new WeakMap();globalThis.wr=undefined;(()=>{let s=Symbol(); ` + root + `; let second=Symbol(); wm.set(s,second); wm.set(second,{s,payload:'x'.repeat(2097152)}); wr=new WeakRef(s);})(); void 0`)
			XJS_RunGC(v.tls, v.rt)
			if got := v.run(`String(wm.get(wm.get(wr.deref())).payload.length===2097152)`); got != "true" {
				t.Fatal(got)
			}
			v.run(`key=null; void 0`)
			XJS_RunGC(v.tls, v.rt)
			if got := v.run(`String(wr.deref()===undefined)`); got != "true" {
				t.Fatal("symbol retained: ", got)
			}
		})
	}
}

func TestEphemeronMutationAfterGC(t *testing.T) {
	v := newShapeVM(t, 0)
	defer v.close()
	XJS_SetGCThreshold(v.tls, v.rt, ^Tsize_t(0))
	v.run(`globalThis.wm=new WeakMap();globalThis.keys=Array.from({length:2000},()=>({}));for(let i=0;i<keys.length;i++){wm.set(keys[i],{key:keys[i],i});if(i%2===0)keys[i]=null};void 0`)
	XJS_RunGC(v.tls, v.rt)
	if got := v.run(`String(keys.every((key,i)=>key===null || wm.get(key).i===i))`); got != "true" {
		t.Fatal(got)
	}
	if got := v.run(`String(keys.every((key,i)=>{if(key===null)return true; if(!wm.delete(key)||wm.has(key))return false;wm.set(key,i);return wm.get(key)===i}))`); got != "true" {
		t.Fatal(got)
	}
	v.run(`keys=null;void 0`)
	XJS_RunGC(v.tls, v.rt)
}

func TestEphemeronRegistryUnregister(t *testing.T) {
	v := newShapeVM(t, 0)
	defer v.close()
	XJS_SetGCThreshold(v.tls, v.rt, ^Tsize_t(0))
	v.run(`globalThis.calls=[];globalThis.fr=new FinalizationRegistry(x=>calls.push(x));globalThis.wm=new WeakMap();globalThis.token={};(()=>{let k={};wm.set(k,{k});fr.register(k,'cancelled',token)})();fr.unregister(token);void 0`)
	XJS_RunGC(v.tls, v.rt)
	if XJS_IsJobPending(v.tls, v.rt) != 0 {
		t.Fatal("unregistered target queued a job")
	}
	if got := v.run(`String(calls.length===0)`); got != "true" {
		t.Fatal(got)
	}
}

// A small independent reachability oracle covers ordering, shared keys,
// garbage maps and cycles spanning several maps. The oracle does not use
// reference counts or the collector's work list.
func TestEphemeronMixedGraph(t *testing.T) {
	for seed := uint64(1); seed <= 20; seed++ {
		t.Run(fmt.Sprint(seed), func(t *testing.T) {
			const n = 64
			type edge struct{ from, to int }
			type ephemeron struct{ owner, key, value int }
			next := seed
			rand := func(limit int) int {
				next = next*6364136223846793005 + 1442695040888963407
				return int((next >> 32) % uint64(limit))
			}
			var strong []edge
			var weak []ephemeron
			for range 40 {
				strong = append(strong, edge{rand(n), rand(n)})
			}
			for range 80 {
				weak = append(weak, ephemeron{rand(8), rand(n), rand(n)})
			}
			// WeakMap.set replaces an earlier entry with the same owner and key.
			entries := map[[2]int]int{}
			for _, e := range weak {
				entries[[2]int{e.owner, e.key}] = e.value
			}
			live := make([]bool, n)
			roots := []int{rand(n), rand(n), rand(n), rand(n)}
			for _, r := range roots {
				live[r] = true
			}
			for changed := true; changed; {
				changed = false
				mark := func(i int) {
					if !live[i] {
						live[i] = true
						changed = true
					}
				}
				for _, e := range strong {
					if live[e.from] {
						mark(e.to)
					}
				}
				for key, value := range entries {
					if live[key[0]] && live[key[1]] {
						mark(value)
					}
				}
			}
			var js strings.Builder
			js.WriteString(`(()=>{const nodes=Array.from({length:64},(_,i)=>i<8?new WeakMap():{});`)
			for i, e := range strong {
				fmt.Fprintf(&js, "nodes[%d].p%d=nodes[%d];", e.from, i, e.to)
			}
			for _, e := range weak {
				fmt.Fprintf(&js, "nodes[%d].set(nodes[%d],nodes[%d]);", e.owner, e.key, e.value)
			}
			fmt.Fprintf(&js, "globalThis.roots=[nodes[%d],nodes[%d],nodes[%d],nodes[%d]];", roots[0], roots[1], roots[2], roots[3])
			js.WriteString(`globalThis.observers=nodes.map(n=>new WeakRef(n));})();void 0`)
			var want strings.Builder
			for _, alive := range live {
				if alive {
					want.WriteByte('1')
				} else {
					want.WriteByte('0')
				}
			}
			v := newShapeVM(t, 0)
			defer v.close()
			XJS_SetGCThreshold(v.tls, v.rt, ^Tsize_t(0))
			if got := v.run(js.String()); got != "<undefined>" {
				t.Fatal(got)
			}
			XJS_RunGC(v.tls, v.rt)
			if got := v.run(`observers.map(r=>r.deref()===undefined?'0':'1').join('')`); got != want.String() {
				t.Fatalf("reachability got %s want %s", got, want.String())
			}
		})
	}
}
