// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// for in over objects of every kind, and hasOwnProperty.call(obj, key) as
// React writes it, with what can change on the way: keys added, deleted and
// added again during the loop, flags and prototypes changed, enumerable keys
// on the prototype chain and on Object.prototype, symbols, array indexes in
// their order, proxies, getters, Function.prototype.call and
// Object.prototype.hasOwnProperty replaced, keys of every type, this values
// that are not objects, and the global object. Loops are also frozen, sealed
// and emptied of most of their keys on the way, left by break, and stopped in
// a generator across gc().
const out = []
const log = s => out.push(s)
const has = Object.prototype.hasOwnProperty
const keys = (o, each) => {
  const r = []
  for (const k in o) {
    r.push(k + (has.call(o, k) ? '' : '^'))
    if (each) each(k, o)
  }
  return r.join(',')
}
const err = f => { try { return String(f()) } catch (e) { return e.constructor.name + (e.message === 'boom' ? ': boom' : '') } }

log('plain: ' + keys({ a: 1, b: 2, c: 3 }))
log('literal again: ' + keys({ a: 1, b: 2, c: 3 }))
log('empty: ' + keys({}))
log('null proto: ' + keys(Object.assign(Object.create(null), { x: 1, y: 2 })))
log('indexes first: ' + keys({ b: 1, 2: 1, a: 1, 1: 1, 10: 1, '-1': 1, '01': 1, 4294967294: 1, 4294967295: 1 }))
log('symbols left out: ' + keys({ a: 1, [Symbol('s')]: 2, b: 3 }))
{
  const o = { a: 1, b: 2 }
  Object.defineProperty(o, 'hidden', { value: 3, enumerable: false })
  o.c = 4
  log('not enumerable: ' + keys(o))
}
{
  const o = { a: 1, get g() { log('getter ran'); return 1 }, b: 2 }
  log('getter: ' + keys(o))
}
log('frozen: ' + keys(Object.freeze({ a: 1, b: 2 })))
log('sealed: ' + keys(Object.seal({ a: 1, b: 2 })))
{
  const big = {}
  for (let i = 0; i < 40; i++) big['k' + i] = i
  log('40 keys: ' + keys(big))
}
{
  const o = { a: 1, b: 2, c: 3, d: 4 }
  log('add during: ' + keys(o, (k, o) => { if (k === 'a') o.e = 5 }))
  log('after add: ' + keys(o))
}
{
  const o = { a: 1, b: 2, c: 3, d: 4 }
  log('delete later key: ' + keys(o, (k, o) => { if (k === 'a') delete o.c }))
}
{
  const o = { a: 1, b: 2, c: 3, d: 4 }
  log('delete and add back: ' + keys(o, (k, o) => { if (k === 'a') { delete o.c; o.c = 9 } }))
}
{
  const o = { a: 1, b: 2, c: 3 }
  log('delete current: ' + keys(o, (k, o) => { delete o[k] }) + ' left ' + JSON.stringify(o))
}
{
  const o = { a: 1, b: 2, c: 3 }
  log('made not enumerable: ' + keys(o, (k, o) => { if (k === 'a') Object.defineProperty(o, 'b', { enumerable: false }) }))
}
{
  const o = { a: 1, b: 2, c: 3 }
  log('made enumerable: ' + keys(o, (k, o) => { if (k === 'a') Object.defineProperty(o, 'z', { value: 1, enumerable: true }) }))
}
{
  const proto = { p: 1, a: 'shadowed' }
  const o = Object.create(proto)
  o.a = 1
  o.b = 2
  log('prototype keys: ' + keys(o))
  Object.defineProperty(proto, 'q', { value: 1, enumerable: false })
  log('prototype not enumerable: ' + keys(o))
}
{
  const o = { a: 1, b: 2 }
  log('prototype set during: ' + keys(o, (k, o) => { if (k === 'a') Object.setPrototypeOf(o, { p: 1, b: 'x' }) }))
}
{
  const o = { a: 1, b: 2 }
  log('prototype gets a key during: ' + keys(o, (k, o) => { if (k === 'a') Object.getPrototypeOf(o).late = 1 }))
  delete Object.prototype.late
}
Object.prototype.inherited = 1
log('Object.prototype enumerable: ' + keys({ a: 1 }))
log('hasOwnProperty.call inherited: ' + has.call({ a: 1 }, 'inherited'))
delete Object.prototype.inherited
log('Object.prototype again: ' + keys({ a: 1 }))
{
  const a = [1, 2, 3]
  a.x = 4
  log('array: ' + keys(a))
  log('array holes: ' + keys([1, , 3]))
  log('array has: ' + [0, 1, 2, 3, '1', 'length', 'x'].map(k => has.call(a, k)).join(','))
}
log('string object: ' + keys(new String('ab')))
log('arguments: ' + (function () { return keys(arguments) })(7, 8))
{
  const calls = []
  const p = new Proxy({ a: 1, b: 2 }, {
    ownKeys(t) { calls.push('ownKeys'); return Reflect.ownKeys(t) },
    getOwnPropertyDescriptor(t, k) { calls.push('gopd ' + String(k)); return Reflect.getOwnPropertyDescriptor(t, k) },
  })
  log('proxy: ' + keys(p) + ' traps ' + calls.join(';'))
}
{
  const o = Object.create(new Proxy({ p: 1 }, {
    ownKeys(t) { log('proto ownKeys'); return Reflect.ownKeys(t) },
  }))
  o.a = 1
  log('proxy prototype: ' + keys(o))
}
{
  class C { constructor() { this.x = 1 } m() {} }
  log('instance: ' + keys(new C()))
}
log('function: ' + keys(Object.assign(function f() {}, { k: 1 })))
log('primitive: ' + keys(5) + '|' + keys('xy') + '|' + keys(null) + '|' + keys(undefined))

// hasOwnProperty.call with keys and this values of every kind
const o = { a: 1, 0: 'zero', 7: 'seven', 4294967295: 'big', '': 'empty', '01': 'lead' }
const sym = Symbol('own')
o[sym] = 1
const ks = ['a', 'b', 0, 7, '7', 1, -1, 1.5, 4294967295, '4294967295', '', '01', sym, Symbol('other'), null, undefined, true, 7n]
log('keys: ' + ks.map(k => err(() => has.call(o, k))).join(','))
log('key object: ' + err(() => has.call(o, { toString() { log('toString ran'); return 'a' } })))
log('key throws: ' + err(() => has.call(o, { toString() { throw new Error('boom') } })))
log('no key: ' + err(() => has.call({ undefined: 1 })) + ',' + err(() => has.call({})))
log('extra arguments: ' + has.call(o, 'a', 'b', 'c'))
log('this undefined: ' + err(() => has.call(undefined, 'a')))
log('this null: ' + err(() => has.call(null, 'a')))
log('this primitives: ' + [['abc', 1], ['abc', 'length'], ['abc', 'x'], [5, 'x'], [true, 'x'], [sym, 'description']].map(([t, k]) => has.call(t, k)).join(','))
log('this function: ' + has.call(function f(a) {}, 'length') + ',' + has.call(function f(a) {}, 'prototype') + ',' + has.call(() => 1, 'prototype'))
log('this array: ' + has.call([5], 0) + ',' + has.call([5], 1) + ',' + has.call([5], 'length'))
log('this getter: ' + has.call({ get g() { log('getter ran'); return 1 } }, 'g'))
log('this deleted: ' + (() => { const x = { a: 1, b: 2 }; delete x.a; return has.call(x, 'a') + ',' + has.call(x, 'b') })())
{
  const calls = []
  const p = new Proxy({ a: 1 }, { getOwnPropertyDescriptor(t, k) { calls.push(String(k)); return Reflect.getOwnPropertyDescriptor(t, k) } })
  log('this proxy: ' + has.call(p, 'a') + ',' + has.call(p, 'b') + ' traps ' + calls.join(';'))
}
log('this global: ' + has.call(globalThis, 'Object') + ',' + has.call(globalThis, 'notAGlobal') + ',' + has.call(globalThis, 'out'))
log('Object.hasOwn: ' + Object.hasOwn(o, 'a') + ',' + Object.hasOwn(o, 'b'))
log('method form: ' + o.hasOwnProperty('a') + ',' + o.hasOwnProperty('b'))
log('Reflect.apply: ' + Reflect.apply(has, o, ['a']))
log('bound: ' + has.bind(o)('a') + ',' + has.call.bind(has)(o, 'a'))
log('apply: ' + has.apply(o, ['a']) + ',' + Function.prototype.call.apply(has, [o, 'a']))

// replaced, then put back
{
  const call = Function.prototype.call
  Function.prototype.call = function (t, k) { log('call replaced ' + k); return 'mine' }
  log('call replaced: ' + has.call(o, 'a'))
  Function.prototype.call = call
  log('call back: ' + has.call(o, 'a'))
}
{
  const own = Object.prototype.hasOwnProperty
  Object.prototype.hasOwnProperty = function (k) { log('hasOwnProperty replaced ' + k); return 'mine' }
  log('hasOwnProperty replaced: ' + Object.prototype.hasOwnProperty.call(o, 'a') + ',' + o.hasOwnProperty('a') + ',' + has.call(o, 'a'))
  Object.prototype.hasOwnProperty = own
}
{
  const f = { call: Function.prototype.call }
  log('call of another object: ' + err(() => f.call(o, 'a')))
}
{
  const other = function (k) { return 'other ' + k + ' ' + (this === o) }
  log('call of another function: ' + other.call(o, 'a'))
}
log('strict this: ' + (function () { 'use strict'; return has.call(this === undefined ? { a: 1 } : this, 'a') })())
{
  let n = 0
  const r = []
  for (const k in { a: 1, b: 2, c: 3 }) {
    if (has.call({ a: 1, c: 1 }, k)) r.push(k)
    n++
  }
  log('React loop: ' + r.join(',') + ' of ' + n)
}
{
  const objs = [{ a: 1, b: 2 }, { b: 2, a: 1 }, { a: 1, b: 2, c: 3 }, { a: 1 }]
  const r = []
  for (let i = 0; i < 20; i++) {
    const x = objs[i % objs.length]
    let s = ''
    for (const k in x) if (has.call(x, k)) s += k
    r.push(s)
  }
  log('shapes in turn: ' + r.join(','))
}
{
  function isObjectEmpty(object) {
    for (const _ in object) return false
    return true
  }
  log('isObjectEmpty: ' + [{}, { a: 1 }, Object.create({ p: 1 }), Object.create(null), [], [1]].map(isObjectEmpty).join(','))
}
{
  const r = []
  const outer = { a: 1, b: 2 }
  for (const k in outer) {
    for (const j in outer) r.push(k + j)
    if (k === 'a') outer.c = 3
  }
  log('nested: ' + r.join(','))
}
{
  const r = []
  try {
    for (const k in { a: 1, b: 2 }) {
      r.push(k)
      throw new Error('stop')
    }
  } catch (e) { r.push(e.message) }
  log('thrown out: ' + r.join(','))
}
{
  const o = { a: 1, b: 2, c: 3, d: 4 }
  log('frozen during: ' + keys(o, (k, o) => {
    if (k === 'b') { Object.freeze(o); o.e = 5; delete o.c; o.d = 9 }
  }) + ' left ' + JSON.stringify(o) + ' ' + Object.isFrozen(o))
}
{
  const o = { a: 1, b: 2, c: 3, d: 4 }
  log('sealed during: ' + keys(o, (k, o) => { if (k === 'a') { Object.seal(o); delete o.b; o.d = 8 } }) + ' left ' + JSON.stringify(o))
}
{
  // deleting most of the keys makes the object compact its shape
  const o = {}
  for (let i = 0; i < 40; i++) o['k' + i] = i
  log('many deleted during: ' + keys(o, (k, o) => {
    if (k === 'k0') for (let i = 1; i < 36; i++) delete o['k' + i]
    if (k === 'k37') o.k5 = 'back'
  }) + ' left ' + Object.keys(o).join(','))
}
{
  const o = {}
  for (let i = 0; i < 20; i++) o['k' + i] = i
  log('every other deleted during: ' + keys(o, (k, o) => {
    const i = +k.slice(1)
    if (i % 2 === 0 && i + 1 < 20) delete o['k' + (i + 1)]
  }))
}
{
  const o = { a: 1, b: 2, c: 3, d: 4 }
  const r = []
  for (const k in o) { r.push(k); if (k === 'b') break }
  o.e = 5
  for (const k in o) { r.push(k); if (k === 'a') break }
  delete o.a
  outer: for (const k in o) {
    for (const j in o) { r.push(k + j); if (j === 'c') continue outer; if (k === 'd') break outer }
  }
  log('break: ' + r.join(','))
}
{
  function* walk(o) { for (const k in o) yield k + (has.call(o, k) ? '' : '^') }
  const r = []
  const o = { a: 1, b: 2, c: 3, d: 4 }
  const g = walk(o)
  r.push(g.next().value)
  gc()
  delete o.c
  o.e = 5
  Object.setPrototypeOf(o, { p: 1 })
  gc()
  for (let n = g.next(); !n.done; n = g.next()) { r.push(n.value); gc() }
  log('generator suspended: ' + r.join(','))
  // the generator alone holds the object
  const lone = walk({ x: 1, y: 2, z: 3 })
  r.length = 0
  r.push(lone.next().value)
  gc()
  r.push(...lone)
  log('generator holds the object: ' + r.join(','))
  // the object holds the generator that walks it, and both are dropped
  let cyc = { a: 1, b: 2 }
  cyc.g = walk(cyc)
  cyc.g.next()
  cyc = null
  gc()
  // a generator left in the loop, returned and thrown out of
  const left = walk({ a: 1, b: 2, c: 3 })
  left.next()
  const ret = walk({ a: 1, b: 2 })
  ret.next()
  const thrown = walk({ a: 1, b: 2 })
  thrown.next()
  log('generator ended: ' + JSON.stringify(ret.return('r')) + ' ' + err(() => thrown.throw(new Error('boom'))) + ' ' + JSON.stringify(ret.next()))
  gc()
}
out.join('\n')
