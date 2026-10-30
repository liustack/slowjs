// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// TypedArray.prototype.at counts from the length the array had before it
// converted its argument. A conversion that resizes the buffer does not move a
// negative index (upstream QuickJS 8ac9d9b). The file is what Node 24 prints.
const out = []
function run(name, f) {
  let r
  try { r = f() } catch (e) { r = 'throws ' + e.name }
  out.push(name + ': ' + String(r))
}
function mk(len, max, ctor = Int8Array) {
  const b = new ArrayBuffer(len * ctor.BYTES_PER_ELEMENT, { maxByteLength: max * ctor.BYTES_PER_ELEMENT })
  const a = new ctor(b)
  for (let i = 0; i < a.length; i++) a[i] = i + 1
  return [b, a]
}
for (const i of [0, 1, 3, 4, -1, -2, -4, -5, 1.9, -1.9, NaN, Infinity, -Infinity, '2', null, undefined]) {
  run('plain at(' + i + ')', () => mk(4, 8)[1].at(i))
}
for (const [from, to] of [[4, 2], [4, 0], [4, 8], [2, 8]]) {
  for (const i of [0, 1, 3, -1, -2, -4]) {
    run('resize ' + from + ' to ' + to + ' at(' + i + ')', () => {
      const [b, a] = mk(from, 8)
      return a.at({ valueOf() { b.resize(to); return i } })
    })
  }
}
run('fixed length shrunk', () => {
  const b = new ArrayBuffer(4, { maxByteLength: 8 })
  const a = new Int8Array(b, 0, 4)
  a.set([1, 2, 3, 4])
  return a.at({ valueOf() { b.resize(2); return -1 } })
})
run('detached by the argument', () => {
  const [b, a] = mk(4, 8)
  return a.at({ valueOf() { b.transfer(); return 0 } })
})
run('detached before', () => {
  const [b, a] = mk(4, 8)
  b.transfer()
  return a.at(0)
})
for (const C of [Uint8Array, Int16Array, Float64Array, Float32Array]) {
  run(C.name + ' resized', () => {
    const [b, a] = mk(4, 8, C)
    return a.at({ valueOf() { b.resize(2 * C.BYTES_PER_ELEMENT); return -1 } })
  })
}
out.join('\n')
