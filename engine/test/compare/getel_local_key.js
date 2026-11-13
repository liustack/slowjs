// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// o[k] with the key in a local or an argument: strings, numbers, symbols and
// objects whose toString runs code, which may throw, collect, change the
// object, or write the key through a closure or the arguments object, and
// getters and Proxy traps that run after the key is made.
var out = []
function show(name, f) {
  try { out.push(name + ' ' + f()) } catch (e) { out.push(name + ' threw ' + e) }
}
function read(o, k) { return o[k] }
function readLocal(o, n) { var k = 'key' + n; var v = o[k]; return v + ':' + k }

var plain = { key1: 'one', key2: 'two', 3: 'three' }
var arr = ['a', 'b', 'c']
var sym = Symbol('s')
plain[sym] = 'symbol'
show('strings', function () { return [read(plain, 'key1'), readLocal(plain, 2), read(plain, 'nope')].join() })
show('numbers', function () { return [read(plain, 3), read(arr, 1), read(arr, '2'), read(arr, 1.5), read(arr, -0)].join() })
show('symbol', function () { return read(plain, sym) })
show('null key', function () { return [read(plain, null), read(plain, undefined)].join() })
show('null object', function () { return read(null, 'key1') })

// keys whose toString runs code
show('key throws', function () { return read(plain, { toString: function () { throw new Error('key') } }) })
show('key collects', function () {
  var k = { toString: function () { gc(); return 'key1' } }
  return [read(plain, k), read(plain, k)].join()
})
show('key changes object', function () {
  var o = { a: 1 }
  return read(o, { toString: function () { o.a = 2; delete o.b; return 'a' } })
})
show('key in a closure', function () {
  var k = { toString: function () { k = 'gone'; return 'key2' } }
  function get() { return plain[k] }
  return get() + ' ' + k
})
show('key through arguments', function () {
  function f(k, o) {
    var a = arguments
    k = { toString: function () { a[0] = 'other'; gc(); return 'key1' } }
    return o[k] + ' ' + k
  }
  return f(0, plain)
})
show('valueOf', function () {
  var calls = []
  var k = {
    toString: function () { calls.push('toString'); return {} },
    valueOf: function () { calls.push('valueOf'); gc(); return 0 }
  }
  return read(arr, k) + ' ' + calls.join()
})

// what runs after the key is made
show('getter', function () {
  var key = 'g'
  var o = {}
  Object.defineProperty(o, 'g', { get: function () { key = null; gc(); return 'got' } })
  return o[key] + ' ' + key
})
show('proxy', function () {
  var key = 'p'
  var seen = []
  var o = new Proxy({}, { get: function (t, k) { seen.push(typeof k + ' ' + String(k)); key = 1; return 'trap' } })
  var r = [o[key], o[key], o[sym]]
  return r.join() + ' ' + seen.join(' ')
})
show('string object', function () {
  var s = new String('abc')
  var k = 1
  return [s[k], 'xyz'[k], s['length']].join()
})
// a loop over keys of a local, many times
show('loop', function () {
  var o = { a: 1, b: 2, c: 3 }, sum = 0
  for (var i = 0; i < 300; i++) {
    var k = 'abc'[i % 3]
    sum += o[k]
  }
  return sum
})
out.join('\n')
