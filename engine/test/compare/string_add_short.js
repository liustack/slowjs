// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

// a + b of two 8-bit strings into a temp, around the lengths where the
// register loop copies them or grows the left one in place itself (left 1 to
// 8192 characters, right 1 to 512) and where it leaves them to the generic
// concatenation (an empty side, a longer one, a wide one, a rope): every
// result is checked against the same string built by a join, and the
// operands must keep their characters when the left one has another holder,
// is a property key, keeps the atom of a key, or is the result of an earlier
// add that the next add may grow. Then the same adds made again and again
// must leave the heap as it was. The file is what Node 24 prints.
function sum(s) {
  let h = 0
  for (let i = 0; i < s.length; i++) h = (h * 31 + s.charCodeAt(i)) | 0
  return h
}
const out = []
function mk(n, c) { let s = ''; for (let i = 0; i < n; i++) s += String.fromCharCode(c + (i % 7)); return s }
const lefts = [1, 2, 63, 64, 65, 511, 512, 513, 4096, 8191, 8192, 8193]
const rights = [0, 1, 2, 63, 511, 512, 513]
const add = (a, b) => a + b
const add3 = (a, b, c) => a + b + c
let bad = [], h = 0
for (const l of lefts) {
  for (const r of rights) {
    for (const [ca, cb] of [[97, 65], [0xe0, 0x30], [0x4e2d, 97], [97, 0x4e2d]]) {
      const a = mk(l, ca), b = mk(r, cb)
      const sa = sum(a), sb = sum(b)
      const s = add(a, b)
      const want = [a, b].join('')
      if (s !== want || s.length !== l + r || sum(s) !== sum(want)) bad.push('add ' + l + '+' + r + ' ' + ca)
      // the left one is a temp that the next add may grow in place: a and b keep their characters
      const t = add3(a, b, b)
      if (t !== [a, b, b].join('') || sum(a) !== sa || sum(b) !== sb || a.length !== l || b.length !== r) bad.push('add3 ' + l + '+' + r + ' ' + ca)
      h = (h * 7 + sum(s) + sum(t) + t.length) | 0
    }
  }
}
out.push('lengths ' + h + (bad.length ? ' WRONG ' + bad.join(' ') : ''))

// the left string has other holders: a variable, an array, an object
bad = []
for (const l of [1, 10, 100, 1000]) {
  const a = mk(l, 98), keep = [a], o = { a }
  const before = sum(a)
  for (let i = 0; i < 20; i++) {
    const s = a + 'x'
    if (s.length !== l + 1) bad.push('len ' + l)
  }
  if (sum(a) !== before || sum(keep[0]) !== before || sum(o.a) !== before || a.length !== l) bad.push('holder ' + l)
}
out.push('holders ' + (bad.length ? 'WRONG ' + bad.join(' ') : 'ok'))

// the left string is a property key, or keeps the atom of one
bad = []
{
  const k1 = mk(12, 99), obj = {}
  obj[k1] = 1
  const k2 = Object.keys(obj)[0]
  const r1 = k2 + 'tail', r2 = k1 + 'tail'
  if (r1 !== k1 + 'tail' || r2 !== r1 || obj[k1] !== 1 || obj[k2] !== 1 || Object.keys(obj).length !== 1) bad.push('key')
  const k3 = mk(12, 99)
  const v = obj[k3]
  const r3 = k3 + 'more'
  if (v !== 1 || r3 !== k1 + 'more' || obj[k3] !== 1 || k3 !== k1 || obj[k1 + 'more'] !== undefined) bad.push('kept key')
  const parts = ['ab', 'cd', 'ef']
  const m = {}
  for (const p of parts) m[p + 'x'] = p
  let acc = ''
  for (const p of parts) acc = acc + m[p + 'x'] + ','
  if (acc !== 'ab,cd,ef,' || Object.keys(m).join() !== 'abx,cdx,efx') bad.push('keys from adds')
}
out.push('keys ' + (bad.length ? 'WRONG ' + bad.join(' ') : 'ok'))

// a chain of adds into temps, as Emotion builds its class names
bad = []
{
  const names = ['css-1x2y3z', 'MuiButton-root', 'MuiButton-text', 'Mui-focusVisible', 'é-latin1']
  for (let round = 0; round < 3; round++) {
    let r = ''
    for (let i = 0; i < 200; i++) r = r + names[i % names.length] + ' '
    const want = Array.from({ length: 200 }, (_, i) => names[i % names.length] + ' ').join('')
    if (r !== want || sum(r) !== sum(want)) bad.push('round ' + round)
  }
}
out.push('chain ' + (bad.length ? 'WRONG ' + bad.join(' ') : 'ok'))

if (typeof memorySize === 'function') {
  const work = () => {
    for (let i = 0; i < 300; i++) {
      const a = mk(1 + (i % 70), 97)
      let s = a + 'x'
      s = s + mk(i % 9 + 1, 65) + ';'
      const t = (a + a) + (s + 'é')
      if (t.length === 0) throw new Error('empty')
    }
  }
  work(); work(); gc()
  work(); work(); work(); gc(); gc()
  const m1 = memorySize(), k1 = memoryBlocks()
  work(); work(); work(); gc(); gc()
  out.push('memory after: ' + heapChange(m1, k1))
} else {
  out.push('memory after: 0')
}
out.join('\n')
