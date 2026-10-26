// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// String.prototype.includes, startsWith, endsWith, indexOf and lastIndexOf,
// at every position, and split, replace and replaceAll, against a search
// written with charCodeAt alone: on strings of 8-bit and 16-bit characters,
// ropes and slices, found and not found, at the ends, more than once and
// longer than the 64 characters a needle of the other width is widened or
// narrowed into. The script is the one of TestStringSearch.
(() => {
  function find(s, v, from, back) {
    const n = s.length, m = v.length
    const at = (i) => { for (let k = 0; k < m; k++) if (s.charCodeAt(i + k) !== v.charCodeAt(k)) return false; return true }
    if (back) { for (let i = Math.min(from, n - m); i >= 0; i--) if (at(i)) return i; return -1 }
    for (let i = Math.max(from, 0); i + m <= n; i++) if (at(i)) return i
    return -1
  }
  function split(s, v) {
    const out = []; let i = 0
    if (v.length === 0) { for (let k = 0; k < s.length; k++) out.push(s[k]); return out }
    for (;;) { const j = find(s, v, i, false); if (j < 0) break; out.push(s.slice(i, j)); i = j + v.length }
    out.push(s.slice(i)); return out
  }
  function replaceAll(s, v, w) { return split(s, v).join(w) }
  const parts = ['a', 'ab', '<!--$!-->', '<!-- -->', '<', '!', '\xe9', '\xff', '\u4e00', '\u6161', '\u0161', 'aa', '\x00', '-->', '\u6100']
  let seed = 7
  const rnd = (n) => (seed = (seed * 1103515245 + 12345) % 2147483648) % n
  const make = (k) => { let s = ''; for (let i = 0; i < k; i++) s += parts[rnd(parts.length)]; return s }
  const hays = [], needles = ['', 'a', 'ab', '<!--$!-->', '\u4e00', '\u6161a', 'a\u6100', '\xe9\xff', 'x', '\x00a', 'a'.repeat(70), '\u4e00'.repeat(70), 'ab'.repeat(40) + '\xe9']
  for (let k = 0; k < 25; k++) { hays.push(make(rnd(40))); needles.push(make(1 + rnd(3))) }
  hays.push('', 'a'.repeat(200), 'a'.repeat(69) + '\u4e00' + 'a'.repeat(71), ('ab'.repeat(40) + '\xe9').repeat(3), ('\u4e00'.repeat(70) + 'a').repeat(2))
  hays.push(('x' + '\u4e00').slice(1) + 'ab', ('\u4e00' + 'abab').slice(1), 'q'.repeat(1000) + '<!--$!-->' + 'q'.repeat(1000))
  let n = 0, bad = []
  const check = (what, got, want) => { n++; if (JSON.stringify(got) !== JSON.stringify(want) && bad.length < 20) bad.push(what + ': ' + JSON.stringify(got) + ' want ' + JSON.stringify(want)) }
  for (const h0 of hays) for (const v of needles) {
    for (const h of [h0, h0 + v, v + h0 + v + h0]) {
      const L = h.length
      const tag = JSON.stringify([h.slice(0, 30), L, v.slice(0, 10), v.length])
      check('includes ' + tag, h.includes(v), find(h, v, 0, false) >= 0)
      check('indexOf ' + tag, h.indexOf(v), find(h, v, 0, false))
      check('lastIndexOf ' + tag, h.lastIndexOf(v), find(h, v, L, true))
      check('startsWith ' + tag, h.startsWith(v), find(h.slice(0, v.length), v, 0, false) === 0)
      check('endsWith ' + tag, h.endsWith(v), L >= v.length && find(h, v, L - v.length, false) === L - v.length)
      for (const p of [-1, 0, 1, 2, 7, L >> 1, L - v.length, L - 1, L, L + 3]) {
        check('includes at ' + p + ' ' + tag, h.includes(v, p), find(h, v, Math.min(Math.max(p, 0), L), false) >= 0)
        check('indexOf at ' + p + ' ' + tag, h.indexOf(v, p), find(h, v, Math.min(Math.max(p, 0), L), false))
        check('lastIndexOf at ' + p + ' ' + tag, h.lastIndexOf(v, p), find(h, v, Math.max(p, 0), true))
        const q = Math.min(Math.max(p, 0), L)
        check('startsWith at ' + p + ' ' + tag, h.startsWith(v, p), q + v.length <= L && find(h, v, q, false) === q)
        check('endsWith at ' + p + ' ' + tag, h.endsWith(v, p), q - v.length >= 0 && find(h.slice(0, q), v, q - v.length, false) === q - v.length)
      }
      check('split ' + tag, h.split(v), split(h, v))
      if (v.length > 0) {
        check('replace ' + tag, h.replace(v, '#'), find(h, v, 0, false) < 0 ? h : h.slice(0, find(h, v, 0, false)) + '#' + h.slice(find(h, v, 0, false) + v.length))
        check('replaceAll ' + tag, h.replaceAll(v, '#'), replaceAll(h, v, '#'))
      }
    }
  }
  // Long needles of few letters, often repeating, in strings of the same
  // letters: the ways through the Two-Way search of the C tree, forward and
  // back, with needles of the other width. The letters of the last set share
  // their low byte, which the table of shifts of 16-bit characters keys on.
  const sets = [['a', 'b'], ['a', '\u4e00'], ['\u4e00', '\u4e01', 'b'], ['a', 'b', 'c'], ['a', '\u0161', '\u4e61']]
  for (const set of sets) {
    const pick = (k) => { let s = ''; for (let i = 0; i < k; i++) s += set[rnd(set.length)]; return s }
    for (let t = 0; t < 12; t++) {
      const unit = pick(1 + rnd(4))
      let v = unit.repeat(1 + rnd(24))
      if (rnd(2)) v += pick(1 + rnd(3))
      if (rnd(3) === 0) v = pick(1 + rnd(2)) + v
      let h = ''
      const len = 150 + rnd(250)
      while (h.length < len) h += rnd(4) ? v.slice(0, rnd(v.length + 1)) + unit : pick(1 + rnd(6))
      if (rnd(2)) h += v
      if (rnd(3) === 0) h += '\u4e00'
      const L = h.length
      const tag = JSON.stringify([h.slice(0, 20), L, v.slice(0, 10), v.length])
      for (const p of [0, L >> 1, L]) {
        check('long indexOf at ' + p + ' ' + tag, h.indexOf(v, p), find(h, v, p, false))
        check('long lastIndexOf at ' + p + ' ' + tag, h.lastIndexOf(v, p), find(h, v, p, true))
        check('long includes at ' + p + ' ' + tag, h.includes(v, p), find(h, v, p, false) >= 0)
      }
    }
  }
  return bad.length ? bad.join('\n') : 'ok ' + n
})()
