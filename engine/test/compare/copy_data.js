// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// The spread of an object literal, the rest of a destructuring and
// Object.assign copy the own keys of the source as CopyDataProperties
// says: all the own keys are listed first, and each one is looked up again
// when its turn comes, so that a getter that ran before it may have deleted
// it or made it not enumerable, which leaves it out, made it enumerable,
// which copies it, or added another key, which is not copied. A property the
// prototype holds under the name of a deleted key is not read, and nor is
// a key the rest of a destructuring leaves out. Sources are ordinary
// objects, arrays, proxies and objects of every other kind that lists its
// keys on its own. The keys of a result are listed with Reflect.ownKeys,
// symbols included, since JSON leaves a symbol out.
const out = []
const keys = (o) => JSON.stringify(Reflect.ownKeys(o).map(String))
const show = (o) => JSON.stringify(o, (k, v) => (v === undefined ? '<undefined>' : v))
const showKeys = (o) => show(o) + ' ' + keys(o)
const sym = Symbol('s')
const sym2 = Symbol('s2')

// each case makes a new source
const cases = [
  ['getter deletes a later key', () => ({ get a() { delete this.b; return 1 }, b: 2, c: 3 })],
  ['getter deletes the next key and adds it again', () => ({ get a() { delete this.b; this.b = 'again'; return 1 }, b: 2, c: 3 })],
  ['getter makes a later key not enumerable', () => ({ get a() { Object.defineProperty(this, 'b', { enumerable: false }); return 1 }, b: 2, c: 3 })],
  ['getter makes a later key an accessor', () => ({ get a() { Object.defineProperty(this, 'b', { get() { return 'got' }, enumerable: true, configurable: true }); return 1 }, b: 2, c: 3 })],
  ['getter adds a key', () => ({ get a() { this.z = 'added'; return 1 }, b: 2 })],
  ['getter deletes an earlier key', () => ({ b: 2, get a() { delete this.b; return 1 }, c: 3 })],
  ['getter deletes a symbol key', () => ({ get a() { delete this[sym]; return 1 }, [sym]: 2, c: 3 })],
  ['getter deletes an index key', () => ({ get a() { delete this[7]; return 1 }, 7: 'seven', c: 3 })],
  ['the second getter deletes the third key', () => ({ get a() { return 1 }, get b() { delete this.c; return 2 }, c: 3, d: 4 })],
  ['the prototype has the deleted key', () => Object.assign(Object.create({ b: 'proto', c: 'proto' }), { get a() { delete this.b; return 1 }, b: 2, c: 3 })],
  ['the prototype has the deleted key, an accessor', () => Object.create({ b: 'proto' }, {
    a: { get() { delete this.b; return 1 }, enumerable: true, configurable: true },
    b: { value: 2, enumerable: true, configurable: true, writable: true },
  })],
  ['getter throws before the last key', () => ({ a: 1, get b() { throw new Error('boom') }, c: 3 })],
  ['array whose element 0 deletes element 1', () => {
    const a = [1, 2, 3]
    Object.defineProperty(a, 0, { get() { delete this[1]; return 'z' }, enumerable: true, configurable: true })
    return a
  }],
  ['array whose element 0 makes element 2 not enumerable', () => {
    const a = [1, 2, 3]
    Object.defineProperty(a, 0, { get() { Object.defineProperty(this, 2, { enumerable: false }); return 'z' }, enumerable: true, configurable: true })
    return a
  }],
  ['array whose named getter deletes an index', () => {
    const a = [1, 2, 3]
    Object.defineProperty(a, 'x', { get() { delete this.y; return 'x' }, enumerable: true, configurable: true })
    a.y = 'y'
    return a
  }],
  ['array with a hole and a named key', () => { const a = [1, , 3]; a.k = 'k'; return a }],
  ['String object', () => new String('ab')],
  ['arguments object', () => (function () { return arguments })(1, 2)],
  ['typed array', () => new Uint8Array([4, 5])],
  ['function with own enumerable keys', () => Object.assign(function f() {}, { p: 1, q: 2 })],
  ['class instance with an accessor of its own', () => {
    class C { constructor() { this.x = 1; Object.defineProperty(this, 'y', { get() { delete this.z; return 'y' }, enumerable: true, configurable: true }); this.z = 3 } }
    return new C()
  }],
  ['many keys, the first getter deletes the last', () => {
    const o = { get first() { delete this.k29; return 'first' } }
    for (let i = 0; i < 30; i++) o['k' + i] = i
    return o
  }],
  ['plain data', () => ({ a: 1, b: 2, [sym]: 3, 5: 'five' })],
  ['non enumerable data', () => Object.defineProperty({ a: 1 }, 'hidden', { value: 2, enumerable: false })],
  // a key that is not enumerable when the keys are listed and is when its
  // turn comes is copied
  ['getter makes a later key enumerable', () => Object.defineProperty({
    get a() { Object.defineProperty(this, 'b', { enumerable: true }); return 1 },
  }, 'b', { value: 2, enumerable: false, configurable: true })],
  ['getter makes a later symbol enumerable', () => Object.defineProperty({
    get a() { Object.defineProperty(this, sym2, { enumerable: true }); return 1 },
  }, sym2, { value: 2, enumerable: false, configurable: true })],
  ['getter makes a later index enumerable', () => Object.defineProperty({
    get 1() { Object.defineProperty(this, 2, { enumerable: true }); return 'one' },
  }, 2, { value: 'two', enumerable: false, configurable: true })],
  ['getter makes a later key enumerable, and another not', () => Object.defineProperty(Object.defineProperty({
    get a() { Object.defineProperty(this, 'b', { enumerable: true }); Object.defineProperty(this, 'c', { enumerable: false }); return 1 },
    c: 3,
  }, 'b', { value: 2, enumerable: false, configurable: true }), 'd', { value: 4, enumerable: false })],
  ['getter makes a later key enumerable and deletes it', () => Object.defineProperty({
    get a() { Object.defineProperty(this, 'b', { enumerable: true }); delete this.b; return 1 },
  }, 'b', { value: 2, enumerable: false, configurable: true })],
  ['array whose element 0 makes a named key enumerable', () => {
    const a = [1, 2, 3]
    Object.defineProperty(a, 'x', { value: 'x', enumerable: false, configurable: true })
    Object.defineProperty(a, 0, { get() { Object.defineProperty(this, 'x', { enumerable: true }); return 'z' }, enumerable: true, configurable: true })
    return a
  }],
  ['a symbol deleted and a symbol made enumerable', () => Object.defineProperty({
    get a() { delete this[sym]; Object.defineProperty(this, sym2, { enumerable: true }); return 1 },
    [sym]: 'gone',
  }, sym2, { value: 'came', enumerable: false, configurable: true })],
  ['error object', () => new Error('m')],
  ['function', () => function f(a, b) {}],
]

for (const [name, make] of cases) {
  let spread, assign, rest
  try { spread = showKeys({ ...make() }) } catch (e) { spread = 'threw ' + e.message }
  try { assign = showKeys(Object.assign({}, make())) } catch (e) { assign = 'threw ' + e.message }
  // the rest of a destructuring leaves out the names before it
  try { const { a, ...r } = make(); rest = showKeys(r) + ' a=' + show(a) } catch (e) { rest = 'threw ' + e.message }
  out.push(name + ': spread ' + spread + ' | assign ' + assign + ' | rest ' + rest)
}

// a key that the rest leaves out is not looked up again when its turn comes,
// even when a getter before it has made it another kind of property; the
// spread reads it
{
  const log = []
  const make = () => ({
    get b() { Object.defineProperty(this, 'a', { get() { log.push('a read'); return 'new' }, enumerable: true, configurable: true }); return 2 },
    a: 1,
    c: 3,
  })
  const { a, ...r } = make()
  out.push('excluded: a=' + a + ' rest ' + showKeys(r) + ' log [' + log.join(',') + ']')
  log.length = 0
  const copy = { ...make() }
  out.push('not excluded: ' + showKeys(copy) + ' log [' + log.join(',') + ']')
  log.length = 0
  const { c: c2, ...r2 } = make()
  out.push('excluded after: c=' + c2 + ' rest ' + showKeys(r2) + ' log [' + log.join(',') + ']')
}

// the keys of the result, in order, of a getter that reorders the source
{
  const o = { get a() { delete this.b; this.b = 'again'; return 1 }, b: 2, c: 3 }
  out.push('order: ' + keys({ ...o }))
}

// a proxy: the traps run in the order of the specification
{
  const log = []
  const target = { get a() { return 1 }, b: 2 }
  const p = new Proxy(target, {
    ownKeys(t) { log.push('ownKeys'); return Reflect.ownKeys(t) },
    getOwnPropertyDescriptor(t, k) { log.push('gopd ' + String(k)); if (k === 'a') delete t.b; return Reflect.getOwnPropertyDescriptor(t, k) },
    get(t, k, r) { log.push('get ' + String(k)); return Reflect.get(t, k, r) },
  })
  const copy = { ...p }
  out.push('proxy: ' + showKeys(copy) + ' ' + log.join(','))
  log.length = 0
  const { b, ...rest } = new Proxy({ a: 1, b: 2, c: 3 }, {
    ownKeys(t) { log.push('ownKeys'); return Reflect.ownKeys(t) },
    getOwnPropertyDescriptor(t, k) { log.push('gopd ' + String(k)); return Reflect.getOwnPropertyDescriptor(t, k) },
    get(t, k, r) { log.push('get ' + String(k)); return Reflect.get(t, k, r) },
  })
  out.push('proxy rest: ' + showKeys(rest) + ' b=' + b + ' ' + log.join(','))
}

// a getter that runs in a spread inside a loop: the object is new each time
{
  let n = 0
  for (let i = 0; i < 50; i++) n += Object.keys({ ...{ get a() { delete this.b; return 1 }, b: 2, c: 3 } }).length
  out.push('loop: ' + n)
}
out.join('\n')
