// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// a[i] and a[i] = v on an Array that is no longer a fast array (filled from
// the top down, written past its end, an element deleted or given a getter),
// with the key in a local, a constant, a temp and the array in a temp, sloppy
// and strict: elements that are there, holes, the end and past it, with
// indexed properties, getters and setters on Array.prototype and on
// Object.prototype, a prototype that is a Proxy, a typed array, a String
// object, an arguments object or an array with elements, own getters,
// setters and read-only elements, frozen, sealed and non-extensible arrays,
// a length that cannot be written, writes that raise the length, a subclass
// of Array and an Array of another realm, and keys at 2^31 - 1, 2^32 - 2 and
// past, negative, fractional and numeric strings. The file is what Node 24
// prints.
var out = []
var log = []
function show(v) {
  if (typeof v === 'symbol') return v.toString()
  if (Array.isArray(v)) {
    var s = []
    for (var i = 0; i < v.length && i < 12; i++) s.push(i in v ? show(v[i]) : '<hole>')
    return '[' + s.join(',') + (v.length > 12 ? ',...' : '') + ']'
  }
  if (v !== null && typeof v === 'object') return 'object'
  if (typeof v === 'function') return 'function'
  return Object.is(v, -0) ? '-0' : typeof v === 'string' ? JSON.stringify(v) : String(v)
}
function t(name, f) {
  var r
  try { r = show(f()) } catch (e) { r = 'throws ' + (e && e.name) }
  out.push(name + ': ' + r + (log.length ? ' log ' + log.join(';') : ''))
  log.length = 0
}
function id(x) { return x }
// the reads: key in a local, array in a temp, key a temp, constants
function rd(a, i) { return a[i] }
function rdt(a, i) { return id(a)[i] }
function rdx(a, i) { return a[i + 0] }
function rd0(a) { return a[0] }
function rd3(a) { return a[3] }
function rdmax(a) { return a[2147483647] }
// the writes, sloppy and strict
function wr(a, i, v) { a[i] = v; return a[i] }
function wrt(a, i, v) { id(a)[i] = v; return a.length }
function wrs(a, i, v) { 'use strict'; a[i] = v; return a[i] }
function wr3(a, v) { a[3] = v; return a.length }
function wrs3(a, v) { 'use strict'; a[3] = v; return a.length }
// read and write in one: a[i] op= v, a[i]++
function addto(a, i, v) { a[i] += v; return a[i] }
function inc(a, i) { return a[i]++ }

// an Array that is not fast, made as crypto.js makes it: filled from the top
function down(n) { var a = new Array(); for (var i = n - 1; i >= 0; i--) a[i] = i * 10; return a }
// written past its end, so it has holes at 0, 1 and 3
function holey() { var a = []; a[4] = 'e'; a[2] = 'c'; return a }
// a fast array that lost its state by a delete or a getter of an element
function deleted() { var a = [0, 1, 2, 3, 4]; delete a[1]; return a }
function getterEl() {
  var a = [0, 1, 2, 3, 4]
  Object.defineProperty(a, 1, { get: function () { log.push('get1 ' + (this === a)); return 'g' }, set: function (v) { log.push('set1 ' + v) }, configurable: true })
  return a
}

var makers = {
  down: function () { return down(5) },
  holey: holey,
  deleted: deleted,
  getterEl: getterEl,
  frozen: function () { return Object.freeze(down(5)) },
  sealed: function () { return Object.seal(down(5)) },
  prevented: function () { return Object.preventExtensions(down(5)) },
  fixedLength: function () { return Object.defineProperty(down(5), 'length', { writable: false }) },
  readOnly1: function () { var a = down(5); Object.defineProperty(a, 1, { value: 'ro', writable: false }); return a },
}
var keys = [0, 1, 2, 3, 4, 5, 9]
for (var name in makers) {
  var mk = makers[name]
  for (var j = 0; j < keys.length; j++) {
    (function (i) {
      t('rd ' + name + ' ' + i, function () { var a = mk(); return [rd(a, i), rdt(a, i), rdx(a, i)] })
      t('wr ' + name + ' ' + i, function () { var a = mk(); return [wr(a, i, 'w'), a.length, a] })
      t('wrt ' + name + ' ' + i, function () { var a = mk(); return [wrt(a, i, 'w'), a] })
      t('wrs ' + name + ' ' + i, function () { var a = mk(); return [wrs(a, i, 'w'), a.length, a] })
      t('addto ' + name + ' ' + i, function () { var a = mk(); return [addto(a, i, 1), a] })
      t('inc ' + name + ' ' + i, function () { var a = mk(); return [inc(a, i), a[i], a] })
    })(keys[j])
  }
  t('rd0 rd3 ' + name, function () { var a = mk(); return [rd0(a), rd3(a), rdmax(a)] })
  t('wr3 ' + name, function () { var a = mk(); return [wr3(a, 'k'), a] })
  t('wrs3 ' + name, function () { var a = mk(); return [wrs3(a, 'k'), a] })
}

// indexed properties on the prototypes, read through a hole and past the end
function withProto(what, f) {
  try {
    if (what === 'array') Array.prototype[3] = 'ap3'
    if (what === 'object') Object.prototype[3] = 'op3'
    if (what === 'getter') Object.defineProperty(Array.prototype, 3, { get: function () { log.push('get3 ' + Array.isArray(this) + ' ' + this.length); return 'pg3' }, set: function (v) { log.push('set3 ' + v + ' ' + this.length) }, configurable: true })
    if (what === 'ogetter') Object.defineProperty(Object.prototype, 3, { get: function () { log.push('oget3 ' + this.length); return 'og3' }, set: function (v) { log.push('oset3 ' + v) }, configurable: true })
    if (what === 'readonly') Object.defineProperty(Array.prototype, 3, { value: 'ro3', writable: false, configurable: true })
    return f()
  } finally {
    delete Array.prototype[3]
    delete Object.prototype[3]
  }
}
var whats = ['array', 'object', 'getter', 'ogetter', 'readonly']
for (var w = 0; w < whats.length; w++) {
  (function (what) {
    t('proto ' + what + ' rd hole', function () { return withProto(what, function () { var a = holey(); return [rd(a, 3), rdt(a, 3), rd3(a), rd(a, 2), 3 in a] }) })
    t('proto ' + what + ' rd past end', function () { return withProto(what, function () { var a = down(2); return [rd(a, 3), rd3(a), a.length] }) })
    t('proto ' + what + ' rd fast past count', function () { return withProto(what, function () { var a = [1, 2]; a.length = 6; return [rd(a, 3), rd3(a), a.length] }) })
    t('proto ' + what + ' rd own', function () { return withProto(what, function () { var a = down(5); return [rd(a, 3), rd3(a)] }) })
    t('proto ' + what + ' wr hole', function () { return withProto(what, function () { var a = holey(); var r = wr(a, 3, 'w'); return [r, a.length, a.hasOwnProperty(3)] }) })
    t('proto ' + what + ' wrs hole', function () { return withProto(what, function () { var a = holey(); var r = wrs(a, 3, 'w'); return [r, a.length, a.hasOwnProperty(3)] }) })
    t('proto ' + what + ' wr own', function () { return withProto(what, function () { var a = down(5); var r = wr(a, 3, 'w'); return [r, a] }) })
    t('proto ' + what + ' addto hole', function () { return withProto(what, function () { var a = holey(); var r = addto(a, 3, '+'); return [r, a.hasOwnProperty(3)] }) })
  })(whats[w])
}

// a prototype that is exotic, or an array with elements
var protos = {
  proxy: function () { return new Proxy([], { get: function (o, k, r) { log.push('trap get ' + String(k) + ' ' + Array.isArray(r)); return 'P' + String(k) }, set: function (o, k, v, r) { log.push('trap set ' + String(k) + ' ' + v); return true } }) },
  int8: function () { return new Int8Array([7, 8, 9, 10]) },
  string: function () { return new String('wxyz') },
  args: function () { return (function () { return arguments })('a0', 'a1', 'a2', 'a3') },
  fastArray: function () { return [100, 101, 102, 103] },
  slowArray: function () { return down(4) },
  nullProto: function () { return null },
}
for (var pn in protos) {
  (function (pn) {
    t('exotic proto ' + pn + ' rd', function () { var a = holey(); Object.setPrototypeOf(a, protos[pn]()); return [rd(a, 0), rd(a, 2), rd(a, 3), rd(a, 6), rd3(a)] })
    t('exotic proto ' + pn + ' wr', function () { var a = holey(); Object.setPrototypeOf(a, protos[pn]()); var r = [wr(a, 3, 'w'), wr(a, 2, 'w2'), wr(a, 6, 'w6')]; return [r, a.length, Object.keys(a).join()] })
  })(pn)
}

// keys around the largest index, negative, fractional, numeric strings
var bigKeys = [2147483646, 2147483647, 2147483648, 4294967294, 4294967295, 4294967296, -1, -0, 1.5, 1.0, NaN, '1', '01', '1.0', '-0', ' 1', '4294967294']
for (var b = 0; b < bigKeys.length; b++) {
  (function (k) {
    var kn = typeof k === 'string' ? JSON.stringify(k) : Object.is(k, -0) ? '-0' : String(k)
    t('key ' + kn + ' wr down', function () { var a = down(3); var r = wr(a, k, 'K'); return [r, rd(a, k), a.length, Object.keys(a).join()] })
    t('key ' + kn + ' wrs holey', function () { var a = holey(); var r = wrs(a, k, 'K'); return [r, rdt(a, k), a.length, Object.keys(a).join()] })
    t('key ' + kn + ' rd down', function () { var a = down(3); return [rd(a, k), rdx(a, k), rdt(a, k)] })
  })(bigKeys[b])
}
t('max index then read and shrink', function () { var a = []; a[4294967294] = 'm'; a[2147483647] = 'i'; var r = [rd(a, 4294967294), rd(a, 2147483647), rdmax(a), a.length]; a.length = 2147483647; r.push(rd(a, 2147483647), rdmax(a), a.length); return r })

// the length changed by writes and by assignment
t('length raised by writes', function () { var a = down(3); var r = []; wr(a, 7, 'x'); r.push(a.length); wr(a, 5, 'y'); r.push(a.length); wrs(a, 20, 'z'); r.push(a.length, rd(a, 6), rd(a, 20)); return r })
t('length cut then read', function () { var a = down(6); a.length = 2; return [rd(a, 1), rd(a, 2), rd(a, 5), a.length, wr(a, 4, 'n'), a.length] })
t('length cut to 0 then write', function () { var a = down(6); a.length = 0; return [wr(a, 2, 'n'), a.length, rd(a, 0), rd(a, 2)] })
t('fixed length write past end', function () { var a = makers.fixedLength(); var r = [wr(a, 7, 'x'), a.length]; try { wrs(a, 7, 'x') } catch (e) { r.push(e.name) } r.push(wrs(a, 2, 'in'), a.length); return r })

// delete then read and write
t('delete then read', function () { var a = down(5); delete a[2]; return [rd(a, 2), rdt(a, 2), 2 in a, a.length] })
t('delete then write', function () { var a = down(5); delete a[2]; return [wr(a, 2, 'back'), 2 in a, a] })
t('delete all then read', function () { var a = down(3); delete a[0]; delete a[1]; delete a[2]; return [rd(a, 0), rd(a, 1), rd(a, 2), a.length] })

// values with references: objects in and out of the elements
t('objects in elements', function () {
  var a = down(4), o = { n: 1 }
  wr(a, 1, o); wr(a, 2, a); wr(a, 1, 'flat'); wr(a, 3, o)
  return [rd(a, 1), rd(a, 2) === a, rd(a, 3) === o, rd(a, 3).n]
})

// a subclass of Array, with a getter of an index on its prototype
class MyArray extends Array {}
Object.defineProperty(MyArray.prototype, 6, { get: function () { log.push('sub get6 ' + (this instanceof MyArray)); return 'sub6' }, set: function (v) { log.push('sub set6 ' + v) }, configurable: true })
t('subclass rd', function () { var a = new MyArray(); a[3] = 'three'; a[1] = 'one'; return [rd(a, 1), rd(a, 2), rd(a, 3), rd(a, 6), a.length] })
t('subclass wr', function () { var a = new MyArray(); a[3] = 'three'; return [wr(a, 3, 'T'), wr(a, 6, 'S'), wrs(a, 2, 'two'), a.length, a.hasOwnProperty(6)] })

// an Array of another realm, and its Array.prototype with an index
var other = newRealm('var a = new Array(); for (var i = 4; i >= 0; i--) a[i] = "r" + i; a[8] = "r8"; Array.prototype[6] = "rp6"; a')
t('realm rd', function () { return [rd(other, 0), rd(other, 4), rd(other, 5), rd(other, 6), rd(other, 8), rdt(other, 6), other.length] })
t('realm wr', function () { return [wr(other, 2, 'x'), wrs(other, 6, 'y'), wr(other, 10, 'z'), other.length, other.hasOwnProperty(6)] })
t('realm proto not ours', function () { var a = holey(); Array.prototype[6] = 'ours6'; try { return [rd(a, 6), rd(other, 7)] } finally { delete Array.prototype[6] } })

// a long loop over a slow array, as crypto.js runs
t('loop', function () {
  var a = down(64), b = new Array(), s = 0
  for (var i = 63; i >= 0; i--) b[i] = 0
  for (var k = 0; k < 3; k++) for (var i = 0; i < 64; i++) { b[i] = (b[i] + a[i] * 3 + k) & 0xffff; s += b[i] }
  return [s, b[63], b.length]
})
out.join('\n')
