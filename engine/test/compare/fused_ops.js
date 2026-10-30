// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// The pairs the compiler joins into one opcode (fuse_code): a read of the
// first or second argument, of the first local or of a local past the fourth,
// then a property read by name, and the case of a switch on a string. Each
// reads every kind of value the pair can meet, and the file is what the
// engine printed before the opcodes, and with them turned off (compare.sh runs
// it both ways). The errors carry the line and column of the read.
const out = []
const show = v => typeof v === 'symbol' ? v.toString() : typeof v === 'bigint' ? v + 'n' :
  typeof v === 'function' ? 'function ' + v.name : Array.isArray(v) ? '[' + v.map(show) + ']' :
  v !== null && typeof v === 'object' ? 'object' : Object.is(v, -0) ? '-0' : String(v)
const where = e => String(e.stack).split('\n').slice(0, 3).map(l => l.trim().replace(/\(.*[\\/]/, '(').replace(/^at [^(]*$/, 'at <native>')).join(' / ')
function t(name, f) {
  let r
  try { r = show(f()) } catch (e) { r = (e && e.constructor && e.constructor.name) + ': ' + (e && e.message) + ' @ ' + where(e) }
  out.push(name + ': ' + r)
}

// the four reads, each in a function of its own
function arg0(o) { return o.x }
function arg1(_, o) { return o.x }
function loc0(v) { let o = v; return o.x }
function loc8(v) { let a = 1, b = 2, c = 3, d = 4, o = v; return o.x + (a + b + c + d) * 0 }
const reads = { arg0, arg1: v => arg1(0, v), loc0, loc8 }

const proto = { x: 'from the prototype' }
const getterObj = { get x() { return 'getter' } }
const setterOnly = { set x(v) { } }
const proxy = new Proxy({ x: 'target' }, { get(t, k, r) { return 'trap ' + String(k) } })
const proxyProto = Object.create(new Proxy({}, { get(t, k) { return 'proto trap ' + String(k) } }))
const withStack = new Error('boom')
const arr = Object.assign([1, 2, 3], { x: 'array key' })
const typed = new Uint8Array(4)
const values = [
  ['own data', () => ({ x: 1 })],
  ['own undefined', () => ({ x: undefined })],
  ['missing', () => ({ y: 1 })],
  ['null prototype', () => Object.create(null)],
  ['inherited', () => Object.create(proto)],
  ['inherited twice', () => Object.create(Object.create(proto))],
  ['getter', () => getterObj],
  ['inherited getter', () => Object.create(getterObj)],
  ['setter only', () => setterOnly],
  ['proxy', () => proxy],
  ['proxy as prototype', () => proxyProto],
  ['frozen', () => Object.freeze({ x: 'frozen' })],
  ['deleted', () => { const o = { x: 1, y: 2 }; delete o.x; return o }],
  ['array', () => arr],
  ['array without key', () => [1, 2]],
  ['typed array', () => typed],
  ['String object', () => new String('abc')],
  ['arguments', function () { return arguments }],
  ['function', () => function x() {}],
  ['class instance', () => new (class { constructor() { this.x = 'field' } })()],
  ['private field class', () => new (class { #p = 1; get x() { return this.#p } })()],
  ['Error stack', () => withStack],
  ['string', () => 'str'],
  ['empty string', () => ''],
  ['string with x on String.prototype', () => 'str'],
  ['number', () => 42],
  ['float', () => 1.5],
  ['boolean', () => true],
  ['symbol', () => Symbol('s')],
  ['bigint', () => 10n],
  ['undefined', () => undefined],
  ['null', () => null],
]
for (const [kind, make] of values) {
  for (const [name, read] of Object.entries(reads)) {
    if (kind === 'string with x on String.prototype') String.prototype.x = 'on String.prototype'
    t(name + ' ' + kind, () => read(make()))
    if (kind === 'string with x on String.prototype') delete String.prototype.x
  }
}

// the read of other names on primitives and objects: the paths get_field
// has of its own
function len0(o) { return o.length }
function up0(o) { return o.toUpperCase }
function fixed0(o) { return o.toFixed }
function name0(o) { return o.name }
function stack0(o) { return o.stack }
function idx0(o) { return o[0] }
function desc0(o) { return o.description }
t('length of a string', () => len0('abcd'))
t('length of an array', () => len0([1, 2]))
t('toUpperCase of a string', () => up0('a') === String.prototype.toUpperCase)
t('toFixed of a number', () => fixed0(1) === Number.prototype.toFixed)
t('toFixed of a float', () => fixed0(1.5) === Number.prototype.toFixed)
t('name of a function', () => name0(function named() {}))
t('name of a class', () => name0(class K {}))
t('stack of an error', () => typeof stack0(new Error('e')))
t('index of an array', () => idx0(['zero']))
t('description of a symbol', () => desc0(Symbol('d')))

// what the property's getter does to the local while the read is on the way
function rewrite(v) {
  let o = v
  o.self = { get x() { o = null; v = null; return 'rewritten' } }
  let s = o.self
  return s.x + ' ' + o
}
t('getter that sets the local to null', () => rewrite({}))
function dropArg(o) {
  Object.defineProperty(o, 'x', { get() { o = undefined; return 'dropped ' + typeof o } })
  return o.x + ' ' + typeof o
}
t('getter that drops the argument', () => dropArg({}))
function freeTheOnly(o) {
  // the only reference to the object is the argument: the getter replaces it
  // and runs a collection, then reads the object through this
  Object.defineProperty(o, 'x', { get() { o = null; gc(); return this.y } })
  o.y = 'still here'
  return o.x
}
t('getter that frees the object of the argument', () => freeTheOnly({}))
function proxyRewrite(v) {
  let o = new Proxy({}, { get(t, k) { o = 0; gc(); return 'proxy ' + String(k) } })
  return o.x + ' ' + o + ' ' + v
}
t('proxy trap that rewrites the local', () => proxyRewrite('v'))

// arguments mapped to the parameters, and not
function mapped(a) { arguments[0] = { x: 'through arguments' }; return a.x }
function unmapped(a) { 'use strict'; arguments[0] = { x: 'through arguments' }; return a.x }
function mappedBack(a) { a = { x: 'parameter' }; return arguments[0].x + ' ' + a.x }
t('mapped arguments', () => mapped({ x: 'first' }))
t('unmapped arguments', () => unmapped({ x: 'first' }))
t('mapped back', () => mappedBack({ x: 'first' }))
t('missing argument', () => arg0())

// a local before its declaration, and one written by a closure
function tdz() { const f = () => o.x; try { return f() } finally { } ; let o = { x: 1 } }
function tdzDirect() { return o.x; let o = { x: 1 } }
t('closure before let', () => tdz())
t('read before let', () => tdzDirect())
function closureWrites() {
  let o = { x: 'first' }
  const set = () => { o = { x: 'second' } }
  const a = o.x
  set()
  return a + ' ' + o.x
}
t('closure writes the local', () => closureWrites())
function loopReads(list) {
  let s = ''
  for (let i = 0; i < list.length; i++) { const o = list[i]; s += o.x }
  return s
}
t('loop', () => loopReads([{ x: 'a' }, { x: 'b' }, Object.create({ x: 'c' }), 'd']))

// generators and async functions resume in the middle of their code
function* gen(o) { yield o.x; yield o.y; return o.z }
t('generator', () => [...gen({ x: 1, y: 2, z: 3 })])
async function asyncRead(o) { await null; return o.x }
asyncRead({ x: 'async' }).then(v => out.push('async: ' + v))

// errors: the message, and the line and column of the read
t('error arg0', () => arg0(undefined))
t('error arg1', () => arg1(0, null))
t('error loc0', () => loc0(undefined))
t('error loc8', () => loc8(null))
t('error in a nested call', () => [1].map(() => arg0(null)))

// switch on strings: the case compares with ===
function sw(v) {
  switch (v) {
    case 'alpha': return 'A'
    case 'beta': return 'B'
    case '': return 'empty'
    case '1': return 'one as a string'
    case 'üñí': return 'wide'
    case '__proto__': return 'proto'
    case 'gamma':
    case 'delta': return 'G or D'
    default: return 'none'
  }
}
const long = 'x'.repeat(10000)
function swLong(v) { switch (v) { case 'ab': return 'ab'; default: return 'other' } }
const switchValues = [
  'alpha', 'beta', '', '1', 1, 'üñí', '__proto__', 'gamma', 'delta', 'epsilon', 'alph', 'alphaa',
  'al' + 'pha', ['be', 'ta'].join(''), String.fromCharCode(97, 108, 112, 104, 97), 'ALPHA'.toLowerCase(),
  new String('alpha'), { toString() { return 'alpha' } }, null, undefined, true, 1.5, 0, -0, 10n,
  Symbol('alpha'), Symbol.for('alpha'),
]
for (const v of switchValues) t('switch ' + show(v) + ' ' + typeof v, () => sw(v))
t('switch on a rope', () => swLong(long + 'ab'))
t('switch on a rope equal', () => { let s = 'a'; s += 'b'; return swLong(s) })
t('switch on a long string', () => swLong(long))
t('switch in a loop', () => { let n = 0; for (const s of ['alpha', 'x', 'beta', 'alpha']) if (sw(s) === 'A') n++; return n })
t('switch on a symbol description', () => { const s = Symbol('alpha'); return sw(s.description) })
t('switch with a getter case', () => {
  let n = 0
  const o = { get k() { n++; return 'k' } }
  switch ('k') { case o.k: return 'k after ' + n }
})

;(() => out.join("\n"))
