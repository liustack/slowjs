// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// Calls of more arguments than the compiler leaves on the operand stack
// (JS_CALL_ARGS_ON_STACK of engine/quickjs.c, 32): past it they go in an
// array and the call is made as one with a spread. Each kind of call at 31 to
// 33 arguments and far past them: a function, a method, an optional call,
// new, super, a direct eval, a tagged template, a spread among them, the order
// in which the arguments and the callee are read, a throw in an argument past
// the limit, and a setter on Array.prototype that the array of the arguments
// must not run. The file is what Node 24 prints.
'use strict'
const out = []
function run(name, f) {
  let r
  try { r = f() } catch (e) { r = 'throws ' + e.name + (e.name === 'TypeError' ? '' : ' ' + e.message) }
  out.push(name + ': ' + String(r))
}
function list(n, f) { const r = []; for (let i = 0; i < n; i++) r.push(f(i)); return r.join(', ') }
function nums(n) { return list(n, String) }
function g() { return arguments.length + ':' + arguments[arguments.length - 1] }
const o = { m() { return (this === o) + ':' + arguments.length + ':' + arguments[arguments.length - 1] } }
function C() { this.r = (new.target === C) + ':' + arguments.length + ':' + arguments[arguments.length - 1] }
function t(strs) { return strs.length + ':' + strs.raw[strs.length - 1] + ':' + Object.isFrozen(strs) + ':' + (arguments.length - 1) + ':' + arguments[arguments.length - 1] }
function B() { this.r = arguments.length + ':' + arguments[arguments.length - 1] }
const log = []
function at(i) { log.push(i); return i }
for (const n of [31, 32, 33, 40, 128, 300]) {
  run('call ' + n, Function('g', "'use strict'; return g(" + nums(n) + ')').bind(null, g))
  run('method ' + n, Function('o', "'use strict'; return o.m(" + nums(n) + ')').bind(null, o))
  run('optional ' + n, Function('o', "'use strict'; return [o?.m(" + nums(n) + '), o.none?.(' + nums(n) + ')]').bind(null, o))
  run('new ' + n, Function('C', "'use strict'; return new C(" + nums(n) + ').r').bind(null, C))
  run('eval ' + n, Function('x', "'use strict'; var v = 5; return eval('v + x', " + nums(n) + ')').bind(null, 7))
  run('tag ' + n, Function('t', "'use strict'; var x = 'x'; return t`a" + list(n, i => '${' + (i % 3 ? i : 'x') + '}' + i) + '`').bind(null, t))
  run('spread ' + n, Function('g', 'a', "'use strict'; return g(" + nums(n) + ", ...a, 'end')").bind(null, g, [41, 42]))
  run('super ' + n, Function('B', "'use strict'; class D extends B { constructor() { super(" + nums(n) + ') } } return new D().r').bind(null, B))
  log.length = 0
  run('order ' + n, Function('at', "'use strict'; var f = function () { return arguments.length }; return at(f)(" + list(n, i => 'at(' + i + ')') + ')').bind(null, at))
  out.push('order ' + n + ' log: ' + log.length + ' ' + log.slice(-3).join(' '))
  run('throw ' + n, Function('g', "'use strict'; return g(" + list(n, i => i === n - 2 ? '(() => { throw new RangeError(\'arg\') })()' : String(i)) + ')').bind(null, g))
  run('not a function ' + n, Function("'use strict'; var f = {}; return f(" + nums(n) + ')'))
}
run('concat 128', Function("'use strict'; var s = 'x'; return s.concat(" + list(128, i => "'" + (i % 10) + "'") + ')'))
const callSet = Function('g', "'use strict'; return g(" + nums(40) + ')').bind(null, g)
const tagSet = Function('t', "'use strict'; return t`" + list(40, i => '${' + i + '}') + '`').bind(null, t)
Object.defineProperty(Array.prototype, '35', { set() { throw new Error('setter') }, configurable: true })
try {
  run('call 40 under a setter', callSet)
  run('tag 40 under a setter', tagSet)
} finally {
  delete Array.prototype[35]
}
out.join('\n')
