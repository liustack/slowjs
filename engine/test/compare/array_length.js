// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// Values converted to an array length: given to the Array constructor (a
// number is the length, anything else the one element), written to
// 'length', which converts twice and compares, so that valueOf is called
// twice there and may disagree, and defined as 'length'.
const show = v => typeof v === 'object' && v !== null ? 'obj' : typeof v === 'symbol' ? 'sym' : typeof v === 'bigint' ? v + 'n' : Object.is(v, -0) ? '-0' : String(v)
let calls = 0
const counted = v => ({ valueOf() { calls++; return v } })
const vals = [0, 3, -0, 1.5, -1, 2 ** 32 - 1, 2 ** 32, NaN, Infinity, true, false, null, '3', ' 4 ', '0x10', '1e2', '', 'x', '-1',
  [], [5], [1, 2], 2n, Symbol(), undefined]
const out = []
for (const v of [...vals, ...vals.map(counted)]) {
  const name = typeof v === 'object' && v !== null && !Array.isArray(v) ? 'valueOf ' + show(v.valueOf()) : show(v)
  calls = 0
  let ctor, set
  try { ctor = 'len ' + new Array(v).length } catch (e) { ctor = e.name }
  const c1 = calls
  calls = 0
  try { const a = [1, 2, 3, 4, 5]; a.length = v; set = 'len ' + a.length } catch (e) { set = e.name }
  out.push(name + ': ' + ctor + ' (' + c1 + ') | ' + set + ' (' + calls + ')')
}
{
  let n = 0
  const flip = { valueOf() { return n++ === 0 ? 2 : 3 } }
  try { const a = []; a.length = flip; out.push('flip: ' + a.length) } catch (e) { out.push('flip: ' + e.name) }
  const throws = { valueOf() { throw new SyntaxError() } }
  try { new Array(throws) ; out.push('throws ctor: none') } catch (e) { out.push('throws ctor: ' + e.name) }
  try { [].length = throws } catch (e) { out.push('throws set: ' + e.name) }
  try { Object.defineProperty([], 'length', { value: counted(7) }); out.push('define: ok') } catch (e) { out.push('define: ' + e.name) }
  out.push('define len: ' + Object.defineProperty([1, 2, 3], 'length', { value: '1' }).length)
}
out.join('\n')
