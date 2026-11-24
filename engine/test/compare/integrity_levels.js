// Copyright (c) 2026 The liustack/beastjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.
// Object.isFrozen, Object.isSealed, Object.freeze, Object.seal and
// Object.preventExtensions on every kind of object: ordinary objects of shared
// and own shapes, deleted keys, symbols, private names, numeric keys,
// accessors, arrays (fast, converted, empty, with holes), typed arrays,
// strings, arguments, functions, errors with a stack, the global object and
// proxies with the order of their traps: the extensibility is asked first,
// and an extensible object is not sealed whatever its keys. The file is what
// Node 24 prints.
'use strict'
const out = []
function run(name, f) {
  let r
  try { r = f() } catch (e) { r = 'throws ' + e.name }
  out.push(name + ': ' + String(r))
}
const lv = (o) => [Object.isExtensible(o), Object.isSealed(o), Object.isFrozen(o)].map((b) => (b ? 1 : 0)).join('')
const flags = (o) => Reflect.ownKeys(o).map((k) => {
  const d = Reflect.getOwnPropertyDescriptor(o, k)
  const s = ('value' in d ? (d.writable ? 'w' : '-') : 'a') + (d.enumerable ? 'e' : '-') + (d.configurable ? 'c' : '-')
  return String(k) + ':' + s
}).join(' ')

run('primitives', () => [1, 'a', null, undefined, true, Symbol('s'), 1n].map((v) => Object.isFrozen(v) + '/' + Object.isSealed(v) + '/' + (Object.freeze(v) === v)).join(' '))
run('plain', () => lv({ a: 1 }))
run('empty', () => lv({}))
run('empty, not extensible', () => lv(Object.preventExtensions({})))
run('one key, not extensible', () => lv(Object.preventExtensions({ a: 1 })))
run('sealed', () => { const o = Object.seal({ a: 1, b: 2 }); return lv(o) + ' ' + flags(o) })
run('frozen', () => { const o = Object.freeze({ a: 1, b: 2 }); return lv(o) + ' ' + flags(o) })
run('freeze returns its argument', () => { const o = { a: 1 }; return Object.freeze(o) === o && Object.seal(o) === o && Object.preventExtensions(o) === o })
run('accessors only, sealed', () => {
  const o = Object.seal({ get a() { return 1 }, set b(v) {} })
  return lv(o) + ' ' + flags(o)
})
run('accessors and data, frozen', () => {
  const o = Object.freeze({ x: 1, get a() { return 1 }, set b(v) {}, get c() { return 2 }, set c(v) {} })
  return lv(o) + ' ' + flags(o) + ' ' + o.a + o.c
})
run('one key left writable', () => {
  const o = { a: 1, b: 2 }
  Object.defineProperty(o, 'a', { configurable: false, writable: false })
  Object.defineProperty(o, 'b', { configurable: false })
  Object.preventExtensions(o)
  return lv(o)
})
run('one key left configurable', () => {
  const o = { a: 1, b: 2 }
  Object.defineProperty(o, 'a', { configurable: false, writable: false })
  Object.defineProperty(o, 'b', { writable: false })
  Object.preventExtensions(o)
  return lv(o)
})
run('keys made frozen one at a time', () => {
  const o = { a: 1, get b() { return 2 } }
  Object.defineProperty(o, 'a', { configurable: false, writable: false })
  Object.defineProperty(o, 'b', { configurable: false })
  const before = lv(o)
  Object.preventExtensions(o)
  return before + ' ' + lv(o)
})
run('deleted key', () => {
  const o = { a: 1, b: 2, c: 3 }
  delete o.b
  Object.defineProperty(o, 'a', { configurable: false, writable: false })
  Object.defineProperty(o, 'c', { configurable: false, writable: false })
  Object.preventExtensions(o)
  return lv(o) + ' ' + flags(o)
})
run('deleted key, then freeze', () => {
  const o = { a: 1, b: 2, c: 3 }
  delete o.a
  Object.freeze(o)
  return lv(o) + ' ' + flags(o)
})
run('symbol keys', () => {
  const s = Symbol('s')
  const o = { [s]: 1, a: 2 }
  Object.defineProperty(o, 'a', { configurable: false, writable: false })
  Object.preventExtensions(o)
  const before = lv(o)
  Object.defineProperty(o, s, { configurable: false })
  const mid = lv(o)
  Object.freeze(o)
  return before + ' ' + mid + ' ' + lv(o) + ' ' + flags(o)
})
run('numeric keys', () => {
  const o = { b: 1, 2: 'x', 1: 'y', a: 2, 10: 'z' }
  Object.freeze(o)
  return lv(o) + ' ' + flags(o)
})
run('many keys', () => {
  const o = {}
  for (let i = 0; i < 300; i++) o['k' + i] = i
  for (let i = 0; i < 40; i++) o[i * 3] = i
  const a = lv(o)
  Object.seal(o)
  const b = lv(o)
  const f1 = flags(o)
  Object.freeze(o)
  const f2 = flags(o)
  return a + ' ' + b + ' ' + lv(o) + ' ' + f1.length + ' ' + f2.length + ' ' + f2.slice(0, 40) + ' ' + f2.slice(-40)
})
run('dictionary of many keys, one deleted', () => {
  const o = {}
  for (let i = 0; i < 2000; i++) o['p' + i] = i
  delete o.p5
  Object.freeze(o)
  let w = 0, c = 0
  for (const k of Reflect.ownKeys(o)) {
    const d = Reflect.getOwnPropertyDescriptor(o, k)
    if (d.writable) w++
    if (d.configurable) c++
  }
  return lv(o) + ' ' + w + ' ' + c + ' ' + Object.keys(o).length
})
run('writes after freeze', () => {
  const o = Object.freeze({ a: 1, n: { m: 1 } })
  const r = []
  try { o.a = 2 } catch (e) { r.push(e.name) }
  try { o.b = 2 } catch (e) { r.push(e.name) }
  try { delete o.a } catch (e) { r.push(e.name) }
  try { Object.defineProperty(o, 'a', { value: 3 }) } catch (e) { r.push(e.name) }
  try { Object.defineProperty(o, 'a', { value: 1 }) } catch (e) { r.push('same value ' + e.name) }
  o.n.m = 2
  return r.join() + ' ' + o.a + ' ' + o.n.m + ' ' + ('b' in o)
})
run('writes after seal', () => {
  const o = Object.seal({ a: 1, get g() { return 7 } })
  const r = []
  o.a = 2
  try { o.b = 2 } catch (e) { r.push(e.name) }
  try { delete o.a } catch (e) { r.push(e.name) }
  try { Object.defineProperty(o, 'a', { enumerable: false }) } catch (e) { r.push(e.name) }
  try { Object.defineProperty(o, 'a', { writable: false }) } catch (e) { r.push(e.name) }
  return r.join() + ' ' + o.a + ' ' + lv(o) + ' ' + flags(o)
})
run('shared shape: the other object keeps its flags', () => {
  const make = (i) => ({ x: i, y: i + 1 })
  const a = make(1), b = make(2)
  Object.freeze(a)
  b.x = 10
  b.z = 3
  const c = make(5)
  c.x = 50
  return lv(a) + ' ' + lv(b) + ' ' + lv(c) + ' ' + flags(a) + ' | ' + flags(b) + ' | ' + flags(c) + ' ' + a.x + b.x + c.x
})
run('shared shape: seal then freeze, then a new object', () => {
  const make = (i) => ({ p: i, q: [i] })
  const objs = []
  for (let i = 0; i < 5; i++) objs.push(make(i))
  Object.seal(objs[1])
  Object.freeze(objs[2])
  Object.seal(objs[3])
  Object.freeze(objs[3])
  const n = make(9)
  n.p = 99
  return objs.map(lv).join() + ' ' + lv(n) + ' ' + n.p + ' ' + flags(objs[1]) + ' | ' + flags(objs[3])
})
run('a lone object, written in a loop before and after freeze', () => {
  const o = { x: 0, y: 0 }
  function setX(t, v) { t.x = v }
  for (let i = 0; i < 50; i++) setX(o, i)
  Object.freeze(o)
  let r = 'ok'
  try { setX(o, 100) } catch (e) { r = e.name }
  const p = { x: 0, y: 0 }
  for (let i = 0; i < 50; i++) setX(p, i)
  return r + ' ' + o.x + ' ' + p.x + ' ' + lv(o) + lv(p)
})
run('an object alone with its shape, written in a loop before and after freeze', () => {
  const o = {}
  o.lone1 = 0
  o.lone2 = 0
  function setLone(t, v) { t.lone1 = v; return t.lone1 }
  for (let i = 0; i < 50; i++) setLone(o, i)
  Object.seal(o)
  setLone(o, 60)
  Object.freeze(o)
  let r = 'ok'
  try { setLone(o, 100) } catch (e) { r = e.name }
  const p = {}
  p.lone1 = 0
  p.lone2 = 0
  for (let i = 0; i < 50; i++) setLone(p, i)
  return r + ' ' + o.lone1 + ' ' + p.lone1 + ' ' + lv(o) + lv(p) + ' ' + flags(p)
})
run('for in over frozen and plain objects of one shape', () => {
  const make = () => ({ a: 1, b: 2, c: 3 })
  const f = Object.freeze(make())
  const p = make()
  const ks = []
  for (const k in f) ks.push(k)
  for (const k in p) ks.push(k)
  for (const k in f) ks.push(k)
  return ks.join('')
})
run('freeze twice and isFrozen again and again', () => {
  const o = { a: 1, b: { c: 2 } }
  Object.freeze(o)
  Object.freeze(o)
  let n = 0
  for (let i = 0; i < 100; i++) if (Object.isFrozen(o) && Object.isSealed(o) && !Object.isFrozen(o.b)) n++
  return n + ' ' + flags(o)
})
run('private names do not count', () => {
  class A {
    #x = 1
    y = 2
    #m() { return this.#x }
    bump() { this.#x++; return this.#m() }
    static has(o) { return #x in o }
  }
  const a = new A()
  Object.defineProperty(a, 'y', { configurable: false, writable: false })
  Object.preventExtensions(a)
  const before = lv(a)
  const b = Object.freeze(new A())
  return before + ' ' + lv(b) + ' ' + b.bump() + ' ' + A.has(b) + ' ' + flags(b)
})
run('class instance with accessors on the prototype', () => {
  class P { constructor() { this.v = 1 } get g() { return 2 } }
  const p = Object.freeze(new P())
  return lv(p) + ' ' + p.g + ' ' + lv(P.prototype) + ' ' + Object.isFrozen(Object.freeze(P.prototype))
})
run('null prototype', () => {
  const o = Object.create(null)
  o.a = 1
  Object.freeze(o)
  return lv(o) + ' ' + flags(o)
})
run('Object.create with descriptors', () => {
  const o = Object.create({}, { a: { value: 1 }, b: { value: 2, writable: true }, c: { get() { return 3 } } })
  Object.preventExtensions(o)
  return lv(o) + ' ' + flags(o)
})
run('arrays', () => {
  const r = []
  r.push(lv([]), lv([1, 2]), lv(Object.preventExtensions([1, 2])))
  r.push(lv(Object.seal([1, 2])), lv(Object.freeze([1, 2])), lv(Object.freeze([])))
  const h = [1, , 3]
  Object.freeze(h)
  r.push(lv(h) + ' ' + flags(h))
  const big = Array.from({ length: 50 }, (u, i) => i)
  Object.freeze(big)
  r.push(lv(big) + ' ' + flags(big).length)
  const e = Object.freeze([])
  try { e.push(1) } catch (x) { r.push(x.name) }
  const s = Object.seal([1])
  s[0] = 5
  try { s.push(1) } catch (x) { r.push(x.name) }
  r.push(s[0] + ' ' + flags(s))
  const x = [1, 2, 3]
  x.length = 2
  x.foo = 1
  Object.freeze(x)
  r.push(flags(x))
  return r.join(' | ')
})
run('arrays made not extensible key by key', () => {
  const a = [1, 2]
  Object.defineProperty(a, 0, { configurable: false, writable: false })
  Object.defineProperty(a, 1, { configurable: false, writable: false })
  Object.preventExtensions(a)
  const r1 = Object.isSealed(a)
  Object.defineProperty(a, 'length', { writable: false })
  return r1 + ' ' + lv(a) + ' ' + flags(a)
})
run('typed arrays', () => {
  const r = []
  r.push(lv(new Uint8Array(0)), lv(Object.freeze(new Uint8Array(0))), lv(Object.seal(new Float64Array(0))))
  try { Object.freeze(new Uint8Array(2)) } catch (e) { r.push('freeze ' + e.name) }
  try { r.push(lv(Object.seal(new Uint8Array(2)))) } catch (e) { r.push('seal ' + e.name) }
  r.push(Object.isFrozen(Object.preventExtensions(new Int16Array(3))))
  return r.join(' ')
})
run('strings', () => {
  const s = new String('ab')
  const r = [lv(s)]
  Object.preventExtensions(s)
  r.push(lv(s))
  const t = new String('cd')
  t.x = 1
  Object.preventExtensions(t)
  r.push(lv(t))
  Object.freeze(t)
  r.push(lv(t), flags(t), lv(Object.seal(new String(''))))
  return r.join(' ')
})
run('arguments', () => {
  const plain = (function () { return arguments })(1, 2)
  const mapped = (function (a, b) { return [arguments, () => a] })(1, 2)
  const r = [lv(plain)]
  Object.freeze(plain)
  r.push(lv(plain), flags(plain))
  Object.freeze(mapped[0])
  r.push(lv(mapped[0]), flags(mapped[0]), mapped[1]())
  return r.join(' ')
})
run('functions', () => {
  function f(a, b) {}
  const g = () => 1
  const b = f.bind(null, 1)
  class C { static s = 1 }
  Object.freeze(f)
  Object.freeze(g)
  Object.freeze(b)
  Object.freeze(C)
  return [f, g, b, C].map(lv).join() + ' ' + flags(f) + ' | ' + flags(b) + ' | ' + flags(C)
})
run('error with a stack', () => {
  function thrower() { throw new Error('boom') }
  let e
  try { thrower() } catch (x) { e = x }
  const before = lv(e)
  Object.freeze(e)
  return before + ' ' + lv(e) + ' ' + (typeof e.stack) + ' ' + /thrower/.test(e.stack) + ' ' + e.message
})
run('built-in objects', () => {
  return lv(Math) + ' ' + lv(JSON) + ' ' + lv(Reflect) + ' ' + lv(Object.prototype) + ' ' + lv(Object.freeze(Object.create(Math)))
})
run('Map and Set objects', () => {
  const m = Object.freeze(new Map([[1, 2]]))
  m.set(3, 4)
  const s = Object.freeze(new Set([1]))
  s.add(2)
  return lv(m) + ' ' + m.size + ' ' + lv(s) + ' ' + s.size
})
run('Date and RegExp objects', () => {
  const d = Object.freeze(new Date(0))
  d.setTime(5)
  const re = Object.freeze(/a/g)
  let r = 'ok'
  try { re.exec('aa') } catch (e) { r = e.name }
  return lv(d) + ' ' + d.getTime() + ' ' + lv(re) + ' ' + r + ' ' + flags(re)
})

function logged(target, log) {
  return new Proxy(target, {
    isExtensible(t) { log.push('isExtensible'); return Reflect.isExtensible(t) },
    preventExtensions(t) { log.push('preventExtensions'); return Reflect.preventExtensions(t) },
    ownKeys(t) { log.push('ownKeys'); return Reflect.ownKeys(t) },
    getOwnPropertyDescriptor(t, k) { log.push('gopd ' + String(k)); return Reflect.getOwnPropertyDescriptor(t, k) },
    defineProperty(t, k, d) { log.push('define ' + String(k) + ' ' + Object.keys(d).sort()); return Reflect.defineProperty(t, k, d) },
  })
}
run('proxy: isFrozen of an extensible target', () => {
  const log = []
  const r = Object.isFrozen(logged({ a: 1, b: 2 }, log))
  return r + ' ' + log.join()
})
run('proxy: isSealed of an extensible target', () => {
  const log = []
  const r = Object.isSealed(logged({ a: 1 }, log))
  return r + ' ' + log.join()
})
run('proxy: isFrozen of a frozen target', () => {
  const log = []
  const r = Object.isFrozen(logged(Object.freeze({ a: 1, b: 2 }), log))
  return r + ' ' + log.join()
})
run('proxy: isFrozen of a sealed target stops at the first writable key', () => {
  const log = []
  const r = Object.isFrozen(logged(Object.seal({ a: 1, b: 2 }), log))
  return r + ' ' + log.join()
})
run('proxy: isSealed of a target not extensible with a configurable key', () => {
  const log = []
  const r = Object.isSealed(logged(Object.preventExtensions({ a: 1, b: 2 }), log))
  return r + ' ' + log.join()
})
run('proxy: freeze', () => {
  const log = []
  const t = { a: 1, get b() { return 2 } }
  Object.freeze(logged(t, log))
  return lv(t) + ' ' + log.join()
})
run('proxy: seal', () => {
  const log = []
  const t = { a: 1, b: 2 }
  Object.seal(logged(t, log))
  return lv(t) + ' ' + log.join()
})
run('proxy: ownKeys throws, target extensible', () => {
  const p = new Proxy({}, { ownKeys() { throw new RangeError('k') } })
  return Object.isFrozen(p) + ' ' + Object.isSealed(p)
})
run('proxy: ownKeys throws, target not extensible', () => {
  const p = new Proxy(Object.preventExtensions({}), { ownKeys() { throw new RangeError('k') } })
  return Object.isFrozen(p)
})
run('proxy: isExtensible throws', () => {
  const p = new Proxy({}, { isExtensible() { throw new SyntaxError('e') } })
  return Object.isFrozen(p)
})
run('proxy: preventExtensions returns false', () => {
  const p = new Proxy({}, { preventExtensions() { return false } })
  return Object.freeze(p)
})
run('revoked proxy', () => {
  const { proxy, revoke } = Proxy.revocable({}, {})
  revoke()
  return Object.isFrozen(proxy)
})
run('proxy of a frozen array', () => {
  const log = []
  const r = Object.isFrozen(logged(Object.freeze([1, 2]), log))
  return r + ' ' + log.join()
})

// V8 answers some cases against the specification, so they are left out
// here and checked in engine/test/unit.c: it takes an empty array that is not
// extensible for frozen (its length is writable), a typed array with elements
// that is not extensible for sealed (its elements are configurable), and an
// array whose elements are frozen and whose length is writable for frozen.
// Node cannot freeze the global object of a context (V8 refuses it):
// engine/test/unit.c freezes the global object of the C tree.
run('global object', () => lv(globalThis))
out.join('\n')
