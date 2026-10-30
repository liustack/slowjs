// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// Atomics.store converts its value first, and the conversion may resize or
// detach the buffer, so the store then fails (a RangeError when the index
// is past the end and a TypeError when the buffer is detached, where Node 24
// throws a TypeError in both, so only that it throws is printed). The
// converted value, a BigInt that does not fit in a short one, was not freed on
// those two paths (upstream QuickJS c8ef889). The script also checks that runs
// of them leave the heap as it was. The file is what Node 24 prints.
const out = []
function run(name, f) {
  let r
  try { r = f() } catch (e) { r = 'throws' }
  out.push(name + ': ' + String(r))
}
function resized(big) {
  const b = new ArrayBuffer(16, { maxByteLength: 16 })
  const a = big ? new BigInt64Array(b) : new Int32Array(b)
  return Atomics.store(a, 1, { valueOf() { b.resize(0); return big ? (1n << 100n) + 5n : 7 } })
}
function detached(big) {
  const b = new ArrayBuffer(16)
  const a = big ? new BigInt64Array(b) : new Int32Array(b)
  return Atomics.store(a, 1, { valueOf() { b.transfer(); return big ? (1n << 100n) + 5n : 7 } })
}
function shrunk(big) {
  const b = new ArrayBuffer(16, { maxByteLength: 16 })
  const a = big ? new BigInt64Array(b) : new Int32Array(b)
  return Atomics.store(a, 1, { valueOf() { b.resize(big ? 8 : 4); return big ? (1n << 100n) + 5n : 7 } })
}
function ok(big) {
  const b = new ArrayBuffer(16)
  const a = big ? new BigInt64Array(b) : new Int32Array(b)
  const r = Atomics.store(a, 1, { valueOf() { return big ? (1n << 100n) + 5n : 7 } })
  return r + ' ' + a[1]
}
for (const big of [false, true]) {
  run('resized to 0 ' + big, () => resized(big))
  run('detached ' + big, () => detached(big))
  run('shrunk ' + big, () => shrunk(big))
  run('ok ' + big, () => ok(big))
}
run('index too large', () => Atomics.store(new BigInt64Array(1), 5, 1n << 100n))
run('index too large int', () => Atomics.store(new Int32Array(1), 5, 3))
run('bad value', () => Atomics.store(new BigInt64Array(1), 0, 1))
run('wide result', () => Atomics.store(new BigInt64Array(1), 0, (1n << 100n) + 3n))
if (typeof memorySize === 'function') {
  const mk = () => {
    for (let i = 0; i < 200; i++) {
      for (const f of [resized, detached, shrunk]) {
        try { f(true) } catch (e) { }
        try { f(false) } catch (e) { }
      }
    }
  }
  // the same blocks in use after two collections (see strict_equal.js)
  mk(); mk(); gc()
  mk(); mk(); mk(); gc(); gc()
  const m1 = memorySize(), k1 = memoryBlocks()
  mk(); mk(); mk(); gc(); gc()
  out.push('memory after: ' + heapChange(m1, k1))
} else {
  out.push('memory after: 0')
}
out.join('\n')
