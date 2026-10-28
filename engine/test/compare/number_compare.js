// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// Every pair of a set of numbers (ints, floats, ints that came out as floats,
// signed zeros, NaN, infinities, subnormals and the edges of the 32 bit and
// the 53 bit ranges) and of a few other values (booleans, strings, null,
// undefined, BigInts, objects), by ==, !=, ===, !==, <, <=, > and >=, in a
// function that gets both from variables so that the interpreter runs the
// operators. Then the order in which valueOf is called by the relational
// operators. The file is what Node 24 prints.
const nums = [
  0, -0, 1, -1, 2, 7, 255, 2 ** 15, 2 ** 31 - 1, 2 ** 31, -(2 ** 31), -(2 ** 31) - 1, 2 ** 32, 2 ** 32 + 1, 2 ** 40, 2 ** 53 - 1, 2 ** 53, 2 ** 53 + 2,
  0.5, -0.5, 1.5, -1.5, 0.1 + 0.2, 0.3, 1e300, -1e300, 5e-324, -5e-324, 1e-320, Infinity, -Infinity, NaN,
  2 ** 31 + 2 ** 31, 4 / 2, 6 / 4, 0 / -1, -0 + 0, 3 * 0.5, 1e21, 1e-7, 123456789.125,
]
const others = [true, false, null, undefined, '', '0', '1', ' 1 ', '1.5', 'abc', '2147483648', 1n, 0n, -1n, 2n ** 64n, { valueOf() { return 1 } }, [], [1], {}]
const vals = nums.concat(others)
function name(v) { return Object.is(v, -0) ? '-0' : typeof v === 'string' ? JSON.stringify(v) : typeof v === 'bigint' ? v + 'n' : typeof v === 'object' && v ? (Array.isArray(v) ? '[' + v + ']' : v.valueOf === Object.prototype.valueOf ? '{}' : '{v}') : String(v) }
function ops(a, b) {
  let r = ''
  r += a == b ? 'E' : '.'
  r += a != b ? 'N' : '.'
  r += a === b ? 'S' : '.'
  r += a !== b ? 'D' : '.'
  r += a < b ? 'l' : '.'
  r += a <= b ? 'L' : '.'
  r += a > b ? 'g' : '.'
  r += a >= b ? 'G' : '.'
  return r
}
const out = []
for (let i = 0; i < vals.length; i++) {
  const cells = []
  for (let j = 0; j < vals.length; j++) {
    let c
    try { c = ops(vals[i], vals[j]) } catch (e) { c = e.name }
    cells.push(c)
  }
  out.push(name(vals[i]) + ' ' + cells.join(' '))
}
// the same inside a loop with a counter, for the paths of a loop condition (a compare that jumps)
{
  let c = 0
  for (let x = 0.5; x < 20; x += 1.25) { if (x > 10) c += 100; if (x <= 3.75) c += 1; if (x == 2.25) c += 1000; if (x === 6.5) c += 10000 }
  for (let x = 20; x > 0.5; x -= 3) { if (x >= 11) c += 7; if (x != 8) c += 1 }
  for (let x = 1; x !== 1e6; x *= 10) c += 3
  for (let x = 0.1; x < 0.5; x += 0.1) c += 5
  out.push('loops ' + c)
}
// NaN: every comparison is false except !=, !==
{
  const n = NaN, f = 1.5, i = 1
  out.push('nan ' + [n == n, n != n, n === n, n !== n, n < n, n <= n, n > n, n >= n, f < n, n < f, i < n, n < i, i >= n, n >= i, n == i, i != n].join())
}
// order of valueOf in the relational operators, with a float on the other side
{
  const log = []
  const mk = (tag, v) => ({ valueOf() { log.push(tag); return v } })
  for (const [name, f] of [['<', (a, b) => a < b], ['<=', (a, b) => a <= b], ['>', (a, b) => a > b], ['>=', (a, b) => a >= b], ['==', (a, b) => a == b], ['!=', (a, b) => a != b]]) {
    log.length = 0
    const r1 = f(mk('a', 1.5), mk('b', 2.5))
    const r2 = f(mk('a', 2), 1.5)
    const r3 = f(0.25, mk('b', 3))
    out.push('valueOf ' + name + ' ' + r1 + ' ' + r2 + ' ' + r3 + ' ' + log.join(''))
  }
}
// a throwing valueOf leaves the float on the stack to be freed, and the error is the right one
{
  const bad = { valueOf() { throw new TypeError('v') } }
  for (const f of [(a, b) => a < b, (a, b) => a > b, (a, b) => a == b, (a, b) => b >= a]) {
    for (const x of [1.5, 1, 'x', null]) {
      try { out.push('throw ' + f(x, bad)) } catch (e) { out.push('throw ' + e.name) }
    }
  }
}
out.join('\n')
