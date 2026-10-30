// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// BigInt.asIntN and BigInt.asUintN of short and long BigInts, negative and not,
// at widths below, at and above the width of the value and at multiples of the
// size of a limb. asUintN of a negative BigInt used to return it unchanged when
// the width was a multiple of 64 or more (upstream QuickJS c17f7fc). The file
// is what Node 24 prints.
const widths = [0, 1, 2, 7, 8, 31, 32, 33, 62, 63, 64, 65, 100, 127, 128, 129, 191, 192, 193, 256, 300]
const vals = [
  0n, 1n, -1n, 2n, -2n, 127n, 128n, -128n, 255n, 2n ** 31n, -(2n ** 31n), 2n ** 32n - 1n,
  2n ** 62n, -(2n ** 62n), 2n ** 63n - 1n, 2n ** 63n, -(2n ** 63n), -(2n ** 63n) - 1n,
  2n ** 64n - 1n, 2n ** 64n, -(2n ** 64n), -(2n ** 64n) + 1n, 2n ** 64n + 1n,
  2n ** 127n, -(2n ** 127n), 2n ** 128n - 1n, -(2n ** 128n), 2n ** 200n + 12345n, -(2n ** 200n) - 12345n,
  0x123456789abcdef0123456789abcdef0n, -0x123456789abcdef0123456789abcdef0n,
]
const out = []
for (const v of vals) {
  const row = []
  for (const w of widths) row.push(BigInt.asIntN(w, v).toString(16) + '/' + BigInt.asUintN(w, v).toString(16))
  out.push(v.toString(16) + ': ' + row.join(' '))
}
const check = []
for (const v of vals) for (const w of widths) {
  const m = 1n << BigInt(w)
  const u = ((v % m) + m) % m
  const s = u >= m >> 1n && w > 0 ? u - m : u
  if (BigInt.asUintN(w, v) !== u) check.push('uint ' + w + ' ' + v)
  if (BigInt.asIntN(w, v) !== s) check.push('int ' + w + ' ' + v)
}
out.push('mismatch with the definition: ' + check.length + ' ' + check.slice(0, 5).join(';'))
out.join('\n')
