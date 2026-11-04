// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// Objects of the built-in classes made one after another, each with the
// prototype of its class, start from the same empty shape. An object that
// gets keys must not change that shape for the next one: each new object
// lists no own keys, whatever the one before it got. new Array(n) starts
// from the shape that has length, and a subclass or another newTarget keeps
// its own prototype.
const out = []
const keys = (o) => JSON.stringify(Reflect.ownKeys(o).map(String))
const sym = Symbol('s')

const kinds = [
  ['Map', () => new Map()],
  ['Set', () => new Set()],
  ['WeakMap', () => new WeakMap()],
  ['Promise', () => new Promise(() => {})],
  ['array iterator', () => [1, 2][Symbol.iterator]()],
  ['map iterator', () => new Map([[1, 2]]).entries()],
  ['string iterator', () => 'ab'[Symbol.iterator]()],
  ['RegExp', () => /a/g],
  ['Date', () => new Date(0)],
  ['Error', () => new Error('e')],
  ['ArrayBuffer', () => new ArrayBuffer(4)],
  ['Uint8Array', () => new Uint8Array(2)],
  ['Boolean', () => new Boolean(true)],
  ['Number', () => new Number(1)],
  ['String', () => new String('ab')],
]

for (const [name, make] of kinds) {
  const first = make()
  first.a = 1
  first[sym] = 2
  Object.defineProperty(first, 'hidden', { value: 3 })
  const second = make()
  second.b = 1
  delete second.b
  const third = make()
  const fourth = make()
  fourth.z = 1
  fourth.y = 2
  out.push(name + ': ' + keys(first) + ' ' + keys(second) + ' ' + keys(third) + ' ' + keys(fourth) +
    ' ' + (Object.getPrototypeOf(third) === Object.getPrototypeOf(first)))
}

// many objects, each with keys of its own, then one without
for (const [name, make] of kinds.slice(0, 5)) {
  const objs = []
  for (let i = 0; i < 50; i++) {
    const o = make()
    o['k' + (i % 7)] = i
    if (i % 3 === 0) o.extra = i
    objs.push(o)
  }
  out.push(name + ' x50: ' + objs.map((o) => Reflect.ownKeys(o).length).join('') + ' ' + keys(make()))
}

// new Array starts with length only, as an array literal does
for (const make of [() => new Array(), () => new Array(3), () => new Array(1, 2), () => Array(2), () => Reflect.construct(Array, [4])]) {
  const a = make()
  a.x = 1
  const b = make()
  out.push('Array: ' + keys(a) + ' ' + keys(b) + ' ' + b.length + ' ' + Array.isArray(b) +
    ' ' + JSON.stringify(Object.getOwnPropertyDescriptor(b, 'length')))
}

// a subclass, or another newTarget, keeps its own prototype and keys
class MyArray extends Array {}
class MyMap extends Map { constructor() { super(); this.own = 1 } }
function F() {}
F.prototype = Object.create(Map.prototype)
for (const [name, make] of [
  ['MyArray', () => new MyArray(2)],
  ['MyMap', () => new MyMap()],
  ['Reflect.construct(Map, [], F)', () => Reflect.construct(Map, [], F)],
  ['Reflect.construct(Array, [1], F)', () => Reflect.construct(Array, [1], F)],
  ['Object.create(Map.prototype)', () => Object.create(Map.prototype)],
]) {
  const a = make()
  a.q = 1
  const b = make()
  out.push(name + ': ' + keys(a) + ' ' + keys(b) + ' ' + (Object.getPrototypeOf(b) === Object.getPrototypeOf(a)) +
    ' ' + Object.prototype.toString.call(b))
}

// an object of a class whose prototype was set, then a new one of the class
const m0 = new Map()
Object.setPrototypeOf(m0, { p: 1 })
const m2 = new Map()
m2.k = 1
out.push('Map with its prototype set: ' + keys(m0) + ' ' + m0.p + ' ' + keys(m2) + ' ' + keys(new Map()) +
  ' ' + (Object.getPrototypeOf(new Map()) === Map.prototype) + ' ' + new Map([[1, 2]]).size)

// an array whose newTarget has the prototype of another class, then objects
// of that class
function G() {}
G.prototype = Map.prototype
const ga = Reflect.construct(Array, [3], G)
ga.x = 1
const gb = Reflect.construct(Array, [], G)
out.push('Array with the prototype of Map: ' + keys(ga) + ' ' + keys(gb) + ' ' + ga.length + ' ' + Array.isArray(gb) +
  ' ' + keys(new Map()) + ' ' + (Object.getPrototypeOf(gb) === Map.prototype))

// arrays after a script changed what Array.prototype inherits from
const oldProto = Object.getPrototypeOf(Array.prototype)
Object.setPrototypeOf(Array.prototype, { inherited: 1 })
const c1 = new Array(2)
c1[5] = 1
delete c1[5]
Object.defineProperty(c1, 'length', { writable: false })
const c2 = new Array(2)
out.push('Array after its prototype changed: ' + keys(c1) + ' ' + keys(c2) + ' ' + c2.inherited +
  ' ' + JSON.stringify(Object.getOwnPropertyDescriptor(c2, 'length')) + ' ' + (Object.getPrototypeOf(c2) === Array.prototype))
Object.setPrototypeOf(Array.prototype, oldProto)

// a prototype of a class that a script changed still makes empty objects
Map.prototype.added = 1
const m1 = new Map()
m1.own = 2
out.push('changed Map.prototype: ' + keys(m1) + ' ' + keys(new Map()) + ' ' + new Map().added)
delete Map.prototype.added

out.join('\n')
