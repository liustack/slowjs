// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// Object.defineProperties and Object.create with properties follow the order
// of the specification: all the keys of the properties object first, then for
// each key its own descriptor and its value, and only after all of them the
// definitions, so that no definition is made when a later key throws
// (upstream QuickJS 535a7c2). Each descriptor is also converted when its key is
// read, so a getter of a later key cannot change an earlier descriptor, a
// descriptor that is not an object or has a field that throws defines nothing,
// and the fields of the descriptors are read in the order of the keys (the
// conversion is ours: upstream kept the descriptor objects). The file is what
// Node 24 prints.
const out = []
function run(name, f) {
  let r
  try { r = f() } catch (e) { r = 'throws ' + e.name }
  out.push(name + ': ' + String(r))
}
function logged(log, props) {
  return new Proxy(props, {
    ownKeys(t) { log.push('ownKeys'); return Reflect.ownKeys(t) },
    getOwnPropertyDescriptor(t, k) { log.push('gopd ' + String(k)); return Reflect.getOwnPropertyDescriptor(t, k) },
    get(t, k) { log.push('get ' + String(k)); return Reflect.get(t, k) },
  })
}
run('order of the traps', () => {
  const log = []
  const target = new Proxy({}, { defineProperty(t, k, d) { log.push('define ' + k); return Reflect.defineProperty(t, k, d) } })
  Object.defineProperties(target, logged(log, { a: { value: 1 }, b: { value: 2 }, c: { value: 3 } }))
  return log.join()
})
run('order with Object.create', () => {
  const log = []
  Object.create(null, logged(log, { a: { value: 1 }, b: { value: 2 } }))
  return log.join()
})
run('not enumerable is skipped', () => {
  const log = []
  const props = Object.defineProperties({ a: { value: 1 } }, { h: { value: { value: 2 }, enumerable: false } })
  const o = Object.defineProperties({}, logged(log, props))
  return log.join() + ' ' + Object.getOwnPropertyNames(o)
})
run('symbols', () => {
  const log = []
  const s = Symbol('s')
  const o = Object.defineProperties({}, logged(log, { a: { value: 1 }, [s]: { value: 2 } }))
  return log.join().replace(/Symbol\(s\)/g, 'S') + ' ' + Reflect.ownKeys(o).length
})
run('a later descriptor is not an object', () => {
  const o = {}
  try { Object.defineProperties(o, { a: { value: 1 }, b: 5 }) } catch (e) { return e.name + ' [' + Reflect.ownKeys(o) + ']' }
})
run('a later getter changes an earlier descriptor', () => {
  const d = { value: 1 }
  const o = Object.defineProperties({}, { a: d, get b() { d.value = 2; return { value: 3 } } })
  return o.a + ',' + o.b
})
run('order of the fields of descriptors and of the keys', () => {
  const log = []
  Object.defineProperties({}, {
    a: { get value() { log.push('a.value'); return 1 } },
    get b() { log.push('b'); return { get value() { log.push('b.value'); return 2 } } },
  })
  return log.join()
})
run('a field of a descriptor throws, nothing is defined', () => {
  const o = {}
  try { Object.defineProperties(o, { a: { value: 1 }, b: { get value() { throw new RangeError('f') } } }) } catch (e) { return e.name + ' [' + Reflect.ownKeys(o) + ']' }
})
run('a field of the first descriptor throws, the next is not read', () => {
  const log = []
  const o = {}
  try { Object.defineProperties(o, { a: { get enumerable() { log.push('a.enumerable'); throw new RangeError('f') } }, get b() { log.push('b'); return {} } }) } catch (e) { log.push(e.name) }
  return log.join() + ' [' + Reflect.ownKeys(o) + ']'
})
run('invalid getter in a later descriptor, nothing is defined', () => {
  const o = {}
  try { Object.defineProperties(o, { a: { value: 1 }, b: { get: 5 } }) } catch (e) { return e.name + ' [' + Reflect.ownKeys(o) + ']' }
})
run('getter and value in a later descriptor, nothing is defined', () => {
  const o = {}
  try { Object.defineProperties(o, { a: { value: 1 }, b: { get() { }, value: 2 } }) } catch (e) { return e.name + ' [' + Reflect.ownKeys(o) + ']' }
})
run('accessor descriptors are kept as read', () => {
  const g = function () { return 7 }
  const d = { get: g, enumerable: true, configurable: true }
  const o = Object.defineProperties({}, { a: d, get b() { d.get = function () { return 8 }; return { value: 0 } } })
  return o.a
})
run('a later getter throws', () => {
  const o = {}
  try { Object.defineProperties(o, { a: { value: 1 }, get b() { throw new RangeError('g') } }) } catch (e) { return e.name + ' [' + Object.getOwnPropertyNames(o) + ']' }
})
run('a later definition throws', () => {
  const o = Object.defineProperty({}, 'b', { value: 0 })
  try { Object.defineProperties(o, { a: { value: 1 }, b: { value: 2 } }) } catch (e) { return e.name + ' ' + Object.getOwnPropertyNames(o) }
})
run('getter changes a later key', () => {
  const props = { get a() { delete this.b; return { value: 1 } }, b: { value: 2 } }
  const o = Object.defineProperties({}, props)
  return Object.getOwnPropertyNames(o)
})
run('getter makes a later key not enumerable', () => {
  const props = { get a() { Object.defineProperty(this, 'b', { enumerable: false }); return { value: 1 } }, b: { value: 2 }, c: { value: 3 } }
  const o = Object.defineProperties({}, props)
  return Object.getOwnPropertyNames(o)
})
run('getter adds a key', () => {
  const props = { get a() { this.z = { value: 9 }; return { value: 1 } } }
  const o = Object.defineProperties({}, props)
  return Object.getOwnPropertyNames(o)
})
run('key order', () => {
  const o = Object.defineProperties({}, { b: { value: 1, enumerable: true }, 2: { value: 2, enumerable: true }, a: { value: 3, enumerable: true }, 1: { value: 4, enumerable: true } })
  return Object.keys(o)
})
run('inherited keys are not used', () => {
  const props = Object.create({ inherited: { value: 1 } }, { own: { value: { value: 2 }, enumerable: true } })
  return Object.getOwnPropertyNames(Object.defineProperties({}, props))
})
run('primitive properties', () => Object.getOwnPropertyNames(Object.defineProperties({}, 'ab')))
run('number properties', () => Object.getOwnPropertyNames(Object.defineProperties({}, 5)))
run('undefined properties', () => Object.defineProperties({}, undefined))
run('null properties', () => Object.defineProperties({}, null))
run('non-object target', () => Object.defineProperties(1, {}))
run('array target', () => { const a = Object.defineProperties([], { 0: { value: 'x', enumerable: true }, length: { value: 3 } }); return a.length + a[0] })
run('frozen target', () => Object.defineProperties(Object.freeze({}), { a: { value: 1 } }))
run('empty properties on frozen', () => Object.keys(Object.defineProperties(Object.freeze({}), {})))
run('descriptor with accessor and value', () => Object.defineProperties({}, { a: { get() { }, value: 1 } }))
run('result is the target', () => { const o = {}; return Object.defineProperties(o, {}) === o })
run('many keys', () => { const props = {}; for (let i = 0; i < 300; i++) props['k' + i] = { value: i, enumerable: true }; const o = Object.defineProperties({}, props); return Object.keys(o).length + o.k299 })
run('proxy ownKeys throws', () => Object.defineProperties({}, new Proxy({}, { ownKeys() { throw new RangeError('k') } })))
run('proxy gopd throws', () => Object.defineProperties({}, new Proxy({ a: 1 }, { getOwnPropertyDescriptor() { throw new RangeError('d') } })))
if (typeof memorySize === 'function') {
  const mk = () => {
    for (let i = 0; i < 200; i++) {
      try { Object.defineProperties({}, { a: { value: 1 }, get b() { throw 1 }, c: { value: 3 } }) } catch (e) { }
      try { Object.defineProperties({}, { a: { value: 1 }, b: { value: 2 }, c: 5 }) } catch (e) { }
      try { Object.defineProperties(Object.freeze({}), { a: { value: 1 }, b: { value: 2 } }) } catch (e) { }
      try { Object.defineProperties({}, { a: { value: 'x'.repeat(i), get: undefined }, b: { get value() { throw 1 } } }) } catch (e) { }
      try { Object.defineProperties({}, { a: { get() { return {} }, set: undefined }, b: { get: 5 } }) } catch (e) { }
      Object.defineProperties({}, { a: { value: 'x'.repeat(i) }, b: { value: {} }, [Symbol()]: { value: [] } })
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
