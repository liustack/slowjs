// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// o[k] = v with the key in a local or an argument: strings, numbers,
// symbols and objects whose toString runs code, which may throw, collect,
// change the object, or write the key through a closure or the arguments
// object, and setters and Proxy traps that run after the key is made.
var out = []
function show(name, f) {
  try { out.push(name + ' ' + f()) } catch (e) { out.push(name + ' threw ' + e) }
}
function write(o, k, v) { o[k] = v; return o }
function writeLocal(o, n, v) { var k = 'key' + n; o[k] = v; return k + '=' + o[k] }
function keys(o) { return Object.keys(o).map(function (k) { return k + ':' + String(o[k]) }).join(',') }

var sym = Symbol('s')
show('strings', function () { var o = {}; write(o, 'a', 1); write(o, 'b', 'two'); return keys(o) + ' ' + writeLocal(o, 3, 'three') })
show('numbers', function () {
  var arr = ['a', 'b', 'c'], o = {}
  write(arr, 1, 'B'); write(arr, '2', 'C'); write(arr, 1.5, 'half'); write(arr, -0, 'zero'); write(o, 7, 'seven')
  return arr.join() + ' ' + arr[1.5] + ' ' + keys(o)
})
show('symbol', function () { var o = write({}, sym, 'symbol'); return o[sym] })
show('null key', function () { var o = {}; write(o, null, 1); write(o, undefined, 2); return keys(o) })
show('null object', function () { return write(null, 'k', 1) })
show('key is the value', function () { var o = {}, k = 'self'; o[k] = k; return keys(o) })
show('object is the value', function () { var o = {}, k = 'me'; o[k] = o; return o.me === o })

// keys whose toString runs code
show('key throws', function () { return write({}, { toString: function () { throw new Error('key') } }, 1) })
show('key collects', function () {
  var k = { toString: function () { gc(); return 'c' } }, o = {}
  write(o, k, { v: 1 }); gc()
  return keys(o) + ' ' + o.c.v
})
show('key changes object', function () {
  var o = { a: 1, b: 2 }
  write(o, { toString: function () { o.a = 3; delete o.b; return 'z' } }, 4)
  return keys(o)
})
show('key in a closure', function () {
  var k = { toString: function () { k = 'gone'; gc(); return 'kc' } }
  var o = {}
  function put() { o[k] = 'v' }
  put()
  return keys(o) + ' ' + k
})
show('key through arguments', function () {
  function f(k, o) {
    var a = arguments
    k = { toString: function () { a[0] = 'other'; gc(); return 'ka' } }
    o[k] = 1
    return keys(o) + ' ' + k
  }
  return f(0, {})
})
show('key through a direct eval', function () {
  function f(o) {
    var k = { toString: function () { w(); gc(); return 'ke' } }
    eval('var w = function () { k = "changed" }')
    o[k] = 1
    return keys(o) + ' ' + k
  }
  return f({})
})
show('key is the object', function () { var o = {}; o[o] = 1; return keys(o) })
show('object key is the value', function () {
  var o = {}, k = { toString: function () { gc(); return 'kv' } }
  o[k] = k
  return Object.keys(o) + ' ' + (o.kv === k)
})
show('valueOf', function () {
  var calls = [], arr = [0, 0, 0]
  var k = {
    toString: function () { calls.push('toString'); return {} },
    valueOf: function () { calls.push('valueOf'); gc(); return 1 }
  }
  write(arr, k, 'one')
  return arr.join() + ' ' + calls.join()
})

// what runs after the key is made
show('setter', function () {
  var key = 's', seen = []
  var o = {}
  Object.defineProperty(o, 's', { set: function (v) { seen.push(v); key = null; gc() } })
  o[key] = 'set'
  return seen.join() + ' ' + key
})
show('proxy', function () {
  var key = 'p', seen = []
  var t = {}
  var o = new Proxy(t, { set: function (tt, k, v) { seen.push(typeof k + ' ' + String(k) + '=' + v); key = 1; gc(); tt[k] = v; return true } })
  o[key] = 'a'; o[key] = 'b'; o[sym] = 'c'
  return seen.join(' ') + ' ' + keys(t)
})
show('frozen', function () {
  'use strict'
  var o = Object.freeze({ f: 1 }), k = 'f'
  o[k] = 2
  return 'no throw'
})
show('frozen sloppy', function () {
  var o = Object.freeze({ f: 1 }), k = 'f'
  o[k] = 2
  return o.f
})
show('string object', function () {
  var s = new String('abc'), k = 'x'
  s[k] = 1
  return s.x + ' ' + s.length
})
// a loop over keys of a local, many times
show('loop', function () {
  var o = {}
  for (var i = 0; i < 300; i++) {
    var k = 'abc'[i % 3]
    o[k] = (o[k] | 0) + i
  }
  return keys(o)
})
out.join('\n')
