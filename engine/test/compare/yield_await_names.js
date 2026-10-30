// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// Where yield and await are names and where they are keywords: in generators,
// async functions, arrow function parameters inside them, the names of
// function expressions, class static blocks, escapes, labels and shorthand
// properties. Each snippet is compiled as the body of a function, sloppy and
// strict, and prints whether it is a syntax error (upstream QuickJS cfc9464).
// The file is what Node 24 prints.
const snippets = [
  'var yield = 1', 'var await = 1', 'yield: 1', 'await: 1', '({ yield })', '({ await })', '({ yield: 1, await: 2 })',
  'function yield() {}', 'function await() {}', 'function* yield() {}', 'async function await() {}', 'async function* await() {}',
  '(function yield() {})', '(function await() {})', '(function* yield() {})', '(async function await() {})', '(async function* yield() {})',
  '(function* await() {})', '(async function yield() {})', '(function* () { function yield() {} })', '(function* () { function* yield() {} })',
  'function* g() { var yield }', 'function* g() { var await }', 'function* g(yield) {}', 'function* g(await) {}', 'function* g(a = yield) {}',
  'async function f() { var await }', 'async function f() { var yield }', 'async function f(await) {}', 'async function f(a = await 1) {}',
  'function* g() { (yield) => 1 }', 'function* g() { (a = yield) => 1 }', 'function* g() { yield => 1 }', 'function* g() { (a = yield 1) => a }',
  'function* g() { async (yield) => 1 }', 'function* g() { () => yield }', 'function* g() { () => { var yield } }',
  'function* g() { (a = () => yield) => a }',
  'async function f() { (await) => 1 }', 'async function f() { await => 1 }', 'async function f() { (a = await 1) => a }', 'async function f() { async (await) => 1 }',
  'async function f() { () => await }', 'async function f() { () => { var await } }', 'async function f() { (a = () => await) => a }',
  'async function f() { async function g() { (await) => 1 } }',
  'async (await) => 1', 'async await => 1', 'async (a = await) => 1', '(await) => 1', 'await => 1', '(yield) => 1', 'yield => 1', 'async (yield) => 1',
  'function* g() { function f() { var yield } }', 'async function f() { function g() { var await } }',
  'function* g() { function f(yield) {} }', 'async function f() { function g(await) {} }',
  'function* g() { ({ yield }) }', 'async function f() { ({ await }) }', 'function* g() { ({ yield: 1 }) }', 'async function f() { ({ await: 1 }) }',
  'function* g() { x.yield }', 'async function f() { x.await }', 'function* g() { yield: 1 }', 'async function f() { await: 1 }',
  'function* g() { var yi\\u0065ld }', 'async function f() { var aw\\u0061it }', 'var yi\\u0065ld', 'var aw\\u0061it', 'function* g() { yi\\u0065ld 1 }', 'async function f() { aw\\u0061it 1 }',
  'class C { static { await } }', 'class C { static { var await } }', 'class C { static { (await) => 1 } }', 'class C { static { function await() {} } }',
  'class C { static { () => await } }', 'class C { static { ({ await }) } }', 'class C { static { x.await } }', 'class C { static { await: 1 } }',
  'class C { static { function f() { var await } } }', 'class C { static { (function await() {}) } }', 'class C { static { class await {} } }',
  'class C { static { (function () { await }) } }', 'class C { static { async function f() { await 1 } } }',
  'class C { static { (a = await) => 1 } }',
  'function* g() { yield\n/1/ }', 'function* g() { yield /1/ }', 'async function f() { await /1/ }',
  'function* g(yield = 1) {}', 'var f = function* (yield) {}', 'var f = async function (await) {}',
  '({ *yield() {} })', '({ async await() {} })', '({ *g() { var yield } })', '({ async f() { var await } })',
  'class C { *yield() {} }', 'class C { async await() {} }', 'class C { *g() { var yield } }',
  '(function* () { yield })', '(async function () { await 1 })', '(function* () { yield\n })',
  'function* g() { var x = { yield } }', 'async function f() { var x = { await } }',
]
const out = []
for (const s of snippets) {
  const row = []
  for (const strict of [false, true]) {
    let r
    try { new Function((strict ? '"use strict";' : '') + s); r = 'ok' } catch (e) { r = e.name === 'SyntaxError' ? 'SyntaxError' : e.name }
    row.push(r)
  }
  out.push(JSON.stringify(s) + ' => ' + row.join(' / '))
}
out.join('\n')
