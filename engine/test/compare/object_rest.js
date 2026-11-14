// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// The rest of an object destructuring, `{a, b, ...r} = s`, with the static
// keys of the pattern left out by a list and the result made at its final
// shape from a cache of the shapes of the sources each place saw
// (engine/regvm-ops-copy.h). Each case runs at one place many times, with
// sources of one shape and of many, so that the cache fills, hits, misses
// and replaces its entries, and with sources it must not take: index keys,
// symbols, getters, keys that are not enumerable, deleted keys, frozen and
// null prototype objects, class instances with private fields, arrays,
// strings, arguments, proxies, errors and functions. The keys of a result
// are listed with Reflect.ownKeys and their descriptors are printed.
const out = []
const show = (o) => {
  const ks = Reflect.ownKeys(o)
  return '{' + ks.map((k) => {
    const d = Object.getOwnPropertyDescriptor(o, k)
    const f = (d.enumerable ? 'e' : '') + (d.writable ? 'w' : '') + (d.configurable ? 'c' : '')
    const v = typeof d.value === 'symbol' ? d.value.toString() : typeof d.value === 'object' && d.value !== null ? '[obj]' : String(d.value)
    return String(k) + ':' + v + '/' + f
  }).join(',') + '}' + (Object.getPrototypeOf(o) === Object.prototype ? '' : ' proto?') + (Object.isExtensible(o) ? '' : ' ne')
}
const sym = Symbol('s')

// one place, every source kind, three runs each so a cached shape is hit
function restA(s) { const { a, b, ...r } = s; return show(r) + ' a=' + String(a) + ' b=' + String(b) }
function restNone(s) { const { ...r } = s; return show(r) }
function restOne(s) { const { z, ...r } = s; return show(r) }

class P { #x = 1; constructor() { this.a = 1; this.c = 2 } get x() { return this.#x } }
const sources = [
  ['plain', () => ({ a: 1, b: 2, c: 3, d: 4 })],
  ['plain, other order', () => ({ d: 4, c: 3, b: 2, a: 1 })],
  ['only excluded keys', () => ({ a: 1, b: 2 })],
  ['empty', () => ({})],
  ['one key', () => ({ c: 3 })],
  ['frozen', () => Object.freeze({ a: 1, c: 3, e: 5 })],
  ['sealed', () => Object.seal({ a: 1, c: 3 })],
  ['not extensible', () => Object.preventExtensions({ c: 3, a: 1 })],
  ['null prototype', () => Object.assign(Object.create(null), { a: 1, c: 3 })],
  ['other prototype', () => Object.assign(Object.create({ inherited: 1 }), { a: 1, c: 3 })],
  ['class with a private field', () => new P()],
  ['key not enumerable', () => Object.defineProperty({ a: 1, c: 3 }, 'h', { value: 'hidden', enumerable: false })],
  ['key not writable', () => Object.defineProperty({ a: 1, c: 3 }, 'nw', { value: 'nw', enumerable: true })],
  ['deleted key', () => { const o = { a: 1, x: 2, c: 3 }; delete o.x; return o }],
  ['deleted excluded key', () => { const o = { a: 1, b: 2, c: 3 }; delete o.b; return o }],
  ['index keys', () => ({ c: 3, 1: 'one', 0: 'zero', a: 1 })],
  ['index-like string key', () => ({ c: 3, '4294967294': 'big', a: 1 })],
  ['not an index', () => ({ c: 3, '4294967295': 'max', '01': 'lead', '-1': 'neg', a: 1 })],
  ['enumerable symbol', () => ({ [sym]: 'sym', c: 3, a: 1 })],
  ['symbol not enumerable', () => Object.defineProperty({ c: 3, a: 1 }, sym, { value: 'hs', enumerable: false })],
  ['getter on a kept key', () => ({ a: 1, get c() { return 'got' } })],
  ['getter on an excluded key', () => ({ get a() { return 'ga' }, c: 3 })],
  ['setter only', () => ({ a: 1, set c(v) {} })],
  ['getter that deletes a later key', () => ({ a: 1, get c() { delete this.d; return 'c' }, d: 4, e: 5 })],
  ['getter of an excluded key that adds one', () => ({ get a() { this.added = 'yes'; return 1 }, c: 3 })],
  ['stack key', () => ({ a: 1, stack: 'st', c: 3 })],
  ['own __proto__', () => JSON.parse('{"a":1,"__proto__":"p","c":3}')],
  ['error', () => Object.assign(new Error('m'), { a: 1, c: 3 })],
  ['array', () => Object.assign([7, 8], { a: 1, c: 3 })],
  ['string', () => 'str'],
  ['number', () => 5],
  ['boolean', () => true],
  ['arguments', function () { return (function () { return arguments })(1, 2) }],
  ['proxy', () => new Proxy({ a: 1, c: 3 }, {})],
  ['function', () => Object.assign(function f() {}, { a: 1, c: 3 })],
  ['map', () => Object.assign(new Map([[1, 2]]), { c: 3 })],
  ['date', () => Object.assign(new Date(0), { c: 3 })],
  ['33 keys', () => { const o = {}; for (let i = 0; i < 33; i++) o['k' + i] = i; return o }],
  ['70 keys', () => { const o = { a: 1 }; for (let i = 0; i < 70; i++) o['k' + i] = i; return o }],
]
for (const [name, make] of sources) {
  const res = []
  for (const f of [restA, restNone, restOne]) {
    let line = ''
    for (let i = 0; i < 3; i++) {
      let r
      try { r = f(make()) } catch (e) { r = 'throws ' + e.constructor.name }
      if (i === 0) line = r
      else if (r !== line) line += ' | run ' + i + ' differs: ' + r
    }
    res.push(line)
  }
  out.push(name + ': ' + res.join(' || '))
}

// the null and undefined sources throw before the rest
for (const v of [null, undefined]) {
  try { restA(v) } catch (e) { out.push(String(v) + ': ' + e.constructor.name) }
}

// one place, sources of many shapes in turn: more than the entries of a
// site, so they are replaced, then kept
function restPoly(s) { const { k1, ...r } = s; return JSON.stringify(r) }
const poly = []
for (let n = 1; n <= 9; n++) {
  const o = {}
  for (let i = 0; i < n; i++) o['k' + ((i * 7 + n) % 9)] = i
  poly.push(o)
}
let polyOut = ''
for (let round = 0; round < 40; round++) {
  for (const o of poly) {
    const s = restPoly(o)
    if (round === 0) polyOut += s + ';'
    else if (!polyOut.includes(s + ';')) out.push('poly differs: ' + s)
  }
}
out.push('poly: ' + polyOut)

// a source whose shape the cache holds, then changed
function restKept(s) { const { a, ...r } = s; return show(r) }
const kept = { a: 1, b: 2, c: 3 }
const kept2 = { a: 1, b: 2, c: 3 }
out.push('kept: ' + restKept(kept) + ' ' + restKept(kept2))
kept.d = 4
out.push('kept, key added: ' + restKept(kept) + ' ' + restKept(kept2))
delete kept2.b
out.push('kept, key deleted: ' + restKept(kept) + ' ' + restKept(kept2))
Object.defineProperty(kept, 'c', { enumerable: false })
out.push('kept, made not enumerable: ' + restKept(kept))
Object.defineProperty(kept, 'b', { get() { return 'getter' }, enumerable: true })
out.push('kept, made a getter: ' + restKept(kept))
Object.freeze(kept)
out.push('kept, frozen: ' + restKept(kept))
const kept3 = { a: 1, b: 2, c: 3 }
out.push('kept, new object of the first shape: ' + restKept(kept3))

// the result is a new ordinary object each time, and grows as one
const r1 = restKeptObj({ a: 1, b: 2, c: 3 })
const r2 = restKeptObj({ a: 1, b: 2, c: 3 })
function restKeptObj(s) { const { a, ...r } = s; return r }
r1.z = 'z'
delete r2.b
out.push('results: ' + show(r1) + ' ' + show(r2) + ' ' + (r1 !== r2))

// computed keys, alone and with static ones, evaluated once, in order
let n = 0
const key = (k) => { n++; out.push('key ' + k); return k }
function restComputed(s) { const { a, [key('c')]: c, b: { x } = {}, ...r } = s; return show(r) + ' c=' + c }
out.push('computed: ' + restComputed({ a: 1, b: { x: 1 }, c: 3, d: 4 }))
out.push('computed again: ' + restComputed({ a: 1, c: 3, d: 4 }) + ' n=' + n)
function restOnlyComputed(s, k) { const { [k]: v, ...r } = s; return show(r) + ' v=' + v }
out.push('only computed: ' + restOnlyComputed({ a: 1, b: 2 }, 'a') + ' ' + restOnlyComputed({ a: 1, b: 2 }, 'zz'))
out.push('computed symbol: ' + restOnlyComputed({ a: 1, [sym]: 2 }, sym))
const kobj = { toString() { n++; return 'b' } }
out.push('computed object key: ' + restOnlyComputed({ a: 1, b: 2 }, kobj) + ' n=' + n)

// a key named twice, number keys, __proto__, a shorthand with a default
function restTwice(s) { const { a, a: a2, ...r } = s; return show(r) + ' ' + a + a2 }
out.push('twice: ' + restTwice({ a: 1, b: 2 }))
function restNum(s) { const { 0: z, 1.5: f, ...r } = s; return show(r) + ' ' + z + f }
out.push('number keys: ' + restNum({ 0: 'z', '1.5': 'f', c: 3 }))
function restProto(s) { const { __proto__: p, ...r } = s; return show(r) + ' ' + (p === Object.prototype) }
out.push('__proto__: ' + restProto({ a: 1 }))
function restDefault(s) { const { a = 'def', b = (() => { out.push('default b'); return 'db' })(), ...r } = s; return show(r) + ' ' + a + b }
out.push('defaults: ' + restDefault({ c: 3 }) + ' ' + restDefault({ a: 1, b: 2, c: 3 }))

// 40 static keys: more than a list holds
const many = Array.from({ length: 40 }, (_, i) => 'm' + i)
const restMany = new Function('s', 'const {' + many.join(', ') + ', ...r} = s; return r')
const srcMany = {}
for (const k of many) srcMany[k] = 1
srcMany.left = 'left'
srcMany.m39 = 2
out.push('40 keys: ' + JSON.stringify(restMany(srcMany)) + ' ' + JSON.stringify(restMany(srcMany)))

// nested patterns, parameters, for of, assignment, catch
function nested({ a: { b, ...inner }, ...outer }) { return JSON.stringify([b, inner, outer]) }
out.push('nested: ' + nested({ a: { b: 1, c: 2 }, d: 3 }) + nested({ a: { b: 1, c: 2 }, d: 3 }))
const forOut = []
for (const { id, ...rest } of [{ id: 1, x: 1 }, { id: 2, x: 2, y: 2 }, { id: 3 }]) forOut.push(JSON.stringify(rest))
out.push('for of: ' + forOut.join(' '))
const tgt = {}
let ra
for (let i = 0; i < 2; i++) ({ a: ra, ...tgt.r } = { a: 1, b: 2, c: 3 })
out.push('assignment: ' + ra + ' ' + JSON.stringify(tgt.r))
try { throw { code: 1, msg: 'm', extra: 'e' } } catch ({ code, ...info }) { out.push('catch: ' + code + ' ' + JSON.stringify(info)) }
const arrow = ({ type, ...props }) => type + JSON.stringify(props)
out.push('arrow: ' + arrow({ type: 'div', id: 'x', className: 'c' }) + arrow({ type: 'span' }))
function* gen(s) { const { a, ...r } = s; yield r; const { b, ...r2 } = r; yield r2 }
out.push('generator: ' + JSON.stringify([...gen({ a: 1, b: 2, c: 3 })]))
// a pattern with a rest inside an array pattern, and a rest at the inner level only
function arr([{ a, ...r }, ...more]) { return JSON.stringify([a, r, more]) }
out.push('array pattern: ' + arr([{ a: 1, b: 2 }, 3]))
function innerOnly({ x: { ...r } }) { return JSON.stringify(r) }
out.push('inner only: ' + innerOnly({ x: { p: 1 } }))
function noRestHere({ a, b: [...c] }) { return JSON.stringify([a, c]) }
out.push('ellipsis not a rest: ' + noRestHere({ a: 1, b: [2, 3] }))

// the values keep their references: objects are shared, not copied
const shared = { v: [1] }
function restShared(s) { const { a, ...r } = s; return r }
const rs = restShared({ a: 1, shared })
out.push('shared: ' + (rs.shared === shared))

// a setter of Object.prototype does not run for the keys of a rest
Object.defineProperty(Object.prototype, 'trap', { set(v) { out.push('setter ran') }, configurable: true })
out.push('setter: ' + show(restShared({ a: 1, trap: 't' })))
delete Object.prototype.trap

// a hot loop over one shape and its result
let total = 0
for (let i = 0; i < 2000; i++) {
  const { a, ...r } = { a: i, b: i + 1, c: i + 2 }
  total += r.b + r.c + (r.a === undefined ? 0 : 1000)
}
out.push('loop: ' + total)
out.join('\n')
