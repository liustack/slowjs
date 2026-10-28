// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// Equality of every pair of a set of values (all the kinds, strings that are
// atoms, wide, long and ropes, small and big BigInts, equal but distinct
// copies) by ===, !==, ==, !=, Object.is, indexOf, lastIndexOf, includes, Set
// and Map, once on a dense Array and once on an array-like that reads its
// elements through getters. Those are the callers of js_strict_eq2, which
// takes its operands without owning them, so the script also checks that runs
// of them on values made new each time leave nothing in memory (a reference
// not given back keeps a value alive) and free nothing twice. The file is what
// Node 24 prints.
function makeVals() {
  const part = n => 'long string of text '.slice(0, n)
  const long1 = part(20).repeat(8)
  const longEq = [part(20), part(20), part(20), part(20), part(20), part(20), part(20), part(20)].join('')
  // a string over 8192 characters with something added is a rope
  const ropeA = 'x'.repeat(9000), ropeB = 'y'.repeat(10), ropeC = 'z'.repeat(10)
  const rope1 = ropeA + ropeB
  const rope2 = ropeA + ropeB
  const rope3 = ropeA + ropeC
  const flat1 = [ropeA, ropeB].join('')
  const wide1 = 'wide 日本語'
  const wide2 = ['wide ', '日本語'].join('')
  const obj = { a: 1 }
  const sym = Symbol('s')
  const vals = [
    undefined, null, true, false, 0, -0, 1, 1.5, NaN, Infinity, -1, 2 ** 31, 2 ** 53,
    '', 'a', 'b', 'abc', 'ABC'.toLowerCase(), '1', '0', 'x'.repeat(3), wide1, wide2, '\u{1f600}',
    longEq, long1, rope1, rope2, rope3, flat1,
    1n, 0n, -1n, 2n ** 64n, 2n ** 64n, 2n ** 200n, BigInt('1'),
    sym, sym, Symbol('s'), Symbol.for('k'), Symbol.for('k'), Symbol.iterator,
    obj, obj, {}, {}, [1], [1], function () {}, function () {}, new String('a'), new Number(1), Object(1n), Object(sym),
  ]
  return Object.assign(vals, { rope1, rope2, obj, sym })
}
function cell(a, b) {
  const r = []
  r.push(a === b ? 1 : 0)
  r.push(a !== b ? 1 : 0)
  r.push(a == b ? 1 : 0)
  r.push(a != b ? 1 : 0)
  r.push(Object.is(a, b) ? 1 : 0)
  r.push([a].indexOf(b) >= 0 ? 1 : 0)
  r.push([a].lastIndexOf(b) >= 0 ? 1 : 0)
  r.push([a].includes(b) ? 1 : 0)
  r.push(new Set([a]).has(b) ? 1 : 0)
  r.push(new Map([[a, 1]]).has(b) ? 1 : 0)
  return r.join('')
}
function cellLike(a, b) {
  // the same checks on array-likes, whose elements come from getters, so that
  // the generic loops of indexOf, lastIndexOf and includes run
  let reads = 0
  const o = { length: 3, get 0() { reads++; return 'pad' }, get 1() { reads++; return a }, get 2() { reads++; return 'pad' } }
  const r = []
  r.push(Array.prototype.indexOf.call(o, b) === 1 ? 1 : 0)
  r.push(Array.prototype.lastIndexOf.call(o, b) === 1 ? 1 : 0)
  r.push(Array.prototype.includes.call(o, b) ? 1 : 0)
  return r.join('') + (reads > 0 ? '' : '!')
}
function matrix() {
  const vals = makeVals()
  const rows = []
  for (let i = 0; i < vals.length; i++) {
    const cells = []
    for (let j = 0; j < vals.length; j++) cells.push(cell(vals[i], vals[j]) + '/' + cellLike(vals[i], vals[j]))
    rows.push(i + ': ' + cells.join(' '))
  }
  return rows
}
const out = matrix()
const { rope1, rope2, obj, sym } = makeVals()
// searches that start somewhere else, skip holes, and compare against many elements
{
  const a = [1, 'a', , NaN, 1n, 'a', 2n ** 64n, rope1, -0, obj, sym]
  const ks = [undefined, 0, 1, 2, 5, -1, -3, -100, 100, NaN, Infinity, -Infinity, 1.9, '2']
  for (const v of [1, 'a', NaN, 1n, 2n ** 64n, rope2, 0, obj, sym, undefined, 'b']) {
    const row = []
    for (const k of ks) {
      row.push(a.indexOf(v, k) + ',' + a.lastIndexOf(v, k) + ',' + (a.includes(v, k) ? 1 : 0))
    }
    out.push('search ' + (typeof v === 'symbol' ? 'sym' : typeof v === 'object' && v ? 'obj' : String(v).slice(0, 8)) + ': ' + row.join(' '))
  }
  const sparse = [, , 1, , 'x']
  out.push('sparse: ' + sparse.indexOf(undefined) + ' ' + sparse.includes(undefined) + ' ' + sparse.lastIndexOf(undefined) + ' ' + sparse.indexOf('x') + ' ' + sparse.lastIndexOf(1))
  const big = Array.from({ length: 1000 }, (_, i) => (i % 3 ? 'k' + i : i % 5 ? i : BigInt(i)))
  out.push('big: ' + big.indexOf('k998') + ' ' + big.indexOf(999n) + ' ' + big.lastIndexOf(0n) + ' ' + big.includes(990) + ' ' + big.indexOf('k' + 997))
}
// exceptions while looking: a getter that throws, one that changes the array
{
  const thrower = { length: 3, 0: 'a', get 1() { throw new RangeError('x') }, 2: 'c' }
  for (const f of ['indexOf', 'lastIndexOf', 'includes']) {
    try { out.push(f + ' ' + Array.prototype[f].call(thrower, 'z')) } catch (e) { out.push(f + ' throws ' + e.name) }
  }
  const a = [1, 2, 3, 4]
  const sneaky = { valueOf() { a.length = 1; return 3 } }
  out.push('from index changes array: ' + a.indexOf(4, sneaky) + ' ' + a.length)
  const b = [1, 2, 3, 4]
  const sneaky2 = { valueOf() { b.length = 0; return 0 } }
  out.push('from index empties array: ' + b.includes(1, sneaky2) + ' ' + b.length)
}
// memory: after a warm-up, more runs of the same checks leave the heap as it was
if (typeof memorySize === 'function') {
  matrix(); matrix(); gc()
  matrix(); matrix(); matrix(); gc()
  const m1 = memorySize()
  matrix(); matrix(); matrix(); gc()
  out.push('memory after: ' + (memorySize() - m1))
} else {
  out.push('memory after: 0')
}
out.join('\n')
