// Copyright (c) 2026 The liustack/tifajs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.
// JSON.stringify with no replacer and no gap writes ordinary objects and
// arrays from their shapes and elements, and leaves to the general path
// every value that may run code. Each case is written three ways, which must
// agree, as they do in Node: with no replacer, with a replacer that gives
// each value back as it is, which takes the general path, and with an empty
// gap. A case that logs what its code saw (getters, toJSON, proxy traps)
// logs the same on each way. Errors print their name only.
const out = []
const log = []
const ways = [
  ['plain', (v) => JSON.stringify(v)],
  ['replacer', (v) => JSON.stringify(v, function (k, x) { return x })],
  ['gap', (v) => JSON.stringify(v, null, '')],
]
const sym = Symbol('s')

const cases = [
  ['flat', () => ({ a: 1, b: 'two', c: true, d: null, e: false, f: 1.5, g: -7 })],
  ['nested', () => ({ a: { b: { c: [1, 2, { d: [] }] } }, e: {} })],
  ['empty', () => ({})],
  ['empty array', () => []],
  ['numbers', () => [0, -0, 1e21, 1e-7, 123456789012, -1.25, NaN, Infinity, -Infinity, 2 ** 31, -(2 ** 31), 0.1 + 0.2]],
  ['left out', () => ({ a: undefined, b: () => 1, c: sym, d: 1, [sym]: 2, e: function () {} })],
  ['null in arrays', () => [undefined, () => 1, sym, 1, function () {}]],
  ['holes', () => [1, , 3]],
  ['length past the elements', () => { const a = [1, 2]; a.length = 4; return a }],
  ['array with a hole and a getter on the prototype', () => {
    Object.defineProperty(Array.prototype, 1, { get() { log.push('proto get 1'); return 'from proto' }, configurable: true })
    return [0, , 2]
  }, () => { delete Array.prototype[1] }],
  ['array with extra keys', () => { const a = [1, 2]; a.x = 3; return a }],
  ['index keys first', () => ({ b: 1, 2: 'two', a: 3, 1: 'one', '4294967295': 'big', '4294967294': 'last index', '01': 'zero one' })],
  ['the empty key and odd keys', () => ({ '': 1, 'a"b': 2, 'c\\d': 3, 'e\nf': 4, '\u0001': 5, 'é': 6, '中': 7, '\ud800': 8, '\udc00x': 9, '😀': 10 })],
  ['strings', () => ['', 'a"b', '\\', '\n\r\t\b\f', '\u0000\u001f', 'é', '中文', '\ud800', 'x\udfff', '😀', 'a'.repeat(300)]],
  ['ropes', () => { let s = 'x'.repeat(5000); s += 'y'.repeat(9000); return { s, t: [s + '"' + s] } }],
  ['non enumerable', () => Object.defineProperty({ a: 1, c: 3 }, 'b', { value: 2, enumerable: false })],
  ['getter', () => ({ a: 1, get b() { log.push('get b'); return 2 }, c: 3 })],
  ['non enumerable getter', () => Object.defineProperty({ a: 1 }, 'b', { get() { log.push('get b'); return 2 }, enumerable: false })],
  ['getter deletes a later key', () => ({ get a() { log.push('get a'); delete this.b; return 1 }, b: 2, c: 3 })],
  ['getter adds a key', () => ({ get a() { log.push('get a'); this.z = 'added'; return 1 }, b: 2 })],
  ['toJSON deletes a later key', () => ({ a: { toJSON() { log.push('toJSON a'); delete o.b; return 'A' } }, b: 2, c: 3 })],
  ['toJSON changes a later value', () => ({ a: { toJSON() { log.push('toJSON a'); o.c = 'changed'; return 'A' } }, b: 2, c: 3 })],
  ['toJSON makes a later key not enumerable', () => ({ a: { toJSON() { Object.defineProperty(o, 'b', { enumerable: false }); return 'A' } }, b: 2, c: 3 })],
  ['toJSON adds a key to an outer object', () => ({ x: { y: { toJSON() { log.push('inner toJSON'); o.z = 'late'; delete o.w; return 'Y' } }, v: 1 }, w: 2, u: { t: 3 } })],
  ['toJSON deep down deletes a key of every level', () => ({ a: { b: { c: { toJSON() { delete o.a.b.d; delete o.a.e; delete o.f; return 'C' } }, d: 1 }, e: 2 }, f: 3, g: 4 })],
  ['toJSON deep in an array changes the array', () => [1, [2, { toJSON() { o.length = 1; o[0] = 'first'; return 'T' } }, 3], 4, 5]],
  ['toJSON pushes onto the array', () => [{ toJSON() { o.push('pushed'); return 'T' } }, 2]],
  ['toJSON on the prototype', () => { const p = { toJSON() { log.push('proto toJSON ' + this.n); return 'P' + this.n } }; return [Object.assign(Object.create(p), { n: 1 }), { n: 2 }] }],
  ['toJSON added to Object.prototype by a toJSON', () => [{ toJSON() { Object.prototype.toJSON = function () { return 'everywhere' }; return 'first' } }, { a: 1 }, [2]]],
  ['toJSON added to Array.prototype by a getter', () => ({ get a() { Array.prototype.toJSON = function () { return 'arr' }; return 1 }, b: [1], c: { d: [2] } })],
  ['toJSON not callable', () => ({ a: { toJSON: 5, b: 1 } })],
  ['toJSON getter', () => ({ a: { get toJSON() { log.push('get toJSON'); return () => 'got' } } })],
  ['dates', () => ({ d: new Date(0), e: [new Date(86400000)] })],
  ['class instances', () => { class P { constructor() { this.x = 1; this.y = [2] } get z() { return 3 } }; return [new P(), { p: new P() }] }],
  ['null prototype', () => { const o = Object.create(null); o.a = 1; o.b = Object.create(null); return o }],
  ['boxed', () => ({ s: new String('str'), n: new Number(4), b: new Boolean(false), arr: [Object('x'), Object(5)] })],
  ['boxed with valueOf', () => { const n = new Number(1); n.valueOf = () => { log.push('valueOf'); return 9 }; const s = new String('a'); s.toString = () => { log.push('toString'); return 'b' }; return [n, s] }],
  ['raw JSON', () => (typeof JSON.rawJSON === 'function' ? { r: JSON.rawJSON('123'), q: [JSON.rawJSON('"x"')] } : { r: 123, q: ['x'] })],
  ['proxy object', () => ({ p: new Proxy({ a: 1, b: 2 }, { get(t, k, r) { log.push('trap get ' + String(k)); return Reflect.get(t, k, r) }, ownKeys(t) { log.push('trap ownKeys'); return Reflect.ownKeys(t) } }) })],
  ['proxy array', () => [new Proxy([1, 2], {})]],
  ['proxy on the prototype', () => ({ a: Object.create(new Proxy({}, { get(t, k, r) { log.push('proto trap ' + String(k)); return undefined } })) })],
  ['map and set', () => ({ m: new Map([[1, 2]]), s: new Set([1]), w: new WeakMap() })],
  ['typed arrays', () => ({ u: new Uint8Array([1, 2]), f: new Float64Array([0.5]) })],
  ['regexp and error', () => ({ r: /a/g, e: new Error('x') })],
  ['arguments', () => (function () { return arguments })(1, 'two')],
  ['bigint', () => ({ a: 1, b: 2n })],
  ['bigint in an array', () => [1, 2n]],
  ['bigint with toJSON', () => { BigInt.prototype.toJSON = function () { return this.toString() + 'n' }; return { a: 3n, b: [4n] } }, () => { delete BigInt.prototype.toJSON }],
  ['symbol keys only', () => ({ [sym]: 1 })],
  ['frozen and sealed', () => ({ f: Object.freeze({ a: 1 }), s: Object.seal([1, 2]) })],
  ['cycle', () => { const a = { b: { c: {} } }; a.b.c.d = a; return a }],
  ['cycle through an array', () => { const a = [1, [2]]; a[1].push(a); return a }],
  ['cycle made by toJSON', () => { const a = { b: { toJSON() { return a } } }; return a }],
  ['the same object twice', () => { const s = { x: 1 }; return { a: s, b: [s, s] } }],
  ['functions with keys', () => { const f = () => 1; f.a = 1; return { f, g: [f] } }],
  ['a deleted key', () => { const o = { a: 1, b: 2, c: 3 }; delete o.b; return o }],
  ['a key added after a delete', () => { const o = { a: 1, b: 2 }; delete o.a; o.a = 'again'; return o }],
  ['many keys', () => { const o = {}; for (let i = 0; i < 100; i++) o['k' + i] = i % 3 ? i : { i }; return o }],
  ['a long key', () => ({ ['k'.repeat(200)]: 1, ['q"'.repeat(80)]: 2 })],
  ['deep', () => { let a = 1; for (let i = 0; i < 1000; i++) a = i % 2 ? [a] : { a }; return a }],
]

let o
for (const [name, make, after] of cases) {
  const res = []
  for (const [way, f] of ways) {
    o = make()
    log.length = 0
    let r
    try { r = f(o) } catch (e) { r = e.name }
    if (after) after()
    if (Object.prototype.hasOwnProperty('toJSON')) delete Object.prototype.toJSON
    if (Array.prototype.hasOwnProperty('toJSON')) delete Array.prototype.toJSON
    res.push([way, (typeof r === 'string' && r.length > 400 ? r.length + ' ' + r.slice(0, 60) + '...' + r.slice(-60) : String(r)) + (log.length ? ' | ' + log.join(', ') : '')])
  }
  if (res.every(([, r]) => r === res[0][1])) {
    out.push(name + ': ' + res[0][1])
  } else {
    for (const [way, r] of res) out.push(name + ' (' + way + '): ' + r)
  }
}

// the deepest array that JSON.stringify writes, with nothing inside and with
// a number inside, and the error past it
const fits = (n, leaf) => { let a = leaf; for (let i = 0; i < n; i++) a = [a]; try { JSON.stringify(a); return true } catch (e) { return e.name } }
out.push('nesting: ' + [fits(1000, 1), fits(1000, []), fits(100000, 1)].join())
out.join('\n')
