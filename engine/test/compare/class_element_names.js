// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// Class elements named get, set and static, and what follows them on the same
// line or the next: a field, a method, a generator, an accessor, a static
// block, with a line terminator or a semicolon between (upstream QuickJS
// 55c5f0b). Each class body is compiled and prints the names of its own keys,
// its static keys and its instance keys, or the error. The file is what Node
// 24 prints.
const bodies = [
  'get', 'set', 'static', 'get;', 'set;', 'static;', 'get = 1', 'set = 2', 'static = 3', 'static static', 'static get', 'static set',
  'get\nx', 'get\nx() {}', 'get\nx;', 'set\nx', 'set\nx(v) {}', 'static\nx', 'static\nx() {}', 'static\nx = 1', 'static\nget', 'static\nstatic', 'static\n*g() {}',
  'get x() { return 1 }', 'set x(v) {}', 'get x() { return 1 } set x(v) {}', 'static get x() { return 1 }', 'static set x(v) {}', 'static get\nx() { return 1 }',
  'get *g() {}', 'set *g() {}', 'get\n*g() {}', 'set\n*g() {}', 'get;*g() {}', 'set;*g() {}', 'get *\ng() {}', 'static *g() {}', 'static\n*g() {}',
  'get() {}', 'set() {}', 'static() {}', 'get(a) { return a }', 'static get() {}', 'static set() {}', 'static static() {}', 'static static = 1',
  'get = 1; set = 2', 'get = 1\nset = 2', 'get\nset', 'get\nset\nstatic', 'static\nget\nset', 'static get\nset', 'static get;set', 'get\n;set',
  'get\n"x"() {}', 'get\n[k]() {}', 'get\n1() {}', 'get\n#p() {}', 'static\n#p() {}', 'static\n#p = 1', 'set\n#p(v) {}', 'get #p() { return 1 }',
  'get\nasync x() {}', 'async\nx() {}', 'async x() {}', 'async *g() {}', 'async\n*g() {}', 'async', 'async = 1', 'async\nasync',
  'static async x() {}', 'static async *g() {}', 'static\nasync x() {}', 'static async\nx() {}',
  'accessor', 'accessor x', 'accessor\nx',
  'static { }', 'static\n{ }', 'static;{ }', 'static {\n}\nget', 'get\nstatic { }', 'static static { }',
  'get x', 'set x', 'static get x', 'get x = 1', 'static x', 'static x;', 'static\nx;', 'get static', 'get\nstatic', 'set static', 'set\nstatic\n',
  'get\n}', 'static\n}',
  'constructor() {}', 'get constructor() {}', 'static constructor() {}', 'static\nconstructor() {}', 'get\nconstructor() {}', 'constructor\n() {}',
  'static prototype() {}', 'static\nprototype() {}', 'static name = 1', 'static\nname = 1', 'static length() {}',
  '"get" = 1', '"get"\nx', "'static'\n*g() {}", '0\nget', 'get 1', 'get "x"() { return 1 }', 'get [k]() { return 1 }',
]
const out = []
const names = o => Reflect.ownKeys(o).filter(k => !['length', 'name', 'prototype'].includes(k)).map(String).join(',')
for (const b of bodies) {
  let r
  try {
    const k = 'k'
    const C = new Function('k', 'return class { ' + b + '\n}')(k)
    r = 'static [' + names(C) + '] proto [' + names(C.prototype) + ']'
    try { const i = new C(); r += ' inst [' + Reflect.ownKeys(i).map(String).join(',') + ']' } catch (e) { r += ' new ' + e.name }
  } catch (e) { r = e.name }
  out.push(JSON.stringify(b) + ' => ' + r)
}
out.join('\n')
