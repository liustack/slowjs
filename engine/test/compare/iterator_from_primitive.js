// Copyright (c) 2026 The liustack/beastjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.
// Iterator.from takes a string, and an object with no Symbol.iterator as the
// iterator itself (GetIteratorFlattenable). A string whose Symbol.iterator is
// gone has no iterator of its own: it is not an object, which is a TypeError.
// The file is what Node 24 prints.
const out = []
function run(name, f) {
  let r
  try { r = f() } catch (e) { r = 'throws ' + e.name }
  out.push(name + ': ' + (typeof r === 'string' ? r : JSON.stringify(r)))
}
const save = Object.getOwnPropertyDescriptor(String.prototype, Symbol.iterator)
run('string', () => Iterator.from('ab').toArray())
run('object without Symbol.iterator', () => { let i = 0; return Iterator.from({ next() { return { done: i > 1, value: i++ } } }).toArray() })
run('number', () => Iterator.from(1))
run('null', () => Iterator.from(null))
for (const [name, value] of [['deleted', undefined], ['null', null], ['undefined', undefined]]) {
  if (name === 'deleted') delete String.prototype[Symbol.iterator]
  else String.prototype[Symbol.iterator] = value
  run('Symbol.iterator ' + name + ', string', () => Iterator.from('a'))
  run('Symbol.iterator ' + name + ', empty string', () => Iterator.from(''))
  run('Symbol.iterator ' + name + ', String object', () => typeof Iterator.from(new String('a')))
}
String.prototype[Symbol.iterator] = function () { return 5 }
run('Symbol.iterator gives a number', () => Iterator.from('a'))
String.prototype[Symbol.iterator] = function () { return { next() { return { done: true } } } }
run('Symbol.iterator gives an object', () => Iterator.from('a').toArray())
Object.defineProperty(String.prototype, Symbol.iterator, save)
run('restored', () => Iterator.from('ab').toArray())
out.join('\n')
