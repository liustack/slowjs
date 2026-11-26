// Copyright (c) 2026 The liustack/beastjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.
// jsrun: -result
// The two functions Promise.allSettled gives the then of each element share
// one flag: once one of them has run, neither does anything. Promise.all and
// Promise.any give one function of their own and the resolve or reject of the
// result, which runs once. The thenables call both of their functions, in
// both orders. The file is what Node 24 prints.
const results = []
const thenables = {
  'fulfill then reject': (f, r) => { f(1); r(2) },
  'reject then fulfill': (f, r) => { r(2); f(1) },
  'fulfill twice': (f, r) => { f(1); f(3) },
  'reject twice': (f, r) => { r(2); r(4) },
  'reject, fulfill, reject': (f, r) => { r(2); f(1); r(4) },
}
const methods = ['all', 'allSettled', 'any']
for (const m of methods) {
  for (const name of Object.keys(thenables)) {
    class P extends Promise {}
    P.resolve = function (v) { return v === 'plain' ? Promise.resolve(9) : { then: thenables[name] } }
    const key = m + ', ' + name
    Promise[m].call(P, [0, 'plain']).then(
      v => results.push(key + ': ' + JSON.stringify(v)),
      e => results.push(key + ': rejected ' + (e instanceof AggregateError ? 'AggregateError ' + JSON.stringify(e.errors) : JSON.stringify(e))))
  }
}
// read after all the jobs have run
Object.defineProperty(globalThis, 'result', { get() { return results.sort().join('\n') } })
