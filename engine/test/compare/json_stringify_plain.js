// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.
// JSON.stringify marks a string it wrote with no escape, a value or a key,
// and copies a marked string with no look at its characters. Each string
// is written several times, as a value and as a key, made at run time and
// written in the source, read back by JSON.parse, and appended to after it
// was written: every write must be the first one.
const out = []
const short = (t) => t.length > 120 ? t.length + ' ' + t.slice(0, 40) + ' ... ' + t.slice(-40) : t
const made = (s) => [...s].join('')   // the same characters, in a string that is no atom
const cases = [
  ['plain', 'plainText'],
  ['empty', ''],
  ['quote', 'say "hi"'],
  ['backslash', 'a\\b'],
  ['newline', 'line\nnext'],
  ['control', 'bell\x07\x1f'],
  ['latin1', 'caf\xe9 \xff'],
  ['cjk', '\u4e2d\u6587'],
  ['pair', 'smile \ud83d\ude00'],
  ['high', 'lone \ud800 high'],
  ['low', 'lone \udc00 low'],
  ['long', 'abcdefgh'.repeat(3000)],
  ['long quote', 'abcdefgh'.repeat(3000) + '"'],
  ['long wide', '\u4e2d'.repeat(20000)],
]
for (const [name, lit] of cases) {
  const s = made(lit)
  const w = []
  for (let i = 0; i < 3; i++) {
    w.push(JSON.stringify(s), JSON.stringify(lit), JSON.stringify([s, lit, s]))
    w.push(JSON.stringify({ [s]: s }), JSON.stringify({ [lit]: 1, k: lit }))
  }
  const back = JSON.parse(JSON.stringify([s, { [s]: s }]))
  w.push(JSON.stringify(back), JSON.stringify(back))
  const same = w.slice(0, 5).every((t, i) => w[i + 5] === t && w[i + 10] === t)
  out.push(name + ': ' + same + ' ' + short(w[2]) + ' ' + short(w[3]) + ' ' + short(w[16]))
}

// strings appended to after a write: built with room at their end
for (const tail of ['"', '\n', '\\', '\ud800', 'x', '\u4e2d']) {
  let m = 'mark'
  for (let i = 0; i < 20; i++) m += 'Room'
  const a = JSON.stringify(m)
  m += tail
  const b = JSON.stringify(m)
  m += 'end'
  const c = JSON.stringify({ m })
  out.push('append ' + JSON.stringify(tail) + ': ' + (a.length - 2) + ' ' + b.slice(-8) + ' ' + c.slice(-12))
}

// strings JSON.parse made, plain and with escapes, written again
const parsed = JSON.parse('{"a":"plain","b":"esc\\"aped","c":"uni\\u0041","d":"\\ud800","e":"caf\u00e9","f":"\u4e2d"}')
out.push('parsed: ' + JSON.stringify(parsed) + ' ' + JSON.stringify(Object.values(parsed)))

// keys with escapes made at run time, written many times through the shape
const keys = cases.map(([n, lit]) => made(lit).slice(0, 12))
const rows = []
for (let i = 0; i < 5; i++) rows.push(Object.fromEntries(keys.map((k, j) => [k, j + i])))
out.push('keys: ' + short(JSON.stringify(rows)))
out.push('keys again: ' + short(JSON.stringify(rows)))
out.join('\n')
