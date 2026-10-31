// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// Node 24 output for QuickJS de4c9e0: an exhausted reduce must not close.
(() => {
  const out = []
  for (const kind of ['normal', 'throw-get', 'throw-call']) {
    let gets = 0, closes = 0, calls = 0
    const iter = {
      next() { return { done: true } },
      get return() {
        gets++
        if (kind === 'throw-get') throw new RangeError()
        return () => { closes++; if (kind === 'throw-call') throw new RangeError(); return {} }
      }
    }
    let error
    try { Iterator.prototype.reduce.call(iter, () => { calls++ }) } catch (e) { error = e.name }
    out.push(kind + ': ' + [error, gets, closes, calls].join(','))
    out.push('initial: ' + Iterator.prototype.reduce.call(iter, () => 0, 42))
  }
  let closes = 0
  const iter = { next() { return { value: 1, done: false } }, return() { closes++; return {} } }
  try { Iterator.prototype.reduce.call(iter, () => { throw new RangeError() }, 0) } catch (e) { out.push('callback: ' + e.name + ',' + closes) }
  return out.join('\n')
})()
