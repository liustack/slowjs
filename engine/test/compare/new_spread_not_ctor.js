// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// new of what is not a constructor, with a spread and without: both throw a
// TypeError that says it is not a constructor when it is an object, a
// function or not. super() of a parent that is not a constructor, with a
// spread and without. A primitive throws a TypeError, whose message is the
// engine's own. The file is what Node 24 prints.
const out = []
function run(name, f) {
  let r
  try { r = f() } catch (e) { r = 'throws ' + e.name + (typeof e.message === 'string' && /constructor/.test(e.message) ? ' constructor' : '') }
  out.push(name + ': ' + String(r))
}
const args = [1, 2]
const values = {
  'an object': {},
  'an array': [],
  'an arrow function': () => 1,
  'a method': { m() {} }.m,
  'a built-in function': Math.max,
  'a bound arrow function': (() => 1).bind(null),
  'a proxy of an object': new Proxy({}, {}),
  'a proxy of an arrow function': new Proxy(() => 1, {}),
}
for (const k of Object.keys(values)) {
  const v = values[k]
  run('new ' + k, () => new v(1, 2))
  run('new ' + k + ' with a spread', () => new v(...args))
}
for (const v of [5, 'str', null, undefined, true, Symbol('s')]) {
  run('new ' + typeof v, () => { try { return new v(1) } catch (e) { return 'throws ' + e.name } })
  run('new ' + typeof v + ' with a spread', () => { try { return new v(...args) } catch (e) { return 'throws ' + e.name } })
}
class D extends Object { constructor(spread) { if (spread) super(...args); else super(1, 2) } }
Object.setPrototypeOf(D, Math.max)
run('super of a built-in function', () => new D(false))
run('super of a built-in function with a spread', () => new D(true))
class E extends Object { constructor() { super(...args) } }
run('super of a constructor with a spread', () => new E() instanceof E)
function F(a, b) { this.s = a + b }
run('new of a constructor with a spread', () => new F(...args).s)
out.join('\n')
