// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// The getOwnPropertyDescriptor trap of a proxy has to report, for a property
// of the target that is not configurable, the same value when it is not
// writable and the same getter and setter when it is an accessor (upstream
// QuickJS ce0dbb7). Every combination of a target property and a reported
// descriptor, once with getOwnPropertyDescriptor and once with defineProperty
// on the proxy, which checks the same invariant. The script also checks that
// runs of the failing ones leave the heap as it was. The file is what Node 24
// prints.
const out = []
const g1 = function () { return 1 }
const g2 = function () { return 2 }
const s1 = function (v) { }
const s2 = function (v) { }
const targets = {
  'data nc ro 1': () => Object.defineProperty({}, 'p', { value: 1, writable: false, configurable: false, enumerable: true }),
  'data nc rw 1': () => Object.defineProperty({}, 'p', { value: 1, writable: true, configurable: false, enumerable: true }),
  'data c ro 1': () => Object.defineProperty({}, 'p', { value: 1, writable: false, configurable: true, enumerable: true }),
  'data c rw 1': () => Object.defineProperty({}, 'p', { value: 1, writable: true, configurable: true, enumerable: true }),
  'accessor nc g1 s1': () => Object.defineProperty({}, 'p', { get: g1, set: s1, configurable: false, enumerable: true }),
  'accessor nc g1': () => Object.defineProperty({}, 'p', { get: g1, configurable: false, enumerable: true }),
  'accessor nc s1': () => Object.defineProperty({}, 'p', { set: s1, configurable: false, enumerable: true }),
  'accessor c g1 s1': () => Object.defineProperty({}, 'p', { get: g1, set: s1, configurable: true, enumerable: true }),
  'missing': () => ({}),
  'missing prevented': () => Object.preventExtensions({}),
}
const reports = {
  'undefined': undefined,
  'data nc ro 1': { value: 1, writable: false, configurable: false, enumerable: true },
  'data nc ro 2': { value: 2, writable: false, configurable: false, enumerable: true },
  'data nc ro NaN': { value: NaN, writable: false, configurable: false, enumerable: true },
  'data nc rw 1': { value: 1, writable: true, configurable: false, enumerable: true },
  'data nc rw 2': { value: 2, writable: true, configurable: false, enumerable: true },
  'data c ro 1': { value: 1, writable: false, configurable: true, enumerable: true },
  'data c rw 2': { value: 2, writable: true, configurable: true, enumerable: true },
  'data nc ro 1 not enumerable': { value: 1, writable: false, configurable: false, enumerable: false },
  'accessor nc g1 s1': { get: g1, set: s1, configurable: false, enumerable: true },
  'accessor nc g2 s1': { get: g2, set: s1, configurable: false, enumerable: true },
  'accessor nc g1 s2': { get: g1, set: s2, configurable: false, enumerable: true },
  'accessor nc g2 s2': { get: g2, set: s2, configurable: false, enumerable: true },
  'accessor nc g1': { get: g1, configurable: false, enumerable: true },
  'accessor nc g2': { get: g2, configurable: false, enumerable: true },
  'accessor nc s1': { set: s1, configurable: false, enumerable: true },
  'accessor nc s2': { set: s2, configurable: false, enumerable: true },
  'accessor c g1 s1': { get: g1, set: s1, configurable: true, enumerable: true },
  'accessor c g2 s2': { get: g2, set: s2, configurable: true, enumerable: true },
}
function show(d) {
  if (d === undefined) return 'undefined'
  return ['value', 'writable', 'get', 'set', 'configurable', 'enumerable'].filter(k => k in d).map(k => k + '=' + (typeof d[k] === 'function' ? (d[k] === g1 ? 'g1' : d[k] === g2 ? 'g2' : d[k] === s1 ? 's1' : 's2') : String(d[k]))).join(' ')
}
function row() {
  const rows = []
  for (const [tn, mk] of Object.entries(targets)) {
    for (const [rn, rep] of Object.entries(reports)) {
      let r
      try {
        const p = new Proxy(mk(), { getOwnPropertyDescriptor() { return rep } })
        r = show(Object.getOwnPropertyDescriptor(p, 'p'))
      } catch (e) { r = 'throws ' + e.name }
      let d
      try {
        const p = new Proxy(mk(), { defineProperty() { return true } })
        d = String(Reflect.defineProperty(p, 'p', rep || {}))
      } catch (e) { d = 'throws ' + e.name }
      rows.push(tn + ' | ' + rn + ' => ' + r + ' | define ' + d)
    }
  }
  return rows
}
out.push(...row())
function nested() {
  // a trap that reports through Reflect and a nested proxy
  const t = Object.defineProperty({}, 'p', { value: 1, writable: false, configurable: false })
  const inner = new Proxy(t, { getOwnPropertyDescriptor(t, k) { return { value: 2, writable: false, configurable: false } } })
  const outer = new Proxy(inner, {})
  try { return String(Object.getOwnPropertyDescriptor(outer, 'p')) } catch (e) { return 'throws ' + e.name }
}
out.push('nested: ' + nested())
out.push('keys: ' + (() => { const p = new Proxy(Object.defineProperty({}, 'p', { value: 1, configurable: false, enumerable: true }), { getOwnPropertyDescriptor() { return { value: 9, configurable: false, enumerable: true } } }); try { return Object.keys(p).join() } catch (e) { return 'throws ' + e.name } })())
out.push('has own: ' + (() => { const p = new Proxy(Object.defineProperty({}, 'p', { value: 1, configurable: false, enumerable: true }), { getOwnPropertyDescriptor() { return { value: 9, configurable: false, enumerable: true } } }); try { return String(Object.hasOwn(p, 'p')) } catch (e) { return 'throws ' + e.name } })())
if (typeof memorySize === 'function') {
  const mk = () => { for (let i = 0; i < 20; i++) row() }
  // the same blocks in use after two collections (see strict_equal.js)
  mk(); mk(); gc()
  mk(); mk(); gc(); gc()
  const m1 = memorySize(), k1 = memoryBlocks()
  mk(); mk(); gc(); gc()
  out.push('memory after: ' + heapChange(m1, k1))
} else {
  out.push('memory after: 0')
}
out.join('\n')
