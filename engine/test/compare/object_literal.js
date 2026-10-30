// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// Object literals whose first fields have static keys are made at once from
// a shape kept for the literal (OP_object_lit), with the values of those
// fields computed first. This checks that nothing shows it: the order in
// which keys and values are computed, the keys and their attributes, a
// literal that has more fields than go on the stack, fields that end the run
// (a repeated key, __proto__, an index, a computed key, a spread, a method,
// an accessor), values that throw, yield or use the variables around them,
// objects of the same literal changed one after the other, and the names
// functions take from their key. The memory part checks that the shapes of
// literals go with their functions. The file is what Node 24 prints.
const out = []
const show = v => typeof v === 'function' ? 'fn:' + v.name : typeof v === 'symbol' ? String(v) : v !== null && typeof v === 'object' ? (Array.isArray(v) ? '[' + v.map(show).join(',') + ']' : 'obj') : String(v)
function d(o) {
  return Reflect.ownKeys(o).map(k => {
    const p = Object.getOwnPropertyDescriptor(o, k)
    return String(k) + ('value' in p ? '=' + show(p.value) : '=get/set') + ':' + (p.writable ? 'w' : '') + (p.enumerable ? 'e' : '') + (p.configurable ? 'c' : '')
  }).join(' ')
}
function t(name, f) {
  try { out.push(name + ' => ' + f()) } catch (e) { out.push(name + ' !! ' + e.name + ': ' + e.message) }
}
const proto = o => { const p = Object.getPrototypeOf(o); return p === Object.prototype ? 'Object' : p === null ? 'null' : p === Array.prototype ? 'Array' : 'other' }

t('plain', () => { const x = 1, y = 'y'; return d({ a: x, b: y, c: null, d: undefined, e: { f: 1 } }) })
t('shorthand', () => { const a = 1, b = [2], c = () => 3; return d({ a, b, c }) })
t('one field', () => d({ only: 1 }))
t('empty', () => d({}) + '|' + proto({}))
t('string keys', () => d({ 'a b': 1, '': 2, 'é': 3, 'constructor': 4, 'toString': 5, 'hasOwnProperty': 6 }))
t('order of values', () => {
  const log = []; const v = n => { log.push(n); return n }
  const o = { a: v(1), b: v(2), c: { d: v(3), e: v(4) }, f: v(5) }
  return log.join() + ' ' + d(o) + ' ' + d(o.c)
})
t('repeated key', () => d({ a: 1, b: 2, a: 3, c: 4 }))
t('repeated key later', () => { const log = []; const o = { a: log.push('a1'), b: log.push('b'), a: log.push('a2') }; return d(o) + ' ' + log.join() })
t('index keys', () => d({ b: 1, 2: 'two', a: 3, 1: 'one', '3': 'three', 1.5: 'f', 4294967295: 'big', '-1': 'neg', 0x10: 'hex' }))
t('index first', () => d({ 0: 'z', a: 1 }))
t('__proto__ value', () => { const p = { inh: 1 }; const o = { a: 1, __proto__: p, b: 2 }; return d(o) + ' inh=' + o.inh })
t('__proto__ string', () => { const o = { a: 1, '__proto__': null, b: 2 }; return d(o) + ' ' + proto(o) })
t('__proto__ shorthand', () => { const __proto__ = 5; const o = { a: 1, __proto__ }; return d(o) + ' ' + proto(o) })
t('__proto__ computed', () => { const o = { a: 1, ['__proto__']: 7, b: 2 }; return d(o) + ' ' + proto(o) })
t('__proto__ not object', () => { const o = { a: 1, __proto__: 3 }; return d(o) + ' ' + proto(o) })
t('__proto__ twice', () => { try { eval('({ a: 1, __proto__: null, __proto__: null })'); return 'no error' } catch (e) { return e.name } })
t('computed keys', () => {
  const log = []; const k = n => ({ toString() { log.push('key' + n); return 'k' + n } }); const v = n => { log.push('v' + n); return n }
  const o = { a: v(1), [k(2)]: v(2), b: v(3), [k(4)]: v(4) }
  return d(o) + ' ' + log.join()
})
t('computed first', () => { const o = { ['x']: 1, a: 2, b: 3 }; return d(o) })
t('spread', () => {
  const log = []; const src = { get s() { log.push('get s'); return 's' }, t: 2 }
  const o = { a: (log.push('a'), 1), ...src, b: (log.push('b'), 2), a2: 3 }
  return d(o) + ' ' + log.join()
})
t('spread overrides', () => d({ a: 1, b: 2, ...{ a: 3, c: 4 }, b: 5 }))
t('spread first', () => d({ ...{ x: 1 }, a: 2 }))
t('spread null', () => d({ a: 1, ...null, ...undefined, b: 2 }))
t('methods', () => {
  const o = { a: 1, m() { return this.a + (super.hasOwnProperty ? 1 : 0) }, b: 2, *g() { yield 1 }, async am() {}, async *ag() {}, c: 3 }
  return d(o) + ' m=' + o.m() + ' g=' + [...o.g()]
})
t('accessors', () => {
  let s = 0
  const o = { a: 1, get b() { return 2 }, set b(v) { s = v }, c: 3, get [('d')]() { return 4 } }
  o.b = 9
  return d(o) + ' b=' + o.b + ' s=' + s + ' d=' + o.d
})
t('get set async as keys', () => {
  const get = 1, set = 2, async = 3
  return d({ get: 'g', set: 's', async: 'a', static: 'st' }) + ' | ' + d({ get, set, async }) + ' | ' + d({ get }) + ' | ' + d({ a: 0, async })
})
t('get set async methods', () => { const o = { get() { return 1 }, set() { return 2 }, async() { return 3 } }; return d(o) + ' ' + o.get() + o.set() + o.async() })
t('async arrow value', () => { const o = { a: async () => 1, b: async x => x }; return d(o) })
t('names', () => {
  const o = { f: function () {}, g: () => 0, c: class {}, h: function named() {}, k: class K {}, n: (0, function () {}), ['comp']: () => 1 }
  return d(o) + ' ' + o.f.name + ',' + o.g.name + ',' + o.c.name + ',' + o.h.name + ',' + o.k.name + ',' + o.n.name + ',' + o.comp.name
})
t('throw in value', () => {
  const log = []
  try { const o = { a: log.push('a'), b: (() => { throw new RangeError('boom') })(), c: log.push('c') }; log.push(d(o)) } catch (e) { log.push(e.name + ' ' + e.message) }
  return log.join()
})
t('throw in nested value', () => {
  let made = 0
  try { ({ a: 1, b: { c: ++made, d: undefinedName, e: ++made }, f: ++made }) } catch (e) { return e.name + ' made=' + made }
  return 'no throw'
})
t('throw then same literal', () => {
  const f = bad => ({ a: 1, b: bad ? null.x : 2, c: 3 })
  let r = ''
  try { f(true) } catch (e) { r = e.name }
  return r + ' ' + d(f(false)) + ' ' + d(f(false))
})
t('outer references', () => {
  let n = 0
  const mk = i => ({ i, get: () => n + i, set: v => { n = v }, self: null })
  const a = mk(1), b = mk(2)
  a.set(10)
  return a.get() + ',' + b.get()
})
t('literal refers to itself', () => { let o; o = { a: 1, f: () => o.a, b: typeof o }; return d(o) + ' ' + o.f() })
t('this and arguments', () => {
  const obj = { m() { return { t: this === obj, a: arguments.length, l: (() => this.x)() } }, x: 'X' }
  return d(obj.m(1, 2))
})
t('same literal, objects apart', () => {
  const f = (x, y) => ({ a: x, b: y, c: x + y })
  const o1 = f(1, 2), o2 = f(3, 4), o3 = f(5, 6)
  o1.d = 'added'; delete o2.a; Object.defineProperty(o3, 'b', { writable: false }); o3.c = 'changed'
  const o4 = f(7, 8)
  Object.freeze(o4); const o5 = f(9, 10); Object.setPrototypeOf(o5, null); const o6 = f(11, 12)
  return [o1, o2, o3, o4, o5, o6].map(o => d(o) + '/' + proto(o) + '/' + (Object.isFrozen(o) ? 'frozen' : 'open')).join(' | ')
})
t('same keys, other ways', () => {
  const lit = { a: 1, b: 2, c: 3 }
  const inc = {}; inc.a = 1; inc.b = 2; inc.c = 3
  const json = JSON.parse('{"a":1,"b":2,"c":3}')
  const spread = { ...lit }
  inc.d = 4; lit.e = 5
  return [lit, inc, json, spread].map(d).join(' | ')
})
t('reads and writes', () => {
  const o = { a: 1, b: 2, c: 3 }
  o.a++; o.b += 'x'; o['c'] = o.c * 2; delete o.b; o.b = 'back'
  const r = []
  for (const k in o) r.push(k)
  return d(o) + ' ' + r.join() + ' ' + JSON.stringify(o) + ' ' + ('a' in o) + o.hasOwnProperty('c') + Object.entries(o).length
})
t('setter on Object.prototype', () => {
  let called = 0
  Object.defineProperty(Object.prototype, 'zz', { set() { called++ }, get() { return 'inherited' }, configurable: true })
  const o = { a: 1, zz: 2, b: 3 }
  delete Object.prototype.zz
  return d(o) + ' called=' + called
})
t('read-only on Object.prototype', () => {
  Object.defineProperty(Object.prototype, 'ro', { value: 0, writable: false, configurable: true })
  const o = { ro: 1, a: 2 }
  delete Object.prototype.ro
  return d(o)
})
t('32 keys', () => {
  const o = { k0: 0, k1: 1, k2: 2, k3: 3, k4: 4, k5: 5, k6: 6, k7: 7, k8: 8, k9: 9, k10: 10, k11: 11, k12: 12, k13: 13, k14: 14, k15: 15, k16: 16, k17: 17, k18: 18, k19: 19, k20: 20, k21: 21, k22: 22, k23: 23, k24: 24, k25: 25, k26: 26, k27: 27, k28: 28, k29: 29, k30: 30, k31: 31 }
  return Object.keys(o).length + ' ' + Object.values(o).join()
})
t('33 keys', () => {
  const o = { k0: 0, k1: 1, k2: 2, k3: 3, k4: 4, k5: 5, k6: 6, k7: 7, k8: 8, k9: 9, k10: 10, k11: 11, k12: 12, k13: 13, k14: 14, k15: 15, k16: 16, k17: 17, k18: 18, k19: 19, k20: 20, k21: 21, k22: 22, k23: 23, k24: 24, k25: 25, k26: 26, k27: 27, k28: 28, k29: 29, k30: 30, k31: 31, k32: 32 }
  return Object.keys(o).join() + ' ' + Object.values(o).join()
})
t('many keys with repeats past 32', () => {
  const src = '({' + Array.from({ length: 70 }, (_, i) => 'k' + (i % 45) + ': ' + i).join(', ') + '})'
  const o = eval(src)
  return Object.keys(o).length + ' ' + Object.keys(o).slice(30, 36).join() + ' ' + o.k0 + ',' + o.k44 + ',' + o.k31 + ',' + o.k32
})
t('nested', () => { let o = { v: 0 }; for (let i = 1; i < 50; i++) o = { v: i, next: o, w: -i }; let n = 0; while (o.next) { n += o.v + o.w; o = o.next } return n })
t('generator values', () => {
  function* g() { const o = { a: yield 1, b: 2, c: yield 3, ...(yield 4), d: yield 5 }; return o }
  const it = g(); const r = [it.next().value, it.next('A').value, it.next('C').value, it.next({ s: 'S' }).value]
  return r.join() + ' ' + d(it.next('D').value)
})
t('generator closed in a literal', () => {
  let fin = 0
  function* g() { try { return { a: 1, b: yield 2, c: 3 } } finally { fin++ } }
  const it = g(); it.next()
  return JSON.stringify(it.return(9)) + ' fin=' + fin
})
t('destructuring still', () => {
  let a, b, c
  ;({ a, b = 2, c: { c } = { c: 3 } } = { a: 1 })
  const f = ({ x, y = 5 }) => x + y
  return [a, b, c, f({ x: 1 })].join()
})
t('cover grammar error', () => { try { eval('({ a = 1 })'); return 'no error' } catch (e) { return e.name } })
t('undeclared shorthand', () => { try { return d({ a: 1, notDeclaredAnywhere }) } catch (e) { return e.name } })
t('eval and Function', () => d(eval('({ e: 1, v: 2 })')) + ' | ' + d(new Function('a', 'return { a, f: a + 1 }')(4)))
t('in a loop', () => { const r = []; for (let i = 0; i < 5; i++) { const o = { i, sq: i * i }; if (i % 2) o.odd = true; r.push(d(o)) } return r.join(' | ') })
t('class bodies', () => {
  class A { constructor() { this.o = { a: 1, b: 2 } } m() { return { a: this.o.a, b: super.constructor === Object } } static s() { return { s: 1 } } }
  const a = new A()
  return d(a.o) + ' ' + d(a.m()) + ' ' + d(A.s())
})
// a literal in the values of another takes its index first: a function of
// more literals than 16 bits number, the last two with one inside each
t('more literals than 16 bits', () => {
  const r = new Function('var r = [];\n' + 'r.push({ a: 1 });\n'.repeat(65533) +
    'r.push({ x: { y: 2 }, w: 4 }, { z: 3, v: { u: 5 } });\nreturn r')()
  return r.length + ' ' + d(r[65532]) + ' | ' + d(r[65533]) + ' | ' + d(r[65533].x) + ' | ' + d(r[65534]) + ' | ' + d(r[65534].v)
})

// memory: literals of functions that are made and dropped leave nothing
// behind once collected, the shapes of the literals included. Each run has
// keys of its own, of the same lengths as those of the other runs: a literal
// of a run before would share its shape, and which shapes the ring keeps
// would then change from run to run, and the bytes they take with it
if (typeof memorySize === 'function') {
  const run = r => {
    for (let i = 0; i < 200; i++) {
      const f = new Function('x', 'return { u' + (1000 + r * 200 + i) + ': x, v' + (i % 7) + ': x + 1, w: [x], [\'c\' + (x + 100)]: 1 }')
      f(r); f(r + 1)
    }
  }
  // the shapes kept once their objects are gone (a ring of 2048) fill first
  for (let k = 0; k < 10; k++) run(k)
  gc(); gc()
  const m1 = memorySize(), k1 = memoryBlocks()
  run(10); run(11); run(12); gc(); gc()
  // the same blocks in use, a few bytes apart at most, after two collections
  // (see strict_equal.js)
  out.push('memory after: ' + heapChange(m1, k1))
} else {
  out.push('memory after: 0')
}
out.join('\n')
