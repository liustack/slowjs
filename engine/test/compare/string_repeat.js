// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// String.prototype.repeat of counts of every kind: an empty string repeats any
// finite count into an empty string, a count of Infinity or below 0 throws a
// RangeError, a count that makes the string too long throws one, and the
// conversion of the count is the one of ToIntegerOrInfinity (upstream QuickJS
// 21cbd86). The file is what Node 24 prints.
const out = []
function run(name, f) {
  let r
  try { r = f() } catch (e) { r = 'throws ' + e.name }
  out.push(name + ': ' + String(r))
}
const show = v => typeof v === 'string' ? JSON.stringify(v) : String(v)
for (const n of [0, 1, 2, 3, 2.7, -0, -0.9, NaN, 'x', '2', null, undefined, true, false, [], [3], { valueOf() { return 2 } }]) {
  run('repeat(' + (typeof n === 'object' ? 'object' : show(n)) + ')', () => 'ab'.repeat(n))
}
for (const n of [-1, -Infinity, Infinity, 2 ** 31 - 1, 2 ** 31, 2 ** 32, 2 ** 53, 1e300, -1e300]) {
  run('repeat(' + n + ')', () => 'ab'.repeat(n).length)
  run('empty repeat(' + n + ')', () => ''.repeat(n).length)
  run('one char repeat(' + n + ')', () => 'a'.repeat(n).length)
}
run('too long', () => 'abc'.repeat(2 ** 30).length)
run('too long wide', () => '日'.repeat(2 ** 30).length)
run('just fits', () => 'a'.repeat(2 ** 20).length)
run('one', () => { const s = 'x'.repeat(5); return s.repeat(1) === s })
run('symbol count', () => 'a'.repeat(Symbol()))
run('bigint count', () => 'a'.repeat(1n))
run('symbol this', () => String.prototype.repeat.call(Symbol(), 1))
run('null this', () => String.prototype.repeat.call(null, 1))
run('order', () => {
  const log = []
  try { String.prototype.repeat.call({ toString() { log.push('this'); return '' } }, { valueOf() { log.push('count'); return Infinity } }) } catch (e) { log.push(e.name) }
  return log.join()
})
out.join('\n')
