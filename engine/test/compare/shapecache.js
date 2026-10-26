// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// Objects built in many ways from a seed: literals, keys one at a time,
// keys no other object has, symbols, deletes, prototypes changed, getters,
// frozen objects, cycles, objects kept alive and objects of a class. The
// functions are those of TestShapeCacheSameResults. Each runs for seeds 1 to
// 3, once with the seed and once with the next one, as the Go test does: the
// second run finds the shapes the first one left. Each line is the length and
// a hash of what one run gives.
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
function hash(s) {
  let h = 0
  for (let i = 0; i < s.length; i++) h = (Math.imul(h, 31) + s.charCodeAt(i)) | 0
  return (h >>> 0).toString(16)
}
const lines = []
for (const fn of [mixed, oneByOne, interleaved, delAdd])
  for (let seed = 1; seed <= 3; seed++)
    for (let round = 0; round < 2; round++) {
      const s = fn(seed + round)
      lines.push(fn.name + ' ' + seed + ' ' + round + ' ' + s.length + ' ' + hash(s))
    }
lines.join('\n')
