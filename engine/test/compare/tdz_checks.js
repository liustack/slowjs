// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// let, const and class read before and after their declaration, where the
// compiler drops the checks of the temporal dead zone it can show never fail
// (tdz_checks in quickjs.c) and where it must keep them: in a switch, in
// loops and their heads, in closures run before the declaration, in try,
// catch and finally, in generators and async functions, with eval and with,
// for this in a derived constructor and for parameters. Each case prints
// its result or the name of the error it threw. The file is what Node 24
// prints.
const out = []
function run(name, f) {
  let r
  try {
    r = String(f())
  } catch (e) {
    r = e instanceof Error ? e.constructor.name : 'thrown ' + String(e)
  }
  out.push(name + ': ' + r)
}

// straight code, and after an if, a loop, a ternary and a short circuit
run('after', () => { let a = 1; const b = a + 1; return a + b })
run('after if', (c = true) => { const a = 2; if (c) { a + 1 } else { a - 1 } return a * 3 })
run('after else', (c = false) => { let a = 2; if (c) a = 5; else a = 7; return a })
run('after loop', () => { let a = 0; for (let i = 0; i < 4; i++) a += i; return a })
run('in loop', () => { const n = 5; let s = 0; let i = 0; while (i < n) { s += i * n; i++ } return s })
run('after ternary', (c = 1) => { const a = c ? 'x' : 'y'; return a + (c ? a : '') })
run('after and', (o = { x: 1 }) => { const a = o && o.x; const b = a || 3; return a + b })
run('nested blocks', () => { let a = 1; { let b = a + 1; { let c = b + a; a = c } } return a })
run('assign after', () => { let a = 1; a = a + 1; a += 2; a++; return a })
run('declared without value', () => { let a; let b = a === undefined; a = 4; return [a, b] })

// read before the declaration in the same block
run('read before', () => { const r = a; let a = 1; return r })
run('write before', () => { a = 2; let a = 1; return a })
run('typeof before', () => { const t = typeof a; let a = 1; return t })
run('read in own initializer', () => { let a = a + 1; return a })
run('const in own initializer', () => { const a = (() => a)(); return a })
run('class before', () => { const c = new C(); class C {} return c })
run('class in extends', () => { class C extends (C, Object) {} return C })
run('read after throw in initializer', () => {
  let r = 'none'
  try { let a = (() => { throw 1 })(); r = a } catch (e) { r = 'caught ' + e }
  return r
})

// a switch: the cases share one block, and the dispatch jumps past the
// declarations of the cases before
function sw(v) {
  switch (v) {
    case 0:
      let a = 'a0'
      return a
    case 1:
      return a
    case 2:
      a = 'set'
      return a
    case 3: {
      let b = 'b3'
      return b
    }
    default:
      return typeof a
  }
}
run('switch 0', () => sw(0))
run('switch 1', () => sw(1))
run('switch 2', () => sw(2))
run('switch 3', () => sw(3))
run('switch default', () => sw(9))
function swFall(v) {
  const r = []
  switch (v) {
    case 0:
      let a = 'x'
      r.push(a)
    case 1:
      r.push(a)
  }
  return r.join()
}
run('switch fall through', () => swFall(0))
run('switch jump past', () => swFall(1))
function swDefaultFirst(v) {
  switch (v) {
    default:
      let a = 'd'
    case 1:
      return a
  }
}
run('switch default first', () => swDefaultFirst(0))
run('switch default skipped', () => swDefaultFirst(1))
function swLoop(n) {
  let s = ''
  for (let i = 0; i < n; i++) {
    switch (i % 3) {
      case 0: { const k = 'z' + i; s += k; break }
      case 1: let k = 'o' + i; s += k; break
      default: s += 'd'
    }
  }
  return s
}
run('switch in loop', () => swLoop(7))

// closures: run before the declaration, after it, and in loops
run('closure before', () => { const f = () => a; const r = f(); let a = 1; return r })
run('closure after', () => { const f = () => a; let a = 1; return f() })
run('hoisted function before', () => { const r = g(); let a = 2; function g() { return a } return r })
run('hoisted function after', () => { let a = 2; function g() { return a } return g() })
run('closure sets before', () => { const f = () => { a = 5 }; let r; try { f() } catch (e) { r = e.constructor.name } let a = 1; return r + ' ' + a })
run('closure sets after', () => { const f = () => { a = 5 }; let a = 1; f(); return a })
run('closures in a for loop', () => {
  const fs = []
  for (let i = 0; i < 3; i++) fs.push(() => i)
  return fs.map(f => f()).join()
})
run('closures in a while loop', () => {
  const fs = []
  let i = 0
  while (i < 3) { const j = i * 2; fs.push(() => j); i++ }
  return fs.map(f => f()).join()
})
run('closure reads the next binding', () => {
  const fs = []
  for (let i = 0; i < 2; i++) { fs.push(() => typeof k); let k = i }
  return fs.map(f => { try { return f() } catch (e) { return e.constructor.name } }).join()
})
run('loop body redeclares', () => {
  const r = []
  for (let n = 0; n < 3; n++) {
    if (n > 0) { try { r.push(v) } catch (e) { r.push(e.constructor.name) } }
    let v = n
    r.push(v)
  }
  return r.join()
})

// loop heads
run('for head', () => { let s = 0; for (let i = 0, j = i + 1; i < 3; i++, j++) s += i * j; return s })
run('for head reads itself', () => { for (let i = i; ;) return i })
run('for of head', () => { let s = ''; for (const x of ['a', 'b']) s += x; return s })
run('for of reads itself', () => { for (let x of [x]) return x })
run('for in head', () => { let s = ''; for (const k in { p: 1, q: 2 }) s += k; return s })
run('for in reads itself', () => { for (let k in { k }) return k })
run('for with continue', () => { let s = 0; for (let i = 0; i < 6; i++) { const d = i * 2; if (d % 4) continue; s += d } return s })
run('labeled break', () => { let s = 0; outer: for (let i = 0; i < 3; i++) { for (let j = 0; j < 3; j++) { const m = i * j; if (m > 1) break outer; s += m } } return s })
run('do while', () => { let i = 0; do { const k = i; i = k + 1 } while (i < 4); return i })

// try, catch and finally
run('try', () => { let a = 1; try { a = 2 } catch (e) { a = 3 } return a })
run('try throws', () => { let a = 1; try { a = 2; throw 0 } catch (e) { a += 3 } return a })
run('catch reads try block', () => { let r; try { let a = 1; throw a } catch (e) { r = e } return r })
run('declared in catch', () => { try { throw 4 } catch (e) { const a = e * 2; return a } })
run('finally', () => { let a = 1; try { a = 2; return a } finally { a = 3 } })
run('finally reads', () => { let r = 0; for (let i = 0; i < 3; i++) { const a = i; try { if (a === 1) continue; r += a } finally { r += a * 10 } } return r })
run('return in try of block', () => { const f = () => { let a = 'k'; try { return a } finally { a = 'f' } }; return f() })
run('throw before in try', () => { let r; try { r = a; } catch (e) { r = e.constructor.name } let a = 1; return r })
run('finally after throw in initializer', () => {
  let r = []
  try { try { let a = (() => { throw 'x' })(); r.push(a) } finally { r.push('f') } } catch (e) { r.push(e) }
  return r.join()
})

// generators and async functions
run('generator', () => { function* g() { let a = 1; yield a; a++; yield a; const b = a * 10; yield b } return [...g()].join() })
run('generator before', () => { function* g() { yield 0; yield a; let a = 1 } const it = g(); it.next(); try { it.next() } catch (e) { return e.constructor.name } })
run('generator loop', () => { function* g(n) { for (let i = 0; i < n; i++) { const d = i * 2; yield d } } return [...g(4)].join() })
async function af() { let a = 1; await null; a += 1; const b = await a; return a + b }
af().then(v => out.push('async: ' + v))
async function afBefore() { await null; return a; let a = 1 }
afBefore().catch(e => out.push('async before: ' + e.constructor.name))

// eval and with
run('eval reads after', () => { let a = 3; return eval('a') + a })
run('eval reads before', () => { const r = eval('typeof a'); let a = 3; return r })
run('eval sets', () => { let a = 3; eval('a = 4'); return a })
run('eval in block before', () => { { try { eval('a') } catch (e) { return e.constructor.name } let a = 1 } })
run('with after', () => { let a = 1; with ({ b: 2 }) { a = a + b } return a })
run('with shadows', () => { let a = 1; with ({ a: 5 }) { a = a + 1 } return a })
run('with before', () => { with ({}) { try { a } catch (e) { return e.constructor.name } } let a = 1 })

// more of a switch, finally, generators, async, classes and parameters
function swConst(v) {
  switch (v) {
    case 0:
      const c = 'c0'
      return c
    case 1:
      class K {}
      return typeof K
    case 2:
      return typeof c
    default:
      return new K()
  }
}
run('switch const', () => swConst(0))
run('switch class', () => swConst(1))
run('switch const skipped', () => swConst(2))
run('switch class skipped', () => swConst(9))
run('case reads a later declaration', () => { switch (1) { case a: return 'a'; case 1: let a = 1; return 'one' } })
run('labeled continue through finally', () => {
  const r = []
  outer: for (let i = 0; i < 3; i++) {
    try { const k = i; if (k === 1) continue outer; r.push(k) } finally { const f = 'f' + i; r.push(f) }
  }
  return r.join()
})
run('generator throw through finally', () => {
  const r = []
  function* g() { try { let a = 1; yield a; a++; yield a } finally { const b = 'fin'; r.push(b) } }
  const it = g(); it.next()
  try { it.throw(new Error('x')) } catch (e) { r.push(e.message) }
  return r.join()
})
run('generator return through finally', () => {
  const r = []
  function* g() { for (let i = 0; i < 3; i++) { try { const k = i; yield k } finally { const f = 'f'; r.push(f + i) } } }
  const it = g(); it.next(); it.next(); it.return(7)
  return r.join()
})
async function afCatch() { let r = 'none'; try { const v = await Promise.reject(new Error('no')); r = v } catch (e) { const m = e.message; r = m } return r }
afCatch().then(v => out.push('async catch: ' + v))
run('this before super', () => { class A {} class B extends A { constructor() { this.x = 1; super() } } try { new B() } catch (e) { return e.constructor.name } })
run('this after super', () => { class A {} class B extends A { constructor() { super(); this.x = 2 } } return new B().x })
run('super twice', () => { class A {} class B extends A { constructor() { super(); super() } } try { new B() } catch (e) { return e.constructor.name } })
run('field initializer throws', () => { class C { x = (() => { throw new Error('f') })() } try { new C() } catch (e) { return e.message } })
run('parameter reads a later one', () => { function f(a = b, b = 1) { return a } try { return f() } catch (e) { return e.constructor.name } })
run('parameter reads an earlier one', () => { function f(a = 1, b = a + 1) { return b } return f() })
run('destructuring in with', () => { let a = 1, b = 2; with ({ c: 3 }) { [a, b] = [b, c] } return a + ',' + b })
run('destructuring before', () => { try { [a] = [1] } catch (e) { return e.constructor.name } let a })

// many lets, so that the indices pass 8 and 256
run('many', () => {
  let a0 = 0, a1 = 1, a2 = 2, a3 = 3, a4 = 4, a5 = 5, a6 = 6, a7 = 7, a8 = 8, a9 = 9
  const s = a0 + a1 + a2 + a3 + a4 + a5 + a6 + a7 + a8 + a9
  return s + a9
})
run('many and a loop', () => {
  const vs = []
  for (let i = 0; i < 300; i++) vs.push('let v' + i + ' = ' + i + ';')
  const f = new Function(vs.join('') + 'let s = 0; for (let k = 0; k < 2; k++) s += v299 + v0 + v8; return s + v299')
  return f()
})
run('many before', () => {
  const vs = []
  for (let i = 0; i < 300; i++) vs.push('let v' + i + ' = ' + i + ';')
  const f = new Function('let r; try { r = v299 } catch (e) { r = e.constructor.name } ' + vs.join('') + 'return r')
  return f()
})

;(() => out.join('\n'))
