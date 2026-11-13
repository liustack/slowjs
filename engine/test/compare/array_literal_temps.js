// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// Array literals made of temps, locals, arguments and constants: strings,
// ropes, objects, functions, symbols and BigInts whose only reference is the
// element, elements whose code collects or drops what an earlier element
// holds, literals of 0 to 40 elements, nested literals, literals that throw
// halfway and literals on a stack deep enough to be folded.
var out = []
function show(name, f) {
  try { out.push(name + ' ' + f()) } catch (e) { out.push(name + ' threw ' + e) }
}
function rope(n) { var s = ''; for (var i = 0; i < n; i++) s += 'abcdefghijklmnopqrstuvwxyz'; return s }
function kind(v) {
  if (v === null) return 'null'
  if (typeof v === 'string') return 's' + v.length
  if (typeof v === 'object') return Array.isArray(v) ? 'a' + v.length : 'o' + Object.keys(v).length
  return typeof v
}
function kinds(a) { return a.length + ':' + a.map(kind).join(',') }

show('kinds', function () {
  var o = { x: 1 }, s = 'str', sym = Symbol('k')
  var a = [o, s, sym, 1n << 70n, function () { return 7 }, 1.5, -0, null, undefined, true, rope(30) + rope(30)]
  gc()
  return kinds(a) + ' ' + (a[0] === o) + ' ' + (a[2] === sym) + ' ' + a[4]() + ' ' + (1 / a[6]) + ' ' + a[10].slice(-3)
})
show('only reference', function () {
  var a = [{ v: 1 }, ['p', 'q'].join('-'), [1, 2, [3]], rope(25) + rope(25), Symbol('t')]
  gc()
  return kinds(a) + ' ' + a[0].v + ' ' + a[2][2][0] + ' ' + String(a[4])
})
show('arguments', function () {
  function f(a, b, c) { var r = [a, b, c, arguments.length]; a = null; b = null; gc(); return r }
  var r = f({ q: 1 }, 'two', [3])
  return kinds(r) + ' ' + r[0].q + ' ' + r[1] + ' ' + r[2][0]
})
show('drop halfway', function () {
  var h = { a: { name: 'first' }, b: rope(40) + rope(40) }
  var a = [h.a, h.b, (h.a = null, h.b = null, gc(), 'third'), h.a]
  gc()
  return kinds(a) + ' ' + a[0].name + ' ' + a[1].length
})
show('the same value twice', function () {
  var o = {}
  var a = [o, o, o]
  o = null
  gc()
  a[0].k = 1
  return a[1].k + a[2].k + ' ' + (a[0] === a[2])
})
show('lengths', function () {
  var r = []
  for (var n = 0; n <= 40; n += 8) {
    var a = Function('o', 'return [' + Array.from({ length: n }, function (_, i) { return i % 2 ? 'o' : "'e" + i + "'" }).join(',') + ']')({ n: n })
    gc()
    r.push(a.length + (n ? '/' + a[n - 1].n : ''))
  }
  var big = [0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32, 33, 34, 35, 36, 37, 38, 39]
  r.push(big.length + '=' + big.reduce(function (s, v) { return s + v }, 0))
  return r.join(' ')
})
show('nested', function () {
  var x = 'x'
  var a = [[x, [x + 1, [{ x: x }]]], [], [[]]]
  gc()
  return JSON.stringify(a)
})
show('throws halfway', function () {
  var made = []
  function mk(v) { var o = { v: v }; made.push(o); return o }
  try {
    var a = [mk(1), mk(2), (function () { throw new Error('third') })(), mk(4)]
  } catch (e) {
    gc()
    return e.message + ' ' + made.length + ' ' + made.map(function (o) { return o.v }).join()
  }
})
show('a getter in an element', function () {
  var o = { get g() { o = null; gc(); return 'got' } }
  var keep = o
  var a = [keep, o.g, keep]
  return kinds(a) + ' ' + (a[0] === a[2]) + ' ' + a[1]
})
show('spread among them', function () {
  var s = ['m', 'n']
  var a = [{ i: 0 }, ...s, 'z', ...'pq']
  gc()
  return kinds(a)
})
show('loop', function () {
  var keep = [], last
  for (var i = 0; i < 300; i++) {
    last = [i, 's' + i, { i: i }]
    if (i % 50 === 0) keep.push(last)
  }
  gc()
  return keep.map(function (a) { return a[0] + a[1] + a[2].i }).join(',') + ' ' + last[2].i
})
show('folded', function () {
  // more values on the stack than the temps of a frame, so the literals are folded
  var deep = 'var v = {k: 1}; return ' + '[v, '.repeat(130) + 'v' + ']'.repeat(130)
  var r2 = Function(deep)()
  gc()
  var d = 0, x = r2
  while (Array.isArray(x)) { d++; x = x[1] }
  return d + ' ' + x.k + ' ' + (r2[0] === r2[1][0])
})
out.join('\n')
