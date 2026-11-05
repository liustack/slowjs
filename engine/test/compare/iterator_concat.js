// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// spec: Node 24 has no Iterator.concat. Expectations follow test262 at
// 91c70759363c46557b694792bf34a2551a7af5ba, test/built-ins/Iterator/concat/:
// iterator-has-no-return, return-method-returns-non-object,
// return-result-is-fresh-iterator-result, completed-after-abrupt-completion,
// underlying-iterator-not-closed-when-step-throws and the reentrancy tests.
(() => {
  const out = []
  const show = f => {
    try { const r = f(); return r === undefined ? 'undefined' : JSON.stringify(r) }
    catch (e) { return e.name }
  }
  for (const kind of ['absent', 'undefined', 'null', 'number', 'primitive', 'object', 'get-throws', 'call-throws', 'reenter']) {
    let gets = 0, calls = 0, opened = 0, innerResult = { done: false, value: 'inner' }, it
    const inner = { next() { return { value: 1, done: false } } }
    if (kind !== 'absent') Object.defineProperty(inner, 'return', { get() {
      gets++
      if (kind === 'get-throws') throw new RangeError()
      if (kind === 'undefined') return undefined
      if (kind === 'null') return null
      if (kind === 'number') return 1
      return function() {
        calls++
        if (this !== inner || arguments.length) throw new Error('receiver or arguments')
        if (kind === 'call-throws') throw new RangeError()
        if (kind === 'reenter') {
          out.push('reenter: ' + show(() => it.next()) + ' ' + show(() => it.return()))
        }
        return kind === 'primitive' ? 1 : innerResult
      }
    } })
    it = Iterator.concat({ [Symbol.iterator]() { return inner } }, { [Symbol.iterator]() { opened++; return [2].values() } })
    it.next()
    out.push(kind + ': ' + show(() => it.return('ignored')) + ' ' + show(() => it.next()) + ' ' + show(() => it.return()) + ' ' + [gets, calls, opened].join(','))
  }
  for (const stage of ['open', 'open-non-object', 'next-get', 'next-undefined', 'next-call', 'non-object', 'done', 'value']) {
    let opens = 0, steps = 0, closes = 0, later = 0
    const inner = {
      get next() {
        if (stage === 'next-get') throw new RangeError()
        if (stage === 'next-undefined') return undefined
        return () => {
          steps++
          if (stage === 'next-call') throw new RangeError()
          if (stage === 'non-object') return null
          return {
            get done() { if (stage === 'done') throw new RangeError(); return false },
            get value() { if (stage === 'value') throw new RangeError(); return 1 }
          }
        }
      },
      return() { closes++; return {} }
    }
    const it = Iterator.concat({ [Symbol.iterator]() { opens++; if (stage === 'open') throw new RangeError(); return stage === 'open-non-object' ? 1 : inner } }, { [Symbol.iterator]() { later++; return [2].values() } })
    out.push(stage + ': ' + show(() => it.next()) + ' ' + show(() => it.next()) + ' ' + show(() => it.return()) + ' ' + [opens, steps, closes, later].join(','))
  }
  const innerResult = { done: false, value: 'inner' }
  const fresh = Iterator.concat({ [Symbol.iterator]() { return { next() { return { value: 1, done: false } }, return() { return innerResult } } } })
  fresh.next()
  const first = fresh.return(), second = fresh.return()
  out.push('fresh: ' + [first !== innerResult, first !== second, first && first.done === true, first && Object.hasOwn(first, 'value') && first.value === undefined].join(','))
  let opens = 0
  const early = Iterator.concat({ [Symbol.iterator]() { opens++; return [1].values() } })
  out.push('early: ' + show(() => early.return()) + ' ' + show(() => early.next()) + ' ' + opens)
  out.push('sequence: ' + JSON.stringify([...Iterator.concat([], [1, 2], [], [3])]))
  const exhausted = Iterator.concat([1])
  exhausted.next(); exhausted.next()
  out.push('exhausted: ' + show(() => exhausted.return()) + ' ' + show(() => exhausted.return()))
  return out.join('\n')
})()
