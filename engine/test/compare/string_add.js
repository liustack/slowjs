// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// a + b where both are strings, in every form the register loop adds in: into
// a temp, into a local or an argument (the left one of them grown in place),
// with a constant on the right and on the left, x += y on a property, an
// element and this, and chains of them. The operands are strings of every
// kind the engine makes: empty, constants, 8-bit and 16-bit, lone
// surrogates and halves of a pair, a property key read back, long ones and
// ropes. Each result is checked against the same string built by a join, by
// length and by a checksum of its code units. Then what must not take the
// path of two strings: a String object, objects with valueOf, toString and
// Symbol.toPrimitive, which log the conversions they run, a Symbol, numbers,
// BigInts and the other primitives, and the conversion hooks of
// String.prototype replaced, which a primitive string never calls. The file
// is what Node 24 prints.
function sum(s) {
  let h = 0
  for (let i = 0; i < s.length; i++) h = (h * 31 + s.charCodeAt(i)) | 0
  return h
}
const out = []
const keyed = Object.keys({ keyName: 1 })[0]
const vals = {
  empty: '',
  k: 'abc',
  n8: 'ab'.repeat(3),
  n16: '中文'.repeat(2),
  hi: '\uD83D',
  lo: '\uDE00',
  key: keyed,
  l600: 'x'.repeat(600),
  w600: 'é'.repeat(299) + '中',
  l9k: 'y'.repeat(9000),
  rope: 'r'.repeat(9000) + 'q'.repeat(9000),
  wrope: 'w'.repeat(700) + '中'.repeat(700),
}
const names = Object.keys(vals)
const forms = {
  temp: (a, b) => a + b,
  local: (a, b) => { let t = 'old'; t = a + b; return t },
  arg: (a, b, t) => { t = a + b; return t },
  self: (a, b) => { let t = a; t = t + b; return t },
  selfRight: (a, b) => { let t = b; t = a + t; return t },
  selfArg: (t, b) => { t = t + b; return t },
  plusEq: (a, b) => { let t = a; t += b; return t },
  prop: (a, b) => { const o = { p: a }; o.p += b; return o.p },
  elem: (a, b) => { const o = { p: a }, k = 'p'; o[k] += b; return o[k] },
  array: (a, b) => { const r = [a]; r[0] += b; return r[0] },
  thisProp: function (a, b) { const o = { p: a, f() { this.p += b; return this.p } }; return o.f() },
  chain: (a, b) => a + b + a + b,
  nested: (a, b) => (a + (b + a)) + b,
}
const kForms = {
  kRight: a => a + '<x>',
  kRightLocal: a => { let t; t = a + '中!'; return t },
  kRightSelf: a => { let t = a; t = t + '</div>'; return t },
  kLeft: b => '<x>' + b,
  kLeftLocal: b => { let t; t = '中!' + b; return t },
  kLeftSelf: b => { let t = b; t = '<div>' + t; return t },
  kEmpty: a => a + '',
  kEmptyLeft: b => '' + b,
}
for (const [fn, f] of Object.entries(forms)) {
  let bad = [], h = 0
  for (const an of names) {
    for (const bn of names) {
      const a = vals[an], b = vals[bn]
      const want = fn === 'chain' ? [a, b, a, b].join('') : fn === 'nested' ? [a, b, a, b].join('') : [a, b].join('')
      const s = f(a, b)
      if (typeof s !== 'string' || s !== want || s.length !== want.length || sum(s) !== sum(want)) bad.push(an + '+' + bn)
      h = (h * 7 + sum(s) + s.length) | 0
    }
  }
  out.push(fn + ' ' + h + (bad.length ? ' WRONG ' + bad.join(' ') : ''))
}
for (const [fn, f] of Object.entries(kForms)) {
  let bad = [], h = 0
  const k = { kRight: '<x>', kRightLocal: '中!', kRightSelf: '</div>', kLeft: '<x>', kLeftLocal: '中!', kLeftSelf: '<div>', kEmpty: '', kEmptyLeft: '' }[fn]
  for (const n of names) {
    const v = vals[n]
    const want = fn.startsWith('kLeft') || fn === 'kEmptyLeft' ? k + v : v + k
    const s = f(v)
    if (typeof s !== 'string' || s !== want || sum(s) !== sum(want)) bad.push(n)
    h = (h * 7 + sum(s) + s.length) | 0
  }
  out.push(fn + ' ' + h + (bad.length ? ' WRONG ' + bad.join(' ') : ''))
}
// the halves of a pair joined by + make one character
{
  const s = vals.hi + vals.lo
  out.push('pair ' + s.length + ' ' + s.codePointAt(0).toString(16) + ' ' + [...s].length + ' ' + (s === '\u{1F600}'))
}
// a string built by + used as a key, and a key read back used in a +
{
  const o = {}
  let k = 'na'
  k = k + 'me'
  o[k] = 1
  o['na' + 'me'] += 1
  const back = Object.keys(o)[0] + '!'
  out.push('key ' + o.name + ' ' + back + ' ' + (Object.keys(o)[0] + Object.keys(o)[0]))
}
// a string grown in place must not change for the other holders
{
  let s = 'z'.repeat(100), kept = []
  for (let i = 0; i < 40; i++) {
    if (i % 9 === 0) kept.push(s)
    s = s + 'ab'
  }
  let t = 'z'.repeat(100)
  const o = { t }
  t = t + '!'
  t = t + t
  out.push('held ' + s.length + ' ' + kept.map(x => x.length).join(',') + ' ' + o.t.length + ' ' + t.length)
}
// what must not take the path of two strings
const log = []
const obj = { valueOf() { log.push('valueOf'); return 'V' }, toString() { log.push('toString'); return 'T' } }
const objS = { toString() { log.push('toString'); return 'T' } }
const prim = { [Symbol.toPrimitive](hint) { log.push('prim:' + hint); return 'P' } }
const strObj = new String('so')
const others = {
  obj, objS, prim, strObj, arr: ['a', 'b'], num: 1.5, int: 7, neg0: -0, nan: NaN,
  t: true, nul: null, undef: undefined, big: 12n, sym: Symbol('s'), fn: function f() {},
}
function show(f) {
  log.length = 0
  let r
  try { r = f() } catch (e) { r = '!' + e.constructor.name }
  if (typeof r === 'string' && r.length > 40) r = r.length + ':' + sum(r)
  return r + (log.length ? '[' + log.join(',') + ']' : '')
}
for (const [n, v] of Object.entries(others)) {
  const row = [
    show(() => 'L' + v),
    show(() => v + 'R'),
    show(() => { let t = 'L'; t = t + v; return t }),
    show(() => { let t = v; t = t + 'R'; return t }),
    show(() => { const s = 'x'; return s + v }),
    show(() => { const s = 'x'; return v + s }),
    show(() => { const o = { p: 'P' }; o.p += v; return o.p }),
  ]
  out.push('other ' + n + ' ' + row.join(' '))
}
// the hooks of String.prototype replaced: a primitive string calls none of
// them, a String object calls them
{
  const saved = [String.prototype.valueOf, String.prototype.toString]
  String.prototype.valueOf = function () { log.push('S.valueOf'); return 'sv' }
  String.prototype.toString = function () { log.push('S.toString'); return 'st' }
  Object.defineProperty(String.prototype, Symbol.toPrimitive, {
    value(hint) { log.push('S.prim:' + hint); return 'sp' }, configurable: true, writable: true,
  })
  const a = 'aa', b = 'b'.repeat(600)
  out.push('hooked ' + [
    show(() => a + 'c'), show(() => 'c' + a), show(() => a + b), show(() => { let t = a; t = t + b; return t }),
    show(() => { let t = b; t = 'k' + t; return t }), show(() => { const o = { p: a }; o.p += a; return o.p }),
    show(() => strObj + 'c'), show(() => 'c' + strObj), show(() => a + strObj),
  ].join(' '))
  delete String.prototype[Symbol.toPrimitive]
  String.prototype.valueOf = saved[0]
  String.prototype.toString = saved[1]
  out.push('restored ' + show(() => strObj + 'c'))
}
// a sub of two strings stays a sub, in each form of sub: SUB, SUBL, SUBK,
// SUBKL and KSUB
{
  const s3 = '3', s1 = '1'
  let l = '10'; l = l - s1
  let lk = '7'; lk = lk - '2'
  const o = { p: '8' }; o.p -= '2'
  out.push('sub ' + [s3 - s1, l, s3 - '1', lk, '9' - s3, o.p, 'a' - 'b'].map(String).join(' '))
}
// a loop of every form, warm, in strict and sloppy code
{
  const strictLoop = new Function('a', 'b', '"use strict"; let s = ""; for (let i = 0; i < 300; i++) { s = s + a; s = b + s; s += "|"; if (s.length > 4000) s = "" + i } return s')
  const sloppyLoop = new Function('a', 'b', 'var s = ""; for (var i = 0; i < 300; i++) { s = s + a; s = b + s; s += "|"; if (s.length > 4000) s = "" + i } return s')
  for (const [an, bn] of [['k', 'n16'], ['n8', 'l600'], ['wrope', 'key'], ['empty', 'hi']]) {
    const x = strictLoop(vals[an], vals[bn]), y = sloppyLoop(vals[an], vals[bn])
    out.push('loop ' + an + ' ' + bn + ' ' + x.length + ' ' + sum(x) + ' ' + (x === y))
  }
}
out.join('\n')
