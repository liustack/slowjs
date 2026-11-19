// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.
// An await whose PromiseResolve throws (Await: ? PromiseResolve) throws the
// error at the await: a catch around it sees it, a finally runs, and the
// function goes on. The error comes from a promise whose constructor is a
// getter that throws, at the top of an async function, in a nested await, in
// an async arrow of a class method, in a loop, with no catch, and from the
// call depth refusing the call of the getter. Each case logs its steps and how
// its promise ended, and the log is the result. The file is what Node 24
// prints.
// jsrun: -result
const results = []
function poisoned(v, msg) {
  const p = Promise.resolve(v)
  Object.defineProperty(p, 'constructor', { get() { throw new Error(msg) } })
  return p
}
async function run(name, body) {
  const log = []
  let end
  try {
    end = 'resolved ' + (await body(log))
  } catch (e) {
    end = 'rejected ' + (e && e.message)
  }
  results.push(name + ': ' + log.concat(end).join(' | '))
}
class Box {
  constructor() { this.n = 0 }
  method(log) {
    const step = async (p) => {
      try { this.n += await p; log.push('arrow after await') } catch (e) { log.push('arrow caught ' + e.message); this.n += 10 } finally { log.push('arrow finally') }
      return this.n
    }
    return step(poisoned(1, 'in arrow'))
  }
}
const cases = [
  ['top', async log => {
    try { await poisoned(1, 'boom'); log.push('not here') } catch (e) { log.push('caught ' + e.message) } finally { log.push('finally') }
    return 'done'
  }],
  ['sync part only', async log => {
    log.push('before')
    try { await poisoned(1, 'first') } catch (e) { log.push('caught ' + e.message) }
    log.push('after')
    return 'x'
  }],
  ['nested await', async log => {
    const inner = async () => {
      log.push('inner start')
      await null
      try { return await poisoned(2, 'nested') } catch (e) { log.push('inner caught ' + e.message); return 'inner value' } finally { log.push('inner finally') }
    }
    const v = await inner()
    log.push('outer got ' + v)
    try { await (async () => { await poisoned(3, 'deeper') })() } catch (e) { log.push('outer caught ' + e.message) }
    return v
  }],
  ['class method arrow', async log => {
    const b = new Box()
    const v = await b.method(log)
    return v + ' ' + b.n
  }],
  ['loop', async log => {
    let n = 0
    for (let i = 0; i < 3; i++) {
      try { await poisoned(i, 'loop ' + i) } catch (e) { n++; log.push(e.message) }
      await i
    }
    return n
  }],
  ['no catch', async log => {
    try { await poisoned(1, 'uncaught') } finally { log.push('finally') }
    log.push('not here')
  }],
  ['finally awaits', async log => {
    try { await poisoned(1, 'f1') } finally { log.push('finally'); await null; log.push('after await in finally') }
  }],
  ['catch awaits again', async log => {
    try { await poisoned(1, 'a') } catch (e) {
      log.push('caught ' + e.message)
      try { await poisoned(2, 'b') } catch (e2) { log.push('caught ' + e2.message) }
    }
    return await 7
  }],
  ['getter returns Promise', async log => {
    const p = Promise.resolve(5)
    Object.defineProperty(p, 'constructor', { get() { log.push('getter'); return Promise } })
    return await p
  }],
]
// The call depth: near the level where a call is refused, the async function
// starts but the calls of the getter at its await are refused. The getter is
// a function bound twice, so it takes more levels than the body did (the body
// left its own level when it suspended at the await). The error must be
// thrown at that await as any other. Each level as the recursion unwinds
// starts one run, and the outcomes of the runs whose body started are
// counted. The body writes what it saw into properties, as a call there could
// be refused too.
function depthCase() {
  const p = Promise.resolve(1)
  Object.defineProperty(p, 'constructor', { get: (function () { return Promise }).bind(null).bind(null) })
  const runs = []
  async function body(rec) {
    rec.started = true
    try { await p; rec.at = 'resumed' } catch (e) { rec.at = 'caught ' + e.name } finally { rec.fin = 'finally' }
    return 'ok'
  }
  function down() {
    try { down() } catch (e) {}
    const rec = { started: false, at: 'suspended', fin: 'no finally' }
    let pr
    try { pr = body(rec) } catch (e) { return }
    runs.push({ rec, pr })
  }
  down()
  return Promise.all(runs.map(r => r.pr.then(v => v, e => 'rejected ' + e.name).then(end => r.rec.started ? [r.rec.at, r.rec.fin, end].join(' ') : 'not started'))).then(outs => {
    const seen = new Set(outs)
    const other = [...seen].filter(k => k !== 'not started' && k !== 'resumed finally ok' && k !== 'caught RangeError finally ok')
    return 'refused at the await: ' + (seen.has('caught RangeError finally ok') ? 'some' : 'none') + ', other outcomes: ' + (other.length ? other.join('; ') : 'none')
  })
}
;(async () => {
  for (const [name, body] of cases) await run(name, body)
  results.push('call depth: ' + await depthCase())
  globalThis.result = results.join('\n')
})()
