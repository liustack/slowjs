// Copyright (c) 2026 The liustack/beastjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.
// +, - and * of numbers that are not both ints, in each form the register
// loop has for them: both operands in slots and the result in a temp (ADD,
// SUB, MUL), the result into a local that held a counted value (ADDL, SUBL),
// a constant operand (ADDK, SUBK, MULK) and both (ADDKL, SUBKL), and the
// constant on the left (KADD, KSUB). The numbers are the edges: -0, NaN, the
// infinities, the largest and smallest doubles, 2^53 and its neighbours,
// floats that are whole numbers, ints that overflow into a float, ints with
// floats. Then the values that are not numbers: strings, booleans, null,
// undefined, objects whose valueOf runs (in order) or throws, an array, a
// BigInt (a TypeError with a number) and a Symbol. Then warm loops that sum
// and scale arrays of floats. Each result is written exactly (String of the
// number, -0 apart), so a result one bit off shows. The file is what Node 24
// prints.
const out = []
function name(v) {
  if (typeof v === 'number') return Object.is(v, -0) ? '-0' : String(v)
  if (typeof v === 'bigint') return v + 'n'
  if (typeof v === 'string') return JSON.stringify(v)
  if (typeof v === 'symbol') return 'sym'
  if (Array.isArray(v)) return '[' + v + ']'
  if (typeof v === 'object' && v) return '{' + v.tag + '}'
  return String(v)
}
const nums = [
  0.5, -0.5, 1.5, 0.1, 0.2, -0, NaN, Infinity, -Infinity,
  1e308, Number.MAX_VALUE, Number.MIN_VALUE, 2.2250738585072014e-308,
  2 ** 53, 2 ** 53 - 1, -(2 ** 53), 2 ** 52 + 0.5, 2 ** 31, -(2 ** 31) - 1, 1e21,
  0.5 + 0.5, 1.5 - 1.5, 1e300 * 1e10,
  0, 1, -1, 2, 2147483647, -2147483648, 46341,
]
const log = []
const others = [
  '1.5', '-0', 'abc', '', ' 2 ', true, false, null, undefined,
  { tag: 'v2.5', valueOf() { log.push('a'); return 2.5 } },
  { tag: 'v3', valueOf() { log.push('b'); return 3 } },
  { tag: 'vs', valueOf() { log.push('c'); return '0.5' } },
  { tag: 'thr', valueOf() { log.push('t'); throw new RangeError('valueOf') } },
  [0.25], 1n, -(2n ** 70n), Symbol('s'),
]
function cell(f, a, b) {
  try { return name(f(a, b)) } catch (e) { return e.name }
}
function table(label, f, rows, cols) {
  for (const a of rows) {
    log.length = 0
    const cells = cols.map(b => cell(f, a, b))
    out.push(label + ' ' + name(a) + ': ' + cells.join(' ') + (log.length ? ' log ' + log.join('') : ''))
  }
}
// both operands in slots, the result in a temp
function add(a, b) { return a + b }
function sub(a, b) { return a - b }
function mul(a, b) { return a * b }
// the result into a local that holds a counted value first (the L forms)
function addl(a, b) { let r = [a]; r = a + b; return r }
function subl(a, b) { let r = [a]; r = a - b; return r }
function mull(a, b) { let r = [a]; r = a * b; return r }
table('add', add, nums, nums)
table('sub', sub, nums, nums)
table('mul', mul, nums, nums)
table('add', add, nums, others)
table('add', add, others, nums)
table('sub', sub, nums, others)
table('sub', sub, others, nums)
table('mul', mul, nums, others)
table('mul', mul, others, nums)
table('add', add, others, others)
table('mul', mul, others, others)
const few = [0.5, -0.5, -0, NaN, Infinity, -Infinity, 1e308, Number.MIN_VALUE, 2 ** 53, 2 ** 31, 0.5 + 0.5, 0, 1, -1, 2147483647, -2147483648, 46341]
table('addl', addl, few, few)
table('subl', subl, few, few)
table('mull', mull, few, few)
table('addl', addl, few, others)
table('subl', subl, others, few)
table('mull', mull, few, others)
// a constant operand, and a constant on the left
function addk(a) { return [a + 1.5, a + -0.5, a + 0.1, a + 1e308, a + 9007199254740992, a + 2147483648, a + 1, a + 0, a + -1] }
function subk(a) { return [a - 1.5, a - -0.5, a - 0.1, a - 1e308, a - 9007199254740992, a - 2147483648, a - 1, a - 0, a - -1] }
function mulk(a) { return [a * 1.5, a * -0.5, a * 0.1, a * 1e308, a * 9007199254740992, a * 2147483648, a * 1, a * 0, a * -1] }
function addkl(a) { let r = [a], s = [a], t = [a]; r = a + 1.5; s = a + 1e-300; t = a + 0; return [r, s, t] }
function subkl(a) { let r = [a], s = [a], t = [a]; r = a - 1.5; s = a - 1e-300; t = a - 0; return [r, s, t] }
function kadd(a) { return [1.5 + a, -0.5 + a, 1e308 + a, 2147483647 + a] }
function ksub(a) { return [1.5 - a, -0.5 - a, 1e308 - a, -2147483648 - a] }
for (const [label, f] of [['addk', addk], ['subk', subk], ['mulk', mulk], ['addkl', addkl], ['subkl', subkl], ['kadd', kadd], ['ksub', ksub]]) {
  for (const a of nums.concat(others)) {
    log.length = 0
    let c
    try { c = f(a).map(name).join(' ') } catch (e) { c = e.name }
    out.push(label + ' ' + name(a) + ': ' + c + (log.length ? ' log ' + log.join('') : ''))
  }
}
// the order of valueOf, and a throw on either side
{
  const o1 = others[9], o2 = others[10], bad = others[12]
  for (const [label, f] of [['add', add], ['sub', sub], ['mul', mul], ['addl', addl], ['subl', subl], ['mull', mull]]) {
    log.length = 0
    const r = [cell(f, o1, o2), cell(f, o2, o1), cell(f, o1, 0.5), cell(f, 0.5, o2), cell(f, bad, o1), cell(f, o1, bad), cell(f, 1.5, bad)]
    out.push('order ' + label + ': ' + r.join(' ') + ' log ' + log.join(''))
  }
}
// BigInts with BigInts work, with a number they throw
{
  const r = []
  for (const [x, y] of [[1n, 2n], [2n ** 64n, 3n], [-5n, 7n], [1n, 1.5], [1.5, 1n], [-0, 0n], [NaN, 1n]]) {
    r.push(cell(add, x, y), cell(sub, x, y), cell(mul, x, y))
  }
  out.push('bigint: ' + r.join(' '))
}
// warm loops over floats, as numeric code runs them
{
  const n = 64, x = [], y = [], z = []
  for (let i = 0; i < n; i++) { x.push(i * 0.37 - 5.5); y.push((i % 7) * 1.25 + 0.001 * i); z.push(i % 3 === 0 ? i : i / 8) }
  let s = 0, p = 1, q = 0, w = -0
  for (let k = 0; k < 50; k++) {
    for (let i = 1; i < n - 1; i++) {
      const c = (x[i - 1] + y[i] * 0.25 + x[i + 1]) * 0.33
      x[i] = c - z[i] * 1e-3
      s = s + c
      p = p * 1.0001 - c * 1e-9
      q = q - y[i] + z[i]
      w = w * 0.5
    }
  }
  out.push('loop ' + name(s) + ' ' + name(p) + ' ' + name(q) + ' ' + name(w) + ' ' + x.map(name).join(' '))
  // a sum that starts as ints and overflows into a float, then comes back to whole floats
  let a = 2147483000, b = -2147483000, m = 3
  for (let i = 0; i < 2000; i++) { a = a + i; b = b - i; m = m * 3; if (m > 1e20) m = m * 1e-13 }
  out.push('overflow ' + name(a) + ' ' + name(b) + ' ' + name(m) + ' ' + name(a + b) + ' ' + name(a * b))
}
out.join('\n')
