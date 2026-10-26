// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// The own keys of objects of many shapes through every path that lists
// them, against the order the script works out from the keys it added:
// array indexes by value, then strings, then symbols, each in the order
// they were added. Objects are made and dropped, so that the memory of a
// freed shape goes to new shapes with other keys, and the kinds of keys the
// key cache keeps must not outlive their shape. Errors with an enumerable
// 'stack' check that a pending stack is formatted before it is handed on.
const syms = [Symbol('s0'), Symbol.for('s1'), Symbol('s2')]
const isIndex = k => typeof k === 'string' && String(Number(k) >>> 0) === k && k !== '4294967295'
function order(keys) {
  const idx = keys.filter(isIndex).sort((a, b) => a - b)
  const str = keys.filter(k => typeof k === 'string' && !isIndex(k))
  return [...idx, ...str, ...keys.filter(k => typeof k === 'symbol')]
}
const out = []
let listings = 0, wrong = 0
function check(what, i, got, want) {
  listings++
  const g = got.map(String).join(), w = want.map(String).join()
  if (g !== w) {
    wrong++
    if (out.length < 20) out.push(what + ' ' + i + ': ' + g + ' want ' + w)
  }
}
for (let i = 0; i < 4000; i++) {
  const o = {}, keys = []
  for (let k = 0, n = 1 + (i * 7) % 11; k < n; k++) {
    let key
    switch ((i * 31 + k * 17) % 6) {
    case 0: key = 'k' + ((i + k) % 5); break
    case 1: key = String((i * 3 + k) % 7); break
    case 2: key = syms[(i + k) % 3]; break
    case 3: key = '4294967295'; break
    case 4: key = '0' + k; break
    default: key = 'x' + (i % 3) + k
    }
    if (!(key in o)) keys.push(key)
    o[key] = k
  }
  if (i % 5 === 0 && keys.length > 1) delete o[keys.splice(i % keys.length, 1)[0]]
  if (i % 11 === 0) {
    Object.defineProperty(o, 'g', { get() { return 7 }, enumerable: true, configurable: true })
    keys.push('g')
  }
  const all = i % 7 === 0 ? [...keys, 'hidden'] : keys
  if (i % 7 === 0) Object.defineProperty(o, 'hidden', { value: 1, configurable: true })
  const strs = order(keys).filter(k => typeof k === 'string')
  check('ownKeys', i, Reflect.ownKeys(o), order(all))
  check('names', i, Object.getOwnPropertyNames(o), order(all).filter(k => typeof k === 'string'))
  check('symbols', i, Object.getOwnPropertySymbols(o), order(all).filter(k => typeof k === 'symbol'))
  check('keys', i, Object.keys(o), strs)
  check('values', i, Object.values(o), strs.map(k => o[k]))
  check('entries', i, Object.entries(o).map(e => e[0] + '=' + e[1]), strs.map(k => k + '=' + o[k]))
  check('spread', i, Reflect.ownKeys({ ...o }), order(keys))
  check('spread values', i, Object.values({ ...o }), strs.map(k => o[k]))
  const fi = []
  for (const k in o) fi.push(k)
  check('for in', i, fi, strs)
  if (i % 3 === 0) {
    const { [keys[0]]: _, ...rest } = o
    check('rest', i, Reflect.ownKeys(rest), order(keys.slice(1)))
  }
  check('assign', i, Reflect.ownKeys(Object.assign({}, o)), order(keys))
  check('json', i, Object.keys(JSON.parse(JSON.stringify(o))), strs)
  if (i % 13 === 0) {
    const e = new Error('e' + i)
    e.a = i
    Object.defineProperty(e, 'stack', { enumerable: true })
    const kinds = [...Object.values(e), ...Object.values({ ...e })].map(v => typeof v).join()
    check('error', i, [kinds], ['string,number,string,number'])
  }
}
// One object listed until the key cache has its shape, then a key added,
// one deleted, one made not enumerable, a getter, an index added and one
// deleted, and other prototypes, with each listing printed after each step.
{
  const o = { b: 1, a: 2, 7: 3, [syms[0]]: 4, 1: 5 }
  const lists = (x) => {
    const fi = []
    for (const k in x) fi.push(k)
    return [Reflect.ownKeys(x), Object.keys(x), Object.getOwnPropertyNames(x), Object.getOwnPropertySymbols(x),
      fi, Object.values(x), Reflect.ownKeys({ ...x })].map((l) => l.map(String).join()).join(' / ')
  }
  const steps = [
    () => {},
    () => {},
    () => { o.c = 6 },
    () => { delete o.a },
    () => { Object.defineProperty(o, 'b', { enumerable: false }) },
    () => { Object.setPrototypeOf(o, { inherited: 1, [syms[2]]: 2 }) },
    () => { Object.defineProperty(o, 'c', { get() { return 8 }, enumerable: true, configurable: true }) },
    () => { o[3] = 9; delete o[7] },
    () => { Object.setPrototypeOf(o, null) },
    () => { o.b = 10; Object.defineProperty(o, 'b', { enumerable: true }) },
  ]
  for (const step of steps) {
    step()
    out.push(lists(o))
  }
}
out.push(listings + ' listings, ' + wrong + ' wrong')
out.join('\n')
