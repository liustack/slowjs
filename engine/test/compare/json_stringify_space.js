// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// The space of JSON.stringify as a string: its first 10 characters are the
// indent of a level, whether the string is flat or a rope (what a long
// concatenation makes), of 8 or 16 bits, with a short piece or a long one
// first, or inside a String object (quickjs-ng 35e195c). Each line is the
// first property of the result, which starts with one indent. The file is
// what Node 24 prints.
const out = []
function esc(s) {
  let r = ''
  for (const ch of String(s)) {
    const c = ch.codePointAt(0)
    r += c >= 0x20 && c < 0x7f ? ch : '\\u{' + c.toString(16) + '}'
  }
  return r
}
function run(name, space) {
  let r
  try { r = esc(JSON.stringify({ x: 1 }, null, space).split('\n')[1]) } catch (e) { r = 'throws ' + e.name }
  out.push(name + ': ' + r)
}
const long8 = 'a'.repeat(600) + 'b'.repeat(600)
const long16 = 'é'.repeat(600) + '中'.repeat(600)
run('flat', '--')
run('flat of 12', 'abcdefghijkl')
run('rope of 8 bits', long8)
run('rope of 16 bits', long16)
run('rope, short piece first', 'xy' + 'z'.repeat(2000))
run('rope, 16 bits after 8', 'pq' + '中'.repeat(2000))
run('rope of ropes', (long16 + long8) + (long8 + long16))
run('String object of a rope', new String(long8))
run('String object of a 16-bit rope', new String(long16))
run('number', 4)
run('Number object', new Number(3))
out.push('nested: ' + esc(JSON.stringify({ x: 1, y: [2, { z: 3 }] }, null, 'ab' + 'c'.repeat(1000))))
;(() => out.join('\n'))
