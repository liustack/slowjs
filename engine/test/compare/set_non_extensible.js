// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// Setting a property whose receiver is not the object the set started from
// (super.x = v, Reflect.set with a receiver) changes the property the receiver
// already has, even when the receiver is not extensible, and fails to add one
// (upstream QuickJS fce112d). The same sets on the object itself, on arrays,
// and on frozen, sealed and prevented objects. The file is what Node 24
// prints.
const out = []
function run(name, f) {
  let r
  try { r = f() } catch (e) { r = 'throws ' + e.name }
  out.push(name + ': ' + (typeof r === 'string' ? r : JSON.stringify(r)))
}
const prep = {
  plain: () => ({ x: 0 }),
  prevented: () => Object.preventExtensions({ x: 0 }),
  sealed: () => Object.seal({ x: 0 }),
  frozen: () => Object.freeze({ x: 0 }),
  array: () => [0, 1],
  preventedArray: () => Object.preventExtensions([0, 1]),
  frozenArray: () => Object.freeze([0, 1]),
  accessor: () => Object.preventExtensions({ get x() { return 1 }, set x(v) { } }),
  readonly: () => Object.preventExtensions(Object.defineProperty({}, 'x', { value: 0, writable: false, configurable: true })),
  proxy: () => new Proxy(Object.preventExtensions({ x: 0 }), {}),
  fn: () => Object.preventExtensions(function () { }),
}
const keys = ['x', 'y', '0', '1', '2', 'length']
for (const [name, mk] of Object.entries(prep)) {
  for (const k of keys) {
    run('Reflect.set on ' + name + ' ' + k, () => { const o = mk(); const r = Reflect.set(o, k, 7); return [r, Object.keys(o).length, o[k]] })
    run('Reflect.set with receiver ' + name + ' ' + k, () => { const o = mk(); const r = Reflect.set({ __proto__: null }, k, 7, o); return [r, Object.keys(o).length, o[k]] })
    run('Reflect.set proto setter ' + name + ' ' + k, () => { const o = mk(); const r = Reflect.set({ set [k](v) { } }, k, 7, o); return [r, Object.keys(o).length, o[k]] })
    run('Reflect.set on a child of ' + name + ' ' + k, () => { const o = mk(); const c = Object.create(o); const r = Reflect.set(c, k, 7); return [r, Object.keys(c).length, c[k]] })
  }
  run('assignment in sloppy mode ' + name, () => { const o = mk(); o.x = 5; o.y = 6; o[2] = 8; return [o.x, o.y, o[2]] })
  run('assignment in strict mode ' + name, () => { 'use strict'; const o = mk(); o.x = 5; return o.x })
  run('adding in strict mode ' + name, () => { 'use strict'; const o = mk(); o.zz = 5; return o.zz })
  run('push ' + name, () => { const o = mk(); if (Array.isArray(o)) o.push(9); return o.length })
}
run('super.x on a prevented receiver', () => {
  const base = { }
  const obj = { __proto__: base, f() { super.x = 5; return this.x } }
  return obj.f.call(Object.preventExtensions({ x: 0 }))
})
run('super.y on a prevented receiver', () => {
  const obj = { __proto__: {}, f() { super.y = 5; return this.y } }
  return obj.f.call(Object.preventExtensions({ x: 0 }))
})
run('super.x on a frozen receiver', () => {
  'use strict'
  const obj = { __proto__: {}, f() { super.x = 5; return this.x } }
  return obj.f.call(Object.freeze({ x: 0 }))
})
run('super.x in a class', () => {
  class A { }
  class B extends A { set(o) { super.x = 5; return this.x } }
  const b = new B()
  Object.preventExtensions(b)
  b.x = 1
  return b.set()
})
run('super.x in a class with x', () => {
  class A { }
  class B extends A { constructor() { super(); this.x = 0; Object.preventExtensions(this) } set() { super.x = 5; return this.x } }
  return new B().set()
})
run('super[k] in a static method', () => {
  class A { static a = 1 }
  class B extends A { static f() { super.a = 2; return [this.a, Object.hasOwn(this, 'a')] } }
  Object.preventExtensions(B)
  return B.f()
})
run('static field of a prevented class', () => {
  class A { static a = 1 }
  class B extends A { static f() { super.b = 2; return Object.hasOwn(this, 'b') } }
  Object.preventExtensions(B)
  return B.f()
})
run('module-like frozen namespace', () => {
  const ns = Object.freeze(Object.create(null, { x: { value: 1, enumerable: true } }))
  return Reflect.set({}, 'x', 2, ns)
})
run('global object prevented', () => {
  const g = Object.preventExtensions({ gx: 1 })
  return [Reflect.set({}, 'gx', 3, g), g.gx]
})
out.join('\n')
