// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// Iterator.prototype.take and drop: the limit becomes a number before the
// check, NaN, a negative number and a finite number past 2**53-1 throw a
// RangeError, Infinity is the largest limit, and a limit that cannot be a
// number throws what it throws (upstream QuickJS d88b1ed). The file is what
// Node 24 prints, except the lines that start with "ES2026", which are what the
// current specification says and Node 24 does not do yet: a finite limit past
// 2**53-1 throws a RangeError and closes the iterator.
const out = []
function run(name, f) {
  let r
  try { r = f() } catch (e) { r = 'throws ' + e.name }
  out.push(name + ': ' + String(r))
}
const limits = [0, 1, 2, 5, 2.5, -0, -0.5, 0.9, -1, -1.5, NaN, Infinity, -Infinity, 2 ** 31, 2 ** 32, 2 ** 53 - 1,
  '3', ' 2 ', '', 'x', null, undefined, true, false, 10n, {}, [], [2], { valueOf() { return 2 } }, Symbol('s')]
const show = v => typeof v === 'symbol' ? 'Symbol' : typeof v === 'bigint' ? v + 'n' : typeof v === 'string' ? JSON.stringify(v) : Array.isArray(v) ? '[' + v + ']' : typeof v === 'object' && v !== null ? 'object' : String(v)
for (const l of limits) {
  for (const m of ['take', 'drop']) {
    run(m + '(' + show(l) + ')', () => [1, 2, 3, 4].values()[m](l).toArray().join())
  }
}
run('take closes on a bad limit', () => {
  let closed = 0
  const it = { next() { return { done: false, value: 1 } }, return() { closed++; return {} }, __proto__: Iterator.prototype }
  try { it.take(NaN) } catch (e) { }
  try { it.drop(-1) } catch (e) { }
  return closed
})
run('ES2026 closes for a limit past 2**53-1', () => {
  let closed = 0
  const it = { next() { return { done: false, value: 1 } }, return() { closed++; return {} }, __proto__: Iterator.prototype }
  for (const l of [2 ** 53, 2 ** 53 + 2, 1e20]) { for (const m of ["take", "drop"]) { try { it[m](l) } catch (e) { closed += 10 } } }
  return closed
})
for (const l of [2 ** 53, 2 ** 53 + 2, 1e20, 2 ** 70]) {
  for (const m of ["take", "drop"]) run("ES2026 " + m + "(" + l + ")", () => [1, 2, 3, 4].values()[m](l).toArray().join())
}
run('limit conversion order', () => {
  const log = []
  const it = { next() { log.push('next'); return { done: true } }, return() { log.push('return'); return {} }, __proto__: Iterator.prototype }
  try { it.take({ valueOf() { log.push('valueOf'); return NaN } }) } catch (e) { log.push(e.name) }
  return log.join()
})
out.join('\n')
