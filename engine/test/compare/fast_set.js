// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// The elements of every kind of fast array written by the paths that take a
// property key rather than a number: Reflect.set and a computed key that is a
// string, and Object.defineProperty on a typed array. The value is converted
// for a typed array, by a valueOf that may throw or shrink the array first,
// and a write to mapped arguments changes the parameter.
const show = v => typeof v === 'bigint' ? v + 'n' : Object.is(v, -0) ? '-0' : String(v)
const list = a => Array.prototype.map.call(a, show).join(',')
const err = f => { try { return show(f()) } catch (e) { return e.name } }
const kinds = [
  ['Array', () => [1, 2, 3], 'v'],
  ['arguments', function () { return (function () { 'use strict'; return arguments })(1, 2, 3) }, 'v'],
  ['Int8Array', () => new Int8Array(3), 200],
  ['Uint8Array', () => new Uint8Array(3), -1],
  ['Uint8ClampedArray', () => new Uint8ClampedArray(3), 7.5],
  ['Int16Array', () => new Int16Array(3), 40000],
  ['Uint16Array', () => new Uint16Array(3), -2],
  ['Int32Array', () => new Int32Array(3), 3e9],
  ['Uint32Array', () => new Uint32Array(3), -3],
  ['BigInt64Array', () => new BigInt64Array(3), 2n ** 63n],
  ['BigUint64Array', () => new BigUint64Array(3), -1n],
  ['Float16Array', () => typeof Float16Array === 'function' ? new Float16Array(3) : null, 1 / 3],
  ['Float32Array', () => new Float32Array(3), 1 / 3],
  ['Float64Array', () => new Float64Array(3), -0],
]
const out = []
for (const [name, make, v] of kinds) {
  if (make() === null) { out.push(name + ': none'); continue }
  const line = []
  let a = make()
  line.push('set ' + err(() => Reflect.set(a, '0', v)) + ' ' + list(a))
  a = make()
  a['1'] = v
  line.push('key ' + list(a))
  a = make()
  line.push('valueOf ' + err(() => Reflect.set(a, '2', { valueOf() { return v } })) + ' ' + list(a))
  a = make()
  line.push('throws ' + err(() => Reflect.set(a, '0', { valueOf() { throw new RangeError() } })) + ' ' + list(a))
  a = make()
  line.push('past ' + err(() => Reflect.set(a, '5', v)) + ' ' + a.length)
  if (ArrayBuffer.isView(a)) {
    a = make()
    line.push('define ' + err(() => Object.defineProperty(a, '1', { value: v, writable: true, enumerable: true, configurable: true })) + ' ' + list(a))
    a = make()
    line.push('define bad ' + err(() => Object.defineProperty(a, '1', { value: v, writable: false })) + ' ' + list(a))
    a = make()
    line.push('define past ' + err(() => Object.defineProperty(a, '3', { value: v })))
    const C = a.constructor, rab = new ArrayBuffer(3 * C.BYTES_PER_ELEMENT, { maxByteLength: 8 * C.BYTES_PER_ELEMENT })
    const t = new C(rab)
    const shrink = { valueOf() { rab.resize(C.BYTES_PER_ELEMENT); return v } }
    line.push('shrink ' + err(() => Reflect.set(t, '2', shrink)) + ' ' + list(t))
    rab.resize(3 * C.BYTES_PER_ELEMENT)
    line.push('shrink define ' + err(() => Object.defineProperty(t, '2', { value: shrink })) + ' ' + list(t))
  }
  out.push(name + ': ' + line.join(' | '))
}
{
  const f = function (a, b) { const args = arguments; Reflect.set(args, '0', 'x'); args['1'] = 'y'; return [a, b, args[0], args[1]].join() }
  out.push('mapped arguments: ' + f(1, 2))
  const g = function (a) { const args = arguments; Object.defineProperty(args, '0', { value: 'z' }); return [a, args[0]].join() }
  out.push('mapped define: ' + g(1))
}
{
  const a = [1, 2, 3]
  Object.freeze(a)
  out.push('frozen: ' + err(() => Reflect.set(a, '0', 9)) + ' ' + list(a))
  const b = Object.create([1, 2, 3])
  out.push('inherited: ' + err(() => Reflect.set(b, '0', 9)) + ' ' + list(b) + ' ' + Object.keys(b))
}
out.join('\n')
