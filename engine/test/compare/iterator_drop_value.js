// Copyright (c) 2026 The liustack/beastjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.
// drop reads done of the items it throws away and never their value (upstream
// QuickJS 2f97a49). The file is what Node 24 prints.
const out = []
function source(n) {
  const log = []
  let i = 0
  const it = {
    next() {
      const k = i++
      log.push('next ' + k)
      return {
        get done() { log.push('done ' + k); return k >= n },
        get value() { log.push('value ' + k); return k },
      }
    },
    return() { log.push('return'); return {} },
  }
  return { it, log }
}
function show(name, f) {
  let r
  try { r = f() } catch (e) { r = 'throws ' + e.name }
  out.push(name + ': ' + (typeof r === 'string' ? r : JSON.stringify(r)))
}
show('drop 2 of 5', () => {
  const { it, log } = source(5)
  const d = Iterator.prototype.drop.call(it, 2)
  const r = d.next()
  return [r.value, r.done, log]
})
show('drop 2 of 1', () => {
  const { it, log } = source(1)
  const d = Iterator.prototype.drop.call(it, 2)
  const r = d.next()
  return [r.value, r.done, log]
})
show('drop 0', () => {
  const { it, log } = source(3)
  const d = Iterator.prototype.drop.call(it, 0)
  const r = d.next()
  return [r.value, r.done, log]
})
show('drop, done throws', () => {
  const log = []
  const it = { next() { return { get done() { log.push('done'); throw new Error('stop') }, get value() { log.push('value'); return 1 } } } }
  const d = Iterator.prototype.drop.call(it, 1)
  try { d.next() } catch (e) { log.push(e.message) }
  return log
})
show('drop, a result that is not an object', () => {
  const it = { next() { return 5 } }
  return Iterator.prototype.drop.call(it, 1).next()
})
show('drop on an array iterator', () => [...[1, 2, 3, 4].values().drop(2)])
out.join('\n')
