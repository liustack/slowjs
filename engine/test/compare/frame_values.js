// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// Calls that end with local variables and operands of every kind, in every
// number up to 70 and a few more, one local holding a value of each kind at
// each of the first and last 9 places and a few between. The end of a call
// frees its values, skipping those with no reference count, and every value
// must be freed once: the sanitizers and the leak check of the host see a
// value freed twice or never, and the line printed holds what each call gave.
const kinds = [
  () => ({ a: 1 }),
  () => 'ab'.repeat(3),
  () => [1, 2, 3],
  () => 10n ** 30n,
  () => 1n,
  () => -5n,
  () => 'x'.repeat(300) + 'y'.repeat(300),
  () => Symbol.for('s'),
  () => 1.5,
  () => 7,
  () => true,
  () => null,
  () => undefined,
]
const show = (v) => typeof v === 'object' && v !== null ? JSON.stringify(v) : typeof v === 'symbol' ? v.description : String(v)
const counts = []
for (let k = 0; k <= 70; k++) counts.push(k)
counts.push(127, 128, 129, 200)
let calls = 0
let sum = 0
for (const k of counts) {
  let decl = ''
  for (let i = 0; i < k; i++) decl += `var v${i};`
  for (let j = 0; j < k; j++) {
    if (j >= 9 && j < k - 9 && j % 11 !== 0) continue
    const f = new Function('mk', `${decl} v${j} = mk(); return v${j};`)
    for (const make of kinds) {
      for (const ch of show(f(make))) sum = (sum * 31 + ch.charCodeAt(0)) % 1000003
      calls++
    }
  }
  // Every local holds an object, and the function returns a number.
  let all = ''
  for (let i = 0; i < k; i++) all += `v${i} = {n: ${i}};`
  sum = (sum * 31 + new Function(decl + all + ` return ${k}`)()) % 1000003
  calls++
}
// Operands left on the stack by a tail call, a return in a loop over an
// iterator, and a return in a try with a finally block. The frame of a
// generator is freed by its own end, not by the loop at the end of a call, and
// is here to see that the calls around it do not change.
function tail(n, o) { return n == 0 ? o.v : tail(n - 1, { v: o.v + 1 }) }
function forOf(a) { for (var x of a) { var u = { x }; return u.x } }
function inFinally() { var a = {}, b = 'x'.repeat(5); try { return a } finally { b = {} } }
function captured() {
  var a = { n: 1 }, b = [2], c = 'cc'.repeat(2), d = 4, e = { n: 5 }, f = [6], g = 7, h = { n: 8 }, i = [9]
  return [() => a.n, () => b[0], () => c, () => d, () => e.n, () => f[0], () => g, () => h.n, () => i[0]]
}
function throws() { var a = {}, b = [1], c = 'z'.repeat(3); null.x }
let caught = 0
for (let i = 0; i < 10; i++) { try { throws() } catch (e) { caught += e instanceof TypeError } }
function* gen() { var a = { n: 1 }, b = [2], c = 'z'.repeat(3), d = 1; yield a; yield b; return c }
`${calls} ${sum} ${tail(50, { v: 0 })} ${typeof forOf([{}, {}])} ${typeof inFinally()} ${captured().map((f) => f()).join()} ${caught} ${(() => { const g = gen(), g1 = g.next(), g2 = g.next(), g3 = g.next(); return [g1.value.n, g1.done, g2.value[0], g2.done, g3.value, g3.done].join() })()}`
