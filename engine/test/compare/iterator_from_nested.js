// Copyright (c) 2026 The liustack/beastjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.
// The next of an Iterator.from wrapper gives back what the next it wraps
// returns, also when that next is the next of another wrapper: Iterator.from
// wraps a wrapper whose prototype was taken away. A reader of the iterator
// (for of, spread, the helpers) still needs an object. The file is what Node
// 24 prints.
const out = []
function show(name, f) {
  let r
  try { r = f() } catch (e) { r = 'throws ' + e.name }
  out.push(name + ': ' + (typeof r === 'string' ? r : JSON.stringify(r)))
}
// a wrapper that Iterator.from wraps again, n times
function nest(it, n) {
  let w = Iterator.from(it)
  for (let i = 0; i < n; i++) {
    w.next = w.next
    w.return = w.return
    Object.setPrototypeOf(w, null)
    w = Iterator.from(w)
  }
  return w
}
for (const n of [1, 2, 3]) {
  show(n + ' primitive', () => nest({ next() { return 5 } }, n).next())
  show(n + ' undefined', () => String(nest({ next() { } }, n).next()))
  show(n + ' same object', () => { const res = { done: false, value: 1, extra: 2 }; const w = nest({ next() { return res } }, n); return [w.next() === res, w.next().extra] })
  show(n + ' a wrapper is new', () => { const w = Iterator.from({ next() { return 5 } }); w.next = w.next; Object.setPrototypeOf(w, null); return Iterator.from(w) !== w })
  show(n + ' next throws', () => nest({ next() { throw new RangeError('x') } }, n).next())
  show(n + ' for of', () => { for (const x of nest({ next() { return 5 } }, n)) return 'ran'; return 'ended' })
  show(n + ' spread', () => [...nest({ next() { return 5 } }, n)])
  show(n + ' map', () => nest({ next() { return 5 } }, n).map(x => x).next())
  show(n + ' toArray', () => nest({ next() { return 5 } }, n).toArray())
  show(n + ' toArray of objects', () => { let i = 0; return nest({ next() { return i < 3 ? { done: false, value: i++ } : { done: true } } }, n).toArray() })
  show(n + ' return primitive', () => nest({ next() { return {} }, return() { return 7 } }, n).return())
}
out.join('\n')
