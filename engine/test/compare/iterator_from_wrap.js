// Copyright (c) 2026 The liustack/beastjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.
// The next of the wrapper Iterator.from makes returns the result of the next
// it wraps as it is, and its return the result of the return it wraps (upstream
// QuickJS fbc494b). The file is what Node 24 prints.
const out = []
function show(name, f) {
  let r
  try { r = f() } catch (e) { r = 'throws ' + e.name }
  out.push(name + ': ' + (typeof r === 'string' ? r : JSON.stringify(r)))
}
show('same object', () => {
  const res = { done: false, value: 1, extra: 42 }
  const w = Iterator.from({ next() { return res } })
  return [w.next() === res, w.next().extra]
})
show('primitive result', () => Iterator.from({ next() { return 5 } }).next())
show('undefined result', () => Iterator.from({ next() { } }).next())
show('string result', () => Iterator.from({ next() { return 'abc' } }).next())
show('return result', () => Iterator.from({ next() { return {} }, return() { return 7 } }).return())
show('return object', () => {
  const r = { done: true, value: 9, extra: 1 }
  const w = Iterator.from({ next() { return {} }, return() { return r } })
  return w.return() === r
})
show('no return', () => JSON.stringify(Iterator.from({ next() { return {} } }).return()))
show('next throws', () => Iterator.from({ next() { throw new RangeError('x') } }).next())
show('helpers over the wrapper', () => {
  let i = 0
  const w = Iterator.from({ next() { return i < 3 ? { done: false, value: i++ } : { done: true } } })
  return w.map((x) => x * 2).toArray()
})
show('wrapper in for of', () => {
  let i = 0
  const r = []
  for (const x of Iterator.from({ next() { return i < 3 ? { done: false, value: i++ } : { done: true } } })) r.push(x)
  return r
})
show('for of over a primitive result', () => { for (const x of Iterator.from({ next() { return 5 } })) return 'ran'; return 'ended' })
show('map over a primitive result', () => Iterator.from({ next() { return 5 } }).map((x) => x).next())
show('toArray over a primitive result', () => Iterator.from({ next() { return 5 } }).toArray())
show('spread of a primitive result', () => [...Iterator.from({ next() { return 5 } })])
show('drop over a primitive result', () => Iterator.from({ next() { return 5 } }).drop(1).next())
const closing = () => Iterator.from({ next() { return { done: false, value: 1 } }, return() { return 3 } })
show('break closes with a primitive', () => { for (const x of closing()) break; return 'ok' })
show('helper return with a primitive', () => { const m = closing().map((x) => x); m.next(); return m.return() })
out.join('\n')
