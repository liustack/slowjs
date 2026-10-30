// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// The ownKeys trap of a proxy has to return an object, as CreateListFromArrayLike
// asks. A primitive result is a TypeError wherever the keys are read, and an
// array-like object is read by its length (upstream QuickJS 1134912). The file
// is what Node 24 prints.
const out = []
function run(name, f) {
  let r
  try { r = f() } catch (e) { r = 'throws ' + e.name }
  out.push(name + ': ' + (typeof r === 'string' ? r : JSON.stringify(r)))
}
const results = {
  'undefined': undefined, 'null': null, 'number': 1, 'string': 'ab', 'empty string': '', 'true': true, 'symbol': Symbol('s'), 'bigint': 1n,
  'array': ['a', 'b'], 'empty array': [], 'array-like': { length: 2, 0: 'a', 1: 'b' }, 'empty array-like': { length: 0 }, 'object without length': {},
  'array with a number': ['a', 1], 'array with duplicates': ['a', 'a'], 'function': function () { }, 'string object': new String('ab'),
  'array with a symbol': ['a', Symbol.iterator], 'length string': { length: '1', 0: 'k' },
}
const readers = {
  'Object.keys': p => Object.keys(p),
  'Object.getOwnPropertyNames': p => Object.getOwnPropertyNames(p),
  'Reflect.ownKeys': p => Reflect.ownKeys(p).map(String),
  'Object.entries': p => Object.entries(p),
  'for in': p => { const r = []; for (const k in p) r.push(k); return r },
  'spread': p => ({ ...p }),
  'assign': p => Object.assign({}, p),
  'JSON.stringify': p => JSON.stringify(p),
  'Object.getOwnPropertySymbols': p => Object.getOwnPropertySymbols(p).map(String),
  'Object.keys of a nested proxy': p => Object.keys(new Proxy(p, {})),
}
for (const [rn, read] of Object.entries(readers)) {
  for (const [name, res] of Object.entries(results)) {
    run(rn + ' ' + name, () => read(new Proxy({}, { ownKeys() { return res } })))
  }
}
run('trap with a target that has keys', () => Reflect.ownKeys(new Proxy({ a: 1 }, { ownKeys() { return 5 } })))
run('trap not a function', () => Reflect.ownKeys(new Proxy({ a: 1 }, { ownKeys: 5 })))
run('trap is undefined', () => Reflect.ownKeys(new Proxy({ a: 1 }, { ownKeys: undefined })))
run('ownKeys of a function proxy', () => Reflect.ownKeys(new Proxy(function () { }, { ownKeys() { return 'x' } })))
if (typeof memorySize === 'function') {
  const mk = () => {
    for (let i = 0; i < 100; i++) {
      for (const res of [1, 'ab', undefined, null, true, [], ['a']]) {
        try { Reflect.ownKeys(new Proxy({}, { ownKeys() { return res } })) } catch (e) { }
        try { Object.keys(new Proxy({ a: 'x'.repeat(i) }, { ownKeys() { return res } })) } catch (e) { }
      }
    }
  }
  // the same blocks in use after two collections (see strict_equal.js)
  mk(); mk(); gc()
  mk(); mk(); mk(); gc(); gc()
  const m1 = memorySize(), k1 = memoryBlocks()
  mk(); mk(); mk(); gc(); gc()
  out.push('memory after: ' + heapChange(m1, k1))
} else {
  out.push('memory after: 0')
}
out.join('\n')
