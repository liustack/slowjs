// Copyright (c) 2026 The liustack/beastjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.
// transfer keeps the maxByteLength of a resizable ArrayBuffer, and a length
// past it is a RangeError, which AllocateArrayBuffer throws.
// transferToFixedLength has no maximum. The file is what Node 24 prints.
const out = []
function run(name, f) {
  let r
  try { r = f() } catch (e) { r = 'throws ' + e.name }
  out.push(name + ': ' + r)
}
for (const len of [undefined, 0, 1, 2, 3, 2 ** 20]) {
  run('transfer(' + len + ')', () => { const t = new ArrayBuffer(1, { maxByteLength: 2 }).transfer(len); return t.byteLength + ' ' + t.resizable + ' ' + t.maxByteLength })
  run('transferToFixedLength(' + len + ')', () => { const t = new ArrayBuffer(1, { maxByteLength: 2 }).transferToFixedLength(len); return t.byteLength + ' ' + t.resizable })
}
run('the source after a RangeError', () => { const b = new ArrayBuffer(1, { maxByteLength: 2 }); try { b.transfer(3) } catch (e) { } return b.detached + ' ' + b.byteLength })
run('detached first', () => { const b = new ArrayBuffer(1, { maxByteLength: 2 }); b.transfer(); return b.transfer(3) })
run('not resizable', () => new ArrayBuffer(1).transfer(3).byteLength)
out.join('\n')
