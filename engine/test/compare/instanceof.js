// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.
// x instanceof C, which the register loop answers without calling
// Function.prototype[Symbol.hasInstance] when that is what C would call (see
// docs/changes/c-engine.md). Every right side and left side that must not
// take that path, or must take it and give the same: classes and their
// subclasses, built-ins, primitives and objects with a null prototype on the
// left, arrow and bound functions, callable proxies and proxies in either
// chain, an own or inherited Symbol.hasInstance (data, getter, not callable,
// null, giving a value that is not a boolean), one added and removed between
// two tests, a prototype that is a getter, inherited, not an object or not
// made yet, and the order of the steps the specification makes observable.
// Each test runs twice in a loop, so that it runs the same once warm. The
// file is what Node 24 prints.
const out = []
const log = []
function run(name, f) {
  const r = []
  for (let i = 0; i < 2; i++) {
    log.length = 0
    let v
    try { v = f() } catch (e) { v = 'throws ' + e.name }
    r.push(String(v) + (log.length ? ' [' + log.join(' ') + ']' : ''))
  }
  out.push(name + ': ' + (r[0] === r[1] ? r[0] : r.join(' | ')))
}
const io = (a, b) => a instanceof b

// functions, classes and their subclasses
function F() { }
function G() { }
G.prototype = Object.create(F.prototype)
class A { }
class B extends A { }
class C extends B { }
const f = new F(), g = new G(), a = new A(), b = new B(), c = new C()
run('function', () => [io(f, F), io(f, G), io(g, F), io(g, G), io({}, F)].join())
run('class chain', () => [io(a, A), io(a, B), io(b, A), io(b, B), io(c, A), io(c, B), io(c, C), io(c, Object)].join())
run('object and function', () => [io(c, Object), io(A, Function), io(A, Object), io(F, Function), io(Object, Function), io(Function, Object)].join())
run('built-ins', () => [io([], Array), io([], Object), io(/x/, RegExp), io(new Date(0), Date), io(new Map(), Map),
  io(new TypeError('x'), Error), io(new TypeError('x'), RangeError), io(new Int8Array(1), Int8Array),
  io(new Int8Array(1), Object), io(Promise.resolve(), Promise)].join())
run('arguments and strings', () => { const r = (function () { return [io(arguments, Object), io(arguments, Array)] })(1, 2); return r.concat([io(new String('s'), String), io(new String('s'), Object)]).join() })
class E extends Error { }
run('subclass of a built-in', () => [io(new E(), E), io(new E(), Error), io(new Error(), E)].join())
run('subclass of Array', () => { class L extends Array { } const l = new L(); return [io(l, L), io(l, Array), io([], L)].join() })
run('generator and async', () => { function* gen() { } async function af() { } return [io(gen(), gen), io(gen, Function), io(af, Function)].join() })
run('deep chain', () => {
  class D0 { }
  let K = D0
  for (let i = 0; i < 40; i++) K = class extends K { }
  const o = new K()
  return [io(o, K), io(o, D0), io(new D0(), K)].join()
})
run('long chain on the left', () => {
  let o = new F()
  for (let i = 0; i < 10000; i++) o = Object.create(o)
  return [io(o, F), io(o, G), io(o, Object)].join()
})

// the left side
run('primitives', () => [io(1, Number), io('s', String), io(true, Boolean), io(Symbol(), Symbol),
  io(1n, BigInt), io(undefined, Object), io(null, Object), io(1.5, Object), io('', F)].join())
run('null prototype object', () => { const o = Object.create(null); return [io(o, Object), io(o, F)].join() })
run('null prototype set later', () => { const o = new F(); Object.setPrototypeOf(o, null); return [io(o, F), io(o, Object)].join() })
run('prototype changed', () => { const o = new F(); const r = [io(o, G)]; Object.setPrototypeOf(o, G.prototype); r.push(io(o, G), io(o, F)); return r.join() })
run('frozen and sealed', () => [io(Object.freeze(new F()), F), io(Object.seal(new G()), F), io(Object.preventExtensions(new A()), A)].join())
run('a function on the left', () => [io(F, F), io(A, A), io(B, A), io(C, Function)].join())
run('proxy on the left', () => {
  const p = new Proxy(new F(), { getPrototypeOf(t) { log.push('getPrototypeOf'); return G.prototype } })
  return [io(p, G), io(p, F)].join()
})
run('proxy in the chain of the left', () => {
  const p = new Proxy({}, { getPrototypeOf(t) { log.push('getPrototypeOf'); return F.prototype } })
  const o = Object.create(Object.create(p))
  return [io(o, F), io(o, G)].join()
})
run('revoked proxy in the chain of the left', () => {
  const r = Proxy.revocable({}, {})
  const o = Object.create(r.proxy)
  r.revoke()
  return io(o, F)
})
run('proxy on the left, primitive right', () => io(new Proxy({}, {}), 1))

// the right side
run('arrow function', () => { const ar = () => 1; return [io(1, ar), io(null, ar)].join() })
run('arrow function, object on the left', () => { const ar = () => 1; return io({}, ar) })
run('method, object on the left', () => { const m = { m() { } }.m; return io({}, m) })
run('async function, object on the left', () => io({}, async function () { }))
run('bound function', () => { const bf = F.bind(null); return [io(f, bf), io(g, bf), io(a, bf), io(1, bf)].join() })
run('bound class', () => { const ba = A.bind(null); return [io(c, ba), io(f, ba)].join() })
run('bound function with an own prototype', () => { const bf = F.bind(null); bf.prototype = A.prototype; return [io(f, bf), io(a, bf)].join() })
run('bound of bound', () => { const bb = F.bind(null).bind(null); return [io(g, bb), io({}, bb)].join() })
run('bound arrow, object on the left', () => io({}, (() => 1).bind(null)))
run('bound arrow, primitive on the left', () => io(1, (() => 1).bind(null)))
run('bound function with its own hasInstance', () => {
  const bf = F.bind(null)
  Object.defineProperty(bf, Symbol.hasInstance, { value(v) { log.push('own'); return v === 1 } })
  return [io(1, bf), io(f, bf)].join()
})
run('callable proxy', () => { const p = new Proxy(F, {}); return [io(f, p), io(g, p), io(a, p), io(1, p)].join() })
run('callable proxy with a get trap', () => {
  const p = new Proxy(F, { get(t, k, r) { log.push(typeof k === 'symbol' ? k.description : k); return Reflect.get(t, k, r) } })
  return [io(f, p), io(1, p)].join()
})
run('callable proxy of an arrow', () => { const p = new Proxy(() => 1, {}); return [io(1, p), io({}, p)].join() })
run('revoked callable proxy', () => { const r = Proxy.revocable(F, {}); r.revoke(); return io(f, r.proxy) })
run('object proxy', () => io({}, new Proxy({}, {})))
run('plain object', () => io({}, {}))
run('primitive', () => [io({}, 1)].join())
run('primitive on both sides', () => io(1, 'x'))
run('right side undefined', () => io({}, undefined))
run('right side null', () => io({}, null))
run('symbol', () => io({}, Symbol()))
run('not callable, with the original hasInstance', () => { const o = Object.create(Function.prototype); return [io({}, o), io(1, o), io(Object.create(o), o)].join() })
run('not callable, with the original hasInstance and a prototype', () => { const o = Object.create(Function.prototype); o.prototype = F.prototype; return [io(f, o), io(g, o), io(1, o)].join() })

// Symbol.hasInstance
run('static method', () => { class S { static [Symbol.hasInstance](v) { log.push('S'); return v === 1 } } return [io(1, S), io(new S(), S)].join() })
run('static method inherited', () => {
  class S { static [Symbol.hasInstance](v) { log.push('S ' + (this === T)); return true } }
  class T extends S { }
  return [io(1, T), io({}, T)].join()
})
run('defineProperty', () => {
  function H() { }
  Object.defineProperty(H, Symbol.hasInstance, { value(v) { log.push('H'); return 'yes' } })
  return [io(1, H), io(new H(), H)].join()
})
run('Reflect.defineProperty', () => {
  function H() { }
  Reflect.defineProperty(H, Symbol.hasInstance, { value(v) { return v === 2 } })
  return [io(2, H), io(new H(), H)].join()
})
run('getter', () => {
  function H() { }
  Object.defineProperty(H, Symbol.hasInstance, { get() { log.push('get'); return () => true } })
  return [io(1, H), io({}, H)].join()
})
run('getter giving undefined', () => {
  function H() { }
  Object.defineProperty(H, Symbol.hasInstance, { get() { log.push('get'); return undefined } })
  return [io(1, H), io(new H(), H), io({}, H)].join()
})
run('hasInstance null', () => {
  function H() { }
  Object.defineProperty(H, Symbol.hasInstance, { value: null })
  return [io(1, H), io(new H(), H), io({}, H)].join()
})
run('undefined, object right side', () => { const o = { [Symbol.hasInstance]: undefined }; return io({}, o) })
run('not callable', () => {
  function H() { }
  Object.defineProperty(H, Symbol.hasInstance, { value: 1 })
  return io(new H(), H)
})
run('not callable object', () => {
  function H() { }
  Object.defineProperty(H, Symbol.hasInstance, { value: {} })
  return io(1, H)
})
run('on a plain object', () => io(1, { [Symbol.hasInstance](v) { log.push('o'); return v } }))
run('results that are not booleans', () => {
  const r = []
  for (const x of [0, 1, '', 'x', null, undefined, {}, NaN, 0n, 2n, Symbol()]) {
    function H() { }
    Object.defineProperty(H, Symbol.hasInstance, { value() { return x } })
    r.push(io({}, H))
  }
  return r.join()
})
run('throws', () => { function H() { } Object.defineProperty(H, Symbol.hasInstance, { value() { throw new RangeError('h') } }); return io({}, H) })
run('this and arguments', () => {
  function H() { }
  H[Symbol.hasInstance] = 1
  Object.defineProperty(H, Symbol.hasInstance, { value(...args) { log.push(String(this === H), String(args.length)); return true } })
  return io(1, H)
})
run('added and removed between tests', () => {
  function H() { }
  const h = new H()
  const r = [io(h, H)]
  Object.defineProperty(H, Symbol.hasInstance, { value: () => false, configurable: true })
  r.push(io(h, H))
  delete H[Symbol.hasInstance]
  r.push(io(h, H))
  return r.join()
})
run('added to a parent class between tests', () => {
  class P { }
  class Q extends P { }
  const q = new Q()
  const r = [io(q, Q)]
  Object.defineProperty(P, Symbol.hasInstance, { value: () => false, configurable: true })
  r.push(io(q, Q), io(q, P))
  delete P[Symbol.hasInstance]
  r.push(io(q, Q))
  return r.join()
})
run('prototype of the function set to an object with one', () => {
  function H() { }
  const h = new H()
  const r = [io(h, H)]
  Object.setPrototypeOf(H, { [Symbol.hasInstance]() { log.push('set'); return false } })
  r.push(io(h, H))
  Object.setPrototypeOf(H, Function.prototype)
  r.push(io(h, H))
  return r.join()
})
run('prototype of the function set to null', () => { function H() { } const h = new H(); Object.setPrototypeOf(H, null); return [io(h, H), io({}, H), io(1, H)].join() })
run('prototype of the function set to an object without one', () => { function H() { } const h = new H(); Object.setPrototypeOf(H, {}); return [io(h, H), io({}, H)].join() })
run('prototype of the function set to a proxy', () => {
  function H() { }
  const h = new H()
  Object.setPrototypeOf(H, new Proxy(Function.prototype, { get(t, k, r) { log.push(typeof k === 'symbol' ? k.description : k); return Reflect.get(t, k, r) } }))
  return [io(h, H), io(1, H)].join()
})
run('Function.prototype keeps its own', () => {
  const r = []
  try { Object.defineProperty(Function.prototype, Symbol.hasInstance, { value: () => true }) } catch (e) { r.push(e.name) }
  try { Function.prototype[Symbol.hasInstance] = () => true; r.push('assigned') } catch (e) { r.push(e.name) }
  try { (function () { 'use strict'; Function.prototype[Symbol.hasInstance] = () => true })() } catch (e) { r.push(e.name) }
  r.push(delete Function.prototype[Symbol.hasInstance], io({}, F), io(f, F))
  return r.join()
})
run('called directly', () => { const h = Function.prototype[Symbol.hasInstance]; return [h.call(F, f), h.call(F, 1), h.call({}, f), h.call(F.bind(null), g)].join() })
run('Object.prototype gets one', () => {
  Object.defineProperty(Object.prototype, Symbol.hasInstance, { value() { log.push('op'); return true }, configurable: true })
  try {
    // functions still find Function.prototype's first, a plain object finds this one
    return [io(1, F), io({}, F), io(1, {}), io(1, Object.create(Object.prototype))].join()
  } finally {
    delete Object.prototype[Symbol.hasInstance]
  }
})

// prototype
run('prototype a getter', () => {
  const ar = () => 1
  Object.defineProperty(ar, 'prototype', { get() { log.push('prototype'); return F.prototype } })
  return [io(f, ar), io(1, ar), io(a, ar)].join()
})
run('prototype a getter that changes the chain', () => {
  const ar = () => 1
  const o = new F()
  Object.defineProperty(ar, 'prototype', { get() { log.push('prototype'); Object.setPrototypeOf(o, G.prototype); return G.prototype } })
  return [io(o, ar), io(o, F)].join()
})
run('prototype a getter that tests instanceof', () => {
  const ar = () => 1
  Object.defineProperty(ar, 'prototype', { get() { log.push(String(f instanceof F), String(g instanceof ar2)); return A.prototype } })
  const ar2 = () => 2
  Object.defineProperty(ar2, 'prototype', { value: G.prototype })
  return [io(c, ar), io(f, ar)].join()
})
run('prototype a getter that throws', () => {
  const ar = () => 1
  Object.defineProperty(ar, 'prototype', { get() { throw new SyntaxError('p') } })
  return io({}, ar) + ' ' + io(1, ar)
})
run('prototype inherited', () => {
  Object.defineProperty(Function.prototype, 'prototype', { value: F.prototype, configurable: true })
  try {
    const ar = () => 1
    return [io(f, ar), io(g, ar), io(a, ar)].join()
  } finally {
    delete Function.prototype.prototype
  }
})
run('prototype not an object', () => {
  const r = []
  for (const x of [1, 'x', null, undefined, true, Symbol(), 1n]) {
    function H() { }
    H.prototype = x
    let v
    try { v = io({}, H) } catch (e) { v = e.name }
    r.push(v, io(1, H))
  }
  return r.join()
})
run('prototype a function', () => { function H() { } H.prototype = F; return [io(Object.create(F), H), io(F, H), io(Object.create(G), H)].join() })
run('prototype replaced between tests', () => {
  function H() { }
  const h = new H()
  const r = [io(h, H)]
  H.prototype = {}
  r.push(io(h, H))
  return r.join()
})
run('prototype not made yet', () => {
  function H() { }
  function I() { }
  const r = [io({}, H), io(1, I)]
  r.push(io(Object.create(H.prototype), H), io(new I(), I))
  return r.join()
})
run('prototype of a class not writable', () => { class P { } P.prototype = {}; return io(new P(), P) })
run('prototype of a built-in', () => [io(Object.create(Array.prototype), Array), io(Array.prototype, Array), io(Function.prototype, Function), io(Object.prototype, Object)].join())

// the order of the steps
run('the left side is not converted', () => {
  const o = { valueOf() { log.push('valueOf'); return 1 }, toString() { log.push('toString'); return 's' } }
  return [io(o, F), io(o, Object)].join()
})
run('hasInstance read once, before the prototype', () => {
  const ar = () => 1
  Object.defineProperty(ar, 'prototype', { get() { log.push('prototype'); return F.prototype } })
  const p = new Proxy(ar, { get(t, k, r) { log.push(typeof k === 'symbol' ? k.description : k); return Reflect.get(t, k, r) } })
  return [io(f, p), io(1, p), io(f, F.bind(null))].join()
})
run('operands evaluated first', () => {
  const r = []
  const L = () => { r.push('L'); return 1 }
  const R = () => { r.push('R'); return {} }
  try { L() instanceof R() } catch (e) { r.push(e.name) }
  return r.join()
})
run('in a loop', () => {
  let n = 0
  const xs = [f, g, a, b, c, 1, 'x', null, Object.create(null), [], new E()]
  for (let i = 0; i < 1000; i++) {
    const x = xs[i % xs.length]
    if (x instanceof F) n += 1
    if (x instanceof A) n += 10
    if (x instanceof Object) n += 100
    if (x instanceof Error) n += 1000
  }
  return n
})
run('result used as a value and in a branch', () => {
  const r = []
  for (const x of [f, a, 1]) { r.push(x instanceof F, !(x instanceof F), (x instanceof F) ? 'y' : 'n', x instanceof F && 'and') }
  return r.join()
})
run('strict code', () => (function () { 'use strict'; return [f instanceof F, 1 instanceof F, g instanceof F] })().join())
out.join('\n')
