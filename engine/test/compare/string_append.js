// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// Strings built by appending to a variable (x += y, x = x + y) of every kind
// the engine stores into: a local, an argument, a closure variable, a let
// checked for its temporal dead zone, with the value of the assignment used
// or not. Each string is long enough to be grown in place and copied with
// room, past the length where QuickJS turns to ropes, in 8-bit and 16-bit
// characters and with the width changing on the way. A copy taken before an
// append, a key made of the string, and a string that both sides of the + hold
// must not change. Each result is checked against the same string built by a
// join, by length and by a checksum of its code units.
function sum(s) {
  let h = 0
  for (let i = 0; i < s.length; i++) h = (h * 31 + s.charCodeAt(i)) | 0
  return h
}
const out = []
const pieces = {
  narrow: i => 'ab'.repeat(i % 4 + 1),
  wide: i => (i % 3 ? 'é' : '中') + 'x',
  widens: i => (i === 700 ? '\u{1F600}' : 'abc'),
  long: i => (i % 50 ? 'q' : 'L'.repeat(600)),
}
const ways = {
  local: (n, p) => { let s = ''; for (let i = 0; i < n; i++) s += p(i); return s },
  localPlus: (n, p) => { let s = ''; for (let i = 0; i < n; i++) s = s + p(i); return s },
  arg: (n, p, s) => { s = ''; for (let i = 0; i < n; i++) s += p(i); return s },
  closure: (n, p) => { let s = ''; const add = x => { s += x }; for (let i = 0; i < n; i++) add(p(i)); return s },
  closureVar: (n, p) => { var s = ''; const add = x => (s = s + x, true); for (let i = 0; i < n; i++) add(p(i)); return s },
  used: (n, p) => { let s = '', last = ''; for (let i = 0; i < n; i++) last = (s += p(i)); return last === s ? s : 'WRONG' },
  usedClosure: (n, p) => { let s = '', t = ''; const add = x => (s += x); for (let i = 0; i < n; i++) t = add(p(i)); return t === s ? s : 'WRONG' },
  tdz: (n, p) => { const f = () => { for (let i = 0; i < n; i++) s += p(i); return s }; let s = ''; return f() },
  keep: (n, p) => {
    let s = '', bad = 0
    for (let i = 0; i < n; i++) {
      const before = s, len = s.length, h = i % 97 === 0 ? sum(s) : 0
      s += p(i)
      if (before.length !== len || (i % 97 === 0 && sum(before) !== h)) bad++
    }
    return bad ? 'WRONG' : s
  },
  keyed: (n, p) => {
    let s = 'k', bad = 0
    const o = {}
    for (let i = 0; i < n; i++) {
      if (i % 101 === 0) { o[s] = i; s += p(i); if (o[s] !== undefined || Object.keys(o).pop() === s) bad++ } else s += p(i)
    }
    return bad ? 'WRONG' : s.slice(1)
  },
}
for (const [pn, p] of Object.entries(pieces)) {
  for (const n of [1, 40, 1000, 6000]) {
    const want = []
    for (let i = 0; i < n; i++) want.push(p(i))
    const flat = want.join('')
    const line = []
    for (const [wn, w] of Object.entries(ways)) {
      const s = w(n, p)
      line.push(wn + (s === flat && s.length === flat.length && sum(s) === sum(flat) ? '' : ':WRONG'))
    }
    out.push(pn + ' ' + n + ' ' + flat.length + ' ' + sum(flat) + ': ' + line.join(' '))
  }
}
// a string the variable and a property both hold, and one that is also the
// right operand
{
  let s = 'x'.repeat(100)
  const o = { s }
  for (let i = 0; i < 50; i++) s += 'y'
  let t = 'z'.repeat(100)
  for (let i = 0; i < 3; i++) t = t + t
  out.push('held ' + o.s.length + ' ' + s.length + ' ' + t.length)
}
out.join('\n')
