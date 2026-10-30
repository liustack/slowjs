// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// The prototype of a bound function is the prototype of the function it binds,
// read when bind is called: Function.prototype for most, the parent class for a
// class that extends one, null or any object a program set, and what the
// getPrototypeOf trap of a proxy says (upstream QuickJS efc48a3). The file is
// what Node 24 prints.
const out = []
const bind = (f, ...a) => Function.prototype.bind.call(f, ...a)
function run(name, f) {
  let r
  try { r = f() } catch (e) { r = 'throws ' + e.name }
  out.push(name + ': ' + String(r))
}
run('plain function', () => Object.getPrototypeOf(function () { }.bind()) === Function.prototype)
run('arrow function', () => Object.getPrototypeOf((() => 1).bind()) === Function.prototype)
run('async function', () => { const f = async function () { }; return Object.getPrototypeOf(bind(f)) === Object.getPrototypeOf(f) })
run('generator function', () => { const f = function* () { }; return Object.getPrototypeOf(bind(f)) === Object.getPrototypeOf(f) && Object.getPrototypeOf(f) !== Function.prototype })
run('async generator', () => { const f = async function* () { }; return Object.getPrototypeOf(bind(f)) === Object.getPrototypeOf(f) })
run('builtin', () => Object.getPrototypeOf(Math.max.bind()) === Function.prototype)
run('class', () => { class A { } return Object.getPrototypeOf(A.bind()) === Function.prototype })
run('derived class', () => { class A { static s() { return 'S' } } class B extends A { } const b = B.bind(); return [Object.getPrototypeOf(b) === A, b.s()].join() })
run('derived class of a derived class', () => { class A { static s() { return 'S' } } class B extends A { } class C extends B { } return C.bind().s() })
run('null prototype', () => { const f = function () { }; Object.setPrototypeOf(f, null); return Object.getPrototypeOf(bind(f)) })
run('set prototype', () => { const p = { p: 1 }; const f = function () { }; Object.setPrototypeOf(f, p); const b = bind(f); return [Object.getPrototypeOf(b) === p, b.p].join() })
run('prototype set after bind', () => { const p = { p: 1 }; const f = function () { }; const b = bind(f); Object.setPrototypeOf(f, p); return [Object.getPrototypeOf(b) === p, b.p].join() })
run('bound of a bound', () => { const p = { p: 1 }; const f = function () { }; Object.setPrototypeOf(f, p); return Object.getPrototypeOf(bind(bind(f))) === p })
run('proxy trap', () => {
  const log = []
  const p = new Proxy(function () { }, { getPrototypeOf(t) { log.push('trap'); return Array.prototype } })
  const b = Function.prototype.bind.call(p)
  return [Object.getPrototypeOf(b) === Array.prototype, log.join()].join()
})
run('proxy trap throws', () => {
  const p = new Proxy(function () { }, { getPrototypeOf() { throw new RangeError('t') } })
  return Function.prototype.bind.call(p)
})
run('proxy trap returns a bad value', () => {
  const p = new Proxy(function () { }, { getPrototypeOf() { return 1 } })
  return Function.prototype.bind.call(p)
})
run('proxy of a class', () => { class A { static s() { return 'S' } } class B extends A { } const p = new Proxy(B, {}); return Function.prototype.bind.call(p).s() })
run('name and length', () => { const b = function foo(a, b, c) { }.bind(null, 1); return b.name + b.length })
run('instanceof', () => { class A { } const B = A.bind(); return new B() instanceof A })
run('call and construct', () => { function F(a) { this.a = a } const B = F.bind(null, 3); return new B().a })
run('not callable', () => Function.prototype.bind.call({}))
run('prototype of Function.prototype bind', () => Object.getPrototypeOf(Function.prototype.bind()) === Function.prototype)
run('hasOwn prototype', () => Object.hasOwn(function () { }.bind(), 'prototype'))
out.join('\n')
