// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// JS_HasProperty walks the prototype chain: through `in`, Reflect.has, the
// fields of a property descriptor, and the holes of an array for every and
// concat. Code that runs during the walk (a Proxy trap, a getter, a toString)
// may cut the chain, delete what it looks for and drop the last references to
// what the walk has not reached yet, and the walk must still give the same
// answers and free each object once.
var out = []
function show(name, f) {
  try { out.push(name + ' ' + f()) } catch (e) { out.push(name + ' threw ' + e) }
}

// plain chains, a fast array, a typed array, a String object
var base = { a: 1 }
var mid = Object.create(base)
mid.b = 2
var top = Object.create(mid)
show('plain', function () { return ['a' in top, 'b' in top, 'c' in top, Reflect.has(top, 'a')].join() })
var arr = [10, 20]
var onArr = Object.create(arr)
show('array', function () { return [0 in onArr, 1 in onArr, 2 in onArr, 'length' in onArr, 'push' in onArr].join() })
var ta = new Uint8Array(2)
var onTa = Object.create(ta)
show('typed', function () { return [0 in onTa, 2 in onTa, '1.5' in onTa, '-0' in onTa, 'x' in onTa, 'subarray' in onTa].join() })
var onStr = Object.create(new String('ab'))
show('string', function () { return [0 in onStr, 1 in onStr, 2 in onStr, 'length' in onStr, 'slice' in onStr].join() })
// the global object keeps its variables as references
var declared = 1
var onGlobal = Object.create(globalThis)
show('global', function () { return ['declared' in onGlobal, 'Object' in onGlobal, 'missing' in onGlobal, 'show' in globalThis].join() })

// a Proxy in the chain whose has trap cuts the chain and drops its target and
// the Proxy itself
var cut = (function () {
  var target = { x: 1 }
  var child = Object.create(new Proxy(target, {
    has: function (t, k) {
      Object.setPrototypeOf(child, null)
      target = null
      return k in t
    }
  }))
  return child
})()
show('proxy has', function () { return ['x' in cut, 'x' in cut, 'y' in cut].join() })
var deep = (function () {
  var t = Object.create({ inherited: 1 })
  var p = new Proxy(t, {
    has: function (tt, k) {
      Object.setPrototypeOf(tt, null)
      Object.setPrototypeOf(mid2, null)
      return Reflect.has(tt, k)
    }
  })
  var mid2 = Object.create(p)
  return Object.create(mid2)
})()
show('proxy deep', function () { return ['inherited' in deep, 'inherited' in deep].join() })

// a property descriptor whose prototype has getters that delete the fields
// the walk looks for next and cut the chain
function descriptor() {
  var proto = { configurable: true, writable: true }
  Object.defineProperty(proto, 'enumerable', {
    get: function () {
      delete proto.configurable
      Object.setPrototypeOf(d, null)
      proto = null
      return true
    }
  })
  var d = Object.create(proto)
  d.value = 7
  return d
}
show('descriptor', function () {
  var o = {}
  Object.defineProperty(o, 'p', descriptor())
  var g = Object.getOwnPropertyDescriptor(o, 'p')
  return [g.value, g.enumerable, g.configurable, g.writable].join()
})
show('descriptor getter', function () {
  var proto = {}
  Object.defineProperty(proto, 'get', {
    get: function () {
      Object.setPrototypeOf(d, null)
      proto = null
      return function () { return 'got' }
    }
  })
  proto.set = undefined
  var d = Object.create(proto)
  var o = Object.defineProperties({}, { q: d })
  return [o.q, 'set' in d, 'get' in d].join()
})

// keys whose toString throws, or changes the object before the walk
show('key throws', function () {
  return { toString: function () { throw new Error('key') } } in top
})
show('key changes', function () {
  var o = Object.create({ k: 1 })
  var key = { toString: function () { Object.setPrototypeOf(o, null); return 'k' } }
  return [key in o, 'k' in o].join()
})
show('key deletes', function () {
  var proto = { k: 1 }
  var o = Object.create(proto)
  var key = { toString: function () { delete proto.k; proto = null; return 'k' } }
  return key in o
})

// the holes of an array, filled by a getter of its prototype that deletes
// itself and cuts the chain
show('every', function () {
  var a = [1, , 3, , 5]
  var proto = Object.create(Array.prototype)
  Object.defineProperty(proto, 1, {
    configurable: true,
    get: function () {
      delete proto[1]
      Object.setPrototypeOf(a, Array.prototype)
      proto = null
      return 2
    }
  })
  Object.setPrototypeOf(a, proto)
  var seen = []
  a.every(function (v, i) { seen.push(i + ':' + v); return true })
  return seen.join(' ')
})
show('concat', function () {
  var a = [1, , 3]
  var proto = Object.create(Array.prototype)
  Object.defineProperty(proto, 1, {
    configurable: true,
    get: function () {
      Object.setPrototypeOf(a, Array.prototype)
      proto = null
      return 'filled'
    }
  })
  Object.setPrototypeOf(a, proto)
  var c = [0].concat(a, [, 9])
  return c.length + ' ' + JSON.stringify(c) + ' ' + (2 in c) + (4 in c)
})
out.join('\n')
