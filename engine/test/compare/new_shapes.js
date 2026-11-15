// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// Objects made by new C() of a function or a class whose body sets keys on
// this, one after another, many of each, some kept and some dropped, so that
// the shapes of earlier objects are there to be found by the next ones. Each
// object must list the keys it got in the order it got them, also while the
// constructor runs, and a store must still meet what the prototype chain has
// at that moment: a setter, a read-only key, a getter alone, a Proxy, a
// prototype changed between two objects, keys deleted and added again,
// objects frozen or given another prototype, constructors that branch, that
// return another object, derived classes and fields, and more keys than a
// shape keeps a parent for.
const out = []
const keys = (o) => JSON.stringify(Reflect.ownKeys(o).map(String))
const desc = (o) => JSON.stringify(Reflect.ownKeys(o).map((k) => {
  const d = Object.getOwnPropertyDescriptor(o, k)
  return String(k) + ':' + ('value' in d ? JSON.stringify(d.value) : 'acc') +
    (d.writable ? 'w' : '') + (d.enumerable ? 'e' : '') + (d.configurable ? 'c' : '')
}))

// a plain function, the objects kept, then dropped, then made again
function Pair(a, b) { this.car = a; this.cdr = b }
{
  const kept = []
  for (let i = 0; i < 20; i++) kept.push(new Pair(i, i * 2))
  out.push('Pair kept: ' + kept.map(keys).join(' ') + ' ' + kept[19].car + ',' + kept[19].cdr)
  kept.length = 0
  for (let i = 0; i < 5; i++) { const p = new Pair(i, 1); out.push('Pair alone ' + i + ': ' + keys(p) + ' ' + desc(p)) }
}

// what the object has while its constructor runs
function Seen(a) {
  this.log = [keys(this)]
  this.x = a
  this.log.push(keys(this))
  this.y = a + 1
  this.log.push(keys(this))
  this.z = a + 2
  this.log.push(keys(this))
}
{
  const objs = []
  for (let i = 0; i < 6; i++) objs.push(new Seen(i))
  out.push('Seen: ' + objs.map((o) => o.log.join('|') + ' ' + keys(o)).join(' / '))
}

// a setter on the prototype, put there after objects were made
function S(v) { this.a = v; this.b = v + 1; this.c = v + 2 }
{
  const objs = []
  for (let i = 0; i < 6; i++) objs.push(new S(i))
  const got = []
  Object.defineProperty(S.prototype, 'b', { set(v) { got.push(v); this._b = v }, get() { return this._b }, configurable: true })
  for (let i = 0; i < 4; i++) objs.push(new S(10 * i))
  out.push('S setter: ' + objs.map(keys).join(' ') + ' got ' + got.join(','))
  delete S.prototype.b
  const after = new S(7)
  out.push('S setter gone: ' + keys(after) + ' ' + after.b)
}

// a read-only key, then a getter alone, on the prototype, sloppy and strict
function R(v) { this.a = v; this.b = v }
function RS(v) { 'use strict'; this.a = v; this.b = v }
for (const C of [R, RS]) {
  const objs = []
  for (let i = 0; i < 5; i++) objs.push(new C(i))
  Object.defineProperty(C.prototype, 'b', { value: 'ro', writable: false, configurable: true })
  let r
  try { r = keys(new C(9)) + ' ' + new C(9).b } catch (e) { r = e.constructor.name }
  out.push(C.name + ' read-only: ' + objs.map(keys).join(' ') + ' then ' + r)
  Object.defineProperty(C.prototype, 'b', { get() { return 'g' }, configurable: true })
  try { r = keys(new C(9)) + ' ' + new C(9).b } catch (e) { r = e.constructor.name }
  out.push(C.name + ' getter alone: ' + r)
  delete C.prototype.b
  out.push(C.name + ' plain again: ' + keys(new C(3)) + ' ' + new C(3).b)
}

// a Proxy as the prototype: its traps run for each new key
function PX(v) { this.a = v; this.b = v }
{
  const objs = []
  for (let i = 0; i < 4; i++) objs.push(new PX(i))
  const trapped = []
  PX.prototype = new Proxy({}, {
    set(t, k, v, recv) { trapped.push('set ' + String(k)); return Reflect.set(t, k, v, recv) },
    get(t, k, recv) { return Reflect.get(t, k, recv) },
    has(t, k) { trapped.push('has ' + String(k)); return Reflect.has(t, k) },
    getOwnPropertyDescriptor(t, k) { trapped.push('gopd ' + String(k)); return Reflect.getOwnPropertyDescriptor(t, k) },
  })
  for (let i = 0; i < 3; i++) objs.push(new PX(i))
  out.push('PX proxy: ' + objs.map(keys).join(' ') + ' traps ' + trapped.join(','))
}

// the prototype changed between two objects, and back
function Ch(v) { this.a = v; this.b = v }
{
  const protoA = Ch.prototype
  const protoB = { kind: 'B' }
  const objs = []
  for (let i = 0; i < 4; i++) objs.push(new Ch(i))
  Ch.prototype = protoB
  for (let i = 0; i < 4; i++) objs.push(new Ch(i))
  Ch.prototype = protoA
  for (let i = 0; i < 2; i++) objs.push(new Ch(i))
  Object.defineProperty(Ch, 'prototype', { value: Object.create(protoB, { b: { set(v) { this.viaSetter = v } } }) })
  for (let i = 0; i < 2; i++) objs.push(new Ch(i))
  out.push('Ch: ' + objs.map((o) => keys(o) + (Object.getPrototypeOf(o) === protoA ? 'A' : Object.getPrototypeOf(o) === protoB ? 'B' : 'C')).join(' '))
  Ch.prototype = 5
  const prim = new Ch(1)
  out.push('Ch primitive prototype: ' + keys(prim) + ' ' + (Object.getPrototypeOf(prim) === Object.prototype))
}

// keys deleted and added again, in and out of the constructor
function D(v) { this.a = v; this.b = v; delete this.a; this.a = v + 1; this.c = v }
{
  const objs = []
  for (let i = 0; i < 6; i++) objs.push(new D(i))
  delete objs[2].b
  objs[3].b = undefined
  delete objs[4].c
  objs[4].c = 1
  for (let i = 0; i < 3; i++) objs.push(new D(i))
  out.push('D: ' + objs.map(keys).join(' '))
}

// an object frozen, sealed, made non-extensible or given another prototype,
// and the next ones
function F2(v) { this.p = v; this.q = v }
{
  const objs = []
  for (let i = 0; i < 4; i++) objs.push(new F2(i))
  Object.freeze(objs[0])
  Object.seal(objs[1])
  Object.preventExtensions(objs[2])
  Object.setPrototypeOf(objs[3], { other: 1 })
  for (let i = 0; i < 4; i++) objs.push(new F2(i))
  objs[5].r = 1
  out.push('F2: ' + objs.map((o) => keys(o) + (Object.isFrozen(o) ? 'F' : '') + (Object.isSealed(o) ? 'S' : '') + (Object.isExtensible(o) ? '' : 'N') + ('other' in o ? 'O' : '')).join(' '))
  let r
  try { (function () { 'use strict'; objs[0].p = 9 })(); r = objs[0].p } catch (e) { r = e.constructor.name }
  out.push('F2 frozen store: ' + r + ' ' + objs[4].p)
}

// a constructor that branches, so its objects get keys in other orders
function Br(k) {
  if (k % 3 === 0) { this.a = 1; this.b = 2 }
  else if (k % 3 === 1) { this.b = 2; this.a = 1 }
  else { this.a = 1; this.c = 3; this.b = 2 }
  this.z = k
}
{
  const objs = []
  for (let i = 0; i < 12; i++) objs.push(new Br(i))
  out.push('Br: ' + objs.map(keys).join(' '))
}

// a constructor that returns another object, or a primitive
function Ret(v) { this.a = v; if (v % 2) return { other: v }; return v }
{
  const objs = []
  for (let i = 0; i < 6; i++) objs.push(new Ret(i))
  out.push('Ret: ' + objs.map((o) => keys(o) + (o instanceof Ret)).join(' '))
}

// classes: fields, a derived class, private names, an accessor in the class
class Base {
  x = 1
  #p = 2
  constructor(v) { this.y = v }
  get p() { return this.#p }
}
class Derived extends Base {
  z = 3
  constructor(v) { super(v); this.w = v }
}
class Acc {
  set b(v) { this._b = v }
  constructor(v) { this.a = v; this.b = v; this.c = v }
}
{
  const objs = []
  for (let i = 0; i < 5; i++) objs.push(new Base(i), new Derived(i), new Acc(i))
  out.push('classes: ' + objs.map((o) => keys(o) + (o.p || '')).join(' '))
  const dr = Reflect.construct(Derived, [5], Base)
  out.push('Reflect.construct(Derived, [5], Base): ' + keys(dr) + ' ' + (Object.getPrototypeOf(dr) === Base.prototype))
  function Other() {}
  for (let i = 0; i < 3; i++) out.push('Reflect.construct(Pair, [i], Other): ' + keys(Reflect.construct(Pair, [i, i], Other)) + ' ' + (Reflect.construct(Pair, [1, 1], Other) instanceof Other))
}

// a constructor's objects and the same keys added to other objects of its
// prototype: Object.create, setPrototypeOf, and an object made apart
function Mix(v) { this.m = v; this.n = v }
{
  const objs = []
  for (let i = 0; i < 4; i++) objs.push(new Mix(i))
  const c = Object.create(Mix.prototype)
  c.m = 1
  c.n = 2
  c.o = 3
  const s = { m: 1 }
  Object.setPrototypeOf(s, Mix.prototype)
  s.n = 2
  for (let i = 0; i < 3; i++) objs.push(new Mix(i))
  objs[1].extra = 1
  out.push('Mix: ' + objs.map(keys).join(' ') + ' ' + keys(c) + ' ' + keys(s) + ' ' + (c instanceof Mix) + (s instanceof Mix))
}

// more keys than a shape keeps a parent for, numbers and symbols as keys
const sym = Symbol('s')
function Many(n) {
  for (let i = 0; i < n; i++) this['k' + i] = i
  this[sym] = n
  this[7] = 'seven'
  this['01'] = 'zero one'
}
{
  const objs = []
  for (let i = 0; i < 4; i++) objs.push(new Many(40))
  objs.push(new Many(3), new Many(31), new Many(32), new Many(33))
  for (let i = 0; i < 2; i++) objs.push(new Many(40))
  out.push('Many: ' + objs.map((o) => Reflect.ownKeys(o).length + ':' + String(Reflect.ownKeys(o).slice(0, 3).map(String)) + ':' + String(Reflect.ownKeys(o).slice(-3).map(String))).join(' '))
}

// for in and JSON over objects of a constructor
function J(v) { this.b = v; this.a = v + 1 }
J.prototype.inherited = 1
{
  const objs = []
  for (let i = 0; i < 5; i++) objs.push(new J(i))
  const fi = []
  for (const k in objs[4]) fi.push(k)
  out.push('J: ' + JSON.stringify(objs) + ' ' + fi.join(','))
}

out.join('\n')
