// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// jsrun: -result
// Young collections. A young collection looks only at the objects made since the last
// collection, young or full. It never frees what an old object, a closure, a
// map, a set or a weak map value still reaches, it frees young cycles as a full
// collection does, and what is left for a full collection afterwards is found
// right. The engine never collects by itself unless a case sets a threshold:
// setGCThreshold(-1).
//
// A case that looks at memory asks memorySize() how much a collection left,
// against the 2 MiB payload it gave the case. The script itself allocates
// between two readings (its report, the results of eval), so a reading is
// allowed SLACK bytes more than the engine alone would give. The payloads and
// the garbage of the cases are a hundred times that.
setGCThreshold(-1)
const out = []
const line = (name, ok, detail) => out.push(name + ": " + (ok ? "ok" : "FAIL " + detail))
const ev = (src) => (0, eval)(src)
const tick = () => Promise.resolve().then(() => {})
const MiB = 1 << 20
const SLACK = 16 * 1024

// What the cases of this file leave in the globals goes before the next one.
function clean() {
	for (const k of ["R", "setHidden", "getHidden", "rnd", "round", "reach", "garbage", "wm", "wr", "fr", "calls", "old", "held", "kept", "make"])
		delete globalThis[k]
	gc()
}

// Random graphs of objects, closures, arrays, maps and sets held from old
// objects, an old closure variable and an old WeakMap whose values point back
// to their keys, and what the roots reach.
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
// The VM of a case: no collection of its own, the setup, one full collection.
function youngVM(setup) {
	clean()
	setGCThreshold(-1)
	ev(setup)
	gc()
}

// youngRounds runs 400 rounds of random graphs, with a young collection after
// each round when young is set, a full one every 7 rounds, and checks after
// each that what the roots reach is all there. The engine collects by itself
// past threshold. It returns what the VM holds after the roots are cleared and
// a full collection runs, and the bytes the young collections freed.
function youngRounds(young, threshold) {
	youngVM(youngSetup)
	setGCThreshold(threshold)
	const clear = "R.arr.length = 0; R.map.clear(); R.set.clear(); setHidden(null); for (const k of Object.keys(R.obj)) delete R.obj[k]; ''"
	ev("round(1)")
	ev(clear)
	gc()
	let freed = 0
	for (let seed = 2; seed < 400; seed++) {
		const want = ev("round(" + seed + ")")
		if (young) {
			const before = memorySize()
			youngGC()
			const after = memorySize()
			if (after < before) freed += before - after
			if (ev("reach()") !== want) throw new Error("seed " + seed + ": after the young collection, what the roots reach changed")
		}
		if (seed % 7 === 0) {
			gc()
			if (ev("reach()") !== want) throw new Error("seed " + seed + ": after the full collection, what the roots reach changed")
		}
	}
	ev(clear)
	gc()
	setGCThreshold(-1)
	return { held: memorySize(), freed }
}

// Nothing a young collection frees is reachable, a full collection after it
// finds every count right, also when the engine collects by itself in between,
// and in the end the full collection frees as much as it does without young
// collections.
function keepsReachable() {
	const base = youngRounds(false, -1).held
	for (const [name, threshold] of [["no engine collection", -1], ["engine collection", 4 * MiB]]) {
		try {
			const r = youngRounds(true, threshold)
			line("keeps reachable, " + name, r.freed > 0 && r.held <= base + base / 50 + SLACK,
				r.freed === 0 ? "the young collections freed nothing" : "the VM holds " + r.held + " bytes, " + base + " without young collections")
		} catch (e) {
			line("keeps reachable, " + name, false, e.message)
		}
	}
}

// A young collection frees young cycles as the full one does, and keeps a
// suspended generator whose frame holds a cycle.
function freesCycles() {
	youngVM(youngSetup)
	const base = memorySize()
	ev("R.g = (function*() { const o = {}; o.o = o; yield 1; yield o.o === o })(); R.g.next(); garbage(20000)")
	const full = memorySize()
	youngGC()
	const after = memorySize()
	line("frees young cycles, keeps a generator", after <= base + (full - base) / 20 && String(ev("R.g.next().value")) === "true",
		"the young collection left " + (after - base) + " of " + (full - base) + " bytes, or the generator broke")
}

// An old object that only young garbage holds is freed by the young
// collection that frees the garbage, whether or not a weak map, a WeakRef or a
// FinalizationRegistry observes it, and its weak references go with it. One
// that is also in a cycle of old objects stays until the next full collection.
async function freesWhatOnlyGarbageHeld() {
	for (const c of [
		{ name: "plain", observe: ``, check: `true`, young: true },
		{ name: "weak-map-key", observe: `wm.set(old, {pay: 'v'.repeat(1 << 20)});`, check: `true`, young: true },
		{ name: "weak-ref", observe: `globalThis.wr = new WeakRef(old);`, check: `wr.deref() === undefined`, young: true },
		{ name: "registry", observe: `fr.register(old, 'gone');`, check: `calls.length === 0`, young: true },
		{ name: "old-cycle", observe: `old.self = old; globalThis.wr = new WeakRef(old);`, check: `wr.deref() !== undefined`, young: false },
	]) {
		const name = "frees what only garbage held, " + c.name
		youngVM(`globalThis.wm = new WeakMap(); globalThis.calls = []; globalThis.fr = new FinalizationRegistry(x => calls.push(x));
			globalThis.old = {pay: 'x'.repeat(2 << 20)}; ` + c.observe + ` void 0`)
		const base = memorySize()
		// A young cycle that alone holds the old object.
		ev(`(() => { const a = {old}, b = {a}; a.b = b; old = null })(); void 0`)
		youngGC()
		const after = memorySize()
		const freed = after + 2 * MiB <= base + SLACK
		if (freed !== c.young) { line(name, false, "the young collection freed the old object: " + freed + ", want " + c.young); continue }
		if (String(ev(c.check)) !== "true") { line(name, false, c.check + " is not true"); continue }
		await tick()
		if (c.name === "registry" && ev(`calls.join()`) !== "gone") { line(name, false, "registry callbacks: " + ev(`calls.join()`)); continue }
		// The weak pass of a second young collection, and the full
		// collection, find no object freed twice.
		youngGC()
		gc()
		const full = memorySize()
		line(name, full + 2 * MiB <= base + SLACK && !(c.young && full > after + 64 * 1024), "held " + full + " after the full collection, " + base + " before, " + after + " after the young one")
	}
}

// Young collections keep the weak map entries a full collection must judge:
// each case of gc_ephemeron.js is freed by one full collection after a young
// one, and the young one alone frees an entry only when its key is garbage on
// its own.
function youngEphemeron() {
	for (const c of [
		{ name: "no-back-reference", body: `let key = {}; wm.set(key, {payload});`, young: true },
		{ name: "back-reference", body: `let key = {}; wm.set(key, {key, payload});`, young: false },
		{ name: "key-cycle", body: `let key = {}; key.self = key; wm.set(key, {payload});`, young: true },
		{ name: "chain", body: `let a = {}, b = {}, c = {}; a.self = a; wm.set(a, b); wm.set(b, c); wm.set(c, {a, payload});`, young: false },
		{ name: "dead-map", body: `let key = {}; globalThis.kept = key; let map = new WeakMap(); map.set(key, {map, payload});`, young: true },
		{ name: "young-map", body: `let m = new WeakMap(); globalThis.held = m; let key = {}; m.set(key, {key, payload});`, young: false },
	]) {
		const name = "young collection and ephemerons, " + c.name
		youngVM(`globalThis.wm = new WeakMap(); void 0`)
		const base = memorySize()
		ev(`(() => { let payload = 'x'.repeat(2*1024*1024); ` + c.body + ` })(); void 0`)
		const peak = memorySize()
		youngGC()
		const young = memorySize()
		youngGC()
		gc()
		const full = memorySize()
		const freed = young < base + MiB
		line(name, peak >= base + 2 * MiB && freed === c.young && full <= base + 128 * 1024,
			peak < base + 2 * MiB ? "the payload was not allocated" : freed !== c.young ? "the young collection freed the payload: " + freed + ", want " + c.young : "one full collection after young ones retained " + (full - base) + " bytes")
	}
}

// After a young collection, no object that only the garbage held is left with
// a count of 0 for the weak pass of the next collection to free twice: the
// weak map, the WeakRef, the registry, an async function and a suspended
// generator among the things that watch an old object. What a script can see
// is that the weak reference is cleared and nothing breaks, which the
// sanitizers hold to the rest.
function leavesNoZeroCount() {
	for (const observe of [``, `wm.set(old, 1);`, `wr = new WeakRef(old);`, `fr.register(old, 1);`, `old.f = async function () {};`, `old.g = (function* () { yield 1 })(); old.g.next();`]) {
		youngVM(`globalThis.wm = new WeakMap(); globalThis.wr = null; globalThis.fr = new FinalizationRegistry(() => {});
			globalThis.old = {inner: {deep: [1, 2, 3]}}; ` + observe + ` void 0`)
		ev(`(() => { const a = {old}, b = {a}; a.b = b; old = null })(); void 0`)
		youngGC()
		youngGC()
		gc()
		line("no zero count left, " + (observe || "no observer"), ev(`wr === null || wr.deref() === undefined`), "the weak reference still holds the object")
	}
}

// A young closure drops the last reference to an old, attached variable
// reference. Unlike ordinary objects, it frees it while it removes cycles,
// dropping the last reference of the suspended async frame in that phase too.
function suspendedAsyncFrame() {
	youngVM(``)
	ev(`(async () => {
		let payload = {text: 'held by the suspended frame'};
		globalThis.make = () => () => payload;
		globalThis.wr = new WeakRef(payload);
		await new Promise(() => {});
	})(); void 0`)
	gc()
	ev(`(() => { const f = make(); f.self = f; make = null; })(); void 0`)
	youngGC()
	line("suspended async frame", ev(`wr.deref() === undefined`), "the suspended frame still holds its payload")
}

async function main() {
	keepsReachable()
	freesCycles()
	await freesWhatOnlyGarbageHeld()
	youngEphemeron()
	leavesNoZeroCount()
	suspendedAsyncFrame()
	globalThis.result = out.join("\n")
}
main()
