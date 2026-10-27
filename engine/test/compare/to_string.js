// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// Objects converted to a string and to a property key: through
// Symbol.toPrimitive, toString and valueOf, giving each kind of primitive.
// A symbol is a key but not a string, and an object that gives an object
// throws.
const sym = Symbol('s')
const prims = [1, -0, 1.5, 2 ** 31, true, null, undefined, 'str', 'a'.repeat(3000) + 'b'.repeat(3000), 10n ** 30n, sym]
const show = v => typeof v === 'symbol' ? v.toString() : typeof v === 'string' && v.length > 20 ? v.length + ' chars' : String(v)
const conv = {
  String: o => String(o),
  template: o => `${o}`,
  concat: o => 'x'.concat(o),
  key: o => { const t = {}; t[o] = 1; return show(Reflect.ownKeys(t)[0]) },
  in: o => String(o in { 1: 0, 'str': 0 }),
  join: o => [o, o].join('|'),
}
const makers = {
  toPrimitive: v => ({ [Symbol.toPrimitive](hint) { return hint === 'string' ? v : 'no' } }),
  toString: v => ({ toString() { return v }, valueOf() { return 'no' } }),
  valueOf: v => ({ toString: undefined, valueOf() { return v } }),
}
const out = []
for (const [mk, make] of Object.entries(makers)) {
  for (const v of prims) {
    const line = []
    for (const [cn, c] of Object.entries(conv)) {
      let r
      try { r = show(c(make(v))) } catch (e) { r = e.name }
      line.push(cn + '=' + (r.length > 40 ? r.length + ' chars' : r))
    }
    out.push(mk + ' ' + show(v) + ': ' + line.join(' '))
  }
}
const bad = { toString() { return {} }, valueOf() { return {} } }
for (const [cn, c] of Object.entries(conv)) {
  try { out.push('object ' + cn + ': ' + c(bad)) } catch (e) { out.push('object ' + cn + ': ' + e.name) }
}
out.push('array: ' + String([1, [2, [3, sym.description]]]) + ' ' + String(new Date(0).getTime()) + ' ' + String(Object(5n)))
out.join('\n')
