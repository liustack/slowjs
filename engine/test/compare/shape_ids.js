// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// Objects that get the same keys one at a time share their shapes, and the
// engine finds the next shape of an object by the shape it grew from. Every
// change to a shape in place (a delete, new flags, a getter, a new prototype,
// an array that is no longer dense, a global function declared again) must
// leave the objects that grow after it with their own keys, in their order,
// with their own flags. Each object below lists its keys and their flags.
const out = []
const desc = (o) => Reflect.ownKeys(o).map((k) => {
  const d = Object.getOwnPropertyDescriptor(o, k)
  const f = (d.writable ? 'w' : '') + (d.enumerable ? 'e' : '') + (d.configurable ? 'c' : '') +
    (d.get || d.set ? 'a' : '')
  return String(k) + ':' + f
}).join(' ')
const show = (name, o) => out.push(name + ' ' + desc(o) + ' ' + JSON.stringify(o))

function build (n, pre) {
  const o = {}
  for (let i = 0; i < n; i++) o[pre + i] = i
  return o
}

// a staircase: every shape from 1 to 8 keys is held by an object
const keep = []
for (let n = 1; n <= 8; n++) keep.push(build(n, 'k'))
show('stair', build(8, 'k'))

// a delete, then the same keys again
const del = build(4, 'k')
delete del.k1
del.k1 = 'again'
del.k4 = 4
show('delete', del)
show('after delete', build(5, 'k'))

// the delete of the last key, on an object with a shape of its own
const last = build(3, 'q')
delete last.q2
last.q2 = 2
last.q3 = 3
show('delete last', last)
show('after delete last', build(4, 'q'))

// flags changed in place
const flags = build(3, 'k')
Object.defineProperty(flags, 'k1', { enumerable: false })
flags.k3 = 3
show('flags', flags)
const ro = build(3, 'k')
Object.freeze(ro)
show('frozen', ro)
const sealed = build(4, 'k')
Object.seal(sealed)
show('sealed', sealed)
show('after flags', build(4, 'k'))

// a data key becomes a getter, and back
const acc = build(3, 'k')
Object.defineProperty(acc, 'k0', { get () { return 'got' }, configurable: true })
acc.k3 = 3
show('getter', acc)
Object.defineProperty(acc, 'k0', { value: 'v', writable: true })
acc.k4 = 4
show('getter back', acc)
show('after getter', build(5, 'k'))

// a new prototype
const proto = build(3, 'k')
Object.setPrototypeOf(proto, { inherited: 1 })
proto.k3 = 3
show('prototype', proto)
out.push('prototype inherited ' + proto.inherited + ' ' + ('inherited' in build(4, 'k')))
const nul = build(2, 'k')
Object.setPrototypeOf(nul, null)
nul.k2 = 2
show('null prototype', nul)
show('after prototype', build(4, 'k'))

// arrays that stop being dense take their elements as keys
const arr = [10, 20, 30]
arr.x = 1
arr[10] = 40
delete arr[0]
arr.y = 2
show('array', arr)
const arr2 = [10, 20, 30]
arr2.x = 1
arr2.y = 2
show('dense array', arr2)
const args = (function () { return arguments })(1, 2, 3)
args.x = 1
delete args[1]
args.y = 2
show('arguments', args)

// objects that share a shape, each growing on
const a = build(3, 's')
const b = build(3, 's')
a.s3 = 'a'
b.s3 = 'b'
b.s4 = 'b'
a.t = 't'
b.t = 't'
show('shared a', a)
show('shared b', b)
const c = build(3, 's')
c.s3 = 'c'
c.s4 = 'c'
c.t = 'c'
show('shared c', c)

// a global function declared again over a configurable property
Object.defineProperty(globalThis, 'gf', { get () { return 1 }, configurable: true })
;(0, eval)('function gf () { return 2 }')
;(0, eval)('function gh () { return 3 }')
;(0, eval)('function gh () { return 4 }')
globalThis.gz = 5
const gd = (k) => {
  const d = Object.getOwnPropertyDescriptor(globalThis, k)
  return k + ':' + (d.writable ? 'w' : '') + (d.enumerable ? 'e' : '') + (d.configurable ? 'c' : '') +
    (d.get ? 'a' : '')
}
out.push('global ' + typeof gf + ' ' + gf() + ' ' + gh() + ' ' + gd('gf') + ' ' + gd('gh') + ' ' + gd('gz'))
show('after global', build(6, 'k'))

// keys that are symbols and numbers, in their order
const sym = Symbol('s')
const mixed = build(2, 'k')
mixed[sym] = 's'
mixed[5] = 5
mixed.z = 'z'
delete mixed.k0
mixed.k0 = 0
show('mixed', mixed)
const mixed2 = build(2, 'k')
mixed2[sym] = 't'
mixed2.z = 'z'
show('mixed again', mixed2)

out.join('\n')
