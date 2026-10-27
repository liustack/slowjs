// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// The elements of every kind of fast array read by the paths that take a
// property key rather than a number: Reflect.get and a computed key that is
// a string, the descriptor of an own property, and a read through an object
// whose prototype is the array. Each reads the elements below the count, the
// first one past it, and a key that is not an index.
const show = v => typeof v === 'bigint' ? v + 'n' : Object.is(v, -0) ? '-0' : String(v)
const desc = d => d === undefined ? 'none' : show(d.value) + ':' + (d.writable ? 'w' : '') + (d.enumerable ? 'e' : '') + (d.configurable ? 'c' : '')
const kinds = [
  ['Array', () => [1, 'two', -0, undefined]],
  ['holey Array', () => { const a = [1, , 3]; return a }],
  ['arguments', function () { return (function () { 'use strict'; return arguments })(1, 'two', -0) }],
  ['mapped arguments', function () { return (function (a, b) { a = 5; return arguments })(1, 'two', 3) }],
  ['Int8Array', () => new Int8Array([127, -128, -1])],
  ['Uint8Array', () => new Uint8Array([255, 0, 7])],
  ['Uint8ClampedArray', () => new Uint8ClampedArray([300, -5, 7.5])],
  ['Int16Array', () => new Int16Array([32767, -32768, -1])],
  ['Uint16Array', () => new Uint16Array([65535, 0, 9])],
  ['Int32Array', () => new Int32Array([2147483647, -2147483648, -1])],
  ['Uint32Array', () => new Uint32Array([4294967295, 0, 2147483648])],
  ['BigInt64Array', () => new BigInt64Array([9223372036854775807n, -9223372036854775808n, -1n])],
  ['BigUint64Array', () => new BigUint64Array([18446744073709551615n, 0n, 1n])],
  ['Float16Array', () => typeof Float16Array === 'function' ? new Float16Array([1.5, -0, 65504]) : null],
  ['Float32Array', () => new Float32Array([0.1, -0, NaN])],
  ['Float64Array', () => new Float64Array([0.1, -0, -Infinity])],
]
const out = []
for (const [name, make] of kinds) {
  const a = make()
  if (a === null) { out.push(name + ': none'); continue }
  const n = a.length
  const line = []
  for (let i = 0; i <= n; i++) {
    const inherit = Object.create(a)
    line.push([show(Reflect.get(a, String(i))), show(a[String(i)]),
               desc(Object.getOwnPropertyDescriptor(a, String(i))),
               show(inherit[i]), String(i in a), String(Object.prototype.hasOwnProperty.call(a, i))].join(' '))
  }
  line.push('x:' + show(Reflect.get(a, 'x')) + ' ' + desc(Object.getOwnPropertyDescriptor(a, 'x')))
  out.push(name + ': ' + line.join(' | '))
}
out.join('\n')
