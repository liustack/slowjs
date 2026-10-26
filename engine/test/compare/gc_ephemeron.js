// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// jsrun: -result
// One full collection frees what a weak map holds for a key that nothing
// else reaches, in every arrangement of keys, values, maps and cycles, and
// keeps everything that is reachable. A weak reference, a WeakSet and a
// finalization registry see what the collection frees and nothing else. The
// engine never collects by itself here: setGCThreshold(-1). The cases run again
// with young collections before and between the full ones: a young collection
// keeps every reachable value, and one full collection afterwards still frees
// the payload.
//
// The cases that look at memory give a script a payload of 2 MiB that only the
// value of a weak map entry holds, and ask memorySize() how much one
// collection left.
setGCThreshold(-1)
const out = []
const line = (name, ok, detail) => out.push(name + ": " + (ok ? "ok" : "FAIL " + detail))
const ev = (src) => (0, eval)(src)
const tick = () => Promise.resolve().then(() => {})
const MiB = 1 << 20

// Everything a case leaves in the globals goes before the next case.
function clean() {
	for (const k of ["wm", "wmA", "wmB", "ws", "key", "kept", "calls", "fr", "wr", "payload", "roots", "observers", "keys", "token", "held"])
		delete globalThis[k]
	gc()
}

// The cases of TestEphemeron: weak collections whose payload one full
// collection must free, and what must stay reachable until drop runs.
const cases = [
	{ name: "prepass-cascade", setup: `globalThis.wmA=new WeakMap(); globalThis.wmB=new WeakMap(); (()=>{let a={},b={};wmA.set(a,{payload});wmB.set(b,a)})();`, live: `true`, drop: `` },
	{ name: "prepass-cascade-reverse", setup: `globalThis.wmB=new WeakMap(); globalThis.wmA=new WeakMap(); (()=>{let a={},b={};wmA.set(a,{payload});wmB.set(b,a)})();`, live: `true`, drop: `` },
	{ name: "cycle", setup: `globalThis.wm = new WeakMap(); (() => {let k = {}; wm.set(k, {k, payload});})();`, live: `true`, drop: `` },
	{ name: "plain-key", setup: `globalThis.wm = new WeakMap(); (() => {let k = {}; wm.set(k, {payload});})();`, live: `true`, drop: `` },
	{ name: "no-backref", setup: `globalThis.wm = new WeakMap(); (() => {let k = {}; k.self=k; wm.set(k, {payload});})();`, live: `true`, drop: `` },
	{ name: "chain", setup: `globalThis.wm = new WeakMap(); (() => {let a={}, b={}, c={}; wm.set(a,b); wm.set(b,c); wm.set(c,{a,payload});})();`, live: `true`, drop: `` },
	{ name: "live-key-chain", setup: `globalThis.wm = new WeakMap(); globalThis.key = {}; (() => {let b={}, c={}; wm.set(key,b); wm.set(b,c); wm.set(c,{key,payload});})();`, live: `wm.get(wm.get(wm.get(key))).payload.length === 2097152`, drop: `key=null;` },
	{ name: "weakset", setup: `globalThis.ws = new WeakSet(); (() => {let k={payload}; k.self=k; ws.add(k);})();`, live: `true`, drop: `` },
	{ name: "weakset-live", setup: `globalThis.ws = new WeakSet(); globalThis.key={payload}; key.self=key; ws.add(key);`, live: `ws.has(key) && key.payload.length === 2097152`, drop: `key=null;` },
	{ name: "symbol-cycle", setup: `globalThis.wm = new WeakMap(); (()=>{let k=Symbol(); wm.set(k,{k,payload});})();`, live: `true`, drop: `` },
	{ name: "symbol-property-cycle", setup: `globalThis.wm = new WeakMap(); (()=>{let k=Symbol(); wm.set(k,{[k]:payload});})();`, live: `true`, drop: `` },
	{ name: "symbol", setup: `globalThis.wm = new WeakMap(); globalThis.key = Symbol(); wm.set(key,{payload});`, live: `wm.get(key).payload.length === 2097152`, drop: `key=null;` },
	{ name: "dead-map", setup: `globalThis.key={}; (() => {let m=new WeakMap(); m.set(key,{m,payload});})();`, live: `true`, drop: `` },
]

function ephemeronCase(c, young) {
	const name = (young ? "young ephemeron " : "ephemeron ") + c.name
	clean()
	const base = memorySize()
	ev(`globalThis.payload='x'.repeat(2097152); ` + c.setup + ` payload=null; void 0`)
	if (young) {
		youngGC()
		if (String(ev(c.live)) !== "true") return line(name, false, "a reachable value was lost by the young collection")
		youngGC()
	}
	gc()
	if (String(ev(c.live)) !== "true") return line(name, false, "a reachable value was lost")
	if (c.drop) {
		ev(c.drop + ` void 0`)
		if (young) youngGC()
		gc()
	}
	const left = memorySize() - base
	line(name, left <= 128 * 1024, "one full collection kept " + left + " bytes")
}
for (const c of cases) ephemeronCase(c, false)
for (const c of cases) ephemeronCase(c, true)

// The cases of TestRunGCEphemeron: the payload is a local of the function that
// makes the entry, so it is a string of 2 MiB the moment the function returns.
for (const [name, body] of [
	["no-back-reference", `let key = {}; wm.set(key, {payload});`],
	["back-reference", `let key = {}; wm.set(key, {key, payload});`],
	["key-cycle", `let key = {}; key.self = key; wm.set(key, {payload});`],
	["chain", `let a = {}, b = {}, c = {}; a.self = a; wm.set(a, b); wm.set(b, c); wm.set(c, {a, payload});`],
	["dead-map", `let key = {}; globalThis.kept = key; let map = new WeakMap(); map.set(key, {map, payload});`],
]) {
	clean()
	ev(`globalThis.wm = new WeakMap(); void 0`)
	gc()
	const base = memorySize()
	ev(`(() => { let payload = 'x'.repeat(2*1024*1024); ` + body + ` })(); void 0`)
	const peak = memorySize()
	gc()
	const left = memorySize() - base
	line("run gc " + name, peak >= base + 2 * MiB && left <= 128 * 1024,
		peak >= base + 2 * MiB ? "one full collection retained " + left + " bytes" : "the payload was not allocated")
}

// Weak references and finalization registries see what a collection frees,
// not before: the callback runs as a job, never inside the collection.
async function weakObservers() {
	clean()
	ev(`globalThis.calls=[]; globalThis.fr=new FinalizationRegistry(x=>calls.push(x)); globalThis.wm=new WeakMap(); globalThis.key={}; wm.set(key,{key}); globalThis.wr=new WeakRef(key); fr.register(key,'done'); void 0`)
	gc()
	let ok = ev(`wr.deref() === key && calls.length === 0`)
	ev(`key=null; void 0`)
	gc()
	ok = ok && ev(`wr.deref() === undefined && calls.length === 0`)
	await tick()
	ok = ok && ev(`calls.length === 1 && calls[0] === 'done'`)
	gc()
	await tick()
	ok = ok && ev(`calls.length === 1`)
	line("weak observers", ok, "calls " + ev(`calls.join()`))
}

// The same with young collections: they see what they free, and not what they
// keep.
async function youngWeakObservers() {
	clean()
	ev(`globalThis.calls=[]; globalThis.fr=new FinalizationRegistry(x=>calls.push(x)); globalThis.wm=new WeakMap(); globalThis.key={}; wm.set(key,{key}); globalThis.wr=new WeakRef(key); fr.register(key,'kept');
		globalThis.wr2 = (() => { const k = {}; k.self = k; fr.register(k, 'cycle'); return new WeakRef(k) })(); void 0`)
	youngGC()
	let ok = ev(`wr.deref() === key && wr2.deref() === undefined && calls.length === 0`)
	await tick()
	ok = ok && ev(`calls.join()`) === "cycle"
	// The key's weak map value points back to it: the young collection
	// keeps it, the full one frees it.
	ev(`key=null; void 0`)
	youngGC()
	ok = ok && ev(`wr.deref() !== undefined`)
	gc()
	ok = ok && ev(`wr.deref() === undefined`)
	await tick()
	ok = ok && ev(`calls.join()`) === "cycle,kept"
	line("young weak observers", ok, "calls " + ev(`calls.join()`))
}

// A symbol that something reachable holds, in a key, a value, a wrapper or a
// map, keeps its weak map entries alive, and goes with them when it goes.
async function liveSymbols() {
	for (const root of [`globalThis.key=s`, `globalThis.key={[s]:true}`, `globalThis.key=Object(s)`, `globalThis.key=new Map([[s,42]])`, `globalThis.key=[s]`]) {
		clean()
		ev(`globalThis.wm=new WeakMap();globalThis.wr=undefined;(()=>{let s=Symbol(); ` + root + `; let second=Symbol(); wm.set(s,second); wm.set(second,{s,payload:'x'.repeat(2097152)}); wr=new WeakRef(s);})(); void 0`)
		gc()
		let ok = ev(`wm.get(wm.get(wr.deref())).payload.length===2097152`)
		ev(`key=null; void 0`)
		gc()
		ok = ok && ev(`wr.deref()===undefined`)
		line("live symbols " + root, ok, "the symbol was lost or kept")
	}
}

// Entries added and deleted after a collection find the table as it was.
function mutationAfterGC() {
	clean()
	ev(`globalThis.wm=new WeakMap();globalThis.keys=Array.from({length:2000},()=>({}));for(let i=0;i<keys.length;i++){wm.set(keys[i],{key:keys[i],i});if(i%2===0)keys[i]=null};void 0`)
	gc()
	let ok = ev(`keys.every((key,i)=>key===null || wm.get(key).i===i)`)
	ok = ok && ev(`keys.every((key,i)=>{if(key===null)return true; if(!wm.delete(key)||wm.has(key))return false;wm.set(key,i);return wm.get(key)===i})`)
	ev(`keys=null;void 0`)
	gc()
	line("mutation after a collection", ok, "an entry was lost or wrong")
}

// A target that was unregistered queues no job.
async function registryUnregister() {
	clean()
	ev(`globalThis.calls=[];globalThis.fr=new FinalizationRegistry(x=>calls.push(x));globalThis.wm=new WeakMap();globalThis.token={};(()=>{let k={};wm.set(k,{k});fr.register(k,'cancelled',token)})();fr.unregister(token);void 0`)
	gc()
	await tick()
	line("registry unregister", ev(`calls.length===0`), "the unregistered target called back")
}

// A small independent reachability oracle covers ordering, shared keys,
// garbage maps and cycles spanning several maps. The oracle does not use
// reference counts or the collector's work list. A graph of seed has 64
// nodes, the first 8 of them weak maps, with roots and strong and weak
// edges, and the oracle says which nodes it leaves reachable. The generator
// is the one of TestEphemeronMixedGraph, in 64-bit arithmetic.
function ephemeronGraph(seed) {
	const n = 64
	let next = BigInt(seed)
	const rand = (limit) => {
		next = (next * 6364136223846793005n + 1442695040888963407n) & 0xffffffffffffffffn
		return Number((next >> 32n) % BigInt(limit))
	}
	const strong = [], weak = []
	for (let i = 0; i < 40; i++) { const from = rand(n), to = rand(n); strong.push({ from, to }) }
	for (let i = 0; i < 80; i++) { const owner = rand(8), key = rand(n), value = rand(n); weak.push({ owner, key, value }) }
	// WeakMap.set replaces an earlier entry with the same owner and key.
	const entries = new Map()
	for (const e of weak) entries.set(e.owner * n + e.key, e.value)
	const live = new Array(n).fill(false)
	const roots = [rand(n), rand(n), rand(n), rand(n)]
	for (const r of roots) live[r] = true
	for (let changed = true; changed;) {
		changed = false
		const mark = (i) => { if (!live[i]) { live[i] = true; changed = true } }
		for (const e of strong) if (live[e.from]) mark(e.to)
		for (const [k, value] of entries) if (live[Math.floor(k / n)] && live[k % n]) mark(value)
	}
	const nodes = Array.from({ length: n }, (_, i) => i < 8 ? new WeakMap() : {})
	strong.forEach((e, i) => { nodes[e.from]["p" + i] = nodes[e.to] })
	for (const e of weak) nodes[e.owner].set(nodes[e.key], nodes[e.value])
	globalThis.roots = roots.map((r) => nodes[r])
	globalThis.observers = nodes.map((x) => new WeakRef(x))
	return live.map((b) => b ? "1" : "0").join("")
}
const survivors = () => observers.map((r) => r.deref() === undefined ? "0" : "1").join("")

function mixedGraph() {
	let bad = []
	for (let seed = 1; seed <= 20; seed++) {
		clean()
		const want = ephemeronGraph(seed)
		gc()
		const got = survivors()
		if (got !== want) bad.push("seed " + seed + " got " + got + " want " + want)
	}
	line("mixed graphs, 20 seeds", bad.length === 0, bad.join("; "))
}

// A young collection keeps every node the oracle says is reachable, and a
// full one afterwards keeps exactly those.
function youngMixedGraph() {
	let bad = []
	for (let seed = 1; seed <= 20; seed++) {
		clean()
		const want = ephemeronGraph(seed)
		youngGC()
		const young = survivors()
		for (let i = 0; i < want.length; i++)
			if (want[i] === "1" && young[i] !== "1") { bad.push("seed " + seed + ": the young collection freed node " + i); break }
		gc()
		const got = survivors()
		if (got !== want) bad.push("seed " + seed + " got " + got + " want " + want)
	}
	line("young mixed graphs, 20 seeds", bad.length === 0, bad.join("; "))
}

async function main() {
	await weakObservers()
	await youngWeakObservers()
	await liveSymbols()
	mutationAfterGC()
	await registryUnregister()
	mixedGraph()
	youngMixedGraph()
	globalThis.result = out.join("\n")
}
main()
