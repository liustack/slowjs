// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// Promise.try calls its callback first and then makes the promise with the
// resolve of the constructor it was called on, so that a promise the callback
// returns comes back as it is, and a callback that throws gives a rejected
// promise (upstream QuickJS 8e68fcf). The lines that start with "ES2026" are
// what the current specification says and Node 24 does not do yet (the promise
// the callback returns is the result when it is made by the same constructor,
// and the callback runs before the constructor is used). The rest is what
// Node 24 prints.
// jsrun: -result
const out = []
const log = []
function show(v) { return typeof v === 'object' && v !== null ? Object.prototype.toString.call(v) : String(v) }
function run(name, f) {
  let r
  try { r = f() } catch (e) { r = 'throws ' + e.name }
  out.push(name + ': ' + show(r))
}
const settled = []
function track(name, p) {
  p.then(v => settled.push(name + ' fulfilled ' + show(v)), e => settled.push(name + ' rejected ' + show(e)))
  return 'pending'
}
run('value', () => track('value', Promise.try(() => 5)))
run('no args', () => track('no args', Promise.try((...a) => a.length)))
run('args', () => track('args', Promise.try((a, b, c) => [a, b, c].join('|'), 1, 'x')))
run('this is undefined', () => track('this', Promise.try(function () { return this === undefined })))
run('throw', () => track('throw', Promise.try(() => { throw 'boom' })))
run('throw error', () => track('throw error', Promise.try(() => { throw new RangeError('r') }).catch(e => e.name)))
run('returns a thenable', () => track('thenable', Promise.try(() => ({ then(r) { r('t') } }))))
run('returns a rejected promise', () => track('rejected promise', Promise.try(() => Promise.reject('no'))))
run('callback is not a function', () => track('not function', Promise.try(5)))
run('callback is missing', () => track('missing', Promise.try()))
run('callback runs now', () => { let ran = false; Promise.try(() => { ran = true }); return ran })
run('result is a promise', () => Promise.try(() => 1) instanceof Promise)
run('this is not an object', () => Promise.try.call(undefined, () => 1))
run('this is a number', () => Promise.try.call(5, () => 1))
run('this is not a constructor', () => Promise.try.call({}, () => 1))
run('this is a function that is not a constructor', () => Promise.try.call(() => {}, () => 1))
run('ES2026 the callback runs before this is checked as a constructor', () => { let ran = false; try { Promise.try.call({}, () => { ran = true }) } catch (e) { } return ran })
class P extends Promise {
  static get [Symbol.species]() { return Promise }
}
run('subclass', () => P.try(() => 1) instanceof P)
run('subclass of a throw', () => P.try(() => { throw 1 }).catch(() => { }) instanceof P)
class Q extends Promise {
  constructor(ex) { log.push('Q'); super(ex) }
}
run('subclass constructor', () => { log.length = 0; const q = Q.try(() => 1); return q instanceof Q && log.join() })
run('subclass constructor on a throw', () => { log.length = 0; const q = Q.try(() => { throw 1 }); q.catch(() => { }); return q instanceof Q && log.join() })
const pr = Promise.resolve(7)
run('ES2026 a promise comes back as it is', () => Promise.try(() => pr) === pr)
run('ES2026 a rejected promise comes back as it is', () => { const r = Promise.reject(1); r.catch(() => { }); const t = Promise.try(() => r); t.catch(() => { }); return t === r })
run('ES2026 constructor read once', () => {
  const p = Promise.resolve(1)
  let reads = 0
  Object.defineProperty(p, 'constructor', { get() { reads++; return Promise } })
  Promise.try(() => p)
  return reads
})
run('not a promise but thenable', () => track('plain object', Promise.try(() => ({}))))
run('capability executor', () => {
  const log = []
  function C(ex) { log.push('C'); ex(v => log.push('res ' + show(v)), e => log.push('rej ' + show(e))) }
  C.resolve = function () { log.push('resolve called'); return {} }
  C.try = Promise.try
  C.try(() => 3)
  C.try(() => { throw 4 })
  return log.join()
})
Promise.resolve().then(() => 0).then(() => 0).then(() => 0).then(() => {
  globalThis.result = out.join('\n') + '\n' + settled.sort().join('\n')
})
