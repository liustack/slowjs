// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// Node 24 output. Covers the compaction regression of QuickJS 3c4c6ed,
// non-configurable indices, shared shapes, accessors and non-index keys.
(() => {
  const out = []
  const a = [0, 1, 2, 3, 4, 5, 6, 7]
  a[10] = 1
  a.length = 0
  out.push('empty: ' + Object.keys(a) + ' ' + a.length)
  a[10] = 2
  out.push('refill: ' + a.length + ' ' + a.toString())
  for (const blocked of [false, true]) {
    const make = () => {
      const b = []
      for (let i = 0; i < 40; i++) b[i * 3] = { i }
      b.extra = 42
      b[Symbol.for('keep')] = 43
      Object.defineProperty(b, '90', { get() { throw Error('read') }, configurable: true })
      if (blocked) Object.defineProperty(b, '30', { configurable: false })
      return b
    }
    const b = make(), other = make()
    const ok = Reflect.set(b, 'length', 3)
    out.push('truncate ' + blocked + ': ' + ok + ' ' + b.length + ' ' + Object.keys(b) + ' ' + b.extra + ' ' + b[Symbol.for('keep')])
    out.push('other: ' + other.length + ' ' + Object.keys(other).length)
    b[120] = 'new'
    out.push('grow: ' + b.length + ' ' + b[120] + ' ' + (90 in b))
  }
  return out.join('\n')
})()
