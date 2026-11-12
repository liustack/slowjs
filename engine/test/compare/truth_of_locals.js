// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// The truth of a local, an argument and a temp in the branches of the
// register loop (if, while, !, &&, ||, the conditional operator) and in the
// built-ins that ask it (Boolean, filter, find, some, every): strings, ropes,
// objects, functions, symbols, BigInts and numbers, and values whose last
// reference goes while their truth is used.
var out = []
function show(name, f) {
  try { out.push(name + ' ' + f()) } catch (e) { out.push(name + ' threw ' + e) }
}
function truth(v) {
  var r = []
  if (v) r.push('if'); else r.push('else')
  r.push(!v ? 'not' : 'yes')
  var a = v && 'and'
  r.push(typeof a)
  var o = v || 'or'
  r.push(typeof o)
  var n = 0
  while (v && n < 2) n++
  r.push(n)
  return r.join('/')
}
function rope(n) { var s = ''; for (var i = 0; i < n; i++) s += 'abcdefghijklmnopqrstuvwxyz'; return s }
// rope(30) + rope(30) is a rope: both sides are longer than JS_STRING_ROPE_SHORT_LEN (512)
var values = ['', 'a', rope(20), rope(30) + rope(30), {}, [], function () {}, Symbol('s'), 0n, 12345678901234567890n, -1n, 0, -0, NaN, 1.5, null, undefined, true, false]
for (var i = 0; i < values.length; i++) {
  show(String(i), function () { return truth(values[i]) })
}
show('arguments', function () {
  function f(a, b) { return (a ? 'a' : '-') + (b ? 'b' : '-') + (!a) + (!b) }
  return [f('', 'x'), f(rope(10), ''), f({}, null)].join(' ')
})
show('dropped', function () {
  var holder = { s: rope(30) + rope(30), o: {} }
  function test(h) {
    var s = h.s, o = h.o
    h.s = null; h.o = null
    gc()
    return (s ? 1 : 0) + (o ? 2 : 0) + (!s ? 4 : 0)
  }
  return test(holder)
})
show('builtins', function () {
  var a = ['', 'x', 0, {}, null, rope(3), 0n, 1n]
  return [
    a.map(Boolean).join(),
    a.filter(function (v) { return v }).length,
    a.findIndex(function (v) { return typeof v === 'string' && v }),
    a.some(function (v) { return v }),
    a.every(function (v) { return v })
  ].join(' ')
})
show('loop', function () {
  var n = 0, s = 'x', e = ''
  for (var i = 0; i < 500; i++) {
    if (s) n++
    if (e) n += 100
    if (!e) n++
  }
  return n
})
out.join('\n')
