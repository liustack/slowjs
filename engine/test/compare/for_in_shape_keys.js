// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// for in over objects that share a shape, and over objects whose shape
// changes between loops and during a loop: a for in loop keeps in a shape
// whether it lists the string keys of its objects from the shape, and what it
// keeps must follow the keys and flags of the shape.
var out = []
function keys(o) {
  var k = []
  for (var x in o) k.push(x)
  return k.join(',')
}
function make() { var o = {}; o.a = 1; o.b = 2; return o }

// the same shape, many times
for (var i = 0; i < 3; i++) out.push('same ' + keys(make()))

// an object of that shape gets an array index, a symbol, a key that is not
// enumerable, a getter, then loses a key
var o = make()
out.push('base ' + keys(o))
o[1] = 'one'
out.push('index ' + keys(o))
o = make()
o[Symbol('s')] = 1
out.push('symbol ' + keys(o))
o = make()
Object.defineProperty(o, 'c', { value: 3, enumerable: false })
out.push('hidden ' + keys(o))
o = make()
Object.defineProperty(o, 'a', { enumerable: false })
out.push('a hidden ' + keys(o) + ' then ' + keys(make()))
o = make()
Object.defineProperty(o, 'g', { get: function () { return 1 }, enumerable: true })
out.push('getter ' + keys(o))
o = make()
delete o.a
out.push('deleted ' + keys(o) + ' then ' + keys(make()))

// a shape with an index first, then the same keys without it
function withIndex() { var o = {}; o[0] = 0; o.a = 1; return o }
out.push('with index ' + keys(withIndex()) + ' ' + keys(withIndex()))

// keys added and deleted while a loop runs
o = make()
var seen = []
for (var x in o) { seen.push(x); if (x === 'a') { o.c = 3; delete o.b } }
out.push('during ' + seen.join(',') + ' after ' + keys(o))

// objects that share a shape, one of them changing in the loop over the other
var p = make(), q = make()
seen = []
for (var x in p) { seen.push(x); q[2] = 2; q.z = 1 }
out.push('shared ' + seen.join(',') + ' q ' + keys(q) + ' p ' + keys(p))

// keys of the prototype, and a prototype that gets one later
var proto = { inherited: 1 }
var c = Object.create(proto)
c.a = 1
out.push('proto ' + keys(c))
var d = Object.create({})
d.a = 1
out.push('plain proto ' + keys(d))
Object.getPrototypeOf(d).later = 1
out.push('proto later ' + keys(d))

// arrays, arguments, strings, typed arrays and the global object
out.push('array ' + keys([1, 2]))
out.push('args ' + (function () { return keys(arguments) })(1, 2))
out.push('string ' + keys(new String('ab')))
out.push('typed ' + keys(new Uint8Array(2)))
globalThis.forInShapeKeys = 1
out.push('global ' + (keys(globalThis).indexOf('forInShapeKeys') >= 0))

// many keys, then the same keys again
function wide() { var o = {}; for (var i = 0; i < 40; i++) o['k' + i] = i; return o }
out.push('wide ' + keys(wide()).length + ' ' + keys(wide()).length)
out.join('\n')
