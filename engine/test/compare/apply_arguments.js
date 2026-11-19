// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.
// f.apply(x, arguments): a function that passes its own arguments on, in
// sloppy code (a mapped arguments object, made with Function so that it
// stays sloppy when the whole file is run as strict code) and in strict
// code, with fewer, as many and more arguments than parameters, parameters
// written before and after the call, the arguments read or written too, two
// such calls in one function, the arguments object kept, spread or passed in
// another way. Then every kind of callee and this, apply replaced, read
// through a getter, shadowed and called through call, Reflect.apply, a
// callee that writes the caller's parameter through a closure, recursion,
// many arguments, and errors thrown by the callee. The file is what Node 24
// prints.
const out = []
function show(v) {
  if (v === undefined) return 'u'
  if (v === null) return 'n'
  if (v === globalThis) return 'global'
  if (typeof v === 'function') return 'fn'
  if (typeof v === 'object') {
    if (Array.isArray(v)) return '[' + v.map(show).join(',') + ']'
    if (v instanceof Number || v instanceof String || v instanceof Boolean) return 'box:' + typeof v.valueOf() + ':' + String(v.valueOf())
    return 'obj' + (v.tag !== undefined ? ':' + v.tag : '')
  }
  if (typeof v === 'symbol') return 'sym'
  return typeof v + ':' + String(v)
}
function run(name, f) {
  let r
  try { r = f() } catch (e) { r = 'throws ' + (e && e.name) + (e && e.tag !== undefined ? ' ' + e.tag : '') }
  out.push(name + ': ' + show(r))
}
// the callee: this, the count and each argument
function seen() {
  'use strict'
  const r = [this === undefined ? 'this:u' : 'this:' + show(this), 'n:' + arguments.length]
  for (let i = 0; i < arguments.length; i++) r.push(show(arguments[i]))
  return r.join(' ')
}
function seenSloppy() {
  const r = ['this:' + show(this), 'n:' + arguments.length]
  for (let i = 0; i < arguments.length; i++) r.push(show(arguments[i]))
  return r.join(' ')
}
const S = Function
const counts = [[], [1], [1, 2], [1, 2, 3], [1, 2, 3, 4, 5]]
const t = { tag: 't' }
function each(name, f) {
  for (const a of counts)
    run(name + ' ' + a.length, () => f.apply(t, a))
}

// the plain forms
const sloppy0 = S('g', 'return function () { return g.apply(this, arguments) }')(seen)
const sloppy2 = S('g', 'return function (a, b) { return g.apply(this, arguments) }')(seen)
const strict0 = (function (g) { 'use strict'; return function () { return g.apply(this, arguments) } })(seen)
const strict2 = (function (g) { 'use strict'; return function (a, b) { return g.apply(this, arguments) } })(seen)
each('sloppy, no parameters', sloppy0)
each('sloppy, two parameters', sloppy2)
each('strict, no parameters', strict0)
each('strict, two parameters', strict2)
// the call's result dropped, and in a return position
const dropped = S('g', 'log', 'return function (a) { g.apply(this, arguments); return log.length }')(function () { return 0 }, [])
each('result dropped', dropped)
const sideLog = []
const dropped2 = S('g', 'return function (a) { g.apply(this, arguments); return "done" }')(function () { sideLog.push(arguments.length) })
each('result dropped, callee logs', dropped2)
out.push('callee logged ' + sideLog.join(','))
// parameters written before the call: a mapped object shows the new values
const writeBefore = S('g', 'return function (a, b) { a = "A"; b = "B"; return g.apply(this, arguments) }')(seen)
each('sloppy, parameters written before', writeBefore)
const writeBeforeStrict = (function (g) { 'use strict'; return function (a, b) { a = 'A'; b = 'B'; return g.apply(this, arguments) } })(seen)
each('strict, parameters written before', writeBeforeStrict)
const writeDefault = S('g', 'return function (a = "d", b) { a = "A"; return g.apply(this, arguments) }')(seen)
each('sloppy with a default, parameter written', writeDefault)
// a parameter written after the call, then the arguments read
const writeAfter = S('g', 'return function (a) { const r = g.apply(this, arguments); a = "late"; return r + " | " + arguments[0] + " " + arguments.length }')(seen)
each('sloppy, parameter written after', writeAfter)
// the arguments read before the call too
const readToo = S('g', 'return function (a) { const n = arguments.length, x = arguments[1]; return n + " " + x + " | " + g.apply(this, arguments) }')(seen)
each('sloppy, reads and forwards', readToo)
const readTooStrict = (function (g) { 'use strict'; return function (a) { const n = arguments.length, x = arguments[1]; return n + ' ' + x + ' | ' + g.apply(this, arguments) } })(seen)
each('strict, reads and forwards', readTooStrict)
// two forwards in one function (the memoize pattern)
const twice = S('g', 'h', 'return function () { const k = h.apply(this, arguments); return k + " | " + g.apply(this, arguments) }')(seen, function (a) { return 'key:' + a })
each('two forwards', twice)
// the object made: written, kept, spread, passed on another way
const writeArg = S('g', 'return function (a) { arguments[0] = "w"; return g.apply(this, arguments) + " a=" + a }')(seen)
each('arguments[0] written', writeArg)
const kept = S('g', 'return function (a) { const k = arguments; a = "late"; return g.apply(this, k) + " | " + k[0] }')(seen)
each('kept in a variable', kept)
const spread = S('g', 'return function (a) { return g(...arguments) }')(seen)
each('spread', spread)
const viaCall = S('g', 'return function () { return Function.prototype.apply.call(g, this, arguments) }')(seen)
each('apply through call', viaCall)
const viaReflect = S('g', 'return function () { return Reflect.apply(g, this, arguments) }')(seen)
each('Reflect.apply', viaReflect)
const extra = S('g', 'return function () { return g.apply(this, arguments, 99) }')(seen)
each('apply with a third argument', extra)
const arrow = S('g', 'return function (a) { const f = () => g.apply(this, arguments); a = "A"; return f() }')(seen)
each('from an arrow function', arrow)
const lengthSet = S('g', 'return function () { arguments.length = 1; return g.apply(this, arguments) }')(seen)
each('length set', lengthSet)
const callee = S('g', 'return function f() { const r = g.apply(this, arguments); return r + " callee " + (arguments.callee === f) }')(seen)
each('callee read after', callee)

// this, and the kinds of callee
const fwd = S('return function () { return this.g.apply(this.t, arguments) }')()
const fwdStrict = (function () { 'use strict'; return function () { return this.g.apply(this.t, arguments) } })()
for (const [name, tv] of [['undefined', undefined], ['null', null], ['a number', 5], ['a string', 's'], ['an object', t]]) {
  run('sloppy callee, this ' + name, () => fwd.call({ g: seenSloppy, t: tv }, 1, 2))
  run('strict callee, this ' + name, () => fwdStrict.call({ g: seen, t: tv }, 1, 2))
}
const calleeKinds = {
  'an arrow function': (...a) => 'arrow ' + a.length,
  'a bound function': seen.bind({ tag: 'bound' }, 'b0'),
  'a bound arrow function': ((...a) => a.join('/')).bind(null, 'x'),
  'a proxy': new Proxy(seen, { apply(target, th, args) { return 'trap ' + args.length + ' ' + target.apply(th, args) } }),
  'a revoked proxy': (() => { const r = Proxy.revocable(seen, {}); r.revoke(); return r.proxy })(),
  'a class': class K { constructor() { this.k = 1 } },
  'a derived class': class D extends Array {},
  'Math.max': Math.max,
  'Array.prototype.push on this': Array.prototype.push,
  'a generator function': function* (a, b) { yield a; yield b },
  'an async function': async function (a) { return a },
  'a method': { m(a, b) { return 'm ' + a + b } }.m,
  'a getter of a function': Object.getOwnPropertyDescriptor({ get x() { return 'getter ' + arguments.length } }, 'x').get,
}
for (const k of Object.keys(calleeKinds)) {
  const g = calleeKinds[k]
  for (const f of [fwd, fwdStrict]) {
    run((f === fwd ? 'sloppy' : 'strict') + ' forward to ' + k, () => {
      const r = f.call({ g, t: k === 'Array.prototype.push on this' ? [0] : t }, 1, 2, 3)
      if (r && typeof r.next === 'function') return 'gen ' + show(r.next().value) + ' ' + show(r.next().value)
      if (r instanceof Promise) return 'promise'
      if (Array.isArray(r)) return 'array ' + r.length
      if (r && typeof r === 'object') return 'object ' + Object.keys(r).join(',')
      return r
    })
  }
}
for (const [name, g] of [['an object', { tag: 'o' }], ['undefined', undefined], ['a number', 1], ['an object with apply', { apply: Function.prototype.apply }]]) {
  run('sloppy forward to ' + name, () => fwd.call({ g, t }, 1))
  run('strict forward to ' + name, () => fwdStrict.call({ g, t }, 1))
}
// new.target of the callee
function nt() { return new.target === undefined ? 'no new.target' : 'new.target' }
run('new.target', () => fwd.call({ g: nt, t }, 1))

// apply changed by the script
const origApply = Function.prototype.apply
run('apply replaced', () => {
  Function.prototype.apply = function (th, a) { return 'replaced ' + show(th) + ' ' + a.length + ' ' + Object.prototype.toString.call(a) }
  try { return sloppy2.call(t, 1, 2, 3) + ' | ' + strict0.call(t, 4) } finally { Function.prototype.apply = origApply }
})
run('apply as a getter', () => {
  const reads = []
  Object.defineProperty(Function.prototype, 'apply', { configurable: true, get() { reads.push(this === seen); return origApply } })
  try { return sloppy0.call(t, 1, 2) + ' | reads ' + reads.join(',') } finally { Object.defineProperty(Function.prototype, 'apply', { value: origApply, writable: true, configurable: true, enumerable: false }) }
})
run('apply deleted', () => {
  delete Function.prototype.apply
  try { return sloppy0.call(t, 1) } finally { Function.prototype.apply = origApply }
})
run('apply of the callee shadowed', () => {
  const g = function () { return 'own' }
  g.apply = function (th, a) { return 'own apply ' + a.length }
  return S('g', 'return function () { return g.apply(this, arguments) }')(g)(1, 2)
})
run('apply restored', () => sloppy0.call(t, 'r'))
run('call of another prototype chain', () => {
  const g = function () { return 'plain' }
  Object.setPrototypeOf(g, { apply(th, a) { return 'chain ' + a.length } })
  return S('g', 'return function () { return g.apply(this, arguments) }')(g)(1, 2, 3)
})

// the callee writes the caller's parameter through a closure, and reads
// its own arguments object
run('callee writes the caller parameter', () => {
  const f = S('g', 'return function (a, b) { const set = (v) => { a = v }; const r = g.apply({ set }, arguments); return r + " a=" + a + " arg0=" + arguments[0] }')(function (x, y) { this.set('changed'); return 'callee sees ' + x + ' ' + arguments[0] })
  return f(1, 2)
})
run('callee writes its own arguments', () => {
  const f = S('g', 'return function (a) { const r = g.apply(this, arguments); return r + " a=" + a }')(function (x) { arguments[0] = 'mine'; x = 'mine2'; return 'callee ' + arguments[0] })
  return f('orig')
})
// recursion through a forward
const rec = S('return function r(n) { return n <= 0 ? 0 : 1 + r.apply(this, [n - 1].concat(Array.prototype.slice.call(arguments, 1))) }')()
const recFwd = S('return function r(n, acc) { if (n <= 0) return acc; return step.apply(this, arguments); function step(n, acc) { return r(n - 1, acc + n) } }')()
run('recursion 1000', () => recFwd(1000, 0))
run('recursion with extra arguments', () => rec(50, 'x', 'y'))
// many arguments
const many = []
for (let i = 0; i < 10000; i++) many.push(i)
const sum = function () { let s = 0; for (let i = 0; i < arguments.length; i++) s += arguments[i]; return s + ':' + arguments.length }
run('10000 arguments', () => S('g', 'return function () { return g.apply(this, arguments) }')(sum).apply(null, many))
run('10000 arguments, strict', () => strict0.apply(null, many).length)
// the callee throws
const thrower = function (k) { const e = new TypeError('thrown'); e.tag = k; throw e }
run('callee throws', () => S('g', 'return function () { return g.apply(this, arguments) }')(thrower)('k2'))
run('callee throws, strict', () => (function (g) { 'use strict'; return function () { return g.apply(this, arguments) } })(thrower)('k3'))
run('callee throws, caught by the caller', () => S('g', 'return function () { try { return g.apply(this, arguments) } catch (e) { return "caught " + e.tag + " " + arguments.length } }')(thrower)('k4', 5))
out.join('\n')
