// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// Array.prototype.sort writes every element back, also the ones that did not
// move, so that a frozen array, a string, a setter and a proxy see a write for
// each index (upstream QuickJS 9a0197f). An array of one element is not tried:
// V8 returns it untouched where the specification writes it back and throws
// for a frozen one. The file is what Node 24 prints.
const out = []
function run(name, f) {
  let r
  try { r = f() } catch (e) { r = 'throws ' + e.name }
  out.push(name + ': ' + (typeof r === 'string' ? r : JSON.stringify(r)))
}
run('frozen sorted', () => Object.freeze([1, 2]).sort())
run('frozen unsorted', () => Object.freeze([2, 1]).sort())
run('frozen empty', () => Object.freeze([]).sort())
run('frozen with undefined', () => Object.freeze([undefined, undefined]).sort())
run('frozen strings', () => Object.freeze(['a', 'b', 'c']).sort())
run('string this', () => Array.prototype.sort.call('aa'))
run('string this empty', () => Array.prototype.sort.call(''))
run('number this', () => Array.prototype.sort.call(5))
run('sealed sorted', () => Object.seal([1, 2, 3]).sort())
run('non-writable element', () => { const a = [1, 2, 3]; Object.defineProperty(a, 1, { writable: false }); return a.sort() })
run('non-writable element unsorted', () => { const a = [3, 2, 1]; Object.defineProperty(a, 1, { writable: false }); return a.sort() })
run('typed array sorted', () => Object.freeze(new Uint8Array(0)).sort())
run('non-extensible sorted', () => Object.preventExtensions([1, 2, 3]).sort())
run('log of sets, sorted', () => {
  const log = []
  const p = new Proxy([1, 2, 3], { set(t, k, v, r) { log.push(k + '=' + v); return Reflect.set(t, k, v, r) } })
  Array.prototype.sort.call(p)
  return log.join()
})
run('log of sets, unsorted', () => {
  const log = []
  const p = new Proxy([3, 1, 2], { set(t, k, v, r) { log.push(k + '=' + v); return Reflect.set(t, k, v, r) } })
  Array.prototype.sort.call(p)
  return log.join()
})
run('log of sets, with undefined and holes', () => {
  const log = []
  const t = [3, undefined, 1, , 2]
  const p = new Proxy(t, { set(t, k, v, r) { log.push(k + '=' + v); return Reflect.set(t, k, v, r) }, deleteProperty(t, k) { log.push('delete ' + k); return Reflect.deleteProperty(t, k) } })
  Array.prototype.sort.call(p)
  return log.join()
})
run('log of sets, comparator', () => {
  const log = []
  const p = new Proxy([1, 2, 3, 4], { set(t, k, v, r) { log.push(k + '=' + v); return Reflect.set(t, k, v, r) } })
  Array.prototype.sort.call(p, (a, b) => b - a)
  return log.join()
})
run('array-like with setters', () => {
  const log = []
  const o = { length: 3 }
  for (let i = 0; i < 3; i++) Object.defineProperty(o, i, { get() { return i + 1 }, set(v) { log.push(i + '=' + v) }, enumerable: true })
  Array.prototype.sort.call(o)
  return log.join()
})
run('comparator throws, nothing written', () => {
  const log = []
  const p = new Proxy([3, 1, 2], { set(t, k, v, r) { log.push(k + '=' + v); return Reflect.set(t, k, v, r) } })
  try { Array.prototype.sort.call(p, () => { throw new RangeError('c') }) } catch (e) { log.push(e.name) }
  return log.join()
})
run('write throws on the second', () => {
  const log = []
  const p = new Proxy([1, 2, 3], { set(t, k, v, r) { log.push(k + '=' + v); if (k === '1') throw new RangeError('s'); return Reflect.set(t, k, v, r) } })
  try { Array.prototype.sort.call(p) } catch (e) { log.push(e.name) }
  return log.join()
})
run('write returns false', () => {
  const p = new Proxy([1, 2, 3], { set() { return false } })
  return Array.prototype.sort.call(p)
})
run('result is this', () => { const a = [2, 1]; return a.sort() === a })
run('plain sort still sorts', () => [5, 3, 9, 1, 3].sort().join())
run('numeric strings', () => ['10', '9', '1'].sort().join())
run('comparator', () => [5, 3, 9, 1].sort((a, b) => a - b).join())
run('toSorted keeps the array', () => { const a = Object.freeze([2, 1]); return a.toSorted().join() })
if (typeof memorySize === 'function') {
  const mk = () => {
    for (let i = 0; i < 200; i++) {
      try { Object.freeze(['x'.repeat(i), 'a', 'b']).sort() } catch (e) { }
      try { Array.prototype.sort.call('abc') } catch (e) { }
      const a = [3, 1, 2, 'q'.repeat(i), undefined]
      a.sort()
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
