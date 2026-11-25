// Copyright (c) 2026 The liustack/beastjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.
// Math.sumPrecise gives the exact sum rounded once: many values of one sign,
// signs that take turns, sums near the largest double and subnormals. A sum
// that stays negative over many carries once lost its sign and gave Infinity.
// Node 24 has no Math.sumPrecise: the file is what Node 24 prints with the
// sum done exactly in BigInt and rounded to nearest, ties to even.
const out = []
const show = v => Object.is(v, -0) ? '-0' : String(v)
function run(name, f) {
  let r
  try { r = show(f()) } catch (e) { r = 'throws ' + e.name }
  out.push(name + ': ' + r)
}
const MAX = Number.MAX_VALUE, MIN = Number.MIN_VALUE
const fill = (n, v) => new Array(n).fill(v)
const turns = (n, a, b) => { const r = []; for (let i = 0; i < n; i++) r.push(i & 1 ? b : a); return r }
for (const n of [125, 126, 127, 250, 251, 2393, 2394, 2395, 4000, 4750, 4751, 6000, 100000]) {
  run('fill ' + n + ' x -1', () => Math.sumPrecise(fill(n, -1)))
  run('fill ' + n + ' x 1', () => Math.sumPrecise(fill(n, 1)))
}
for (const v of [0.1, -0.1, 2 ** 53 - 1, -(2 ** 53 - 1), 1e300, -1e300, 1e-300, -1e-300, MIN, -MIN, 2 ** -1022, -(2 ** -1022), 3.5e-310, -3.5e-310]) {
  run('fill 5000 x ' + show(v), () => Math.sumPrecise(fill(5000, v)))
}
run('turns 10000 1 -1', () => Math.sumPrecise(turns(10000, 1, -1)))
run('turns 10001 1 -1', () => Math.sumPrecise(turns(10001, 1, -1)))
run('turns 10001 -1 1', () => Math.sumPrecise(turns(10001, -1, 1)))
run('turns 9999 -1 0.5', () => Math.sumPrecise(turns(9999, -1, 0.5)))
run('turns 9999 1e308 -1e308', () => Math.sumPrecise(turns(9999, 1e308, -1e308)))
run('turns 9999 -1e308 1e308', () => Math.sumPrecise(turns(9999, -1e308, 1e308)))
run('turns 6000 -MIN 1e-320', () => Math.sumPrecise(turns(6000, -MIN, 1e-320)))
run('turns 6000 -1 1e-300', () => Math.sumPrecise(turns(6000, -1, 1e-300)))
run('-1 then 1e-300', () => Math.sumPrecise([...fill(5000, -1), ...fill(5000, 1e-300)]))
run('MAX MAX', () => Math.sumPrecise([MAX, MAX]))
run('-MAX -MAX', () => Math.sumPrecise([-MAX, -MAX]))
run('MAX MAX -MAX', () => Math.sumPrecise([MAX, MAX, -MAX]))
run('-MAX -MAX MAX', () => Math.sumPrecise([-MAX, -MAX, MAX]))
run('5000 MAX then 4999 -MAX', () => Math.sumPrecise([...fill(5000, MAX), ...fill(4999, -MAX)]))
run('5000 -MAX then 4999 MAX', () => Math.sumPrecise([...fill(5000, -MAX), ...fill(4999, MAX)]))
run('5000 -MAX then 5000 MAX', () => Math.sumPrecise([...fill(5000, -MAX), ...fill(5000, MAX)]))
run('5000 -MAX then 5001 MAX', () => Math.sumPrecise([...fill(5000, -MAX), ...fill(5001, MAX)]))
run('5000 -MAX then 5000 MAX then -1', () => Math.sumPrecise([...fill(5000, -MAX), ...fill(5000, MAX), -1]))
run('-MAX then MAX and half ulp', () => Math.sumPrecise([-MAX, -MAX, MAX, MAX, -MAX, -(2 ** 970)]))
run('-MAX less than half ulp', () => Math.sumPrecise([-MAX, -(2 ** 969)]))
run('-MAX more than half ulp', () => Math.sumPrecise([-MAX, -(2 ** 970), -MIN]))
run('MAX half ulp', () => Math.sumPrecise([MAX, 2 ** 970]))
run('MAX less than half ulp', () => Math.sumPrecise([MAX, 2 ** 970, -MIN]))
run('1 and a tie', () => Math.sumPrecise([1, 2 ** -53]))
run('-1 and a tie', () => Math.sumPrecise([-1, -(2 ** -53)]))
run('-1 past a tie', () => Math.sumPrecise([-1, -(2 ** -53), -MIN]))
run('-1 and a tie down', () => Math.sumPrecise([-1, -(2 ** -52), -(2 ** -53)]))
run('empty', () => Math.sumPrecise([]))
run('-0', () => Math.sumPrecise([-0]))
run('-0 -0', () => Math.sumPrecise([-0, -0]))
run('-0 0', () => Math.sumPrecise([-0, 0]))
run('1 -1', () => Math.sumPrecise([1, -1]))
run('-1 1 -0', () => Math.sumPrecise([-1, 1, -0]))
run('Infinity -Infinity', () => Math.sumPrecise([Infinity, -Infinity]))
run('-Infinity and -MAX', () => Math.sumPrecise([-Infinity, -MAX, -MAX]))
run('NaN', () => Math.sumPrecise([1, NaN]))
// values of every size and sign from a fixed generator
let seed = 0x2545f491
function rnd() { seed ^= seed << 13; seed ^= seed >>> 17; seed ^= seed << 5; return seed >>> 0 }
const dv = new DataView(new ArrayBuffer(8))
function rndDouble(emin, emax) {
  const e = emin + rnd() % (emax - emin + 1)
  dv.setUint32(0, ((rnd() & 1) << 31) | (e << 20) | (rnd() & 0xfffff))
  dv.setUint32(4, rnd())
  return dv.getFloat64(0)
}
for (const [emin, emax] of [[0, 2046], [1000, 1046], [1, 60], [2000, 2046], [0, 3]]) {
  for (const n of [10, 127, 1000, 10000]) {
    const a = []
    for (let i = 0; i < n; i++) a.push(rndDouble(emin, emax))
    run('random ' + n + ' e ' + emin + '..' + emax, () => Math.sumPrecise(a))
    const neg = a.map(x => -Math.abs(x))
    run('random ' + n + ' e ' + emin + '..' + emax + ' all negative', () => Math.sumPrecise(neg))
  }
}
out.join('\n')
