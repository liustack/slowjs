// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// Function.prototype.bind reads the length of its target with
// HasOwnProperty and Get (a getter runs, a value that is not a number gives
// 0, Infinity is kept) and its name with Get (a getter runs, a value that is
// not a string gives the empty string), after GetPrototypeOf, and gives the
// bound function own length and name, in that order, configurable only.
// Targets of every kind: plain functions, arrows, methods, classes,
// generators and async functions, C functions, bound functions, proxies,
// functions with a changed prototype, a deleted or replaced length or name,
// getters, frozen functions, names outside latin-1 and long names, and many
// binds of one target, whose bound functions must not share what one of
// them changes. The file is what Node 24 prints.
var out = []
var log = []
function run(name, f) {
  var r
  log = []
  try { r = f() } catch (e) { r = 'throws ' + e.name }
  out.push(name + ': ' + String(r) + (log.length ? ' [' + log.join() + ']' : ''))
}
function desc(o, k) {
  var d = Object.getOwnPropertyDescriptor(o, k)
  if (!d) return k + ' none'
  return k + '=' + (typeof d.value === 'string' ? JSON.stringify(d.value) : Object.is(d.value, -0) ? '-0' : String(d.value)) +
    (d.writable ? ' w' : '') + (d.enumerable ? ' e' : '') + (d.configurable ? ' c' : '')
}
function show(b) {
  return Reflect.ownKeys(b).map(String).join('+') + ' ' + desc(b, 'length') + ' ' + desc(b, 'name') +
    ' proto=' + protoName(Object.getPrototypeOf(b))
}
function protoName(p) {
  if (p === Function.prototype) return 'Function'
  if (p === null) return 'null'
  if (p === Object.getPrototypeOf(function* () { })) return 'Generator'
  if (p === Object.getPrototypeOf(async function () { })) return 'Async'
  if (p === Object.getPrototypeOf(async function* () { })) return 'AsyncGenerator'
  return typeof p
}

// plain targets
function f3(a, b, c) { return [this && this.k, a, b, c].join() }
run('function', function () { return show(f3.bind()) })
run('function, one argument', function () { return show(f3.bind(null, 1)) })
run('function, more arguments than its length', function () { return show(f3.bind(null, 1, 2, 3, 4, 5)) })
run('call', function () { return f3.bind({ k: 'K' }, 1)(2, 3) })
run('arrow', function () { var a = (x, y) => x; return show(a.bind()) })
run('anonymous', function () { return show((0, function () { }).bind()) })
run('method', function () { var o = { m(a) { } }; return show(o.m.bind()) })
run('getter as a function', function () { var g = Object.getOwnPropertyDescriptor({ get p() { return 1 } }, 'p').get; return show(g.bind()) })
run('symbol named method', function () { var o = { [Symbol('sym')](a, b) { } }; return show(o[Object.getOwnPropertySymbols(o)[0]].bind()) })
run('index named method', function () { var o = { 7(a) { } }; return show(o[7].bind()) })
run('class', function () { class A { constructor(a, b) { } } return show(A.bind()) })
run('class with a static name method', function () { class A { static name() { } } return show(A.bind()) })
run('class with a static length getter', function () { class A { static get length() { log.push('length'); return 4 } } return show(A.bind()) })
run('derived class', function () { class A { } class B extends A { } var b = B.bind(); return show(b) + ' ' + (Object.getPrototypeOf(b) === A) })
run('generator', function () { return show(function* gen(a) { }.bind()) })
run('async', function () { return show(async function asy(a, b) { }.bind()) })
run('async arrow', function () { return show((async (a) => a).bind()) })
run('async generator', function () { return show(async function* ag() { }.bind()) })
run('C function', function () { return show(Math.max.bind(null, 1)) })
run('C function of a prototype', function () { return show(Array.prototype.push.bind([])) })
run('C constructor', function () { return show(Date.bind(null, 0)) + ' ' + (new (Date.bind(null, 0))()).getTime() })
run('bound function', function () { return show(f3.bind(null, 1).bind(null, 2)) })
run('bound bound bound', function () { return show(f3.bind().bind().bind(null, 1, 2, 3)) })
run('Function.prototype', function () { return show(Function.prototype.bind()) })
run('new of a bound function', function () { function F(a, b) { this.s = a + b } var B = F.bind(null, 'x'); var o = new B('y'); return o.s + ' ' + (o instanceof F) + ' ' + (o instanceof B) })

// the length of the target
function withLength(v) { var f = function (a, b) { }; Object.defineProperty(f, 'length', { value: v }); return f }
var lengths = [0, 1, 5, 2.5, -1, -2.5, -0, 2 ** 31, 2 ** 53 + 2, Infinity, -Infinity, NaN, '3', null, undefined, true, 3n, {}, Symbol('l')]
lengths.forEach(function (v, i) {
  run('length ' + (typeof v === 'symbol' ? 'symbol' : typeof v === 'bigint' ? '3n' : Object.is(v, -0) ? '-0' : typeof v === 'string' ? JSON.stringify(v) : String(v)), function () {
    var f = withLength(v)
    return desc(f.bind(), 'length') + ' ' + desc(f.bind(null, 1, 2), 'length')
  })
})
run('length deleted', function () { var f = function (a, b) { }; delete f.length; return show(f.bind()) })
run('length deleted, on the prototype', function () { var f = function (a, b) { }; delete f.length; Object.setPrototypeOf(f, { length: 9, __proto__: Function.prototype }); return show(f.bind()) })
run('length getter', function () { var f = function (a) { }; Object.defineProperty(f, 'length', { get() { log.push('length'); return 7 } }); return show(f.bind(null, 1)) })
run('length getter throws', function () { var f = function (a) { }; Object.defineProperty(f, 'length', { get() { log.push('length'); throw new RangeError() } }); return show(f.bind()) })
run('length writable', function () { var f = function (a) { }; Object.defineProperty(f, 'length', { writable: true }); f.length = 6; return show(f.bind()) })
run('length non-configurable', function () { var f = function (a) { }; Object.defineProperty(f, 'length', { configurable: false }); return show(f.bind()) })

// the name of the target
function withName(v) { var f = function (a) { }; Object.defineProperty(f, 'name', { value: v }); return f }
var names = ['x', '', 'ünïcödé', '中文', '\u{1F600}', 'a\u0000b', 1, null, undefined, {}, Symbol('n'), 'k'.repeat(5000), 'w中'.repeat(3000)]
names.forEach(function (v) {
  run('name ' + (typeof v === 'symbol' ? 'symbol' : typeof v === 'string' ? (v.length > 100 ? 'long ' + v.length : JSON.stringify(v)) : String(v)), function () {
    var n = withName(v).bind().name
    return n.length > 100 ? n.length + ' ' + JSON.stringify(n.slice(0, 12)) + ' ' + (n === 'bound ' + v) : JSON.stringify(n)
  })
})
run('name a rope', function () { var s = 'part'; for (var i = 0; i < 40; i++) s = s + 'abcdefghij'; var b = withName(s).bind(); return b.name.length + ' ' + (b.name === 'bound ' + s) })
run('name deleted', function () { var f = function g() { }; delete f.name; return show(f.bind()) })
run('name deleted, on the prototype', function () { var f = function g() { }; delete f.name; Object.setPrototypeOf(f, { name: 'up', __proto__: Function.prototype }); return show(f.bind()) })
run('name deleted, getter on the prototype', function () {
  var f = function g() { }; delete f.name
  Object.setPrototypeOf(f, { get name() { log.push('name'); return 'got' }, __proto__: Function.prototype })
  return show(f.bind())
})
run('name getter', function () { var f = function (a) { }; Object.defineProperty(f, 'name', { get() { log.push('name'); return 'gn' } }); return show(f.bind()) })
run('name getter throws', function () { var f = function (a) { }; Object.defineProperty(f, 'name', { get() { log.push('name'); throw new RangeError() } }); return show(f.bind()) })
run('both getters, in order', function () {
  var f = function (a) { }
  Object.defineProperty(f, 'name', { get() { log.push('name'); return 'n' } })
  Object.defineProperty(f, 'length', { get() { log.push('length'); return 1 } })
  return show(f.bind())
})
run('name writable', function () { var f = function (a) { }; Object.defineProperty(f, 'name', { writable: true }); f.name = 'w'; return show(f.bind()) })
run('Function.prototype.name changed', function () {
  var old = Object.getOwnPropertyDescriptor(Function.prototype, 'name')
  Object.defineProperty(Function.prototype, 'name', { value: 'fp' })
  try { var f = function () { }; delete f.name; return show(f.bind()) + ' ' + show(function h() { }.bind()) } finally { Object.defineProperty(Function.prototype, 'name', old) }
})
run('Function.prototype.length getter', function () {
  var old = Object.getOwnPropertyDescriptor(Function.prototype, 'length')
  Object.defineProperty(Function.prototype, 'length', { get() { log.push('proto length'); return 3 }, configurable: true })
  try { var f = function () { }; delete f.length; return show(f.bind()) + ' ' + show(function h(a) { }.bind()) } finally { Object.defineProperty(Function.prototype, 'length', old) }
})

// the prototype and other changes of the target
run('null prototype', function () { var f = function n(a) { }; Object.setPrototypeOf(f, null); return show(Function.prototype.bind.call(f)) })
run('object prototype', function () { var f = function n(a) { }; var p = { q: 1 }; Object.setPrototypeOf(f, p); var b = Function.prototype.bind.call(f); return show(b) + ' ' + b.q })
run('generator prototype on a plain function', function () { var f = function n(a) { }; Object.setPrototypeOf(f, Object.getPrototypeOf(function* () { })); return show(f.bind()) })
run('frozen', function () { var f = Object.freeze(function fr(a, b) { }); return show(f.bind()) })
run('not extensible', function () { var f = Object.preventExtensions(function ne(a) { }); return show(f.bind()) })
run('extra keys', function () { var f = function ex(a) { }; f.x = 1; f[Symbol('s')] = 2; return show(f.bind()) })
run('keys in another order', function () { var f = function ko(a) { 'use strict' }; var n = f.name; delete f.name; Object.defineProperty(f, 'name', { value: n, configurable: true }); return Reflect.ownKeys(f).join('+') + ' ' + show(f.bind()) })

// proxies as the target
function trapped(t) {
  return new Proxy(t, {
    getPrototypeOf(t) { log.push('getPrototypeOf'); return Reflect.getPrototypeOf(t) },
    getOwnPropertyDescriptor(t, k) { log.push('gopd ' + String(k)); return Reflect.getOwnPropertyDescriptor(t, k) },
    get(t, k, r) { log.push('get ' + String(k)); return Reflect.get(t, k, r) },
  })
}
run('proxy', function () { return show(Function.prototype.bind.call(trapped(function px(a, b) { }), null, 1)) })
run('proxy of a class', function () { class PC { } return show(Function.prototype.bind.call(trapped(PC))) })
run('proxy with no length', function () { var f = function pn(a) { }; delete f.length; return show(Function.prototype.bind.call(trapped(f))) })
run('revoked proxy', function () { var r = Proxy.revocable(function () { }, {}); r.revoke(); return Function.prototype.bind.call(r.proxy) })
run('not callable', function () { return Function.prototype.bind.call({ name: 'o', length: 1 }) })

// many binds of one target: a change to one bound function stays in it
run('many binds', function () {
  var f = function many(a, b) { }
  var bs = []
  for (var i = 0; i < 6; i++) bs.push(f.bind(null, i))
  delete bs[1].name
  Object.defineProperty(bs[2], 'length', { value: 9 })
  bs[3].extra = 1
  Object.setPrototypeOf(bs[4], null)
  var g = f.bind()
  return bs.map(show).join(' | ') + ' | ' + show(g)
})
run('binds in a loop of many kinds', function () {
  var kinds = [function k1(a) { }, (a, b) => 0, function* k3() { }, async function k4(a) { }, Math.min, class K6 { }]
  var r = []
  for (var i = 0; i < 12; i++) { var b = kinds[i % kinds.length].bind(null, i); r.push(b.name + '/' + b.length) }
  return r.join(' ')
})
run('another realm', function () {
  var f = newRealm('(function realm(a, b, c) { })')
  var b = Function.prototype.bind.call(f, null, 1)
  return Reflect.ownKeys(b).join('+') + ' ' + desc(b, 'length') + ' ' + desc(b, 'name') + ' ' + (Object.getPrototypeOf(b) === Object.getPrototypeOf(f))
})
out.join('\n')
