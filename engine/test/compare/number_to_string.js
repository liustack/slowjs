// Copyright (c) 2026 The liustack/tifajs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.
// Numbers converted to strings in radix 10 (Number::toString): the shortest
// digits that read back as the number, and where the exponent form starts.
// Each value goes through every way a script converts a number (String, a
// template, both sides of +, toString, toString(10), JSON.stringify, a
// property key, join), which must give one string, and the string must read
// back as the number. The fixed values print their string, the long lists
// (every power of two, the powers of ten and their neighbours, float32 values,
// random bit patterns, numbers with few decimals) print a hash of all their
// strings and a few samples. toFixed, toPrecision, toExponential and other
// radixes 2 and 16 are printed for a few values, to see they did not move.
const dv = new DataView(new ArrayBuffer(8))
const fromBits = b => { dv.setBigUint64(0, b); return dv.getFloat64(0) }
const toBits = x => { dv.setFloat64(0, x); return dv.getBigUint64(0) }
const next = x => fromBits(toBits(x) + 1n)
const prev = x => fromBits(toBits(x) - 1n)

const ways = {
  String: x => String(x),
  template: x => `${x}`,
  addLeft: x => '' + x,
  addRight: x => x + '',
  addText: x => ('a' + x).slice(1),
  toString: x => x.toString(),
  toString10: x => x.toString(10),
  json: x => JSON.stringify(x),
  jsonArray: x => JSON.stringify([x]).slice(1, -1),
  key: x => Object.keys({ [x]: 1 })[0],
  join: x => [x, x].join(';').split(';')[1],
  concat: x => ''.concat(x),
}
// JSON gives null for NaN and the infinities, and a property key and JSON
// give "0" for -0 as String does
const jsonSame = x => Number.isFinite(x)

let mismatches = 0, badReads = 0
const firstBad = []
function conv(x) {
  const s = String(x)
  for (const [name, f] of Object.entries(ways)) {
    if ((name === 'json' || name === 'jsonArray') && !jsonSame(x)) continue
    const t = f(x)
    if (t !== s) {
      mismatches++
      if (firstBad.length < 10) firstBad.push(name + ' ' + s + ' ' + t)
    }
  }
  if (Number(s) !== x && !(x !== x && s === 'NaN')) {
    badReads++
    if (firstBad.length < 10) firstBad.push('read ' + s)
  }
  return s
}

// FNV-1a over the strings, with a separator
function hasher() {
  let h = 0x811c9dc5, n = 0
  return {
    add(s) {
      for (let i = 0; i < s.length; i++) h = Math.imul(h ^ s.charCodeAt(i), 0x01000193)
      h = Math.imul(h ^ 10, 0x01000193)
      n++
    },
    get() { return n + ' ' + (h >>> 0).toString(16) },
  }
}

const out = []
const fixed = [
  0, -0, NaN, Infinity, -Infinity, 1, -1, 1.5, -1.5, 0.1, 0.2, 0.3, 0.1 + 0.2, 1 / 3, 2 / 3, -2 / 3,
  0.5, 0.25, 0.125, 0.0625, 1e-7, 1.5e-7, 1e-6, 1.5e-6, 0.000001234, 0.0000001234,
  1e20, 1e21, 1.5e21, 123456789012345680000, 1234567890123456800000, 9007199254740991, 9007199254740992,
  9007199254740993, 9007199254740994, 2 ** 53 + 4, 2 ** 54, 2 ** 60, 2 ** 64, 2 ** 70, 2 ** 100,
  Number.MAX_SAFE_INTEGER, Number.EPSILON, Number.MIN_VALUE, -Number.MIN_VALUE, Number.MAX_VALUE,
  -Number.MAX_VALUE, 2.2250738585072014e-308, 2.225073858507201e-308, 4.9406564584124654e-324,
  9.8813129168249309e-324, 1.7976931348623157e308, 1.7976931348623155e308,
  117.74058342906295, 117.74058342906294, 26.417095212464115, 3.141592653589793, 2.718281828459045,
  1e23, 8.41e21, 5e-324, 9.5e-5, 1.1, 1.01, 100.1, 4.35, 0.3e-15, 123.456, -123.456e-10,
  2 ** -1022, 2 ** -1023, 2 ** -1074, 2 ** 1023, 2 ** -44, 2 ** -43, 2 ** 89, 2 ** 90,
  // values with a closer lower neighbour (powers of two) whose shortest form is above them
  1.8014398509481984e16, 9.671406556917033e24, 5.764607523034235e17, 2.9514790517935283e20,
  // hard cases of the literature: long shortest forms and values near a tie
  9007199254740991.0, 2.0e-3, 3.0e-3, 5.0e-324, 1.0e23, 4.9e-324, 1.9156918820264798e-56,
  6.6564021122018745e+264, 4.91e-6, 5.547e-6, 1.7800590868057611e-307, 2.0522684006491881e-308,
  2.4703282292062327e-324, 2.4703282292062328e-324, 2.2250738585072011e-308,
  9.0071992547409915e15, 9.0071992547409925e15, 3.4366717556271665e10, 1.7339860103599998e-29,
]
for (const x of fixed) out.push(conv(x))

const groups = {}
function group(name, xs, show) {
  const h = hasher()
  const sample = []
  for (let i = 0; i < xs.length; i++) {
    const s = conv(xs[i])
    h.add(s)
    if (show(i)) sample.push(s)
  }
  out.push(name + ': ' + h.get() + ' ' + sample.join(' '))
}

// every power of two, and the values next to each
{
  const xs = []
  for (let e = -1074; e <= 1023; e++) {
    const p = 2 ** e
    xs.push(p)
    if (e > -1074) xs.push(prev(p))
    if (e < 1023) xs.push(next(p))
    xs.push(-p)
  }
  xs.push(prev(Infinity))
  group('powers of two', xs, i => i % 509 === 0)
}
// the powers of ten in the range, and the values next to each
{
  const xs = []
  for (let k = -323; k <= 308; k++) {
    const p = Number('1e' + k)
    xs.push(p, prev(p), next(p))
    xs.push(Number('5e' + k), Number('9.999999999999999e' + k))
  }
  group('powers of ten', xs, i => i % 211 === 0)
}
// every exponent with a few significands: the smallest, the largest, and ones
// with few or many digits
{
  const xs = []
  const sig = [0n, 1n, 2n, 3n, 0xfffffffffffffn, 0xffffffffffffen, 0x8000000000000n, 0x123456789abcdn, 0x5555555555555n, 0xaaaaaaaaaaaaan]
  for (let be = 0n; be < 2047n; be++) {
    for (const s of sig) {
      if (be === 0n && s === 0n) continue
      xs.push(fromBits((be << 52n) | s))
    }
  }
  group('each exponent', xs, i => i % 1999 === 0)
}
// subnormals
{
  const xs = []
  for (let b = 1n; b < 2000n; b++) xs.push(fromBits(b))
  for (let b = 1n; b < 0x10000000000000n; b *= 3n) xs.push(fromBits(b), fromBits(0x10000000000000n - b))
  group('subnormals', xs, i => i % 307 === 0)
}
// random bit patterns, from a fixed xorshift
let st = 0x9e3779b97f4a7c15n
const mask = (1n << 64n) - 1n
function rnd() {
  st ^= (st << 13n) & mask
  st ^= st >> 7n
  st ^= (st << 17n) & mask
  return st
}
{
  const xs = []
  while (xs.length < 30000) {
    const x = fromBits(rnd())
    if (Number.isFinite(x)) xs.push(x)
  }
  group('random bits', xs, i => i % 3001 === 0)
}
// float32 values widened, as Math.fround and Float32Array give them
{
  const xs = []
  const f = new Float32Array(1)
  const u = new Uint32Array(f.buffer)
  for (let i = 0; i < 20000; i++) {
    u[0] = Number(rnd() & 0xffffffffn)
    if (Number.isFinite(f[0])) xs.push(f[0])
  }
  for (let i = 1; i < 2000; i++) xs.push(Math.fround(i / 7), Math.fround(i * 0.1))
  group('float32', xs, i => i % 2003 === 0)
}
// numbers with a few decimals, as charts and dates make them
{
  const xs = []
  for (let i = 0; i < 20000; i++) {
    const r = Number(rnd() >> 11n) / 2 ** 53
    xs.push(r * 1000, Math.round(r * 1e6) / 100, Math.round(r * 1e4) / 1e4 * 360, r * 1e-3, r / 3 * 1e15, r * 1e22, -r)
  }
  for (let i = 0; i < 2000; i++) xs.push(i / 10, i / 100, i * 0.1, i * 0.01, i / 3, i * 1.1, 0.1 * i + 0.2)
  group('decimals', xs, i => i % 9001 === 0)
}

out.push('mismatches ' + mismatches + ', bad reads ' + badReads + (firstBad.length ? ': ' + firstBad.join(' | ') : ''))

// the other formats and radixes stay as they were
const other = [0, -0, 1.5, 0.1, 1 / 3, 1e21, 1e-7, 123.456, 2 ** 60, Number.MAX_VALUE, Number.MIN_VALUE, -2.5, NaN, Infinity]
for (const x of other) {
  out.push(String(x) + ': ' + [x.toFixed(2), x.toFixed(0), x.toPrecision(5), x.toPrecision(1), x.toExponential(), x.toExponential(3),
    x.toString(2).slice(0, 40), x.toString(16).slice(0, 40)].join(' '))
}
out.join('\n')
