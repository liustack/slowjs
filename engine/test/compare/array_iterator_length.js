// Copyright (c) 2026 The liustack/beastjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.
// The length an Array Iterator, the ownKeys of a Proxy and the replace of a
// RegExp read from an array-like is ToLength (a negative length is 0, a length
// above 2**32 is kept), not ToUint32 (upstream QuickJS e25861c). The file is
// what Node 24 prints, but for the ownKeys of a length above 2**31 - 1, where
// the engine throws a RangeError as V8 does.
const out = []
function run(name, f) {
  let r
  try { r = f() } catch (e) { r = 'throws ' + e.name }
  out.push(name + ': ' + (typeof r === 'string' ? r : JSON.stringify(r)))
}
function count(it, max) {
  let n = 0
  for (const x of it) { if (++n > max) break }
  return n
}
run('values, length -1', () => count(Array.prototype.values.call({ length: -1, 0: 'a' }), 3))
run('keys, length -1', () => count(Array.prototype.keys.call({ length: -1 }), 3))
run('entries, length -1', () => count(Array.prototype.entries.call({ length: -1 }), 3))
run('keys, length 2**32 + 1', () => count(Array.prototype.keys.call({ length: 2 ** 32 + 1 }), 5))
run('values, length 2**32 + 1', () => count(Array.prototype.values.call({ length: 2 ** 32 + 1, 0: 1, 1: 2 }), 5))
run('entries, length 2**32', () => [...Array.prototype.entries.call({ length: 2 ** 32 }).take(3)])
run('keys, length 2**53', () => [...Array.prototype.keys.call({ length: 2 ** 53 }).take(2)])
run('values, length 3', () => [...Array.prototype.values.call({ length: 3, 0: 'a', 2: 'c' })])
run('spread of an array', () => [...[1, 2, 3].values()])
run('spread after one step', () => { const it = [1, 2, 3][Symbol.iterator](); it.next(); return [...it] })
run('spread of a typed array', () => [...new Uint8Array([4, 5, 6]).keys()])
run('proxy ownKeys, length 2**32 + 1', () =>
  Reflect.ownKeys(new Proxy({}, { ownKeys() { return { length: 2 ** 32 + 1, 0: 'a' } } })).length)
run('proxy ownKeys, length -1', () =>
  Reflect.ownKeys(new Proxy({}, { ownKeys() { return { length: -1, 0: 'a' } } })).length)
run('proxy ownKeys, length 2', () =>
  Reflect.ownKeys(new Proxy({}, { ownKeys() { return { length: 2, 0: 'a', 1: 'b' } } })))
run('replace, result of length -1', () => {
  const re = /x/
  re.exec = function () { re.exec = () => null; return { length: -1, 0: 'x', index: 0 } }
  return 'x'.replace(re, '[$&]')
})
run('replace, result of length 2', () => {
  const re = /x/
  re.exec = function () { re.exec = () => null; return { length: 2, 0: 'x', 1: 'y', index: 0 } }
  return 'x'.replace(re, '[$&$1]')
})
out.join('\n')
