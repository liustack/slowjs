// Copyright (c) 2026 The liustack/tifajs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.
// jsrun: -locale-args
// Array.prototype.toLocaleString and %TypedArray%.prototype.toLocaleString
// of a realm with ECMA-402 (JS_SetLocaleArgs) hand their locales and options
// to the toLocaleString of each element: always two arguments, undefined for
// those the call left out, and no call for an element that is null or
// undefined. Without it they pass none, as test262 checks
// (built-ins/Array/prototype/toLocaleString/invoke-element-tolocalestring.js
// and the toLocaleString-nointl.js tests of staging/sm). The file is what
// Node 24 prints.
const out = []
function run(name, f) {
  let r
  try { r = f() } catch (e) { r = 'throws ' + e.name }
  out.push(name + ': ' + JSON.stringify(r))
}
const show = (x) => typeof x === 'object' && x !== null ? JSON.stringify(x) : typeof x + ' ' + String(x)
const args = (a) => a.length + ':' + a.map(show).join('|')
const seen = (tag) => ({ toLocaleString(...a) { return tag + '(' + args(a) + ')' } })

run('array, no argument', () => [seen('a')].toLocaleString())
run('array, locales', () => [seen('a'), seen('b')].toLocaleString('de'))
run('array, locales and options', () => [seen('a')].toLocaleString('de', { style: 'percent' }))
run('array, three arguments', () => [seen('a')].toLocaleString('de', { a: 1 }, 'extra'))
run('array, undefined locales', () => [seen('a')].toLocaleString(undefined, { a: 1 }))
run('array, list of locales', () => [seen('a')].toLocaleString(['de', 'en'], 7))
run('array, the same objects', () => {
  const locales = ['de'], options = {}
  let got
  ;[{ toLocaleString(l, o) { got = l === locales && o === options; return '' } }].toLocaleString(locales, options)
  return got
})
run('array, null, undefined and a hole', () => [null, undefined, , seen('b'), null].toLocaleString('x', 'y'))
run('array, empty', () => [].toLocaleString(''))
run('array, nested', () => [[seen('a'), seen('b')], seen('c')].toLocaleString('de', 1))
run('array-like', () => Array.prototype.toLocaleString.call({ length: 2, 0: seen('a'), 1: seen('b') }, 'de', 2))
run('array, element throws', () => { try { [seen('a'), { toLocaleString(l) { throw new RangeError('bad ' + l) } }].toLocaleString('zz') } catch (e) { return e.name + ' ' + e.message } })
run('array, result converted', () => [{ toLocaleString(l) { return { toString() { return 'S' + l } } } }].toLocaleString('de'))
run('array, toLocaleString not callable', () => [{ toLocaleString: 1 }].toLocaleString('de'))

const numberTLS = Number.prototype.toLocaleString
const bigintTLS = BigInt.prototype.toLocaleString
Number.prototype.toLocaleString = function () { 'use strict'; const a = [...arguments]; return typeof this + ' ' + this + '(' + args(a) + ')' }
BigInt.prototype.toLocaleString = function () { 'use strict'; const a = [...arguments]; return typeof this + ' ' + this + '(' + args(a) + ')' }
run('array of numbers', () => [1.5, 2].toLocaleString('de', { style: 'percent' }))
run('array of bigints', () => [1n].toLocaleString('de'))
run('array of numbers, no argument', () => [3].toLocaleString())
for (const T of [Int8Array, Uint8ClampedArray, Int32Array, Float32Array, Float64Array]) {
  run(T.name + ', no argument', () => new T([1, 2]).toLocaleString())
  run(T.name + ', locales and options', () => new T([1, 2]).toLocaleString('de', { style: 'percent' }))
}
for (const T of [BigInt64Array, BigUint64Array]) {
  run(T.name + ', locales and options', () => new T([1n, 2n]).toLocaleString('de', { a: 1 }))
}
run('Float64Array, empty', () => new Float64Array(0).toLocaleString(''))
run('Int8Array, the same objects', () => {
  const locales = ['de'], options = {}
  let got
  Number.prototype.toLocaleString = function (l, o) { got = l === locales && o === options; return '' }
  new Int8Array(1).toLocaleString(locales, options)
  return got
})
Number.prototype.toLocaleString = numberTLS
BigInt.prototype.toLocaleString = bigintTLS
out.join('\n')
