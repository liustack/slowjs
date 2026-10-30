// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// An index past the end of an array that is not extensible cannot be added
// by a set, a define or a push, and the length of the array stays as it was.
// Arrays that are extensible, sealed, frozen, with a length that cannot be
// written, dense and with holes. The file is what Node 24 prints.
const out = []
function run(name, f) {
  let r
  try { r = f() } catch (e) { r = 'throws ' + e.name }
  out.push(name + ': ' + (typeof r === 'string' ? r : JSON.stringify(r)))
}
const makers = {
  plain: () => [1, 2, 3],
  prevented: () => Object.preventExtensions([1, 2, 3]),
  sealed: () => Object.seal([1, 2, 3]),
  frozen: () => Object.freeze([1, 2, 3]),
  holes: () => Object.preventExtensions([1, , 3]),
  empty: () => Object.preventExtensions([]),
  sparse: () => { const a = [1, 2, 3]; a[100] = 4; return Object.preventExtensions(a) },
  fixedLength: () => Object.defineProperty([1, 2, 3], 'length', { writable: false }),
  preventedFixedLength: () => Object.preventExtensions(Object.defineProperty([1, 2, 3], 'length', { writable: false })),
}
const indexes = [0, 2, 3, 4, 7, 100, 4294967294, 4294967295, '5', 1.5, -1]
for (const [name, mk] of Object.entries(makers)) {
  for (const i of indexes) {
    run('set ' + name + ' ' + i, () => { const a = mk(); const r = Reflect.set(a, i, 'v'); return [r, a.length, Object.keys(a).length] })
    run('define ' + name + ' ' + i, () => { const a = mk(); const r = Reflect.defineProperty(a, i, { value: 'v', writable: true, enumerable: true, configurable: true }); return [r, a.length, Object.keys(a).length] })
    run('assign ' + name + ' ' + i, () => { const a = mk(); a[i] = 'v'; return [a.length, Object.keys(a).length] })
    run('strict assign ' + name + ' ' + i, () => { 'use strict'; const a = mk(); a[i] = 'v'; return [a.length, Object.keys(a).length] })
    run('Object.defineProperty ' + name + ' ' + i, () => { const a = mk(); Object.defineProperty(a, i, { value: 'v' }); return [a.length, Object.keys(a).length] })
  }
  run('push ' + name, () => { const a = mk(); try { a.push(9) } catch (e) { return [e.name, a.length] } return [a.length] })
  run('unshift ' + name, () => { const a = mk(); try { a.unshift(9) } catch (e) { return [e.name, a.length] } return [a.length] })
  run('length set ' + name, () => { const a = mk(); try { a.length = 10 } catch (e) { return [e.name, a.length] } return [a.length] })
  run('splice grow ' + name, () => { const a = mk(); try { a.splice(1, 0, 'x') } catch (e) { return [e.name, a.length] } return [a.length] })
  run('fill ' + name, () => { const a = mk(); try { a.fill(0) } catch (e) { return [e.name, a.length] } return [a.length, a[0]] })
  run('copyWithin ' + name, () => { const a = mk(); try { a.copyWithin(0, 1) } catch (e) { return [e.name, a.length] } return [a.length, a[0]] })
}
out.join('\n')
