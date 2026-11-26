// Copyright (c) 2026 The liustack/beastjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.
// The maxByteLength option of the ArrayBuffer constructor goes through ToIndex:
// a number that does not fit in an integer of 64 bits is a RangeError, it does
// not wrap around to a small one (upstream QuickJS 66fdddf). The file is what
// Node 24 prints.
const out = []
for (const len of [0, 4]) {
  for (const m of [undefined, 0, 4, 8, 8.9, -0.5, '16', 2 ** 53 - 1, 2 ** 53, 2 ** 63, 2 ** 64, 2 ** 65, 1e20, Infinity, -Infinity, -1, NaN, null, true, {}, { valueOf() { return 12 } }]) {
    let r
    try {
      const ab = new ArrayBuffer(len, { maxByteLength: m })
      r = ab.resizable + ' ' + ab.maxByteLength
    } catch (e) { r = e.name }
    out.push(len + ' ' + (typeof m === 'object' && m ? 'object' : String(m)) + ': ' + r)
  }
}
for (const bad of [Symbol('s'), 1n]) {
  try { new ArrayBuffer(0, { maxByteLength: bad }); out.push('no throw') } catch (e) { out.push(typeof bad + ': ' + e.name) }
}
out.join('\n')
