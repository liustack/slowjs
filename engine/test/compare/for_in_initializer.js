// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// The initializer of a for-in head, for (var x = f in o) in sloppy code (Annex
// B), names an anonymous function, class or arrow function it gives to x, as
// every other initializer does (upstream QuickJS 13876a2). The loop sets x to
// each key, so the name is seen on an object with no key. The file is what
// Node 24 prints.
const out = []
function run(name, f) {
  let r
  try { r = f() } catch (e) { r = 'throws ' + e.name }
  out.push(name + ': ' + String(r))
}
run('function', () => { for (var a = function () { } in {}) ; return a.name })
run('arrow', () => { for (var b = () => { } in {}) ; return b.name })
run('class', () => { for (var c = class { } in {}) ; return c.name })
run('generator', () => { for (var d = function* () { } in {}) ; return d.name })
run('async arrow', () => { for (var e = async () => { } in {}) ; return e.name })
run('named function', () => { for (var f = function g() { } in {}) ; return f.name })
run('class with a static name', () => { for (var h = class { static name = 'N' } in {}) ; return h.name })
run('a paren', () => { for (var j = (function () { }) in {}) ; return j.name })
run('a comma', () => { for (var l = (0, function () { }) in {}) ; return l.name })
run('loop variable', () => { const r = []; for (var n = function () { } in { k: 1, j: 2 }) r.push(n); return r.join() })
run('not a function', () => { for (var i = 5 in {}) ; return i })
run('not a function with keys', () => { for (var i2 = 5 in { z: 1 }) ; return i2 })
run('function name with keys', () => { for (var f2 = function () { } in { z: 1 }) ; return f2 })
run('var in a function', () => (function () { for (var p = function () { } in {}) ; return typeof p })())
run('initializer throws', () => { for (var q = (() => { throw new RangeError('x') })() in {}) ; })
run('the initializer runs once', () => { let n = 0; for (var s = (n++, function () { }) in { a: 1, b: 2, c: 3 }) ; return n })
out.join('\n')
