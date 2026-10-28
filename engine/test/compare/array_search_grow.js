// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// indexOf and includes on a dense array that fromIndex changes while it is
// converted: the search covers the length read before the conversion, never
// the elements added or the room left by elements taken away. The file is what
// Node 24 prints.
const out = []
const show = v => typeof v === 'string' ? JSON.stringify(v) : String(v)
function run(name, make, mutate, from, needle) {
  for (const method of ['indexOf', 'includes']) {
    const a = make()
    const f = { valueOf() { mutate(a); return from } }
    let r
    try { r = show(a[method](needle, f)) } catch (e) { r = e.name }
    out.push(name + ' ' + method + ': ' + r + ' ' + JSON.stringify(a))
  }
}
const push2 = a => { a.push(2) }
run('push, from 0, absent', () => [1], push2, 0, 2)
run('push, from 1, absent', () => [1], push2, 1, 2)
run('push, from 2, absent', () => [1], push2, 2, 2)
run('push, from -1, absent', () => [1], push2, -1, 2)
run('push, from 0, present', () => [1, 2], push2, 0, 2)
run('push many, from 0', () => [1, 1, 1], a => { for (let i = 0; i < 50; i++) a.push(7) }, 0, 7)
run('push many, from 2', () => [1, 1, 1], a => { for (let i = 0; i < 50; i++) a.push(7) }, 2, 7)
run('push, strings', () => ['a'], a => { a.push('b') }, 0, 'b')
run('push, undefined', () => [1], a => { a.push(undefined) }, 0, undefined)
run('push, NaN', () => [1], a => { a.push(NaN) }, 0, NaN)
run('pop, from 0', () => [1, 2, 3], a => { a.pop(); a.pop() }, 0, 3)
run('pop, from 0, hit first', () => [1, 2, 3], a => { a.pop(); a.pop() }, 0, 1)
run('shift, from 0', () => [1, 2, 3], a => { a.shift() }, 0, 3)
run('length 0, from 0', () => [1, 2, 3], a => { a.length = 0 }, 0, 1)
run('length grow, from 0', () => [1, 2, 3], a => { a.length = 5 }, 0, undefined)
run('replace, from 0', () => [1, 2, 3], a => { a[0] = 9; a[2] = 8 }, 0, 8)
run('push then pop, from 0', () => [1], a => { a.push(2); a.pop() }, 0, 2)
// no change: the same search as before
run('no change, present', () => [1, 2, 3], () => {}, 1, 3)
run('no change, absent', () => [1, 2, 3], () => {}, 1, 1)
out.join('\n')
