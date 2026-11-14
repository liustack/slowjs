// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// Node 24 output: how a spread uses the iterator protocol, in an array
// literal, a call, a new, a super call and a method call. It never closes
// its iterator: when next throws, or its result is not an object, or
// reading its done or value throws, the error goes on and return is
// neither read nor called (ArrayAccumulation and ArgumentListEvaluation
// call no IteratorClose). It reads Symbol.iterator of its value once
// (GetIterator).
(() => {
  const out = []
  let log = []
  const iter = (how) => ({
    [Symbol.iterator]() {
      let i = 0
      return {
        next() {
          i++
          if (i < 3) return { value: i, done: false }
          if (how === 'throw') throw new RangeError('next')
          if (how === 'primitive') return 5
          if (how === 'done') return { get done() { throw new RangeError('done') } }
          if (how === 'value') return { done: false, get value() { throw new RangeError('value') } }
          return { done: true }
        },
        get return() {
          log.push('get return')
          return () => { log.push('return'); return {} }
        }
      }
    }
  })
  function f(...a) { return a.length }
  class A { constructor(...a) { this.n = a.length } }
  class B extends A { constructor(it) { super(0, ...it) } }
  const o = { m(...a) { return a.length } }
  const forms = {
    literal: (it) => [...it].length,
    middle: (it) => [0, ...it, 9].length,
    call: (it) => f(...it),
    new: (it) => new A(...it).n,
    super: (it) => new B(it).n,
    method: (it) => o.m(1, ...it)
  }
  for (const how of ['end', 'throw', 'primitive', 'done', 'value']) {
    for (const name of Object.keys(forms)) {
      log = []
      let r
      try { r = forms[name](iter(how)) } catch (e) { r = e.name + ' ' + (e instanceof RangeError ? e.message : '') }
      out.push(how + ' ' + name + ': ' + r + ' [' + log.join(',') + ']')
    }
  }
  // an array with a hole past its count, read through a getter of
  // Array.prototype that throws, with a return on the array iterator
  const AIP = Object.getPrototypeOf([][Symbol.iterator]())
  Object.defineProperty(AIP, 'return', {
    get() { log.push('get AIP return'); return () => { log.push('AIP return'); return {} } },
    configurable: true
  })
  Object.defineProperty(Array.prototype, 2, { get() { throw new RangeError('hole') }, configurable: true })
  try {
    for (const name of Object.keys(forms)) {
      log = []
      const a = [1, 2]
      a.length = 3
      let r
      try { r = forms[name](a) } catch (e) { r = e.name + ' ' + e.message }
      out.push('hole ' + name + ': ' + r + ' [' + log.join(',') + ']')
    }
  } finally {
    delete Array.prototype[2]
    delete AIP.return
  }
  // Symbol.iterator is read once: as a getter of the value, of
  // Array.prototype, and through a Proxy
  const own = () => ({
    get [Symbol.iterator]() { log.push('get own iterator'); return function* () { yield 'o' } }
  })
  const px = () => new Proxy([1, 2], {
    get(t, k) { log.push('get ' + String(k)); return t[k] }
  })
  const values = Array.prototype[Symbol.iterator]
  for (const [what, mk] of [['own', own], ['proxy', px], ['array', () => [1, 2]]]) {
    Object.defineProperty(Array.prototype, Symbol.iterator, {
      get() { log.push('get Array.prototype iterator'); return values },
      configurable: true
    })
    try {
      for (const name of Object.keys(forms)) {
        log = []
        let r
        try { r = forms[name](mk()) } catch (e) { r = e.name }
        out.push('read ' + what + ' ' + name + ': ' + r + ' [' + log.join(',') + ']')
      }
    } finally {
      Object.defineProperty(Array.prototype, Symbol.iterator, { value: values, writable: true, configurable: true })
    }
  }
  return out.join('\n')
})()
