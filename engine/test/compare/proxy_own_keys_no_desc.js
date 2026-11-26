// Copyright (c) 2026 The liustack/beastjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.
// The ownKeys of a proxy checks the trap result against the keys of the
// target. A key whose descriptor the target gives as undefined counts with
// the keys that are configurable ([[OwnPropertyKeys]] of a Proxy, step 16):
// a target that is not extensible needs it in the result, and an extensible
// one does not. The target here is a proxy whose getOwnPropertyDescriptor
// deletes the key and returns undefined. The file is what Node 24 prints.
const out = []
function run(name, f) {
  let r
  try { r = f() } catch (e) { r = 'throws ' + e.name }
  out.push(name + ': ' + (typeof r === 'string' ? r : JSON.stringify(r)))
}
function inner(target) {
  return new Proxy(target, { getOwnPropertyDescriptor(t, k) { delete t[k]; return undefined } })
}
for (const [tname, make] of [
  ['not extensible', () => Object.preventExtensions({ x: 1 })],
  ['not extensible, two keys', () => Object.preventExtensions({ x: 1, y: 2 })],
  ['extensible', () => ({ x: 1 })],
]) {
  for (const result of [['x'], [], ['x', 'y'], ['y'], ['z']]) {
    run(tname + ', ' + JSON.stringify(result), () => Reflect.ownKeys(new Proxy(inner(make()), { ownKeys() { return result } })))
  }
}
run('revoked by the trap', () => {
  let revoke
  const r = Proxy.revocable(inner(Object.preventExtensions({ x: 1 })), { ownKeys() { revoke(); return ['x'] } })
  revoke = r.revoke
  return Reflect.ownKeys(r.proxy)
})
out.join('\n')
