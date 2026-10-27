// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// Ropes of many shapes, as deep as concatenation lets them get, turned into
// flat strings by every path that copies the strings of a rope in order:
// reading a character, a key, join, String.prototype.concat, a template, a
// regexp and JSON. Each result is checked against the same string built by
// a join of its pieces, by length and by a checksum of its code units.
function sum(s) {
  let h = 0
  for (let i = 0; i < s.length; i++) h = (h * 31 + s.charCodeAt(i)) | 0
  return h
}
const out = []
const piece = i => (i % 7 === 0 ? 'é' : i % 11 === 0 ? '\u{1F600}' : 'ab'.repeat(i % 5 + 1)) + 'x'.repeat(i % 3 ? 300 : 0)
const shapes = {
  left: n => { let s = ''; const p = []; for (let i = 0; i < n; i++) { s = s + piece(i); p.push(piece(i)) } return [s, p.join('')] },
  right: n => { let s = ''; const p = []; for (let i = 0; i < n; i++) { s = piece(i) + s; p.unshift(piece(i)) } return [s, p.join('')] },
  both: n => { let s = ''; const p = []; for (let i = 0; i < n; i++) { if (i & 1) { s = s + piece(i); p.push(piece(i)) } else { s = piece(i) + s; p.unshift(piece(i)) } } return [s, p.join('')] },
  tree: n => { let a = [...Array(n).keys()].map(piece); const flat = a.join(''); while (a.length > 1) { const b = []; for (let i = 0; i < a.length; i += 2) b.push(i + 1 < a.length ? a[i] + a[i + 1] : a[i]); a = b } return [a[0], flat] },
}
for (const [name, make] of Object.entries(shapes)) {
  for (const n of [2, 10, 61, 62, 200, 3000]) {
    const paths = {
      charAt: s => { const c = s[s.length >> 1]; return s },
      key: s => { const o = {}; o[s] = 1; return Object.keys(o)[0] },
      join: s => [s, s].join(''),
      concat: s => ''.concat(s, s),
      template: s => `${s}${s}`,
      regexp: s => /^[\s\S]*$/.exec(s)[0],
      json: s => JSON.parse(JSON.stringify(s)),
      eq: s => s,
    }
    const line = []
    for (const [pn, f] of Object.entries(paths)) {
      const [s, flat] = make(n)
      const r = f(s)
      const want = pn === 'join' || pn === 'concat' || pn === 'template' ? flat + flat : flat
      line.push(pn + (r === want && r.length === want.length && sum(r) === sum(want) ? '' : ':WRONG'))
    }
    const [s] = make(n)
    out.push(name + ' ' + n + ' ' + s.length + ' ' + sum(s) + ': ' + line.join(' '))
  }
}
out.join('\n')
