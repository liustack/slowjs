// Copyright (c) 2026 The liustack/beastjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.
// The ownKeys of a proxy reads the descriptor of every key of the target
// before it checks the trap result ([[OwnPropertyKeys]] of a Proxy, step 16
// and after): an error from reading a descriptor comes before the TypeError
// of a key missing from the result. The target is a proxy whose
// getOwnPropertyDescriptor throws for one key. The file is what Node 24
// prints.
const out = []
function run(name, f) {
  let r
  try { r = f() } catch (e) { r = 'throws ' + e.name }
  out.push(name + ': ' + (typeof r === 'string' ? r : JSON.stringify(r)))
}
function target(order, extensible) {
  const t = {}
  for (const k of order) {
    if (k === 'a') Object.defineProperty(t, 'a', { value: 1, configurable: false })
    else t[k] = 2
  }
  return extensible ? t : Object.preventExtensions(t)
}
const log = []
function throwing(t, bad) {
  return new Proxy(t, { getOwnPropertyDescriptor(t, k) { log.push(k); if (k === bad) throw new RangeError(k); return Reflect.getOwnPropertyDescriptor(t, k) } })
}
for (const order of [['a', 'b'], ['b', 'a']]) {
  for (const extensible of [true, false]) {
    for (const result of [[], ['a'], ['b'], ['a', 'b'], ['a', 'b', 'c']]) {
      for (const bad of ['a', 'b', 'none']) {
        log.length = 0
        run(order.join('') + (extensible ? '' : ' not extensible') + ', ' + JSON.stringify(result) + ', throws on ' + bad,
          () => Reflect.ownKeys(new Proxy(throwing(target(order, extensible), bad), { ownKeys() { return result } })))
        out[out.length - 1] += ' (read ' + log.join() + ')'
      }
    }
  }
}
run('own trap throws', () => {
  const t = {}
  Object.defineProperty(t, 'a', { value: 1, configurable: false })
  t.b = 2
  return Reflect.ownKeys(new Proxy(t, { ownKeys() { return [] }, getOwnPropertyDescriptor(t, k) { throw new RangeError(k) } }))
})
out.join('\n')
