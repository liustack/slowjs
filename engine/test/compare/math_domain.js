// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// The domain of the Math functions whose libc code can go wrong for an input
// out of range: acosh, asinh, atanh, log, log1p, log2, log10, sqrt and pow
// with a negative base. Each result is printed as its kind only (NaN,
// Infinity, -Infinity, 0, -0, or the sign of a finite number), so that a
// difference in the last digit does not show and a result that should be NaN
// or an infinity does. Then acosh of the values from -1 to -2^40, at eight
// steps between each power of two: musl's acosh gave a finite number for many
// x between -2^26 and -2, from about -8192 down. The file is what Node 24
// prints.
function kind(v) {
  if (v !== v) return 'NaN'
  if (v === Infinity) return 'Infinity'
  if (v === -Infinity) return '-Infinity'
  if (v === 0) return Object.is(v, -0) ? '-0' : '0'
  return v > 0 ? '+' : '-'
}
function name(v) { return Object.is(v, -0) ? '-0' : String(v) }
const xs = [
  0, -0, 5e-324, -5e-324, 2.2250738585072014e-308, -2.2250738585072014e-308, 1e-300, -1e-300, 1e-16, -1e-16,
  0.5, -0.5, 0.9999999999999999, -0.9999999999999999, 1, -1, 1.0000000000000002, -1.0000000000000002,
  1.5, -1.5, 1.9999999999999998, -1.9999999999999998, 2, -2, 3, -3, 9115.56, -9115.56,
  67108863, -67108863, 67108864, -67108864, 1e10, -1e10, 1e300, -1e300,
  1.7976931348623157e308, -1.7976931348623157e308, Infinity, -Infinity, NaN,
]
const out = []
for (const f of ['acosh', 'asinh', 'atanh', 'log', 'log1p', 'log2', 'log10', 'sqrt']) {
  out.push(f + ' ' + xs.map(x => name(x) + ':' + kind(Math[f](x))).join(' '))
}
const bases = [-0, -5e-324, -0.5, -1, -1.5, -2, -3, -1e300, -1.7976931348623157e308, -Infinity]
const exps = [0, -0, 1, -1, 2, -2, 3, -3, 0.5, -0.5, 1.5, 1 / 3, 2 ** 53 - 1, 2 ** 53, -(2 ** 53 - 1), 1025, -1025, 1075, -1075, 1e300, -1e300, Infinity, -Infinity, NaN]
for (const b of bases) {
  out.push('pow ' + name(b) + ' ' + exps.map(e => name(e) + ':' + kind(Math.pow(b, e)) + '/' + kind(b ** e)).join(' '))
}
const notNaN = []
for (let k = 0; k <= 320; k++) {
  const x = -(1 + (k % 8) / 8) * 2 ** Math.floor(k / 8)
  if (Math.acosh(x) === Math.acosh(x)) notNaN.push(x)
}
out.push('acosh of 321 values from -1 to -2^40, not NaN: ' + notNaN.length + (notNaN.length ? ' (' + notNaN.slice(0, 3).join(', ') + ' ...)' : ''))
out.join('\n')
