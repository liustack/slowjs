// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// toWellFormed puts U+FFFD for each lone surrogate and keeps the pairs. Its
// loops count their work to read the interrupt flag every 4096 characters,
// and a pair is two characters of one turn of the loop, so the strings have
// a lone surrogate before, at and after that place, a pair across it, and
// long runs of pairs after a lone one. A result is told by how many U+FFFD it
// has, the first of their places, and a sum over all its characters.
const out = []
const sum = (s) => {
  let h = 0
  for (let i = 0; i < s.length; i++) h = (h * 31 + s.charCodeAt(i)) % 1000000007
  return h
}
const bad = (s) => {
  const r = []
  let n = 0
  for (let i = 0; i < s.length; i++) {
    if (s.charCodeAt(i) === 0xfffd) {
      n++
      if (r.length < 6) r.push(i)
    }
  }
  return n + ' at ' + r.join(',')
}
const hi = '\ud83d', lo = '\ude00', pair = hi + lo
const a = (n) => 'a'.repeat(n)
const cases = [
  ['well formed pairs', pair.repeat(9000)],
  ['a lone high first, 3000 pairs', '\ud800' + pair.repeat(3000)],
  ['a lone high first, 5000 pairs', '\ud800' + pair.repeat(5000)],
  ['a lone high first, 20000 pairs', '\ud800' + pair.repeat(20000)],
  ['a lone low first, 5000 pairs', '\udc00' + pair.repeat(5000)],
  ['a lone high, 5000 pairs, a lone low, 5000 pairs', '\ud800' + pair.repeat(5000) + '\udc00' + pair.repeat(5000)],
  ['5000 pairs, a lone high last', pair.repeat(5000) + hi],
  ['a lone low then a lone high then pairs', '\udc00\ud800' + pair.repeat(5000)],
  ['two highs and a pair', hi + hi + lo],
  ['9000 lone highs', '\ud800'.repeat(9000)],
  ['9000 lone lows', '\udc00'.repeat(9000)],
  ['a pair across the place', a(4095) + pair + a(10)],
  ['a pair ending at the place', a(4094) + pair + a(10)],
  ['a pair starting after the place', a(4096) + pair + a(10)],
  ['a lone high before the place', a(4094) + hi + a(10)],
  ['a lone high at the place', a(4095) + hi + a(10)],
  ['a lone high after the place', a(4096) + hi + a(10)],
  ['a lone low at the place', a(4095) + lo + a(10)],
  ['a lone low after the place', a(4096) + lo + a(10)],
  ['a lone high first, then a pair across the place', '\ud800' + a(4094) + pair + a(10)],
  ['a lone high first, pairs and a lone low across the place', '\ud800' + pair.repeat(2047) + lo + pair.repeat(3)],
  ['a lone high first, 8192 characters of a, pairs', '\ud800' + a(8192) + pair.repeat(4100)],
  ['2 places of lone ones among pairs', pair.repeat(2047) + hi + pair.repeat(2048) + lo + pair.repeat(10)],
  ['wide, no surrogate', '中'.repeat(10000)],
  ['wide, a lone high at the end', '中'.repeat(10000) + hi],
]
for (const [name, s] of cases) {
  const w = s.toWellFormed()
  out.push(name + ': length ' + s.length + ' ' + s.isWellFormed() + ' -> ' + w.length + ' ' + w.isWellFormed() +
    ' ' + (w === s) + ' ' + bad(w) + ' sum ' + sum(w) + ' of ' + sum(s))
}
out.join('\n')
