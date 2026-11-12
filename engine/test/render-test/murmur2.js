// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// A bundle for engine/test/render-test.sh: its render checks __tifa_murmur2__
// of render tifa=1 as tifa's internal/ssr/hash_test.go checks the Go one, and
// gives what it found as the HTML.

// emotionHash is murmur2 of @emotion/hash 0.9.2 (MIT, Copyright (c) Emotion
// team and other contributors), as its dist files have it, without its
// comments.
function emotionHash(str) {
  var h = 0;
  var k,
      i = 0,
      len = str.length;
  for (; len >= 4; ++i, len -= 4) {
    k = str.charCodeAt(i) & 0xff | (str.charCodeAt(++i) & 0xff) << 8 | (str.charCodeAt(++i) & 0xff) << 16 | (str.charCodeAt(++i) & 0xff) << 24;
    k =
    (k & 0xffff) * 0x5bd1e995 + ((k >>> 16) * 0xe995 << 16);
    k ^=
    k >>> 24;
    h =
    (k & 0xffff) * 0x5bd1e995 + ((k >>> 16) * 0xe995 << 16) ^
    (h & 0xffff) * 0x5bd1e995 + ((h >>> 16) * 0xe995 << 16);
  }
  switch (len) {
    case 3:
      h ^= (str.charCodeAt(i + 2) & 0xff) << 16;
    case 2:
      h ^= (str.charCodeAt(i + 1) & 0xff) << 8;
    case 1:
      h ^= str.charCodeAt(i) & 0xff;
      h =
      (h & 0xffff) * 0x5bd1e995 + ((h >>> 16) * 0xe995 << 16);
  }
  h ^= h >>> 13;
  h =
  (h & 0xffff) * 0x5bd1e995 + ((h >>> 16) * 0xe995 << 16);
  return ((h ^ h >>> 15) >>> 0).toString(36);
}

// The hash of every string: each length of the tail that murmur2 mixes at the
// end, code units of one and two bytes, characters outside the BMP, lone
// surrogates, long styles, and ropes. Then only strings, and only while
// String.prototype.charCodeAt and Number.prototype.toString are the built-ins.
function sameHashes() {
  let seed = 7
  const next = (n) => (seed = (seed * 1103515245 + 12345) % 2147483648) % n
  const strings = ['', 'a', 'ab', 'abc', 'abcd', 'abcde', 'color:red;', 'éÿĀ', '😀x', 'x\ud83d', '\udc00\ud800']
  for (let n = 0; n < 2000; n++) {
    let s = ''
    const length = n < 1900 ? next(80) : 1000 + next(9000)
    for (let i = 0; i < length; i++) {
      const kind = next(5)
      s += String.fromCharCode(kind === 0 ? next(0x80) : kind === 1 ? next(0x100) : kind === 2 ? next(0x10000) : kind === 3 ? 0xd800 + next(0x800) : 32 + next(95))
    }
    strings.push(s)
  }
  const wrong = strings.filter((s) => __tifa_murmur2__(s) !== emotionHash(s))
  return [strings.length, wrong.length, ...wrong.slice(0, 3).map((s) => JSON.stringify(s))].join(' ')
}

function restToJavaScript() {
  const out = []
  for (const v of [undefined, null, 1, true, {}, ['a'], new String('abc'), { toString: () => 'abc' }]) out.push(__tifa_murmur2__(v))
  out.push(__tifa_murmur2__('abc') === emotionHash('abc'))
  const charCodeAt = String.prototype.charCodeAt
  String.prototype.charCodeAt = function (i) { return 0 }
  out.push(__tifa_murmur2__('abc'))
  String.prototype.charCodeAt = charCodeAt
  const toString = Number.prototype.toString
  Number.prototype.toString = function () { return 'x' }
  out.push(__tifa_murmur2__('abc'))
  Number.prototype.toString = toString
  out.push(__tifa_murmur2__('abc') === emotionHash('abc'))
  out.push(Object.keys(globalThis).includes('__tifa_murmur2__'))
  return JSON.stringify(out)
}

globalThis.startRender = function () {
  globalThis.renderResult = typeof __tifa_murmur2__ !== 'function'
    ? 'no __tifa_murmur2__\n'
    : sameHashes() + '\n' + restToJavaScript() + '\n'
}
