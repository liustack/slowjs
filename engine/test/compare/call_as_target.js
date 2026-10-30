// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// Annex B: in sloppy code a call expression may be the target of an
// assignment, a compound assignment, ++, -- and a for-in or for-of head. It
// compiles, and throws a ReferenceError when it runs, after the call was
// made. In strict code, with &&=, ||= and ??=, as a destructuring target and
// after a tagged template it is a syntax error (upstream QuickJS df8de73).
// Only sloppy code is compiled here, as V8 compiles these in strict code
// too and throws at run time, where the specification and QuickJS make them
// a syntax error. The file is what Node 24 prints.
const out = []
function compile(src, strict) {
  try { return new Function((strict ? '"use strict";' : '') + src) } catch (e) { return e.name }
}
const snippets = [
  'f() = 1', 'f() += 1', 'f() -= 1', 'f() **= 2', 'f() <<= 1', 'f() &&= 1', 'f() ||= 1', 'f() ??= 1',
  'f()++', 'f()--', '++f()', '--f()', 'f(1, 2) = 3', 'o.m() = 1', 'o.m()++', 'o[0]() = 1', 'f()() = 1', 'new f() = 1', 'new f = 1',
  '(f()) = 1', '((f())) = 1', '(f())++', 'f`x` = 1', 'f`x`++', 'o.m`x` = 1',
  'for (f() in o) ;', 'for (f() of [1]) ;', 'for (o.m() in o) ;', 'for (o.m() of [1]) ;',
  '[f()] = [1]', '({ a: f() } = { a: 1 })', '[...f()] = [1]', '({ ...f() } = {})', '[(f())] = [1]',
  'async () => {}; f() = 1', 'x = f() = 1', 'f() = g() = 1', 'import.meta = 1', 'super() = 1',
  'f?.() = 1', 'f?.x = 1', 'o?.m() = 1',
  '(a, f()) = 1', 'delete f()', 'typeof f() = 1', 'f() = 1 + 2',
]
for (const s of snippets) {
  const f = compile(s, false)
  out.push(JSON.stringify(s) + ' => ' + (typeof f === 'string' ? f : 'compiles'))
}
function runs(src, strict) {
  const log = []
  const f = (...a) => { log.push('f'); return {} }
  const g = () => { log.push('g'); return 1 }
  const o = { m() { log.push('m'); return {} } }
  let fn
  try { fn = new Function('f', 'g', 'o', 'log', (strict ? '"use strict";' : '') + src) } catch (e) { return e.name }
  try { fn(f, g, o, log) } catch (e) { log.push(e.name) }
  return log.join()
}
for (const s of ['f() = g()', 'f() += g()', 'f()++', '++f()', 'o.m() = g()', 'o.m().x = g()', 'f(g()) = 1', 'for (f() in { a: 1 }) log.push("body")', 'for (f() of [1]) log.push("body")',
  'for (f() of []) log.push("body")', 'try { f() = 1 } catch (e) { log.push("caught " + e.name) }', 'var r = function () { f() = 1 }; r()', 'if (false) f() = 1; log.push("skipped")',
  'f() = 1; log.push("after")', 'x = () => { f() = 1 }; log.push("defined")']) {
  out.push(JSON.stringify(s) + ' => ' + runs(s, false))
}
out.join('\n')
