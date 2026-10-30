// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// A for await loop that leaves early (break, continue to an outer loop, return,
// a labelled break) calls the return method of its iterator and waits for the
// promise it gives before it goes on, then checks what it resolved to. A loop
// that runs to the end does not call return, and a body that throws calls it
// and the error stays (upstream QuickJS 1ec58aa). A body that throws does not
// wait for a return() that succeeds in QuickJS, where Node 24 does, so no case
// checks that. Each case logs the steps in order, and the log is the result.
// The file is what Node 24 prints.
// jsrun: -result
const results = []
const tick = () => Promise.resolve()
function iter(log, n, opts = {}) {
  let i = 0
  const it = {
    [Symbol.asyncIterator]() { return it },
    next() { log.push('next ' + i); return tick().then(() => i < n ? { done: false, value: i++ } : { done: true }) },
  }
  if (!opts.noReturn) {
    it.return = function () {
      log.push('return called')
      if (opts.returnThrows) throw new RangeError('sync')
      if (opts.returnPrimitive) return opts.returnPrimitive === 'promise' ? Promise.resolve(5) : 5
      if (opts.returnNull) return null
      if (opts.returnRejects) return tick().then(() => { log.push('return rejects'); throw new RangeError('async') })
      return tick().then(() => tick()).then(() => { log.push('return resolved'); return { done: true } })
    }
  }
  return it
}
async function run(name, body) {
  const log = []
  try { await body(log) } catch (e) { log.push('threw ' + e.name) }
  results.push(name + ': ' + log.join(' | '))
}
const cases = [
  ['break', async log => { for await (const x of iter(log, 3)) { log.push('body ' + x); break } log.push('after') }],
  ['break after two', async log => { for await (const x of iter(log, 3)) { log.push('body ' + x); if (x === 1) break } log.push('after') }],
  ['no break', async log => { for await (const x of iter(log, 2)) log.push('body ' + x); log.push('after') }],
  ['no return method', async log => { for await (const x of iter(log, 3, { noReturn: true })) { break } log.push('after') }],
  ['return is null', async log => { const it = iter(log, 3); it.return = null; for await (const x of it) { break } log.push('after') }],
  ['return is undefined', async log => { const it = iter(log, 3); it.return = undefined; for await (const x of it) { break } log.push('after') }],
  ['return is not a function', async log => { const it = iter(log, 3); it.return = 5; for await (const x of it) { break } log.push('after') }],
  ['return returns null', async log => { for await (const x of iter(log, 3, { returnNull: true })) { break } log.push('after') }],
  ['return returns a primitive', async log => { for await (const x of iter(log, 3, { returnPrimitive: true })) { break } log.push('after') }],
  ['return returns a promise of a primitive', async log => { for await (const x of iter(log, 3, { returnPrimitive: 'promise' })) { break } log.push('after') }],
  ['return throws', async log => { for await (const x of iter(log, 3, { returnThrows: true })) { break } log.push('after') }],
  ['return rejects', async log => { for await (const x of iter(log, 3, { returnRejects: true })) { break } log.push('after') }],
  ['body throws, return rejects', async log => { for await (const x of iter(log, 3, { returnRejects: true })) { throw new TypeError('body') } }],
  ['body throws, return throws', async log => { for await (const x of iter(log, 3, { returnThrows: true })) { throw new TypeError('body') } }],
  ['return statement', async log => { const f = async () => { for await (const x of iter(log, 3)) { return 'r' + x } }; log.push(await f()); log.push('after') }],
  ['return statement in an async generator', async log => {
    const g = async function* () { for await (const x of iter(log, 3)) { return 'r' + x } }
    const r = await g().next(); log.push(JSON.stringify(r)); log.push('after')
  }],
  ['return statement in try finally', async log => {
    const f = async () => { try { for await (const x of iter(log, 3)) { return 'r' + x } } finally { log.push('finally') } }
    log.push(await f())
  }],
  ['labelled break', async log => { outer: for (const y of [1, 2]) { for await (const x of iter(log, 3)) { log.push('body ' + y + x); break outer } } log.push('after') }],
  ['continue outer', async log => { outer: for (const y of [1, 2]) { for await (const x of iter(log, 3)) { log.push('body ' + y + x); continue outer } } log.push('after') }],
  ['nested for await, break inner', async log => {
    for await (const a of iter(log, 2)) { for await (const b of iter(log, 2)) { log.push('body ' + a + b); break } log.push('mid') }
    log.push('after')
  }],
  ['nested for await, break both', async log => {
    o: for await (const a of iter(log, 2)) { for await (const b of iter(log, 2)) { log.push('body ' + a + b); break o } }
    log.push('after')
  }],
  ['sync iterable', async log => {
    const it = { [Symbol.iterator]() { return { next() { log.push('sync next'); return { done: false, value: 1 } }, return() { log.push('sync return'); return {} } } } }
    for await (const x of it) { log.push('body'); break }
    log.push('after')
  }],
  ['sync iterable no return', async log => {
    const it = { [Symbol.iterator]() { return { next() { return { done: false, value: 1 } } } } }
    for await (const x of it) { break }
    log.push('after')
  }],
  ['array', async log => { for await (const x of [Promise.resolve(1), 2]) { log.push('body ' + x); break } log.push('after') }],
  ['async generator', async log => {
    const g = async function* () { try { yield 1; yield 2 } finally { log.push('generator finally'); await tick(); log.push('generator finally done') } }
    for await (const x of g()) { log.push('body ' + x); break }
    log.push('after')
  }],
  ['async generator runs to the end', async log => {
    const g = async function* () { try { yield 1 } finally { log.push('generator finally') } }
    for await (const x of g()) { log.push('body ' + x) }
    log.push('after')
  }],
  ['destructuring throws', async log => { for await (const { a } of iter(log, 1)) { break } log.push('after') }],
  ['for await with let and closure', async log => { const fs = []; for await (let x of iter(log, 3)) { fs.push(() => x); if (x === 1) break } log.push(fs.map(f => f()).join()) }],
  ['for of is not awaited', async log => {
    const it = { [Symbol.iterator]() { return { next() { return { done: false, value: 1 } }, return() { log.push('sync return'); return Promise.resolve(5) } } } }
    for (const x of it) { break }
    log.push('after')
  }],
  ['order of microtasks', async log => {
    let n = 0
    const t = (async () => { while (n < 30) { await null; n++ } })()
    for await (const x of iter(log, 3)) { log.push('body ' + x + ' at ' + n); break }
    log.push('after at ' + n)
    await t
  }],
]
;(async () => {
  for (const [name, body] of cases) await run(name, body)
  globalThis.result = results.join('\n')
})()
