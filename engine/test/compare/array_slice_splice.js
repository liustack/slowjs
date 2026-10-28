// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// Array.prototype.slice and splice on dense arrays, arrays with holes,
// array-likes, proxies, frozen, sealed and non-extensible arrays, arrays with
// a read-only length, subclasses, a changed Symbol.species, and arrays that a
// getter of 'constructor' or of Symbol.species changes while the call runs.
// The dense Array fast paths must give what the generic steps of the
// specification give. The file is what Node 24 prints.
const out = []
let tag = ""
const log = s => out.push(tag + s)

function show(v) {
  if (v === undefined) return 'undefined'
  if (v === null) return 'null'
  if (typeof v === 'string') return JSON.stringify(v)
  if (typeof v === 'symbol') return 'sym'
  if (typeof v === 'bigint') return v + 'n'
  if (typeof v === 'function') return 'fn'
  if (Object.is(v, -0)) return '-0'
  if (typeof v === 'object') {
    if (Array.isArray(v)) return showArr(v)
    return showObj(v)
  }
  return String(v)
}
function showArr(a) {
  const n = a.length
  if (n > 40) {
    let sum = 0, holes = 0
    for (let i = 0; i < n; i++) { if (i in a) sum += typeof a[i] === 'number' ? a[i] : 1; else holes++ }
    return 'big[' + n + ' sum=' + sum + ' holes=' + holes + ']'
  }
  const items = []
  for (let i = 0; i < n; i++) items.push(i in a ? show(a[i]) : '<hole>')
  const extra = Object.keys(a).filter(k => !/^(0|[1-9][0-9]*)$/.test(k))
  return '[' + items.join(',') + ']' + (extra.length ? '+{' + extra.join(',') + '}' : '')
}
function showObj(o) {
  const keys = Object.keys(o)
  return '{' + keys.map(k => k + ':' + (typeof o[k] === 'object' && o[k] !== null ? 'obj' : show(o[k]))).join(',') + '}'
}
function run(name, f) {
  let r
  try { r = f() } catch (e) { r = 'throws ' + (e && e.name) }
  log(name + ': ' + (typeof r === 'string' ? r : show(r)))
}
const slice = (a, ...args) => Array.prototype.slice.apply(a, args)
const splice = (a, ...args) => Array.prototype.splice.apply(a, args)
const range = n => Array.from({ length: n }, (_, i) => i)
const mk = () => [10, 20, 30, 40, 50]

// The fast paths for dense Arrays are on only while Array.prototype has no index property: the first time
// one is added the engine stops using them for every array (and does not start again). So the whole suite runs
// once before and once after that.
function suite() {
  // ---- slice: indexes
  const idx = [undefined, 0, 1, 2, 4, 5, 6, -1, -2, -5, -6, -100, 100, 1.9, -1.9, NaN, Infinity, -Infinity, '2', '-2', 'x', null, true, false,
    { valueOf() { return 3 } }, [2], 2 ** 31, -(2 ** 31), 2 ** 32, 2 ** 53, -(2 ** 53), 4294967295, 4294967296]
  for (const s of idx) {
    for (const e of [undefined, 3, -1, 0, 100, NaN, Infinity, -Infinity, '4']) {
      run('slice(' + show(s) + ',' + show(e) + ')', () => slice(mk(), s, e))
    }
  }
  run('slice()', () => slice(mk()))
  run('slice(1)', () => slice(mk(), 1))
  run('slice empty', () => slice([]))
  run('slice empty 0,0', () => slice([], 0, 0))
  run('slice one', () => slice([7], 0, 1))
  run('slice one past', () => slice([7], 1, 5))

  // ---- slice: result is a new independent dense Array, elements shared
  {
    const o1 = { n: 1 }, o2 = { n: 2 }
    const a = [o1, o2, 'x', 3.5, null, undefined, 7n]
    const r = a.slice(1, 6)
    log('slice share: ' + (r[0] === o2) + ' ' + (r === a) + ' ' + Array.isArray(r) + ' ' + r.length + ' ' + show(r))
    r[0] = 'changed'; a[2] = 'src'
    log('slice independent: ' + show(a) + ' ' + show(r))
    r.push('p'); log('slice push: ' + r.length + ' ' + a.length)
    log('slice proto: ' + (Object.getPrototypeOf(r) === Array.prototype) + ' ' + Object.getOwnPropertyNames(r).join(','))
    log('slice desc: ' + JSON.stringify(Object.getOwnPropertyDescriptor(r, 0)) + ' ' + JSON.stringify(Object.getOwnPropertyDescriptor(r, 'length')))
  }
  // ---- slice: big arrays
  {
    const a = range(100000)
    run('slice big all', () => slice(a))
    run('slice big mid', () => slice(a, 1000, 99000))
    run('slice big neg', () => slice(a, -5))
    const b = slice(a, 10, 20)
    log('slice big small: ' + show(b))
  }
  // ---- slice: holes and inherited elements
  run('slice holes', () => slice([1, , 3, , 5], 0))
  run('slice holes mid', () => slice([1, , 3, , 5], 1, 4))
  run('slice Array(5)', () => slice(Array(5), 1, 4))
  run('slice holes at end', () => { const a = [1, 2, 3]; a.length = 6; return slice(a, 2, 6) })
  run('slice delete', () => { const a = [1, 2, 3, 4]; delete a[1]; return slice(a) })
  run('slice after proto change', () => {
    const a = [1, 2, 3]
    Object.setPrototypeOf(a, { 1: 'x', 5: 'y' })
    a.length = 6
    return slice(a)
  })
  // ---- slice: array-likes
  run('slice arraylike', () => slice({ length: 3, 0: 'a', 1: 'b', 2: 'c' }, 1))
  run('slice arraylike holes', () => slice({ length: 4, 0: 'a', 3: 'd' }))
  run('slice arraylike no length', () => slice({ 0: 'a' }))
  run('slice arraylike neg length', () => slice({ length: -3, 0: 'a' }))
  run('slice arraylike str length', () => slice({ length: '2', 0: 'a', 1: 'b', 2: 'c' }))
  run('slice arraylike nan length', () => slice({ length: NaN, 0: 'a' }))
  run('slice arraylike float length', () => slice({ length: 2.9, 0: 'a', 1: 'b', 2: 'c' }))
  run('slice arraylike huge', () => slice({ length: 2 ** 53 - 1 }, 2 ** 53 - 3).length)
  run('slice arraylike huge elems', () => { const o = { length: 2 ** 53 - 1 }; o[2 ** 53 - 2] = 'z'; return slice(o, 2 ** 53 - 3) })
  run('slice arraylike 2^32', () => slice({ length: 2 ** 32 }, 2 ** 32 - 2).length)
  run('slice arraylike 2^32 tail', () => slice({ length: 2 ** 32 + 2 }, 2 ** 32).length)
  run('slice arraylike 2^32 all', () => slice({ length: 2 ** 32 }, 0, 2 ** 32).length)
  run('slice arraylike 2^32-1 count', () => slice({ length: 2 ** 32 - 1 }, 2 ** 32 - 3).length)
  run('slice string', () => slice('hello', 1, 4))
  run('slice number', () => slice(5))
  run('slice true', () => slice(true))
  run('slice null', () => slice(null))
  run('slice undefined', () => slice(undefined))
  run('slice arguments', () => (function () { return slice(arguments, 1) })(1, 2, 3, 4))
  run('slice typed', () => slice(new Uint8Array([1, 2, 3, 4]), 1, 3))
  run('slice function', () => slice(function (a, b, c) {}, 0))
  run('slice length getter', () => {
    const log2 = []
    const o = { get length() { log2.push('length'); return 3 }, get 0() { log2.push('0'); return 'a' }, get 1() { log2.push('1'); return 'b' }, get 2() { log2.push('2'); return 'c' } }
    const r = slice(o, 1)
    return show(r) + ' ' + log2.join(',')
  })
  run('slice index valueOf order', () => {
    const log2 = []
    const r = slice([1, 2, 3, 4], { valueOf() { log2.push('s'); return 1 } }, { valueOf() { log2.push('e'); return 3 } })
    return show(r) + ' ' + log2.join(',')
  })
  run('slice index throws', () => slice([1, 2, 3], { valueOf() { throw new RangeError('x') } }))
  run('slice end throws', () => slice([1, 2, 3], 0, { valueOf() { throw new RangeError('x') } }))
  run('slice index symbol', () => slice([1, 2, 3], Symbol()))
  run('slice index bigint', () => slice([1, 2, 3], 1n))
  run('slice shrinks in valueOf', () => {
    const a = [1, 2, 3, 4, 5]
    return slice(a, 0, { valueOf() { a.length = 2; return 5 } })
  })
  run('slice grows in valueOf', () => {
    const a = [1, 2, 3]
    return slice(a, 0, { valueOf() { a.push(4, 5); return 5 } })
  })
  run('slice deletes in valueOf', () => {
    const a = [1, 2, 3, 4, 5]
    return slice(a, 0, { valueOf() { delete a[2]; return 5 } })
  })
  run('slice getter in valueOf', () => {
    const a = [1, 2, 3, 4, 5]
    return slice(a, 0, { valueOf() { Object.defineProperty(a, 1, { get() { return 'g' }, configurable: true }); return 5 } })
  })
  // ---- slice: frozen, sealed, non-extensible, read-only length
  run('slice frozen', () => { const a = Object.freeze([1, 2, 3]); const r = slice(a, 1); r.push(9); return show(r) + ' ' + Object.isFrozen(r) })
  run('slice sealed', () => slice(Object.seal([1, 2, 3]), 0, 2))
  run('slice noext', () => { const r = slice(Object.preventExtensions([1, 2, 3]), 1); r.push(0); return r })
  run('slice ro length', () => { const a = [1, 2, 3]; Object.defineProperty(a, 'length', { writable: false }); const r = slice(a); r.push(4); return r })
  run('slice frozen empty', () => slice(Object.freeze([]), 0))
  // ---- slice: accessor and non-default elements
  run('slice accessor', () => {
    const a = [1, 2, 3]
    const log2 = []
    Object.defineProperty(a, 1, { get() { log2.push('g1'); return 'G' }, enumerable: true, configurable: true })
    return show(slice(a)) + ' ' + log2.join(',')
  })
  run('slice nonwritable elem', () => { const a = [1, 2, 3]; Object.defineProperty(a, 1, { writable: false }); const r = slice(a); r[1] = 'w'; return r })
  run('slice extra props', () => { const a = [1, 2, 3]; a.x = 1; return slice(a) })
  // ---- slice: species and subclasses
  class Sub extends Array {}
  run('slice subclass', () => { const s = Sub.from([1, 2, 3, 4]); const r = slice(s, 1, 3); return (r instanceof Sub) + ' ' + show(r) + ' ' + r.constructor.name })
  run('slice subclass full', () => { const s = Sub.from([1, 2, 3, 4]); const r = s.slice(); return (r instanceof Sub) + ' ' + r.length })
  run('slice subclass empty', () => { const s = new Sub(); const r = s.slice(); return (r instanceof Sub) + ' ' + r.length })
  {
    const calls = []
    class Counting extends Array {
      constructor(...args) { super(...args); calls.push(args.length + ':' + args[0]) }
    }
    run('slice ctor args', () => { calls.length = 0; const c = Counting.from([1, 2, 3, 4, 5]); calls.length = 0; const r = c.slice(1, 4); return calls.join(',') + ' ' + show(r) })
    run('slice ctor args empty', () => { const c = Counting.from([1, 2, 3]); calls.length = 0; c.slice(2, 1); return calls.join(',') })
  }
  run('slice species undefined', () => {
    class S extends Array { static get [Symbol.species]() { return undefined } }
    const r = S.from([1, 2, 3]).slice(1)
    return (r instanceof S) + ' ' + Object.getPrototypeOf(r === null ? 0 : r) .constructor.name + ' ' + show(r)
  })
  run('slice species null', () => {
    class S extends Array { static get [Symbol.species]() { return null } }
    const r = S.from([1, 2, 3]).slice(1)
    return (r instanceof S) + ' ' + show(r)
  })
  run('slice species Array', () => {
    class S extends Array { static get [Symbol.species]() { return Array } }
    const r = S.from([1, 2, 3]).slice(1)
    return (r instanceof S) + ' ' + show(r)
  })
  run('slice species custom', () => {
    const made = []
    function C(n) { made.push(n); return { length: 0, mark: 'C' } }
    const a = [1, 2, 3, 4]
    a.constructor = { [Symbol.species]: C }
    const r = slice(a, 1, 3)
    return show(r) + ' ' + made.join(',')
  })
  run('slice species custom returns frozen', () => {
    function C() { return Object.freeze({ length: 0 }) }
    const a = [1, 2, 3, 4]
    a.constructor = { [Symbol.species]: C }
    return slice(a, 1, 3)
  })
  run('slice species custom returns array', () => {
    const target = []
    function C() { return target }
    const a = [1, 2, 3, 4]
    a.constructor = { [Symbol.species]: C }
    const r = slice(a, 1, 3)
    return (r === target) + ' ' + show(target)
  })
  run('slice species custom returns primitive', () => {
    function C() { return 5 }
    const a = [1, 2, 3, 4]
    a.constructor = { [Symbol.species]: C }
    return show(slice(a, 1, 3))
  })
  run('slice species not ctor', () => { const a = [1, 2, 3]; a.constructor = { [Symbol.species]: {} }; return slice(a) })
  run('slice species arrow', () => { const a = [1, 2, 3]; a.constructor = { [Symbol.species]: () => [] }; return slice(a) })
  run('slice species ctor undefined', () => { const a = [1, 2, 3]; a.constructor = undefined; return slice(a, 1) })
  run('slice species ctor null', () => { const a = [1, 2, 3]; a.constructor = null; return slice(a, 1) })
  run('slice species ctor number', () => { const a = [1, 2, 3]; a.constructor = 5; return slice(a, 1) })
  run('slice species ctor object', () => { const a = [1, 2, 3]; a.constructor = {}; return slice(a, 1) })
  run('slice species ctor function', () => {
    const a = [1, 2, 3]
    function F(n) { return { length: 0, tag: 'F' + n } }
    a.constructor = F
    return show(slice(a, 1))
  })
  run('slice species ctor Array', () => { const a = [1, 2, 3]; a.constructor = Array; return slice(a, 1) })
  run('slice species getter throws', () => {
    class S extends Array { static get [Symbol.species]() { throw new SyntaxError('s') } }
    return S.from([1, 2]).slice()
  })
  run('slice constructor getter throws', () => {
    const a = [1, 2]
    Object.defineProperty(a, 'constructor', { get() { throw new EvalError('c') } })
    return slice(a)
  })
  run('slice ctor getter shrinks', () => {
    const a = [1, 2, 3, 4, 5]
    Object.defineProperty(a, 'constructor', { get() { a.length = 2; return Array }, configurable: true })
    return slice(a, 1, 5)
  })
  run('slice ctor getter grows', () => {
    const a = [1, 2, 3]
    Object.defineProperty(a, 'constructor', { get() { a.push(4, 5, 6); return Array }, configurable: true })
    return slice(a, 1, 3)
  })
  run('slice ctor getter replaces element', () => {
    const a = [1, 2, 3]
    Object.defineProperty(a, 'constructor', { get() { a[1] = 'new'; return Array }, configurable: true })
    return slice(a, 0, 3)
  })
  run('slice ctor getter makes holes', () => {
    const a = [1, 2, 3, 4]
    Object.defineProperty(a, 'constructor', { get() { delete a[2]; return Array }, configurable: true })
    return slice(a)
  })
  run('slice ctor getter freezes', () => {
    const a = [1, 2, 3, 4]
    Object.defineProperty(a, 'constructor', { get() { Object.freeze(a); return Array }, configurable: true })
    return slice(a, 1)
  })
  run('slice ctor getter sets length 0', () => {
    const a = [1, 2, 3, 4]
    Object.defineProperty(a, 'constructor', { get() { a.length = 0; return Array }, configurable: true })
    return slice(a, 0, 4)
  })
  run('slice species getter shrinks', () => {
    const a = [1, 2, 3, 4, 5]
    const C = function () {}
    Object.defineProperty(C, Symbol.species, { get() { a.length = 1; return undefined } })
    a.constructor = C
    return slice(a)
  })
  run('slice species getter grows', () => {
    const a = [1, 2, 3]
    const C = function () {}
    Object.defineProperty(C, Symbol.species, { get() { a.push('n'); return null } })
    a.constructor = C
    return slice(a, 1)
  })
  run('slice constructor order', () => {
    const log2 = []
    const a = [1, 2, 3]
    Object.defineProperty(a, 'constructor', { get() { log2.push('ctor'); return { get [Symbol.species]() { log2.push('species'); return undefined } } } })
    const r = slice(a, { valueOf() { log2.push('start'); return 1 } }, { valueOf() { log2.push('end'); return 3 } })
    return show(r) + ' ' + log2.join(',')
  })
  // ---- slice: proxies
  function traced(target, label) {
    const l = []
    const p = new Proxy(target, {
      get(t, k, r) { l.push('get:' + String(k)); return Reflect.get(t, k, r) },
      has(t, k) { l.push('has:' + String(k)); return Reflect.has(t, k) },
      set(t, k, v, r) { l.push('set:' + String(k)); return Reflect.set(t, k, v, r) },
      deleteProperty(t, k) { l.push('delete:' + String(k)); return Reflect.deleteProperty(t, k) },
      defineProperty(t, k, d) { l.push('define:' + String(k)); return Reflect.defineProperty(t, k, d) },
      getOwnPropertyDescriptor(t, k) { l.push('gopd:' + String(k)); return Reflect.getOwnPropertyDescriptor(t, k) },
    })
    return [p, l]
  }
  run('slice proxy array', () => { const [p, l] = traced([1, 2, 3, 4]); const r = slice(p, 1, 3); return show(r) + ' ' + l.join(',') })
  run('slice proxy arraylike', () => { const [p, l] = traced({ length: 3, 0: 'a', 2: 'c' }); const r = slice(p); return show(r) + ' ' + l.join(',') })
  run('slice proxy revoked', () => { const { proxy, revoke } = Proxy.revocable([1, 2, 3], {}); revoke(); return slice(proxy) })
  // ---- splice: shapes of calls
  const spl = [
    [], [0], [1], [2], [5], [6], [-1], [-5], [-6], [-100], [100], [undefined], [NaN], [Infinity], [-Infinity], ['2'], [1.7], [-1.7], [null],
    [0, 0], [0, 1], [0, 2], [0, 5], [0, 6], [0, 100], [0, -1], [0, NaN], [0, undefined], [0, Infinity], [0, -Infinity], [0, '2'], [0, 1.9],
    [1, 0], [1, 1], [1, 2], [1, 3], [1, 4], [1, 5], [2, 2], [2, 3], [3, 1], [4, 1], [4, 0], [5, 0], [5, 1], [5, 3], [-1, 1], [-1, 0], [-2, 1], [-2, 5], [-5, 5],
    [0, 0, 'a'], [0, 0, 'a', 'b'], [0, 1, 'a'], [0, 1, 'a', 'b'], [0, 2, 'a'], [0, 5, 'a'], [0, 5, 'a', 'b', 'c', 'd', 'e', 'f'],
    [1, 0, 'a'], [1, 1, 'a'], [1, 2, 'a'], [1, 1, 'a', 'b', 'c'], [1, 3, 'a', 'b'], [2, 0, undefined], [2, 1, undefined, undefined],
    [5, 0, 'a'], [5, 0, 'a', 'b'], [5, 1, 'a'], [6, 0, 'a'], [-1, 0, 'a'], [-1, 1, 'a', 'b'], [-100, 2, 'a'], [100, 2, 'a'],
    [4, 1, 'a'], [3, 2, 'a'], [3, 1, 'a', 'b'], [0, Infinity, 'a'], [Infinity, Infinity, 'a'], [-Infinity, 1, 'a'], [NaN, NaN, 'a'],
    [{ valueOf() { return 1 } }, { valueOf() { return 2 } }, 'z'],
  ]
  for (const args of spl) {
    run('splice' + show(args), () => { const a = mk(); const r = splice(a, ...args); return show(r) + ' | ' + show(a) })
  }
  run('splice no args', () => { const a = mk(); return show(splice(a)) + ' ' + show(a) })
  run('splice empty', () => { const a = []; return show(splice(a, 0, 0, 1, 2)) + ' ' + show(a) })
  run('splice empty none', () => { const a = []; return show(splice(a)) + ' ' + show(a) })
  run('splice empty del', () => { const a = []; return show(splice(a, 0, 5)) + ' ' + show(a) })
  run('splice one', () => { const a = [7]; return show(splice(a, 0, 1)) + ' ' + show(a) })
  run('splice one insert', () => { const a = [7]; return show(splice(a, 1, 0, 8)) + ' ' + show(a) })
  run('splice returns new array', () => {
    const o = { n: 1 }
    const a = [o, 'b', 'c']
    const r = splice(a, 0, 2)
    r[0] = 'changed'
    return show(a) + ' ' + show(r) + ' ' + Array.isArray(r) + ' ' + (Object.getPrototypeOf(r) === Array.prototype)
  })
  run('splice identity', () => {
    const o1 = { n: 1 }, o2 = { n: 2 }
    const a = [o1, o2, 'x']
    const r = splice(a, 0, 1, o2)
    return (r[0] === o1) + ' ' + (a[0] === o2) + ' ' + (a[1] === o2) + ' ' + a.length
  })
  run('splice index types', () => {
    const log2 = []
    const a = [1, 2, 3, 4]
    const r = splice(a, { valueOf() { log2.push('s'); return 1 } }, { valueOf() { log2.push('d'); return 2 } }, { valueOf() { log2.push('item') } })
    return show(r) + ' ' + show(a.map(x => typeof x)) + ' ' + log2.join(',')
  })
  run('splice start throws', () => splice([1, 2], { valueOf() { throw new TypeError('t') } }, 1))
  run('splice count throws', () => { const a = [1, 2]; try { splice(a, 0, { valueOf() { throw new TypeError('t') } }) } catch (e) { return e.name + ' ' + show(a) } })
  run('splice symbol count', () => splice([1, 2], 0, Symbol()))
  run('splice bigint', () => splice([1, 2], 0, 1n))
  run('splice valueOf shrinks', () => { const a = [1, 2, 3, 4, 5]; const r = splice(a, 1, { valueOf() { a.length = 3; return 3 } }); return show(r) + ' ' + show(a) })
  run('splice valueOf grows', () => { const a = [1, 2, 3]; const r = splice(a, 1, { valueOf() { a.push(4, 5); return 1 } }, 'x'); return show(r) + ' ' + show(a) })
  run('splice valueOf start grows', () => { const a = [1, 2, 3]; const r = splice(a, { valueOf() { a.push(9); return 1 } }, 1); return show(r) + ' ' + show(a) })
  // ---- splice: growth and shrink of dense arrays
  run('splice grow big', () => { const a = [1, 2, 3]; const items = range(5000); const r = splice(a, 1, 1, ...items); return show(r) + ' ' + a.length + ' ' + a[0] + ' ' + a[1] + ' ' + a[5000] + ' ' + a[5001] + ' ' + a[5002] })
  run('splice grow at end', () => { const a = [1, 2, 3]; splice(a, 3, 0, ...range(1000)); return a.length + ' ' + a[2] + ' ' + a[3] + ' ' + a[1002] })
  run('splice grow at start', () => { const a = range(100); splice(a, 0, 0, ...range(1000)); return a.length + ' ' + a[0] + ' ' + a[999] + ' ' + a[1000] + ' ' + a[1099] })
  run('splice big shrink', () => { const a = range(100000); const r = splice(a, 10, 99980); return show(r) + ' ' + show(a) })
  run('splice big shrink tail', () => { const a = range(100000); const r = splice(a, 99000); return r.length + ' ' + a.length + ' ' + a[98999] })
  run('splice big middle', () => { const a = range(100000); const r = splice(a, 50000, 3, 'a', 'b'); return show(r) + ' ' + a.length + ' ' + a[49999] + ' ' + a[50000] + ' ' + a[50001] + ' ' + a[50002] })
  run('splice big all', () => { const a = range(100000); const r = splice(a, 0); return r.length + ' ' + a.length })
  run('splice repeated', () => {
    const a = range(2000)
    let n = 0
    for (let i = 0; i < 1500; i++) { n += splice(a, i % 7, 2, i, i + 1, i + 2).length; if (i % 3 === 0) splice(a, 1, 3) }
    let sum = 0
    for (let i = 0; i < a.length; i++) sum += a[i]
    return a.length + ' ' + n + ' ' + sum
  })
  run('splice queue', () => {
    const q = []
    let s = 0
    for (let i = 0; i < 300; i++) { splice(q, q.length, 0, i, i + 1); if (i % 2) s += splice(q, 0, 3).length }
    return q.length + ' ' + s + ' ' + show(q.slice(0, 10))
  })
  run('splice length property', () => { const a = [1, 2, 3]; splice(a, 0, 1); return a.length + ' ' + Object.keys(a).join(',') })
  run('splice after grow push', () => { const a = [1, 2, 3]; splice(a, 1, 0, 'a', 'b', 'c', 'd', 'e', 'f', 'g', 'h'); a.push('z'); return show(a) })
  run('splice then index write', () => { const a = [1, 2, 3, 4]; splice(a, 1, 2); a[5] = 'z'; return show(a) })
  run('splice objects refcount', () => {
    const objs = range(50).map(i => ({ i }))
    const a = objs.slice()
    const r = splice(a, 10, 30, ...objs.slice(0, 5))
    const sum = a.reduce((s, o) => s + o.i, 0)
    return r.length + ' ' + a.length + ' ' + sum
  })
  // ---- splice: holes
  run('splice holes', () => { const a = [1, , 3, , 5]; const r = splice(a, 1, 2); return show(r) + ' ' + show(a) })
  run('splice holes insert', () => { const a = [1, , 3]; const r = splice(a, 1, 0, 'x', 'y'); return show(r) + ' ' + show(a) })
  run('splice holes tail', () => { const a = [1, 2, 3, 4]; delete a[3]; const r = splice(a, 1, 1); return show(r) + ' ' + show(a) })
  run('splice Array(5)', () => { const a = Array(5); const r = splice(a, 1, 2, 'x'); return show(r) + ' ' + show(a) })
  run('splice length grown', () => { const a = [1, 2, 3]; a.length = 6; const r = splice(a, 1, 1); return show(r) + ' ' + show(a) })
  run('splice proto changed', () => {
    const a = [1, 2, 3]
    Object.setPrototypeOf(a, { 3: 'p', 4: 'q' })
    const r = splice(a, 0, 1, 'x', 'y')
    return show(r) + ' ' + show(a) + ' ' + Object.keys(a).join(',')
  })
  run('splice proto null', () => { const a = [1, 2, 3]; Object.setPrototypeOf(a, null); const r = Array.prototype.splice.call(a, 1, 1, 'x', 'y'); return show(r) + ' ' + show(a) })
  // ---- splice: array-likes
  run('splice arraylike', () => { const o = { length: 3, 0: 'a', 1: 'b', 2: 'c' }; const r = splice(o, 1, 1, 'x', 'y'); return show(r) + ' ' + show(o) })
  run('splice arraylike shrink', () => { const o = { length: 4, 0: 'a', 1: 'b', 2: 'c', 3: 'd' }; const r = splice(o, 0, 2); return show(r) + ' ' + show(o) })
  run('splice arraylike holes', () => { const o = { length: 4, 0: 'a', 3: 'd' }; const r = splice(o, 1, 1); return show(r) + ' ' + show(o) })
  run('splice arraylike no length', () => { const o = { 0: 'a' }; const r = splice(o, 0, 1, 'x'); return show(r) + ' ' + show(o) })
  run('splice arraylike no args', () => { const o = { length: 2, 0: 'a', 1: 'b' }; const r = splice(o); return show(r) + ' ' + show(o) })
  run('splice arraylike huge', () => { const o = { length: 2 ** 53 - 1 }; return splice(o, 0, 0, 'x') })
  run('splice arraylike huge del', () => { const o = { length: 2 ** 53 - 1 }; const r = splice(o, 2 ** 53 - 3, 2); return r.length + ' ' + o.length })
  run('splice arraylike huge grow ok', () => { const o = { length: 2 ** 53 - 3 }; const r = splice(o, 2 ** 53 - 3, 0, 'x', 'y'); return r.length + ' ' + o.length + ' ' + o[2 ** 53 - 3] })
  run('splice arraylike 2^32', () => { const o = { length: 2 ** 32 + 2 }; o[2 ** 32] = 'a'; o[2 ** 32 + 1] = 'b'; const r = splice(o, 2 ** 32, 1); return show(r) + ' ' + o.length + ' ' + o[2 ** 32] })
  run('splice arraylike 2^32 del all', () => splice({ length: 2 ** 32 }, 0).length)
  run('splice string', () => splice('abc', 0, 1))
  run('splice number', () => splice(5, 0, 0))
  run('splice null', () => splice(null))
  run('splice undefined', () => splice(undefined, 0, 1))
  run('splice typed', () => splice(new Uint8Array([1, 2, 3]), 0, 1))
  run('splice arguments', () => (function () { const r = splice(arguments, 1, 1, 'x', 'y'); return show(r) + ' ' + arguments.length + ' ' + arguments[1] + arguments[2] })(1, 2, 3))
  run('splice length getter order', () => {
    const log2 = []
    const o = {
      get length() { log2.push('length'); return 3 },
      set length(v) { log2.push('setlength:' + v) },
      get 0() { log2.push('get0'); return 'a' }, get 1() { log2.push('get1'); return 'b' }, get 2() { log2.push('get2'); return 'c' },
      set 0(v) { log2.push('set0:' + v) }, set 1(v) { log2.push('set1:' + v) }, set 2(v) { log2.push('set2:' + v) },
    }
    const r = splice(o, 1, 1, 'x')
    return show(r) + ' ' + log2.join(',')
  })
  // ---- splice: frozen, sealed, non-extensible, read-only length
  run('splice frozen noop', () => { const a = Object.freeze([1, 2, 3]); try { splice(a, 0, 0) } catch (e) { return e.name + ' ' + show(a) } return 'no throw' })
  run('splice frozen no args', () => { const a = Object.freeze([1, 2, 3]); try { splice(a) } catch (e) { return e.name + ' ' + show(a) } return 'no throw' })
  run('splice frozen delete', () => { const a = Object.freeze([1, 2, 3]); try { splice(a, 0, 1) } catch (e) { return e.name + ' ' + show(a) } return 'no throw' })
  run('splice frozen insert', () => { const a = Object.freeze([1, 2, 3]); try { splice(a, 1, 0, 'x') } catch (e) { return e.name + ' ' + show(a) } return 'no throw' })
  run('splice frozen empty', () => { const a = Object.freeze([]); try { splice(a, 0, 0) } catch (e) { return e.name + ' ' + show(a) } return 'no throw' })
  run('splice frozen empty insert', () => { const a = Object.freeze([]); try { splice(a, 0, 0, 1) } catch (e) { return e.name + ' ' + show(a) } return 'no throw' })
  run('splice sealed shrink', () => { const a = Object.seal([1, 2, 3]); try { splice(a, 0, 1) } catch (e) { return e.name + ' ' + show(a) } return 'no throw' })
  run('splice sealed same', () => { const a = Object.seal([1, 2, 3]); try { return show(splice(a, 1, 1, 'x')) + ' ' + show(a) } catch (e) { return e.name + ' ' + show(a) } })
  run('splice sealed grow', () => { const a = Object.seal([1, 2, 3]); try { splice(a, 1, 0, 'x') } catch (e) { return e.name + ' ' + show(a) } return 'no throw' })
  run('splice noext shrink', () => { const a = Object.preventExtensions([1, 2, 3, 4]); const r = splice(a, 1, 2); return show(r) + ' ' + show(a) })
  run('splice noext same', () => { const a = Object.preventExtensions([1, 2, 3, 4]); const r = splice(a, 1, 2, 'x', 'y'); return show(r) + ' ' + show(a) })
  run('splice noext grow', () => { const a = Object.preventExtensions([1, 2, 3]); try { splice(a, 1, 0, 'x') } catch (e) { return e.name + ' ' + show(a) } return 'no throw' })
  run('splice noext grow end', () => { const a = Object.preventExtensions([1, 2, 3]); try { splice(a, 3, 0, 'x') } catch (e) { return e.name + ' ' + show(a) } return 'no throw' })
  run('splice noext noop', () => { const a = Object.preventExtensions([1, 2, 3]); return show(splice(a, 0, 0)) + ' ' + show(a) })
  run('splice ro length noop', () => { const a = [1, 2, 3]; Object.defineProperty(a, 'length', { writable: false }); try { splice(a, 0, 0) } catch (e) { return e.name + ' ' + show(a) } return 'no throw' })
  run('splice ro length shrink', () => { const a = [1, 2, 3, 4]; Object.defineProperty(a, 'length', { writable: false }); try { splice(a, 0, 1) } catch (e) { return e.name + ' ' + a.length + ' ' + show(a) } return 'no throw' })
  run('splice ro length same', () => { const a = [1, 2, 3]; Object.defineProperty(a, 'length', { writable: false }); try { return show(splice(a, 1, 1, 'x')) + ' ' + show(a) } catch (e) { return e.name + ' ' + show(a) } })
  run('splice ro length grow', () => { const a = [1, 2, 3]; Object.defineProperty(a, 'length', { writable: false }); try { splice(a, 1, 0, 'x') } catch (e) { return e.name + ' ' + a.length + ' ' + show(a) } return 'no throw' })
  run('splice ro length no args', () => { const a = [1, 2, 3]; Object.defineProperty(a, 'length', { writable: false }); try { splice(a) } catch (e) { return e.name + ' ' + show(a) } return 'no throw' })
  run('splice nonwritable elem', () => { const a = [1, 2, 3, 4]; Object.defineProperty(a, 1, { writable: false }); try { splice(a, 0, 1) } catch (e) { return e.name + ' ' + show(a) } return 'no throw' })
  run('splice nonconfig elem', () => { const a = [1, 2, 3, 4]; Object.defineProperty(a, 3, { configurable: false }); try { splice(a, 0, 1) } catch (e) { return e.name + ' ' + show(a) } return 'no throw' })
  run('splice accessor elem', () => {
    const a = [1, 2, 3, 4]
    const log2 = []
    Object.defineProperty(a, 1, { get() { log2.push('g'); return 'G' }, set(v) { log2.push('s:' + v) }, enumerable: true, configurable: true })
    const r = splice(a, 0, 2, 'x')
    return show(r) + ' ' + log2.join(',') + ' ' + a.length
  })
  run('splice extra props', () => { const a = [1, 2, 3]; a.x = 1; const r = splice(a, 1, 1); return show(r) + ' ' + show(a) })
  // ---- splice: species and subclasses
  run('splice subclass', () => { const s = Sub.from([1, 2, 3, 4]); const r = splice(s, 1, 2, 'x'); return (r instanceof Sub) + ' ' + show(r) + ' ' + (s instanceof Sub) + ' ' + show(s) })
  run('splice subclass noop', () => { const s = Sub.from([1, 2, 3, 4]); const r = splice(s, 1, 0); return (r instanceof Sub) + ' ' + r.length + ' ' + s.length })
  {
    const calls = []
    class Counting extends Array {
      constructor(...args) { super(...args); calls.push(args.length + ':' + args[0]) }
    }
    run('splice ctor args', () => { const c = Counting.from([1, 2, 3, 4, 5]); calls.length = 0; const r = c.splice(1, 3, 'x'); return calls.join(',') + ' ' + show(r) + ' ' + show(c) })
    run('splice ctor args none', () => { const c = Counting.from([1, 2, 3]); calls.length = 0; c.splice(); return calls.join(',') })
    run('splice ctor args one', () => { const c = Counting.from([1, 2, 3]); calls.length = 0; c.splice(1); return calls.join(',') })
  }
  run('splice species undefined', () => {
    class S extends Array { static get [Symbol.species]() { return undefined } }
    const s = S.from([1, 2, 3, 4])
    const r = s.splice(1, 2)
    return (r instanceof S) + ' ' + show(r) + ' ' + show(s)
  })
  run('splice species null', () => {
    class S extends Array { static get [Symbol.species]() { return null } }
    const s = S.from([1, 2, 3, 4])
    const r = s.splice(1, 2, 'a')
    return (r instanceof S) + ' ' + show(r) + ' ' + show(s)
  })
  run('splice species Array', () => {
    class S extends Array { static get [Symbol.species]() { return Array } }
    const s = S.from([1, 2, 3, 4])
    const r = s.splice(1, 2)
    return (r instanceof S) + ' ' + show(r) + ' ' + show(s)
  })
  run('splice species custom', () => {
    const made = []
    function C(n) { made.push(n); return { length: 0, mark: 'C' } }
    const a = [1, 2, 3, 4]
    a.constructor = { [Symbol.species]: C }
    const r = splice(a, 1, 2, 'x')
    return show(r) + ' ' + made.join(',') + ' ' + show(a)
  })
  run('splice species custom returns frozen', () => {
    function C() { return Object.freeze({ length: 0 }) }
    const a = [1, 2, 3, 4]
    a.constructor = { [Symbol.species]: C }
    try { splice(a, 1, 2) } catch (e) { return e.name + ' ' + show(a) }
    return 'no throw'
  })
  run('splice species custom returns self', () => {
    const a = [1, 2, 3, 4]
    function C() { return a }
    a.constructor = { [Symbol.species]: C }
    const r = splice(a, 1, 2)
    return (r === a) + ' ' + show(a)
  })
  run('splice species not ctor', () => { const a = [1, 2, 3]; a.constructor = { [Symbol.species]: {} }; try { splice(a, 0, 1) } catch (e) { return e.name + ' ' + show(a) } })
  run('splice species ctor undefined', () => { const a = [1, 2, 3]; a.constructor = undefined; return show(splice(a, 1)) + ' ' + show(a) })
  run('splice species ctor function', () => {
    const a = [1, 2, 3, 4]
    function F(n) { return { length: 0, tag: 'F' + n } }
    a.constructor = F
    const r = splice(a, 1, 2, 'x')
    return show(r) + ' ' + show(a)
  })
  run('splice species getter throws', () => {
    class S extends Array { static get [Symbol.species]() { throw new SyntaxError('s') } }
    const s = S.from([1, 2, 3])
    try { s.splice(0, 1) } catch (e) { return e.name + ' ' + show(s) }
  })
  run('splice ctor getter shrinks', () => {
    const a = [1, 2, 3, 4, 5]
    Object.defineProperty(a, 'constructor', { get() { a.length = 2; return Array }, configurable: true })
    const r = splice(a, 1, 3)
    return show(r) + ' ' + show(a)
  })
  run('splice ctor getter shrinks to 0', () => {
    const a = [1, 2, 3, 4, 5]
    Object.defineProperty(a, 'constructor', { get() { a.length = 0; return Array }, configurable: true })
    const r = splice(a, 1, 3, 'x')
    return show(r) + ' ' + show(a)
  })
  run('splice ctor getter grows', () => {
    const a = [1, 2, 3]
    Object.defineProperty(a, 'constructor', { get() { a.push(4, 5, 6); return Array }, configurable: true })
    const r = splice(a, 1, 1, 'x', 'y', 'z')
    return show(r) + ' ' + show(a)
  })
  run('splice ctor getter grows shrink call', () => {
    const a = [1, 2, 3]
    Object.defineProperty(a, 'constructor', { get() { a.push(4, 5, 6); return Array }, configurable: true })
    const r = splice(a, 0, 2)
    return show(r) + ' ' + show(a)
  })
  run('splice ctor getter grows insert', () => {
    const a = [1, 2, 3]
    Object.defineProperty(a, 'constructor', { get() { a.push(99); return Array }, configurable: true })
    const r = splice(a, 1, 0, 'x')
    return show(r) + ' ' + show(a)
  })
  run('splice ctor getter makes holes', () => {
    const a = [1, 2, 3, 4]
    Object.defineProperty(a, 'constructor', { get() { delete a[2]; return Array }, configurable: true })
    const r = splice(a, 1, 2)
    return show(r) + ' ' + show(a)
  })
  run('splice ctor getter freezes', () => {
    const a = [1, 2, 3, 4]
    Object.defineProperty(a, 'constructor', { get() { Object.freeze(a); return Array }, configurable: true })
    try { splice(a, 1, 2) } catch (e) { return e.name + ' ' + show(a) }
  })
  run('splice ctor getter seals', () => {
    const a = [1, 2, 3, 4]
    Object.defineProperty(a, 'constructor', { get() { Object.seal(a); return Array }, configurable: true })
    try { splice(a, 1, 1) } catch (e) { return e.name + ' ' + show(a) }
  })
  run('splice ctor getter noext', () => {
    const a = [1, 2, 3, 4]
    Object.defineProperty(a, 'constructor', { get() { Object.preventExtensions(a); return Array }, configurable: true })
    try { return show(splice(a, 1, 0, 'x')) + ' ' + show(a) } catch (e) { return e.name + ' ' + show(a) }
  })
  run('splice ctor getter ro length', () => {
    const a = [1, 2, 3, 4]
    Object.defineProperty(a, 'constructor', { get() { Object.defineProperty(a, 'length', { writable: false }); return Array }, configurable: true })
    try { splice(a, 1, 1) } catch (e) { return e.name + ' ' + show(a) + ' ' + a.length }
  })
  run('splice ctor getter changes proto', () => {
    const a = [1, 2, 3, 4]
    Object.defineProperty(a, 'constructor', { get() { Object.setPrototypeOf(a, { 4: 'p' }); return Array }, configurable: true })
    const r = splice(a, 1, 1)
    return show(r) + ' ' + show(a)
  })
  run('splice ctor getter replaces element', () => {
    const a = [1, 2, 3, 4]
    Object.defineProperty(a, 'constructor', { get() { a[1] = 'new'; return Array }, configurable: true })
    const r = splice(a, 1, 2)
    return show(r) + ' ' + show(a)
  })
  run('splice species getter shrinks', () => {
    const a = [1, 2, 3, 4, 5]
    const C = function () {}
    Object.defineProperty(C, Symbol.species, { get() { a.length = 1; return undefined } })
    a.constructor = C
    const r = splice(a, 0, 3)
    return show(r) + ' ' + show(a)
  })
  run('splice species getter grows', () => {
    const a = [1, 2, 3]
    const C = function () {}
    Object.defineProperty(C, Symbol.species, { get() { a.push('n', 'm'); return null } })
    a.constructor = C
    const r = splice(a, 1, 1, 'x', 'y')
    return show(r) + ' ' + show(a)
  })
  run('splice constructor order', () => {
    const log2 = []
    const a = [1, 2, 3]
    Object.defineProperty(a, 'constructor', { get() { log2.push('ctor'); return { get [Symbol.species]() { log2.push('species'); return undefined } } } })
    const r = splice(a, { valueOf() { log2.push('start'); return 1 } }, { valueOf() { log2.push('del'); return 1 } })
    return show(r) + ' ' + log2.join(',')
  })
  run('splice length too long', () => splice({ length: 2 ** 53 - 1 }, 0, 0, 1))
  // ---- splice: proxies
  run('splice proxy array', () => { const [p, l] = traced([1, 2, 3, 4]); const r = splice(p, 1, 1, 'x', 'y'); return show(r) + ' ' + l.join(',') })
  run('splice proxy shrink', () => { const [p, l] = traced([1, 2, 3, 4]); const r = splice(p, 0, 2); return show(r) + ' ' + l.join(',') })
  run('splice proxy none', () => { const [p, l] = traced([1, 2, 3, 4]); const r = splice(p); return show(r) + ' ' + l.join(',') })
  run('splice proxy arraylike', () => { const [p, l] = traced({ length: 3, 0: 'a', 2: 'c' }); const r = splice(p, 1, 1); return show(r) + ' ' + l.join(',') })
  run('splice proxy revoked', () => { const { proxy, revoke } = Proxy.revocable([1, 2, 3], {}); revoke(); return splice(proxy, 0, 1) })
  // ---- splice and slice together: stack and queue code of the kind libraries write
  run('mixed', () => {
    let a = range(30)
    const taken = []
    for (let i = 0; i < 40; i++) {
      taken.push(slice(a, i % 5, i % 5 + 3).join('-'))
      splice(a, i % 4, 1 + (i % 3), 'n' + i)
      a = a.concat(slice(a, 0, 2))
    }
    return a.length + ' ' + taken.slice(0, 6).join('|') + ' ' + show(a.slice(0, 8))
  })
}
tag = 'dense: '
suite()
tag = 'inherited: '
run('slice inherited', () => {
  Array.prototype[1] = 'P'
  try { return slice([0, , 2]) } finally { delete Array.prototype[1] }
})
run('slice inherited tail', () => {
  Array.prototype[4] = 'T'
  try { const a = [0, 1, 2]; a.length = 5; return slice(a) } finally { delete Array.prototype[4] }
})
run('splice inherited', () => {
  Array.prototype[1] = 'P'
  try { const a = [0, , 2, 3]; const r = splice(a, 0, 1); return show(r) + ' ' + show(a) } finally { delete Array.prototype[1] }
})
run('splice inherited dense', () => {
  Array.prototype[3] = 'P'
  try { const a = [0, 1, 2]; const r = splice(a, 1, 0, 'x'); return show(r) + ' ' + show(a) } finally { delete Array.prototype[3] }
})
run('splice inherited dense shrink', () => {
  Array.prototype[2] = 'P'
  try { const a = [0, 1, 2]; const r = splice(a, 0, 1); return show(r) + ' ' + show(a) } finally { delete Array.prototype[2] }
})
tag = 'generic: '
suite()
out.join('\n')
