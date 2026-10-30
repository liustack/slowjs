// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// copyProps (JS_NewCopyPropsFunction) against the loops of React 19.3 in
// createElement and cloneElement, which it stands in for. The two functions
// below are those of react.production.js, once with the loop and once with
// copyProps in front of it, as the tifa plugin writes it. Every config gives
// the same key, the same props in the same order with the same descriptors,
// the same error and the same reads and calls in the same order, and the
// configs where the function can say so are copied by it and the others are
// left to the loop (expect: it answers false and has changed nothing).
'use strict'
const hasOwnProperty = Object.prototype.hasOwnProperty
const assign = Object.assign
const call = Function.prototype.call
const out = []
let events = []
const note = s => void events.push(s)
const undos = []
const later = f => void undos.push(f)

function reactCreate(config) {
  var propName,
    props = {},
    key = null
  if (null != config)
    for (propName in (void 0 !== config.key && (key = '' + config.key), config))
      hasOwnProperty.call(config, propName) &&
        'key' !== propName &&
        '__self' !== propName &&
        '__source' !== propName &&
        (props[propName] = config[propName])
  return [key, props]
}
let lastAnswer
function nativeCreate(config) {
  var propName,
    props = {},
    key = null
  if (null != config)
    if (
      (void 0 !== config.key && (key = '' + config.key),
      !(lastAnswer = copyProps(config, props, hasOwnProperty, false)))
    )
      for (propName in config)
        hasOwnProperty.call(config, propName) &&
          'key' !== propName &&
          '__self' !== propName &&
          '__source' !== propName &&
          (props[propName] = config[propName])
  return [key, props]
}
function reactClone(base, config) {
  var props = assign({}, base),
    key = 'baseKey'
  if (null != config)
    for (propName in (void 0 !== config.key && (key = '' + config.key), config))
      !hasOwnProperty.call(config, propName) ||
        'key' === propName ||
        '__self' === propName ||
        '__source' === propName ||
        ('ref' === propName && void 0 === config.ref) ||
        (props[propName] = config[propName])
  var propName
  return [key, props]
}
function nativeClone(base, config) {
  var props = assign({}, base),
    key = 'baseKey'
  if (null != config)
    if (
      (void 0 !== config.key && (key = '' + config.key),
      !(lastAnswer = copyProps(config, props, hasOwnProperty, true)))
    )
      for (propName in config)
        !hasOwnProperty.call(config, propName) ||
          'key' === propName ||
          '__self' === propName ||
          '__source' === propName ||
          ('ref' === propName && void 0 === config.ref) ||
          (props[propName] = config[propName])
  var propName
  return [key, props]
}

function show(v) {
  if (typeof v === 'symbol') return v.toString()
  if (typeof v === 'string') return JSON.stringify(v.length > 100 ? v.slice(0, 40) + '...' + v.length : v)
  if (typeof v === 'function') return 'function'
  if (v !== null && typeof v === 'object') return Array.isArray(v) ? '[array]' : '{object}'
  return String(v)
}
function describe(p) {
  return (
    (Object.getPrototypeOf(p) === Object.prototype ? '' : 'proto:other ') +
    (Object.isExtensible(p) ? '' : 'sealed ') +
    Reflect.ownKeys(p)
      .map(k => {
        const d = Object.getOwnPropertyDescriptor(p, k)
        const flags = d.enumerable && d.writable && d.configurable ? '' : '!flags'
        return show(k) + ':' + ('value' in d ? show(d.value) : 'accessor') + flags
      })
      .join(',')
  )
}
// What a program can see of one run.
function run(f, make, base) {
  events = []
  lastAnswer = undefined
  let r
  try {
    const config = make()
    const [key, props] = base ? f(base(), config) : f(config)
    r = { key: show(key), props: describe(props) }
  } catch (e) {
    r = { err: e && e.name + ': ' + e.message }
  } finally {
    while (undos.length) undos.pop()()
  }
  r.log = events.join(' | ')
  r.answers = lastAnswer === undefined ? [] : [lastAnswer]
  return r
}

const cases = []
// native: whether copyProps takes the config for createElement and for
// cloneElement (true, false, or [create, clone])
const add = (name, make, native = true) => void cases.push({ name, make, native })
const onObjectPrototype = (name, descriptor) => {
  const saved = Object.getOwnPropertyDescriptor(Object.prototype, name)
  // the undo first: with a read-only 0 on Object.prototype, undos.push(...) throws
  later(() => (saved ? void Object.defineProperty(Object.prototype, name, saved) : void delete Object.prototype[name]))
  Object.defineProperty(Object.prototype, name, { configurable: true, ...descriptor })
}

add('plain', () => ({ a: 1, b: 'x', children: 'c' }))
add('empty', () => ({}))
add('no config', () => undefined, 'none')
add('null config', () => null, 'none')
add('one prop', () => ({ className: 'x' }))
add('same literal again', () => ({ className: 'x' }))
add('key string', () => ({ key: 'k', a: 1 }))
add('key first', () => ({ key: 'k' }))
add('key last', () => ({ a: 1, b: 2, key: 'k' }))
add('key in the middle', () => ({ a: 1, key: 'k', b: 2 }))
add('key number zero', () => ({ key: 0, a: 1 }))
add('key null', () => ({ key: null, a: 1 }))
add('key undefined', () => ({ key: undefined, a: 1 }))
add('key object', () => ({ key: { toString: () => (note('key.toString'), 'T') }, a: 1 }))
add('key object that adds a prop', () => {
  const o = { a: 1, key: { toString: () => (note('key.toString'), (o.late = 2), 'T') } }
  return o
})
add('key object that deletes a prop', () => {
  const o = { a: 1, key: { toString: () => (note('key.toString'), delete o.a, 'T') }, b: 2 }
  return o
})
add('key object that changes Object.prototype', () => {
  const o = {
    a: 1,
    key: {
      toString: () => {
        note('key.toString')
        Object.defineProperty(Object.prototype, 'a', { configurable: true, set(v) { note('setter a ' + v) } })
        later(() => void delete Object.prototype.a)
        return 'T'
      },
    },
  }
  return o
}, false)
add('key symbol', () => ({ key: Symbol('s'), a: 1 }), 'none')
add('__self and __source', () => ({ __self: 1, __source: 2, a: 1 }))
add('children, ref, style and onClick', () => ({ ref: () => {}, children: [1, 2], style: { color: 'red' }, onClick: () => {} }))
add('ref undefined', () => ({ ref: undefined, a: 1 }))
add('ref undefined only', () => ({ ref: undefined }))
add('ref null', () => ({ ref: null, a: 1 }))
add('ref string', () => ({ a: 1, ref: 'r', b: 2 }))
add('ref last', () => ({ a: 1, b: 2, ref: 'r' }))
add('ref and key', () => ({ ref: 'r', key: 'k', a: 1 }))
add('integer keys', () => ({ b: 1, 2: 'two', a: 2, 1: 'one', 0: 'zero' }), false)
add('numeric string keys', () => ({ b: 1, '01': 'x', '-1': 2, 4294967295: 3 }))
add('index key only', () => ({ 0: 'x' }), false)
add('non-enumerable own', () => Object.defineProperty({ a: 1 }, 'h', { value: 2, enumerable: false }))
add('non-enumerable key', () => Object.defineProperty({ a: 1 }, 'key', { value: 'K', enumerable: false }))
add('non-enumerable accessor', () => Object.defineProperty({ a: 1 }, 'h', { get() { note('get h'); return 1 }, enumerable: false }))
add('32 keys', () => Object.fromEntries(Array.from({ length: 32 }, (_, i) => ['k' + i, i])))
add('33 keys', () => Object.fromEntries(Array.from({ length: 33 }, (_, i) => ['k' + i, i])), false)
add('100 keys', () => Object.fromEntries(Array.from({ length: 100 }, (_, i) => ['k' + i, i])), false)
add('25 keys and key', () => Object.fromEntries(['key', ...Array.from({ length: 25 }, (_, i) => 'k' + i)].map((k, i) => [k, i])))
add('frozen', () => Object.freeze({ a: 1, key: 'k' }))
add('frozen without key', () => Object.freeze({ a: 1, b: 2 }))
add('sealed', () => Object.seal({ a: 1 }))
add('not extensible', () => Object.preventExtensions({ a: 1 }))
add('one read-only prop', () => Object.defineProperty({ a: 1 }, 'b', { value: 2, enumerable: true, writable: false, configurable: true }))
add('one fixed prop', () => Object.defineProperty({ a: 1 }, 'b', { value: 2, enumerable: true, writable: true, configurable: false }))
add('prop deleted', () => {
  const o = { a: 1, b: 2, c: 3 }
  delete o.b
  return o
})
add('prop deleted and added', () => {
  const o = { a: 1, b: 2, c: 3 }
  delete o.b
  o.b = 4
  return o
})
add('all props deleted', () => {
  const o = { a: 1 }
  delete o.a
  return o
})
add('getter on key', () => ({ get key() { note('get key'); return 'K' }, a: 1 }))
add('getter on key undefined', () => ({ get key() { note('get key'); return undefined }, a: 1 }))
add('getter on key throws', () => ({ get key() { throw new Error('boom key') }, a: 1 }), 'none')
add('setter only on key', () => ({ set key(v) {}, a: 1 }))
add('getter on __self', () => ({ get __self() { note('get __self'); return 1 }, a: 1 }))
add('getter on __source', () => ({ get __source() { note('get __source'); return 1 }, a: 1 }))
add('getter on ref', () => ({ get ref() { note('get ref'); return 'R' }, a: 1 }), false)
add('getter on ref undefined', () => ({ get ref() { note('get ref'); return undefined }, a: 1 }), false)
add('getters on props', () => ({ get a() { note('get a'); return 1 }, b: 2, get c() { note('get c'); return 3 } }), false)
add('setter on a prop', () => ({ a: 1, set b(v) { note('set b') } }), false)
add('getter throws', () => ({ a: 1, get b() { note('get b'); throw new Error('boom') }, get c() { note('get c'); return 3 } }), false)
add('getter changes a later value', () => { const o = { get a() { note('get a'); o.b = 'changed'; return 1 }, b: 2 }; return o }, false)
add('getter adds a key', () => { const o = { get a() { note('get a'); o.z = 9; return 1 }, b: 2 }; return o }, false)
add('getter deletes a later key', () => { const o = { get a() { note('get a'); delete o.b; return 1 }, b: 2, c: 3 }; return o }, false)
add('getter makes a later key not enumerable', () => { const o = { get a() { note('get a'); Object.defineProperty(o, 'b', { enumerable: false }); return 1 }, b: 2, c: 3 }; return o }, false)
add('getter on key changes a prop', () => { const o = { get key() { note('get key'); o.b = 'changed'; return 'K' }, a: 1, b: 2 }; return o })
add('getter on key deletes a prop', () => { const o = { get key() { note('get key'); delete o.b; return 'K' }, a: 1, b: 2 }; return o })
add('symbol key', () => ({ a: 1, [Symbol('s')]: 2 }))
add('symbol key first', () => ({ [Symbol('s')]: 2, a: 1 }))
add('symbol key between', () => ({ a: 1, [Symbol('s')]: 2, b: 3 }))
add('symbol key not enumerable', () => Object.defineProperty({ a: 1 }, Symbol('s'), { value: 2, enumerable: false }))
add('symbol getter', () => Object.defineProperty({ a: 1 }, Symbol('s'), { enumerable: true, get() { note('get symbol'); return 2 } }))
add('Symbol.for key', () => ({ a: 1, [Symbol.for('registered')]: 2 }))
add('well-known symbol key', () => ({ a: 1, [Symbol.iterator]: 2, b: 3 }))
add('inherited enumerable', () => Object.assign(Object.create({ inh: 1 }), { own: 2 }), false)
add('inherited key', () => Object.create({ key: 'inherited' }, { a: { value: 1, enumerable: true } }), false)
add('inherited getter on key', () => Object.create({ get key() { note('get inherited key'); return 'IK' } }, { a: { value: 1, enumerable: true } }), false)
add('inherited getter on __self', () => Object.create({ get __self() { note('get inherited __self'); return 1 } }, { a: { value: 1, enumerable: true } }), false)
add('inherited ref', () => Object.create({ ref: 'inherited' }, { a: { value: 1, enumerable: true } }), false)
add('same name own and inherited', () => Object.assign(Object.create({ a: 'inherited' }), { a: 'own' }), false)
add('prototype with a Proxy', () => Object.assign(Object.create(new Proxy({ inh: 1 }, handler('proto'))), { a: 1 }), false)
add('class instance', () => new (class { x = 1; get y() { note('get y'); return 2 } })(), false)
add('class instance with only fields', () => new (class { x = 1; z = 2 })(), false)
add('null prototype', () => Object.assign(Object.create(null), { a: 1, b: 2 }), false)
add('null prototype with key', () => Object.assign(Object.create(null), { key: 'k', a: 1 }), false)
add('Object.create(Object.prototype)', () => Object.assign(Object.create(Object.prototype), { a: 1 }))
add('array', () => ['x', 'y'], false)
add('array with a prop', () => Object.assign(['x'], { extra: 1 }), false)
add('empty array', () => [], false)
add('function with props', () => Object.assign(() => {}, { a: 1, b: 2 }), false)
add('class with static props', () => class { static a = 1 }, false)
add('string', () => 'ab', false)
add('number', () => 5, false)
add('boolean', () => true, false)
add('symbol', () => Symbol('s'), false)
add('bigint', () => 10n, false)
add('String object', () => new String('ab'), false)
add('Number object', () => Object.assign(new Number(1), { a: 1 }), false)
add('string with String.prototype.key', () => { String.prototype.key = 'SK'; later(() => void delete String.prototype.key); return 'ab' }, false)
add('Date', () => new Date(0), false)
add('Map with a prop', () => Object.assign(new Map(), { a: 1 }), false)
add('Promise with a prop', () => Object.assign(Promise.resolve(1), { a: 1 }), false)
add('arguments', () => (function () { return arguments })(1, 2), false)
add('typed array', () => new Uint8Array([1, 2]), false)
add('error', () => Object.assign(new Error('x'), { a: 1 }), false)
add('plain with stack', () => ({ stack: 'x', a: 1 }), false)
add('error stack copied', () => ({ ...new Error('e'), a: 1 }))
add('own __proto__ object', () => JSON.parse('{"__proto__": {"x": 1}, "a": 1}'), false)
add('own __proto__ null', () => JSON.parse('{"__proto__": null, "a": 1}'), false)
add('own __proto__ number', () => JSON.parse('{"__proto__": 5, "a": 1}'), false)
add('own __proto__ not enumerable', () => Object.defineProperty({ a: 1 }, '__proto__', { value: 5, enumerable: false }))
add('computed __proto__', () => ({ ['__proto__']: { x: 1 }, a: 1 }), false)
add('__proto__ in a literal sets the prototype', () => ({ __proto__: { x: 1 }, a: 1 }), false)
add('__proto__ in a literal sets null', () => ({ __proto__: null, a: 1 }), false)
add('Object.prototype enumerable', () => { Object.prototype.bar = 1; later(() => void delete Object.prototype.bar); return { a: 1 } })
add('Object.prototype writable foo', () => { Object.prototype.foo = 1; later(() => void delete Object.prototype.foo); return { a: 1, foo: 5 } })
add('Object.prototype key', () => { Object.prototype.key = 'OK'; later(() => void delete Object.prototype.key); return { a: 1 } })
add('Object.prototype ref', () => { Object.prototype.ref = 'OR'; later(() => void delete Object.prototype.ref); return { a: 1 } })
add('Object.prototype getter on key', () => { onObjectPrototype('key', { get() { note('get Object.prototype.key'); return 'OK' } }); return { a: 1 } }, false)
add('Object.prototype getter on __self', () => { onObjectPrototype('__self', { get() { note('get Object.prototype.__self'); return 1 } }); return { a: 1 } }, false)
add('Object.prototype getter on ref', () => { onObjectPrototype('ref', { get() { note('get Object.prototype.ref'); return 'OR' } }); return { a: 1 } }, false)
add('Object.prototype setter', () => { onObjectPrototype('foo', { set(v) { note('Object.prototype.foo setter ' + v) } }); return { a: 1, foo: 5 } }, false)
add('Object.prototype setter, other key', () => { onObjectPrototype('foo', { set(v) { note('Object.prototype.foo setter ' + v) } }); return { a: 1 } }, false)
add('Object.prototype read-only', () => { onObjectPrototype('foo', { value: 1, writable: false }); return { a: 1, foo: 5 } }, false)
add('Object.prototype getter only', () => { onObjectPrototype('foo', { get() { return 1 } }); return { a: 1, foo: 5 } }, false)
add('Object.prototype symbol setter', () => { const s = Symbol('s'); onObjectPrototype(s, { set(v) { note('symbol setter') } }); return { a: 1, [s]: 2 } }, false)
add('Object.prototype getter on an index', () => { onObjectPrototype(7, { get() { return 1 } }); return { a: 1 } }, false)
add('Object.prototype frozen', () => {
  const names = Object.getOwnPropertyNames(Object.prototype).filter(n => n !== '__proto__')
  const saved = Object.fromEntries(names.map(n => [n, Object.getOwnPropertyDescriptor(Object.prototype, n)]))
  for (const n of names) Object.defineProperty(Object.prototype, n, { writable: false })
  later(() => { for (const n of names) Object.defineProperty(Object.prototype, n, saved[n]) })
  return { a: 1, toString: 5 }
}, false)
add('Object.prototype replaced __proto__ property', () => {
  const d = Object.getOwnPropertyDescriptor(Object.prototype, '__proto__')
  Object.defineProperty(Object.prototype, '__proto__', { value: 1, writable: true, configurable: true })
  later(() => void Object.defineProperty(Object.prototype, '__proto__', d))
  return { a: 1 }
})
add('Object.prototype with 50 more writable properties', () => {
  for (let i = 0; i < 50; i++) onObjectPrototype('w' + i, { value: i, writable: true })
  return { a: 1, w3: 'own' }
}, 'none')
add('Object.prototype with 100 more writable properties', () => {
  for (let i = 0; i < 100; i++) onObjectPrototype('w' + i, { value: i, writable: true })
  return { a: 1, w3: 'own' }
}, false)
add('Object.prototype with 100 more writable properties, 98 deleted', () => {
  for (let i = 0; i < 100; i++) onObjectPrototype('w' + i, { value: i, writable: true })
  for (let i = 0; i < 98; i++) delete Object.prototype['w' + i]
  return { a: 1 }
}, 'none')
add('Object.prototype extended then restored', () => {
  Object.prototype.tmp = 1
  delete Object.prototype.tmp
  return { a: 1, tmp: 2 }
})
add('Function.prototype.call replaced', () => {
  Function.prototype.call = function (...args) { note('call replaced'); return call.apply(this, args) }
  later(() => void (Function.prototype.call = call))
  return { a: 1, b: 2 }
}, false)
add('Function.prototype.call deleted', () => {
  delete Function.prototype.call
  later(() => void Object.defineProperty(Function.prototype, 'call', { value: call, writable: true, configurable: true }))
  return { a: 1 }
}, false)
add('Function.prototype.call replaced and put back', () => {
  Function.prototype.call = 1
  Function.prototype.call = call
  return { a: 1 }
})
add('Function.prototype got another method', () => {
  Function.prototype.other = 1
  later(() => void delete Function.prototype.other)
  return { a: 1 }
})
add('Function.prototype.call a getter', () => {
  Object.defineProperty(Function.prototype, 'call', { get() { note('get call'); return call }, configurable: true })
  later(() => void Object.defineProperty(Function.prototype, 'call', { value: call, writable: true, configurable: true }))
  return { a: 1 }
}, false)
add('hasOwnProperty.call has an own call', () => {
  Object.defineProperty(hasOwnProperty, 'call', { configurable: true, writable: true, value(...args) { note('hasOwnProperty.call'); return call.apply(this, args) } })
  later(() => void delete hasOwnProperty.call)
  return { a: 1 }
}, false)
add('hasOwnProperty got another prototype', () => {
  const proto = Object.getPrototypeOf(hasOwnProperty)
  Object.setPrototypeOf(hasOwnProperty, { call(...args) { note('other call'); return call.apply(this, args) } })
  later(() => void Object.setPrototypeOf(hasOwnProperty, proto))
  return { a: 1 }
}, false)
add('hasOwnProperty got an own property', () => {
  hasOwnProperty.extra = 1
  later(() => void delete hasOwnProperty.extra)
  return { a: 1 }
})
add('Object.getOwnPropertySymbols replaced', () => {
  const original = Object.getOwnPropertySymbols
  Object.getOwnPropertySymbols = function (o) { note('getOwnPropertySymbols replaced'); return original(o) }
  later(() => void (Object.getOwnPropertySymbols = original))
  return { a: 1 }
})
const handler = tag => {
  const h = {}
  for (const trap of ['get', 'set', 'has', 'ownKeys', 'getOwnPropertyDescriptor', 'getPrototypeOf', 'defineProperty', 'deleteProperty', 'isExtensible', 'preventExtensions', 'setPrototypeOf']) {
    h[trap] = (...args) => {
      note(tag + '.' + trap + (['get', 'has', 'getOwnPropertyDescriptor'].includes(trap) ? ' ' + show(args[1]) : ''))
      return Reflect[trap](...args)
    }
  }
  return h
}
add('Proxy', () => new Proxy({ a: 1, b: 2, key: 'k' }, handler('P')), false)
add('Proxy without key', () => new Proxy({ a: 1, b: 2 }, handler('P')), false)
add('Proxy of an array', () => new Proxy([1, 2], handler('P')), false)
add('Proxy that adds a key', () => new Proxy({ a: 1 }, {
  ownKeys: () => ['a', 'ghost'],
  getOwnPropertyDescriptor: (t, k) => (k === 'ghost' ? { value: 1, enumerable: true, configurable: true } : Reflect.getOwnPropertyDescriptor(t, k)),
  get: (t, k) => (k === 'ghost' ? 'G' : t[k]),
}), false)
add('Proxy with get only', () => new Proxy({ a: 1 }, { get: (t, k) => (note('get ' + show(k)), t[k]) }), false)
add('revoked Proxy', () => { const r = Proxy.revocable({}, {}); r.revoke(); return r.proxy }, 'none')
add('values: undefined, null, objects, functions', () => ({ a: undefined, b: null, c: {}, d: () => 1, e: 1.5, f: 10n, g: Symbol('v'), h: NaN, i: -0 }))
add('long names', () => ({ ['x'.repeat(300)]: 1, ['y'.repeat(70000)]: 2 }))
add('names that look like prototype keys', () => ({ constructor: 1, toString: 2, valueOf: 3, hasOwnProperty: 4, isPrototypeOf: 5, propertyIsEnumerable: 6, toLocaleString: 7 }))
add('the names of Object.prototype read first', () => ({ hasOwnProperty: 'mine', a: 1 }))
add('__lookupGetter__ and friends', () => ({ __lookupGetter__: 1, __defineGetter__: 2, __lookupSetter__: 3, __defineSetter__: 4 }))
add('a config used twice', (() => { const shared = { a: 1, b: 2 }; return () => shared })())
add('props that are the config', () => { const o = { a: 1 }; o.self = o; return o })

// Bases of cloneElement: props of the element, copied by assign into a new object.
const bases = {
  none: undefined,
  plain: () => ({ a: 'base', z: 'last', ref: 'baseRef' }),
  empty: () => ({}),
  same: () => ({ a: 1, b: 2, c: 3 }),
  overlap: () => ({ b: 'B', zz: 1, a: 'A' }),
  many: () => Object.fromEntries(Array.from({ length: 40 }, (_, i) => ['m' + i, i])),
  symbol: () => ({ [Symbol('s')]: 1, a: 'base' }),
  numeric: () => ({ 1: 'one', a: 'base' }),
  frozenSource: () => Object.freeze({ a: 'base', c: 'c' }),
  gettersInSource: () => ({ get a() { note('get base a'); return 'got' }, get c() { return 'c' } }),
  protoKey: () => JSON.parse('{"__proto__": {"inh": 1}, "a": "base"}'),
  nullProtoKey: () => JSON.parse('{"__proto__": null, "a": "base"}'),
  childrenOnly: () => ({ children: 'child' }),
  // more slots in the shape of props than the copy reads (64): it leaves them to the loop
  slots64: () => Object.fromEntries(Array.from({ length: 64 }, (_, i) => ['s' + i, i])),
  slots65: () => Object.fromEntries(Array.from({ length: 65 }, (_, i) => ['s' + i, i])),
  slots200: () => Object.fromEntries(Array.from({ length: 200 }, (_, i) => ['s' + i, i])),
  // slots of deleted keys count
  slotsDeleted: () => { const o = Object.fromEntries(Array.from({ length: 80 }, (_, i) => ['d' + i, i])); for (let i = 0; i < 75; i++) delete o['d' + i]; return o },
}

let bad = 0
let count = 0
let fast = 0
let first = ''
function check(label, a, b, want) {
  count++
  const same = JSON.stringify([a.key, a.props, a.err, a.log]) === JSON.stringify([b.key, b.props, b.err, b.log])
  let ok = same
  let why = 'differs'
  if (ok && want !== 'none') {
    const used = a.answers[0] === true
    if (used) fast++
    if (want === true && !used) { ok = false; why = 'should have been copied by copyProps' }
    if (want === false && used) { ok = false; why = 'was copied by copyProps' }
  }
  if (!ok) {
    bad++
    if (bad <= (globalThis.showAll ? 1000 : 8)) out.push('FAIL ' + label + ' ' + why + '\n  native: ' + JSON.stringify(a) + '\n  react:  ' + JSON.stringify(b))
  }
}
for (const c of cases) {
  check('createElement ' + c.name, run(nativeCreate, c.make), run(reactCreate, c.make), c.native)
  for (const [bname, base] of Object.entries(bases)) {
    if (!base) continue
    // a base whose props have a __proto__ key of their own leave props with another prototype: not copied
    // (with __proto__ a data property of Object.prototype, assign makes an own property of it, and the copy is fine)
    const many = bname === 'slots65' || bname === 'slots200'
    const want = c.name.includes('replaced __proto__') ? 'none' : c.native === true && (bname === 'protoKey' || bname === 'nullProtoKey' || many) ? false : c.native
    check('cloneElement(' + bname + ') ' + c.name, run(nativeClone, c.make, base), run(reactClone, c.make, base), want)
  }
}
// Configs drawn at random, from a fixed seed: keys that matter to the loop and to the copy, data and accessor
// properties, flags, symbols, a prototype, a getter that changes the config, a setter on Object.prototype.
let seed = 12345
let randomCopied = 0
const rnd = n => ((seed = (Math.imul(seed, 1103515245) + 12345) & 0x7fffffff), seed % n)
const pool = ['a', 'b', 'c', 'd', 'key', 'ref', '__self', '__source', 'className', 'children', 'style', 'constructor', 'toString', 'stack', 'length', '__proto__', '0', '12', 'x-y']
const def = (o, k, d) => { try { Object.defineProperty(o, k, d) } catch (e) {} }
function randomConfig(r) {
  const o = {}
  const n = r(6)
  const onGet = []
  for (let i = 0; i < n; i++) {
    const k = r(30) === 0 ? Symbol('s' + i) : pool[r(pool.length)]
    const kind = r(12)
    const value = [undefined, null, 0, 'v' + i, {}, f, i][r(7)]
    const flags = { configurable: true, enumerable: r(8) !== 0, writable: r(8) !== 0 }
    if (kind === 0) def(o, k, { get() { note('get ' + String(k)); return value }, enumerable: flags.enumerable, configurable: true })
    else if (kind === 1) def(o, k, { set(v) {}, enumerable: flags.enumerable, configurable: true })
    else if (kind === 2) def(o, k, { get() { note('mutate ' + String(k)); const j = pool[r(pool.length)]; if (r(2)) delete o[j]; else o[j] = i; return value }, enumerable: true, configurable: true })
    else def(o, k, { value, ...flags })
    if (r(10) === 0) { try { Object.freeze(o) } catch (e) {} }
  }
  if (r(10) === 0) try { Object.setPrototypeOf(o, r(2) ? null : { inherited: 1, key: 'ik' }) } catch (e) {}
  return o
}
function f() {}
for (let t = 0; t < 3000; t++) {
  const r = n => rnd(n)
  const make = () => {
    const o = randomConfig(r)
    if (r(12) === 0) {
      const name = pool[r(pool.length)]
      onObjectPrototype(name, r(2) ? { set(v) { note('proto setter ' + name) } } : { value: 1, writable: false })
    }
    return o
  }
  // the config is made again for each run, with the same draws
  const start = seed
  const again = () => { seed = start; return make() }
  const bname = ['plain', 'empty', 'same', 'overlap', 'childrenOnly'][t % 5]
  const a = run(nativeCreate, again), b = run(reactCreate, again)
  const c = run(nativeClone, again, bases[bname]), d = run(reactClone, again, bases[bname])
  if (a.answers[0] === true) randomCopied++
  if (c.answers[0] === true) randomCopied++
  check('random createElement ' + t, a, b, 'none')
  check('random cloneElement ' + t, c, d, 'none')
  seed = start; again(); while (undos.length) undos.pop()()
}
out.push(count + ' runs: ' + (bad ? bad + ' differ' : 'all as React') + '; copyProps took ' + fast + ' of the listed ones and ' + randomCopied + ' of the random ones')

// copyProps called on its own: what it does to props it is given, and what it answers
// for arguments React never gives it. When it answers false it has changed nothing.
function direct(name, config, props, h, clone, want) {
  const before = props && typeof props === 'object' ? describe(props) : ''
  let got
  try { got = copyProps(config, props, h, clone) } catch (e) { got = 'threw ' + e.name }
  const after = props && typeof props === 'object' ? describe(props) : ''
  const changed = before !== after
  if (got !== want || (got === false && changed)) {
    bad++
    out.push('FAIL direct ' + name + ': answered ' + got + ' (want ' + want + ')' + (changed ? ', props changed to ' + after : ''))
  }
}
direct('another hasOwnProperty', { a: 1 }, {}, function () {}, false, false)
direct('a bound hasOwnProperty', { a: 1 }, {}, hasOwnProperty.bind(null), false, false)
direct('hasOwnProperty of a primitive', { a: 1 }, {}, 1, false, false)
direct('hasOwn in its place', { a: 1 }, {}, Object.hasOwn, false, false)
direct('props frozen', { a: 1 }, Object.freeze({}), hasOwnProperty, false, false)
direct('props not extensible', { a: 1 }, Object.preventExtensions({}), hasOwnProperty, false, false)
direct('props with a prototype', { a: 1 }, Object.create({}), hasOwnProperty, false, false)
direct('props null prototype', { a: 1 }, Object.create(null), hasOwnProperty, false, false)
direct('props an array', { a: 1 }, [], hasOwnProperty, false, false)
direct('props a function', { a: 1 }, () => {}, hasOwnProperty, false, false)
direct('props a Proxy', { a: 1 }, new Proxy({}, {}), hasOwnProperty, false, false)
direct('props a number', { a: 1 }, 5, hasOwnProperty, false, false)
direct('props missing', { a: 1 }, undefined, hasOwnProperty, false, false)
direct('config missing', undefined, {}, hasOwnProperty, false, false)
direct('config a number', 5, {}, hasOwnProperty, false, false)
direct('config null', null, {}, hasOwnProperty, false, false)
direct('props with a getter', { a: 1 }, { get b() { return 1 }, c: 2 }, hasOwnProperty, true, false)
direct('props with a setter for the key', { a: 1 }, { set a(v) { note('set') } }, hasOwnProperty, true, false)
direct('props with a read-only prop', { a: 1 }, Object.defineProperty({ c: 1 }, 'a', { value: 0, writable: false, enumerable: true, configurable: true }), hasOwnProperty, true, false)
direct('props with a read-only prop that the config does not name', { a: 1 }, Object.defineProperty({ c: 1 }, 'x', { value: 0, writable: false, enumerable: true, configurable: true }), hasOwnProperty, true, false)
direct('props with a symbol key', { a: 1 }, { [Symbol('s')]: 1, c: 2 }, hasOwnProperty, true, true)
direct('props with the config key already', { a: 1 }, { a: 0, c: 2 }, hasOwnProperty, true, true)
direct('props with index keys', { a: 1 }, { 0: 1, 5: 2 }, hasOwnProperty, true, true)
direct('createElement mode on props with props', { a: 1 }, { c: 2 }, hasOwnProperty, false, true)
direct('clone flag a truthy number', { ref: undefined, a: 1 }, {}, hasOwnProperty, 1, true)
{
  // config and props the same object: nothing changes
  const o = { a: 1, b: { c: 2 }, d: [3] }
  const before = describe(o)
  const got = copyProps(o, o, hasOwnProperty, true)
  if (got !== true || describe(o) !== before || o.b.c !== 2) { bad++; out.push('FAIL config is props: ' + got + ' ' + describe(o)) }
}
{
  // the objects that share a shape with the copy are not touched when one of them is frozen, sealed,
  // given another prototype or made not extensible, and each can still grow
  for (const how of ['freeze config', 'freeze props', 'seal props', 'prevent props', 'proto config', 'proto props', 'freeze sibling']) {
    const config = { a: 1, b: 2 }
    const props = {}
    const sibling = { a: 3, b: 4 }
    const again = { a: 5, b: 6 }
    if (copyProps(config, props, hasOwnProperty, false) !== true) { bad++; out.push('FAIL shared shapes: not copied'); continue }
    const all = { config, props, sibling, again }
    const target = how.endsWith('config') ? config : how.endsWith('props') ? props : sibling
    const before = Object.fromEntries(Object.entries(all).map(([k, o]) => [k, describe(o)]))
    if (how.startsWith('freeze')) Object.freeze(target)
    else if (how.startsWith('seal')) Object.seal(target)
    else if (how.startsWith('prevent')) Object.preventExtensions(target)
    else Object.setPrototypeOf(target, { inherited: 1 })
    const wrong = []
    for (const [k, o] of Object.entries(all)) {
      if (o === target) continue
      if (describe(o) !== before[k]) wrong.push(k + ' changed to ' + describe(o))
      o.grow = 1
      o.a = 9
      if (o.grow !== 1 || o.a !== 9 || Object.isFrozen(o) || Object.isSealed(o) || !Object.isExtensible(o) || Object.getPrototypeOf(o) !== Object.prototype) wrong.push(k + ' cannot grow')
    }
    if (wrong.length) { bad++; out.push('FAIL shared shapes, ' + how + ': ' + wrong.join('; ')) }
  }
}
{
  // the clone flag changes only what the loops differ in
  const create = {}, clone = {}
  copyProps({ ref: undefined, a: 1 }, create, hasOwnProperty, false)
  copyProps({ ref: undefined, a: 1 }, clone, hasOwnProperty, true)
  const flagged = Reflect.ownKeys(create).join() + '/' + Reflect.ownKeys(clone).join()
  if (flagged !== 'ref,a/a' && flagged !== 'a,ref/a') {
    // a clone flag that is no boolean true is false, so 1 copies ref too
    out.push('FAIL ref and the clone flag: ' + flagged)
    bad++
  }
}
{
  // what it answers for the same shape, many times, and what it leaves behind
  let wrong = 0
  for (let i = 0; i < 2000; i++) {
    const props = {}
    const config = { className: 'c' + (i % 3), id: i, key: 'k', children: [i] }
    if (copyProps(config, props, hasOwnProperty, false) !== true || Reflect.ownKeys(props).join() !== 'className,id,children' || props.id !== i) wrong++
    const adopted = {}
    const plain = { x: i, y: 'y' }
    if (copyProps(plain, adopted, hasOwnProperty, false) !== true || adopted.x !== i || adopted.y !== 'y') wrong++
    adopted.z = 1
    if (Reflect.ownKeys(adopted).join() !== 'x,y,z' || Reflect.ownKeys(plain).join() !== 'x,y') wrong++
    delete adopted.x
    if (Reflect.ownKeys(plain).join() !== 'x,y' || plain.x !== i) wrong++
  }
  if (wrong) { bad++; out.push('FAIL repeated copies: ' + wrong) }
}
{
  // a props object that took the shape of a config does not change it: add and delete on either
  const config = { a: 1, b: 2 }
  const props = {}
  copyProps(config, props, hasOwnProperty, false)
  props.c = 3
  config.d = 4
  delete props.a
  const other = {}
  copyProps({ a: 1, b: 2 }, other, hasOwnProperty, false)
  const r = [Reflect.ownKeys(config).join(), Reflect.ownKeys(props).join(), Reflect.ownKeys(other).join()].join('/')
  if (r !== 'a,b,d/b,c/a,b') { bad++; out.push('FAIL shared shape: ' + r) }
}
{
  // the function is a built-in
  const f = copyProps
  const shape = JSON.stringify([f.name, f.length, Object.hasOwn(f, 'prototype'), Object.getPrototypeOf(f) === Function.prototype,
    (() => { try { new f({}, {}, hasOwnProperty, false); return 'made' } catch (e) { return e.name } })(), String(f).includes('[native code]')])
  if (shape !== '["copyProps",4,false,true,"TypeError",true]') { bad++; out.push('FAIL shape of the function: ' + shape) }
}
out.push(bad ? bad + ' failed' : 'ok')
out.join('\n')
