// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// The truth of an object in the branches and the ! of the register loop: a
// local, an argument, and a temp that holds the last reference (a literal,
// the result of a call) or not (a property read, an element, a closure
// variable), of every kind of object: plain, array, function, class,
// boxed primitives, Proxy, Date, RegExp, Map and the like. An object is
// true even when it converts to false. Then the same tests made again and
// again must leave the heap as it was: a temp gives its reference back
// whether or not it was the last one. The file is what Node 24 prints.
const out = []
function show(name, f) {
  try { out.push(name + ' ' + f()) } catch (e) { out.push(name + ' threw ' + e) }
}
const kinds = {
  plain: () => ({}),
  array: () => [],
  fn: () => function () {},
  arrow: () => () => 0,
  klass: () => class {},
  boolFalse: () => new Boolean(false),
  numZero: () => new Number(0),
  strEmpty: () => new String(''),
  proxy: () => new Proxy({}, {}),
  revoked: () => { const r = Proxy.revocable({}, {}); r.revoke(); return r.proxy },
  date: () => new Date(0),
  regexp: () => /x/,
  map: () => new Map(),
  valueOfFalse: () => ({ valueOf() { return false }, toString() { return '' } }),
  nullProto: () => Object.create(null),
}
function local(v) {
  const r = []
  if (v) r.push('if'); else r.push('else')
  r.push(!v ? 'not' : 'yes')
  r.push(v ? 1 : 0)
  let n = 0
  while (v && n < 2) n++
  r.push(n)
  return r.join('/')
}
function temps(make) {
  const holder = { p: make() }, arr = [make()], cl = make()
  const r = []
  if (holder.p) r.push('prop')
  if (arr[0]) r.push('elem')
  if ((() => cl)()) r.push('closure')
  if (make()) r.push('call')
  r.push(!holder.p, !arr[0], !make(), holder.p ? 1 : 0, make() ? 1 : 0)
  let n = 0
  while (holder.p && n < 3) n++
  r.push(n)
  return r.join('/')
}
for (const [name, make] of Object.entries(kinds)) {
  show(name, () => local(make()) + ' ' + temps(make))
}
show('literals', () => [!{}, ![], !function () {}, !!{}, {} ? 1 : 0, [] ? 1 : 0].join())
show('dropped temp', () => {
  let r = 0
  for (let i = 0; i < 100; i++) {
    if ({ i }) r++
    if (!{ i }) r += 1000
    if ([i]) r++
  }
  return r
})

if (typeof memorySize === 'function') {
  const shared = { o: {}, a: [] }
  const work = () => {
    let n = 0
    for (let i = 0; i < 200; i++) {
      if ({ i }) n++
      if (!{ i }) n--
      if (shared.o) n++
      if (!shared.a) n--
      for (const make of Object.values(kinds)) if (make()) n++
    }
    return n
  }
  work(); work(); gc()
  work(); work(); work(); gc(); gc()
  const m1 = memorySize(), k1 = memoryBlocks()
  work(); work(); work(); gc(); gc()
  out.push('memory after: ' + heapChange(m1, k1))
} else {
  out.push('memory after: 0')
}
out.join('\n')
