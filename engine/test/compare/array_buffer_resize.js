// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// ArrayBuffer.prototype.resize reads its length as ToIndex does: a value that
// wraps around 2**64 is not a small length, and a length past the maximum
// throws a RangeError (upstream QuickJS 5b6356a). The file is what Node 24
// prints.
const out = []
function run(name, f) {
  let r
  try { r = f() } catch (e) { r = 'throws ' + e.name }
  out.push(name + ': ' + String(r))
}
const lens = [0, 1, 8, 15, 16, 17, 2.9, -0, -0.5, -1, NaN, Infinity, -Infinity, 2 ** 31, 2 ** 32, 2 ** 32 + 8, 2 ** 53 - 1, 2 ** 53, 2 ** 63,
  2 ** 64, 2 ** 64 + 4096, 2 ** 65 + 8, 2 ** 70, 1e300, '8', '', 'x', null, undefined, true, [], [4], { valueOf() { return 12 } }, 3n, Symbol('s')]
const show = v => typeof v === 'symbol' ? 'Symbol' : typeof v === 'bigint' ? v + 'n' : typeof v === 'string' ? JSON.stringify(v) : Array.isArray(v) ? '[' + v + ']' : typeof v === 'object' && v !== null ? 'object' : String(v)
for (const l of lens) {
  run('resize(' + show(l) + ')', () => {
    const b = new ArrayBuffer(8, { maxByteLength: 16 })
    b.resize(l)
    return b.byteLength
  })
}
run('resize()', () => { const b = new ArrayBuffer(8, { maxByteLength: 16 }); b.resize(); return b.byteLength })
run('max bigger', () => { const b = new ArrayBuffer(8, { maxByteLength: 2 ** 20 }); b.resize(2 ** 20 + 1); return b.byteLength })
run('max equal', () => { const b = new ArrayBuffer(8, { maxByteLength: 2 ** 20 }); b.resize(2 ** 20); return b.byteLength })
run('not resizable', () => new ArrayBuffer(8).resize(4))
run('detached', () => { const b = new ArrayBuffer(8, { maxByteLength: 16 }); b.transfer(); b.resize(4) })
run('length tracking', () => { const b = new ArrayBuffer(8, { maxByteLength: 16 }); const a = new Uint8Array(b); b.resize(12); return a.length })
run('order', () => {
  const b = new ArrayBuffer(8, { maxByteLength: 16 })
  const log = []
  try { b.resize({ valueOf() { log.push('valueOf'); b.transfer(); return 4 } }) } catch (e) { log.push(e.name) }
  return log.join()
})
out.join('\n')
