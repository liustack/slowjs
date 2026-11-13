// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// Arrays that grow: 0 to 9 elements made by push, by index and by spread, with
// holes, a length set smaller and larger, concat of sparse arrays and of
// array-likes with Symbol.isConcatSpreadable, map on an array that changes
// length in its callback and on a subclass (Symbol.species), Array(n) for n of
// 0, 1 and large, and the arrays of own keys. The file is what the engine
// printed before an array that is appended to, the result of concat and map,
// Array(n) and the array of own keys took room for what they would hold.
const out = []
const show = v => typeof v === 'symbol' ? v.toString() : typeof v === 'bigint' ? v + 'n' :
  Array.isArray(v) ? '[' + Array.from({ length: v.length }, (_, i) => i in v ? show(v[i]) : '<hole>') + ']' :
  v !== null && typeof v === 'object' ? 'object' : Object.is(v, -0) ? '-0' : String(v)
function t(name, f) {
  let r
  try { r = show(f()) } catch (e) { r = (e && e.constructor && e.constructor.name) + ': ' + (e && e.message) }
  out.push(name + ': ' + r)
}

// by push, by index, by spread, from 0 to 9 elements
for (let n = 0; n <= 9; n++) {
  t('push ' + n, () => { const a = []; for (let i = 0; i < n; i++) a.push(i); return [a, a.length] })
  t('push many ' + n, () => { const a = []; a.push(...Array.from({ length: n }, (_, i) => i)); return [a, a.length] })
  t('index ' + n, () => { const a = []; for (let i = 0; i < n; i++) a[i] = i * 2; return [a, a.length] })
  t('index back ' + n, () => { const a = []; for (let i = 0; i < n; i++) a[a.length] = 'x' + i; return [a, a.length] })
  t('spread ' + n, () => { const s = Array.from({ length: n }, (_, i) => i); return [[...s], [0, ...s, 99], [...s, ...s]] })
  t('spread set ' + n, () => [...new Set(Array.from({ length: n }, (_, i) => i % 3))])
  t('spread generator ' + n, () => [...(function* () { for (let i = 0; i < n; i++) yield i })()])
  t('unshift ' + n, () => { const a = []; for (let i = 0; i < n; i++) a.unshift(i); return a })
  t('splice ' + n, () => { const a = []; for (let i = 0; i < n; i++) a.splice(a.length, 0, i); return a })
  t('literal then push ' + n, () => { const a = [n, n]; for (let i = 0; i < n; i++) a.push(i); return a })
  t('two arrays ' + n, () => { const a = [], b = []; for (let i = 0; i < n; i++) { a.push(i); b[i] = -i } return [a, b] })
  t('pop and push ' + n, () => { const a = []; for (let i = 0; i < n; i++) a.push(i); while (a.length > 1) a.pop(); a.push('e'); return a })
}
// holes, a length set smaller and larger
t('holes by index', () => { const a = []; a[3] = 'x'; a[1] = 'y'; return [a, a.length, Object.keys(a)] })
t('holes literal', () => { const a = [1, , 3, , ]; a.push(5); return [a, a.length, 1 in a, 3 in a, 4 in a] })
t('hole then fill', () => { const a = []; a[2] = 2; a[0] = 0; a[1] = 1; return [a, a.length] })
t('length larger', () => { const a = [1, 2]; a.length = 10; a.push('p'); return [a, a.length, 5 in a] })
t('length larger then fill', () => { const a = []; a.length = 4; for (let i = 0; i < 4; i++) a[i] = i; a.push(4); return [a, a.length] })
t('length smaller', () => { const a = [1, 2, 3, 4, 5]; a.length = 2; a.push(9); return [a, a.length, 3 in a] })
t('length zero then push', () => { const a = [1, 2, 3]; a.length = 0; for (let i = 0; i < 6; i++) a.push(i); return [a, a.length] })
t('length smaller than count', () => { const a = [1, 2, 3, 4, 5, 6, 7, 8, 9]; a.length = 1; a[1] = 'b'; a.push('c'); return a })
t('length very large', () => { const a = []; a.length = 2 ** 32 - 1; return [a.length, a.push === Array.prototype.push] })
t('length too large', () => { const a = []; a.length = 2 ** 32 })
t('push past 2^32-1', () => { const a = []; a.length = 2 ** 32 - 1; a.push(1) })
t('frozen then push', () => { const a = [1]; Object.freeze(a); a.push(2) })
t('non-writable length', () => { const a = [1, 2]; Object.defineProperty(a, 'length', { writable: false }); a.push(3) })
t('non-extensible then index', () => { const a = [1]; Object.preventExtensions(a); a[1] = 2; return [a, a.length] })
t('setter on the prototype', () => { const log = []; Object.defineProperty(Array.prototype, '3', { set(v) { log.push(v) }, configurable: true }); try { const a = [0, 1, 2]; a.push('p'); a[4] = 'q'; return [a, a.length, log] } finally { delete Array.prototype[3] } })
t('a getter reads the array while it grows', () => { const a = []; Object.defineProperty(a, 'x', { get() { return a.length } }); for (let i = 0; i < 6; i++) a.push(a.x); return a })
t('push in a loop with a callback', () => { const a = []; [5, 4, 3, 2, 1, 0].forEach(x => a.push(x * x)); return a })

// concat
t('concat plain', () => [1, 2].concat([3], 4, [[5]]))
t('concat empty', () => [].concat([], [], 1))
t('concat nothing', () => [].concat())
t('concat sparse', () => [1, , 3].concat([, 5], [, ,]))
t('concat sparse keys', () => Object.keys([1, , 3].concat([, 5])))
t('concat a hole at the end', () => { const a = [1, 2]; a.length = 5; const r = a.concat([9]); return [r, r.length, 3 in r] })
t('concat sparse huge', () => { const a = []; a[1e6] = 'z'; const r = [0].concat(a, [1]); return [r.length, r[1e6 + 1], r[1e6 + 2], Object.keys(r).length] })
t('concat array-like spreadable', () => [0].concat({ length: 2, 0: 'a', 1: 'b', [Symbol.isConcatSpreadable]: true }, 'c'))
t('concat array-like not spreadable', () => [0].concat({ length: 2, 0: 'a', 1: 'b' }).length)
t('concat array not spreadable', () => { const a = [1, 2]; a[Symbol.isConcatSpreadable] = false; const r = [0].concat(a); return [r.length, r[1] === a] })
t('concat spreadable with a length getter', () => [0].concat({ get length() { return 3 }, 0: 'a', 2: 'c', [Symbol.isConcatSpreadable]: true }))
t('concat spreadable string object', () => [0].concat(Object.assign(new String('ab'), { [Symbol.isConcatSpreadable]: true })))
t('concat arguments', () => (function () { return [0].concat(arguments) })(1, 2))
t('concat spreadable arguments', () => (function () { arguments[Symbol.isConcatSpreadable] = true; return [0].concat(arguments) })(1, 2))
t('concat typed array', () => [0].concat(new Uint8Array([1, 2])).length)
t('concat on a non-array', () => Array.prototype.concat.call('ab', 'c'))
t('concat on an object', () => Array.prototype.concat.call({ a: 1 }, 1).length)
t('concat getter throws', () => [0].concat({ get [Symbol.isConcatSpreadable]() { throw new RangeError('spreadable') } }))
t('concat length getter changes', () => { const a = [1, 2, 3]; return [0].concat({ length: 3, get 0() { a.length = 0; return 'x' }, 1: 'y', 2: 'z', [Symbol.isConcatSpreadable]: true }) })
t('concat many arguments', () => [].concat(...Array.from({ length: 40 }, (_, i) => [i, i])).length)
t('concat result then push', () => { const r = [1, 2].concat([3, 4, 5]); r.push(6, 7); return r })
t('concat species', () => { class A extends Array {}; const r = new A(1, 2).concat([3, 4]); return [r instanceof A, r.length, r] })
t('concat species constructor undefined', () => { const a = [1, 2]; a.constructor = undefined; const r = a.concat([3]); return [Array.isArray(r), r.length] })
t('concat species null', () => { const a = [1, 2]; a.constructor = { [Symbol.species]: null }; return a.concat([3]) })
t('concat species function', () => { const a = [1, 2]; a.constructor = { [Symbol.species]: function (n) { return { length: n, tag: 'obj' } } }; const r = a.concat([3]); return [r.tag, r.length, r[0], r[2]] })
t('concat species not a constructor', () => { const a = [1, 2]; a.constructor = { [Symbol.species]: 1 }; return a.concat([3]) })
t('concat species gives a frozen array', () => { const a = [1, 2]; a.constructor = { [Symbol.species]: function () { return Object.freeze([]) } }; return a.concat([3]) })

// map
t('map plain', () => [1, 2, 3].map(x => x * 2))
t('map empty', () => [].map(x => x))
t('map holes', () => [1, , 3].map(x => x * 2))
t('map holes keys', () => Object.keys([1, , 3, , ].map(x => x)))
t('map index and array', () => ['a', 'b'].map((x, i, a) => x + i + a.length))
t('map this', () => [1].map(function () { return this.v }, { v: 'self' }))
t('map push inside', () => { const a = [1, 2, 3]; return [a.map(x => { if (a.length < 6) a.push(x); return x * 10 }), a.length] })
t('map pop inside', () => { const a = [1, 2, 3, 4, 5]; return [a.map(x => { a.pop(); return x }), a.length] })
t('map shrink inside', () => { const a = [1, 2, 3, 4, 5]; return a.map(x => { a.length = 2; return x }) })
t('map clear inside', () => { const a = [1, 2, 3, 4, 5]; return a.map(x => { a.length = 0; return x }) })
t('map grow inside', () => { const a = [1, 2]; return a.map((x, i) => { a.length = 6; return x + i }) })
t('map delete inside', () => { const a = [1, 2, 3, 4]; return a.map((x, i) => { delete a[2]; return x }) })
t('map throws', () => { const seen = []; try { [1, 2, 3].map(x => { seen.push(x); if (x === 2) throw new Error('stop'); return x }) } catch (e) { return [seen, e.message] } })
t('map result then push', () => { const r = [1, 2, 3].map(x => x); r.push(4); r.length = 2; r.push(5); return r })
t('map result is longer than the callbacks filled', () => { const a = [1, 2, 3, 4]; const r = a.map((x, i) => { if (i === 1) a.length = 2; return x }); return [r.length, r] })
t('map subclass', () => { class A extends Array {}; const a = A.from([1, 2, 3]); const r = a.map(x => x + 1); return [r instanceof A, r.length, r, r.constructor === A] })
t('map subclass with a constructor that counts', () => { const calls = []; class A extends Array { constructor(...args) { super(...args); calls.push(args.length ? args[0] : 'none') } }; const r = A.from([1, 2, 3]).map(x => x); return [calls, r.length] })
t('map subclass species Array', () => { class A extends Array { static get [Symbol.species]() { return Array } }; const r = new A(1, 2, 3).map(x => x); return [r instanceof A, Array.isArray(r), r.length] })
t('map subclass species object', () => { const a = [1, 2, 3]; a.constructor = { [Symbol.species]: function (n) { this.length = n; this.made = n } }; const r = a.map(x => x * 2); return [r.made, r.length, r[0], r[2]] })
t('map subclass species is huge', () => { const a = [1, 2]; a.constructor = { [Symbol.species]: function (n) { return new Array(1e6) } }; const r = a.map(x => x); return [r.length, r[0], r[1], 2 in r] })
t('map species null', () => { const a = [1, 2]; a.constructor = { [Symbol.species]: null }; return a.map(x => x) })
t('map species not a constructor', () => { const a = [1, 2]; a.constructor = { [Symbol.species]: () => 1 }; return a.map(x => x) })
t('map array-like', () => Array.prototype.map.call({ length: 3, 0: 'a', 2: 'c' }, x => x + x))
t('map array-like huge length', () => Array.prototype.map.call({ length: 2 ** 32 }, x => x))
t('map array-like length 1e6 with no elements', () => { const r = Array.prototype.map.call({ length: 1e6 }, x => x); return [r.length, 5 in r] })
t('map string', () => Array.prototype.map.call('abc', x => x.toUpperCase()))
t('map arguments', () => (function () { return Array.prototype.map.call(arguments, x => x * 2) })(1, 2, 3))
t('map a proxy', () => { const p = new Proxy([1, 2, 3], {}); return Array.prototype.map.call(p, x => x * 2) })
t('map on a long array', () => { const a = Array.from({ length: 5000 }, (_, i) => i); const r = a.map(x => x + 1); return [r.length, r[0], r[4999], r.reduce((s, x) => s + x, 0)] })
t('filter keeps working', () => [1, 2, 3, 4, 5, 6].filter(x => x % 2 === 0))
t('slice keeps working', () => [1, 2, 3, 4, 5].slice(1, 4))
t('flatMap', () => [1, 2, 3].flatMap(x => [x, x * 2]))
t('flat', () => [1, [2, [3, [4]]]].flat(Infinity))
t('Array.from length', () => Array.from({ length: 4 }, (_, i) => i * i))

// Array(n)
for (const n of [0, 1, 2, 3, 4, 5, 8, 9, 16, 100, 1000, 100000, 2 ** 20, 2 ** 31 - 1, 2 ** 32 - 1]) {
  t('Array(' + n + ')', () => { const a = Array(n); return [a.length, 0 in a, a.length ? a.length - 1 in a : 'none'] })
  t('new Array(' + n + ') then the first four', () => { const a = new Array(n); for (let i = 0; i < 4 && i < n; i++) a[i] = i; return [a.length, a[0], a[1], a[2], a[3], Object.keys(a).length] })
}
t('Array(n) then fill', () => Array(6).fill(7))
t('Array(n) then push', () => { const a = Array(3); a.push(4); return [a, a.length] })
t('Array(n) then index back', () => { const a = Array(5); for (let i = 4; i >= 0; i--) a[i] = i; return a })
t('Array(n) then every index', () => { const a = Array(12); for (let i = 0; i < 12; i++) a[i] = i; return a })
t('Array(n) then join', () => Array(4).join('-'))
t('Array(n) then map', () => Array(3).map(x => 1))
t('Array.apply', () => Array.apply(null, Array(4)).map((_, i) => i))
t('Array(-1)', () => Array(-1))
t('Array(1.5)', () => Array(1.5))
t('Array(2**32)', () => Array(2 ** 32))
t('Array(NaN)', () => Array(NaN))
t("Array('3')", () => Array('3'))
t('Array(3, 4)', () => Array(3, 4))
t('Array(undefined)', () => Array(undefined))
t('Array(null)', () => Array(null))
t('Array(true)', () => Array(true))
t('Array(2n)', () => Array(2n))
t('Array() and new Array()', () => [Array(), new Array(), Array.length])
t('Array(n) subclass', () => { class A extends Array {}; const a = new A(5); return [a.length, a instanceof A, A.from([1, 2]).length] })
t('Array of with a number', () => Array.of(3))
t('Array(n) from a function with new.target', () => Reflect.construct(Array, [3], function () {}.bind()).length)
t('Array(n) with a prototype', () => { const f = function () {}; f.prototype = { z: 1 }; const a = Reflect.construct(Array, [2], f); return [a.length, a.z, Array.isArray(a)] })

// own keys
for (let n = 0; n <= 9; n++) {
  const o = {}
  for (let i = 0; i < n; i++) o['k' + i] = i
  t('keys ' + n, () => [Object.keys(o), Object.getOwnPropertyNames(o), Reflect.ownKeys(o), Object.entries(o), Object.values(o)])
  t('for in ' + n, () => { const r = []; for (const k in o) r.push(k); return r })
}
t('keys with symbols', () => { const s = Symbol('s'); const o = { a: 1, [s]: 2, b: 3 }; return [Object.keys(o), Reflect.ownKeys(o), Object.getOwnPropertySymbols(o)] })
t('keys with integers', () => Reflect.ownKeys({ b: 1, 2: 1, a: 1, 1: 1, [Symbol.iterator]: 1, 10: 1 }))
t('keys of an array', () => { const a = [1, , 3]; a.x = 1; return [Object.keys(a), Object.getOwnPropertyNames(a), Reflect.ownKeys(a)] })
t('keys of a long array', () => Object.keys(Array.from({ length: 300 }, (_, i) => i)).length)
t('keys of a string', () => [Object.keys('abc'), Object.getOwnPropertyNames('ab')])
t('keys of a typed array', () => [Object.keys(new Uint8Array(3)), Object.getOwnPropertyNames(new Uint8Array(2))])
t('keys of a function', () => Object.getOwnPropertyNames(function f(a, b) {}))
t('keys of a class', () => Object.getOwnPropertyNames(class { static m() {} }))
t('keys of an arguments object', () => (function () { return [Object.keys(arguments), Object.getOwnPropertyNames(arguments)] })(1, 2))
t('keys of a proxy', () => { const p = new Proxy({ a: 1, b: 2 }, { ownKeys: () => ['b', 'a', 'c'], getOwnPropertyDescriptor: (t, k) => ({ value: 1, enumerable: true, configurable: true }) }); return [Object.keys(p), Reflect.ownKeys(p)] })
t('keys of a proxy with a throwing trap', () => Object.keys(new Proxy({}, { ownKeys() { throw new RangeError('trap') } })))
t('keys of a big object', () => { const o = {}; for (let i = 0; i < 2000; i++) o['p' + i] = i; return [Object.keys(o).length, Object.getOwnPropertyNames(o)[1999], Object.entries(o).length] })
t('keys of a null prototype object', () => Object.keys(Object.assign(Object.create(null), { x: 1, y: 2 })))
t('keys then push', () => { const k = Object.keys({ a: 1, b: 2, c: 3 }); k.push('d'); k.length = 2; k.push('e'); return k })
t('Object.assign and spread order', () => Object.keys({ ...{ b: 1, a: 2 }, ...{ c: 3 } }))
t('getOwnPropertyDescriptors', () => Object.keys(Object.getOwnPropertyDescriptors({ a: 1, get b() { return 2 } })))
t('Object.fromEntries', () => Object.fromEntries([['a', 1], ['b', 2]]))
t('Object.groupBy', () => { const g = Object.groupBy([1, 2, 3, 4, 5], x => x % 2 ? 'odd' : 'even'); return [g.odd, g.even] })
t('string split', () => ['a,b,c,d,e'.split(','), ''.split(','), 'abc'.split(''), 'a b  c'.split(' ')])
t('JSON.parse arrays', () => JSON.parse('[1,[2,3],[],[[4,5,6,7,8]]]'))
t('regexp match all', () => 'a1b22c333'.match(/\d+/g))
t('regexp exec groups', () => /(a)(b)?(c)/.exec('ac'))
out.join('\n')
