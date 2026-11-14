// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// The descriptor of Object.defineProperty, Object.defineProperties,
// Object.create with properties, Reflect.defineProperty and the result of a
// getOwnPropertyDescriptor trap is read as ToPropertyDescriptor says: the
// fields enumerable, configurable, value, writable, get and set in that order,
// each with HasProperty and then Get, so that getters, inherited fields, a
// changed Object.prototype and proxies see the reads they would see in any
// order of keys, and an exception of a read ends the conversion. Descriptors
// of every kind: plain, inherited fields, fields on Object.prototype, getters,
// proxies, exotic objects, frozen and null prototype ones, another realm,
// symbols and numeric keys, values of every truth, getters and setters that
// are and are not callable. The file is what Node 24 prints.
var out = []
var log = []
function run(name, f) {
  var r
  log = []
  try { r = f() } catch (e) { r = 'throws ' + e.name }
  out.push(name + ': ' + String(r) + (log.length ? ' [' + log.join() + ']' : ''))
}
function show(o, k) {
  var d = Object.getOwnPropertyDescriptor(o, k)
  if (!d) return 'none'
  if ('value' in d)
    return 'v=' + (typeof d.value === 'symbol' ? 'sym' : String(d.value)) + ' w=' + d.writable + ' e=' + d.enumerable + ' c=' + d.configurable
  return 'g=' + typeof d.get + ' s=' + typeof d.set + ' e=' + d.enumerable + ' c=' + d.configurable
}
function def(d) { return show(Object.defineProperty({}, 'x', d), 'x') }
function getter(name, v) { return function () { log.push(name); return v } }
function logged(t) {
  return new Proxy(t, {
    has(t, k) { log.push('has ' + String(k)); return Reflect.has(t, k) },
    get(t, k, r) { log.push('get ' + String(k)); return Reflect.get(t, k, r) },
  })
}
var fn = function () { return 1 }

// plain descriptors, as most programs write them
run('data', function () { return def({ value: 1, writable: true, enumerable: true, configurable: true }) })
run('data, keys in another order', function () { return def({ configurable: true, writable: false, value: 2, enumerable: true }) })
run('value only', function () { return def({ value: 3 }) })
run('empty', function () { return def({}) })
run('accessor', function () { return def({ get: fn, set: fn, enumerable: true }) })
run('get undefined', function () { return def({ get: undefined }) })
run('set only', function () { return def({ set: fn, configurable: true }) })
run('extra keys', function () { return def({ a: 1, value: 4, b: 2, enumerable: 1, c: 3 }) })
run('symbol and numeric keys', function () { var d = { 0: 'z', value: 5 }; d[Symbol('s')] = 1; d['1.5'] = 2; return def(d) })
run('deleted and added again', function () { var d = { value: 6, enumerable: true }; delete d.value; d.value = 7; return def(d) })
run('symbol value', function () { return def({ value: Symbol('v'), writable: true }) })
run('frozen', function () { return def(Object.freeze({ value: 8, enumerable: true })) })
run('sealed', function () { return def(Object.seal({ value: 9, writable: true })) })
run('not extensible', function () { return def(Object.preventExtensions({ value: 10, configurable: true })) })
run('null prototype', function () { var d = Object.create(null); d.value = 11; d.enumerable = true; return def(d) })
run('field not enumerable', function () { return def(Object.defineProperty({ enumerable: true }, 'value', { value: 12 })) })
run('field read only', function () { return def(Object.defineProperty({}, 'writable', { value: true, enumerable: true })) })
run('class instance', function () { class D { constructor() { this.value = 13; this.configurable = true } } return def(new D()) })
run('many descriptors in a loop', function () {
  var o = {}
  for (var i = 0; i < 20; i++) Object.defineProperty(o, 'k' + i, { value: i, enumerable: i % 2 === 0, writable: i % 3 === 0 })
  return Object.keys(o).join() + ' ' + show(o, 'k3') + ' ' + show(o, 'k4')
})

// the truth of the boolean fields
run('truth', function () {
  var vals = [0, -0, NaN, '', 'x', 1, -1, null, undefined, {}, [], 0n, 1n, Symbol(), fn]
  return vals.map(function (v) { return def({ enumerable: v, configurable: v, writable: v }) }).join(' | ')
})

// getters on the descriptor: read once each, in the order of the fields
run('getters of a data descriptor', function () {
  return def({ get writable() { log.push('writable'); return true }, get value() { log.push('value'); return 1 }, get configurable() { log.push('configurable'); return true }, get enumerable() { log.push('enumerable'); return true } })
})
run('getters of an accessor descriptor', function () {
  return def({ get set() { log.push('set'); return fn }, get get() { log.push('get'); return fn }, get enumerable() { log.push('enumerable'); return false } })
})
run('a getter deletes a later field', function () {
  var d = { get enumerable() { log.push('enumerable'); delete this.value; return true }, value: 1, writable: true }
  return def(d)
})
run('a getter adds a later field', function () {
  var d = { get configurable() { log.push('configurable'); this.value = 'added'; return true } }
  return def(d)
})
run('a getter adds get to Object.prototype', function () {
  var d = { get value() { log.push('value'); Object.prototype.get = fn; return 1 } }
  try { return def(d) } finally { delete Object.prototype.get }
})
run('one plain field and one getter', function () { return def({ value: 1, get enumerable() { log.push('enumerable'); return true } }) })

// a read that throws ends the conversion, with its own exception
var names = ['enumerable', 'configurable', 'value', 'writable', 'get', 'set']
names.forEach(function (n) {
  run('getter of ' + n + ' throws', function () {
    var d = {}
    names.forEach(function (m) { Object.defineProperty(d, m, { get: function () { log.push(m); if (m === n) throw new RangeError(m); return undefined }, enumerable: true }) })
    return def(d)
  })
})
names.forEach(function (n) {
  run('has trap throws at ' + n, function () {
    var p = new Proxy({ value: 1 }, {
      has(t, k) { log.push('has ' + String(k)); if (k === n) throw new RangeError(n); return Reflect.has(t, k) },
      get(t, k) { log.push('get ' + String(k)); return Reflect.get(t, k) },
    })
    return def(p)
  })
})
run('get trap throws', function () {
  var p = new Proxy({ enumerable: true, value: 1 }, { get(t, k) { log.push('get ' + String(k)); throw new RangeError('g') } })
  return def(p)
})

// fields inherited from a prototype
run('inherited value', function () { return def(Object.create({ value: 'up', enumerable: true })) })
run('inherited from two up', function () { return def(Object.create(Object.create({ writable: true, value: 'two' }))) })
run('own and inherited', function () { var d = Object.create({ value: 'up', configurable: true }); d.value = 'own'; return def(d) })
run('inherited getter', function () { return def(Object.create({ get value() { log.push('value'); return 'got' } })) })
run('inherited get', function () { return def(Object.create({ get: fn })) })
run('inherited from a function', function () { var f = function () { }; f.value = 'fn'; return def(Object.create(f)) })
run('inherited from an array', function () { var a = [1]; a.value = 'arr'; return def(Object.create(a)) })
run('prototype is a proxy', function () { return def(Object.create(logged({ enumerable: true }))) })
run('prototype is a proxy, own fields', function () { var d = Object.create(logged({})); d.value = 1; d.enumerable = true; d.configurable = true; d.writable = true; return def(d) })
run('prototype is a String object', function () { return def(Object.create(new String('ab'))) })
run('prototype set later', function () { var d = { value: 1 }; Object.setPrototypeOf(d, { enumerable: true }); return def(d) })

// Object.prototype changed by the program
function withProto(k, v, f) {
  Object.defineProperty(Object.prototype, k, { value: v, writable: true, configurable: true })
  try { return f() } finally { delete Object.prototype[k] }
}
run('Object.prototype.enumerable', function () { return withProto('enumerable', true, function () { return def({ value: 1 }) }) })
run('Object.prototype.value', function () { return withProto('value', 'proto', function () { return def({ writable: true }) }) })
run('Object.prototype.get', function () { return withProto('get', fn, function () { return def({ enumerable: true }) }) })
run('Object.prototype.get with a value', function () { return withProto('get', fn, function () { return def({ value: 1 }) }) })
run('Object.prototype.set not callable', function () { return withProto('set', 5, function () { return def({}) }) })
run('Object.prototype.get, null prototype descriptor', function () { return withProto('get', fn, function () { var d = Object.create(null); d.value = 2; return def(d) }) })
run('Object.prototype getter', function () {
  Object.defineProperty(Object.prototype, 'writable', { get: function () { log.push('writable ' + (this === Object.prototype ? 'proto' : 'desc')); return true }, configurable: true })
  try { return def({ value: 1 }) } finally { delete Object.prototype.writable }
})
run('Object.prototype changed and back in a loop', function () {
  var r = []
  for (var i = 0; i < 6; i++) {
    if (i === 2) Object.prototype.configurable = true
    if (i === 4) delete Object.prototype.configurable
    r.push(def({ value: i, enumerable: true }))
  }
  return r.join(' | ')
})

// proxies and exotic objects as the descriptor
run('proxy', function () { return def(logged({ value: 1, enumerable: true })) })
run('proxy of an accessor', function () { return def(logged({ get: fn, configurable: true })) })
run('revoked proxy', function () { var r = Proxy.revocable({}, {}); r.revoke(); return def(r.proxy) })
run('array', function () { var a = [1, 2]; a.value = 'a'; a.enumerable = true; return def(a) })
run('function', function () { var f = function () { }; f.value = 'f'; f.writable = true; return def(f) })
run('arguments', function () { var a = (function () { return arguments })(1); a.value = 'args'; return def(a) })
run('String object', function () { var s = new String('ab'); s.value = 's'; return def(s) })
run('typed array', function () { var t = new Uint8Array(2); t.configurable = true; return def(t) })
run('Date', function () { var d = new Date(0); d.value = 'date'; return def(d) })
run('RegExp', function () { var r = /x/g; r.value = 're'; return def(r) })
run('Map', function () { var m = new Map(); m.value = 'map'; return def(m) })
run('Error', function () { var e = new Error('m'); e.value = 'err'; e.enumerable = true; return def(e) })
run('global object', function () { globalThis.value = 'g'; try { return def(globalThis) } finally { delete globalThis.value } })
run('another realm', function () { var d = newRealm('({ value: "realm", enumerable: true })'); return def(d) })
run('another realm, its Object.prototype.get', function () {
  var d = newRealm('Object.prototype.get = function () { }; ({ enumerable: true })')
  return def(d)
})

// not an object
run('not an object', function () {
  return [1, 'str', null, undefined, true, Symbol(), 1n].map(function (v) {
    try { Object.defineProperty({}, 'x', v); return 'ok' } catch (e) { return e.name }
  }).join()
})

// getters and setters that are not callable, and both kinds at once
run('get not callable', function () { return def({ get: 1 }) })
run('get null', function () { return def({ get: null }) })
run('set not callable', function () { return def({ set: {} }) })
run('get a class', function () { return def({ get: class { } }) })
run('get a proxy of a function', function () { return def({ get: new Proxy(fn, {}) }) })
run('get a bound function', function () { return def({ get: fn.bind(null) }) })
run('get and value', function () { return def({ get: fn, value: 1 }) })
run('set and writable', function () { return def({ set: fn, writable: false }) })
run('get undefined and value undefined', function () { return def({ get: undefined, value: undefined }) })
run('get and set not callable, read in order', function () {
  return def({ get set() { log.push('set'); return 1 }, get get() { log.push('get'); return 2 } })
})
run('get not callable and value', function () {
  return def({ get value() { log.push('value'); return 1 }, get get() { log.push('get'); return 2 } })
})
run('get and value, all read first', function () {
  return def({ get set() { log.push('set'); return undefined }, get value() { log.push('value'); return 1 }, get get() { log.push('get'); return fn } })
})

// the other callers
run('defineProperties', function () {
  var o = Object.defineProperties({}, { a: { value: 1, enumerable: true }, b: Object.create({ get: fn }), c: logged({ value: 3 }) })
  return show(o, 'a') + ' ' + show(o, 'b') + ' ' + show(o, 'c')
})
run('defineProperties, a later descriptor throws', function () {
  var o = {}
  try { Object.defineProperties(o, { a: { value: 1 }, b: { get: 1 } }) } catch (e) { return e.name + ' ' + Object.keys(o).length }
})
run('create', function () {
  var o = Object.create(null, { a: { value: 1, writable: true }, b: { get: fn, enumerable: true }, c: Object.create({ value: 'up' }) })
  return show(o, 'a') + ' ' + show(o, 'b') + ' ' + show(o, 'c')
})
run('create, inherited get on Object.prototype', function () { return withProto('get', fn, function () { var o = Object.create({}, { a: {} }); return show(o, 'a') }) })
run('Reflect.defineProperty', function () {
  var o = {}
  return [Reflect.defineProperty(o, 'a', { value: 1 }), Reflect.defineProperty(o, 'a', { value: 2 }), Reflect.defineProperty(o, 'b', Object.create({ get: fn })), show(o, 'a'), show(o, 'b')].join()
})
run('Reflect.defineProperty, not an object', function () { return Reflect.defineProperty({}, 'a', 1) })
run('Reflect.defineProperty, get not callable', function () { return Reflect.defineProperty({}, 'a', { get: 1 }) })
run('getOwnPropertyDescriptor trap', function () {
  var p = new Proxy({ x: 1 }, { getOwnPropertyDescriptor(t, k) { return { value: 'trap', configurable: true, enumerable: true, writable: true } } })
  return show(p, 'x')
})
run('getOwnPropertyDescriptor trap, inherited fields', function () {
  var p = new Proxy({ x: 1 }, { getOwnPropertyDescriptor(t, k) { var d = Object.create({ configurable: true, value: 'up' }); return d } })
  return show(p, 'x')
})
run('getOwnPropertyDescriptor trap, getter field', function () {
  var p = new Proxy({ x: 1 }, { getOwnPropertyDescriptor(t, k) { return { configurable: true, get value() { log.push('value'); return 'got' } } } })
  return show(p, 'x')
})
run('getOwnPropertyDescriptor trap, get not callable', function () {
  var p = new Proxy({ x: 1 }, { getOwnPropertyDescriptor(t, k) { return { configurable: true, get: 1 } } })
  return show(p, 'x')
})
out.join('\n')
