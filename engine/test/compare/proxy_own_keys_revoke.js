// Copyright (c) 2026 The liustack/beastjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.
// The ownKeys trap of a proxy that revokes the proxy, while it runs or while
// its result is read, does not stop the call: the checks after the trap use
// the target the proxy had when the call began. The file is what Node 24
// prints.
const out = []
function run(name, f) {
  let r
  try { r = f() } catch (e) { r = 'throws ' + e.name }
  out.push(name + ': ' + (typeof r === 'string' ? r : JSON.stringify(r)))
}
function revocable(target, keys) {
  const p = Proxy.revocable(target, { ownKeys() { return keys(p.revoke) } })
  return p.proxy
}
const length = (revoke, list) => { const o = { get length() { revoke(); return list.length } }; list.forEach((k, i) => o[i] = k); return o }
const element = (revoke, list) => { const o = { length: list.length }; list.forEach((k, i) => Object.defineProperty(o, i, { get() { revoke(); return k } })); return o }
const nonConfigurable = () => Object.defineProperty({}, 'x', { value: 1, configurable: false })
run('length revokes, empty', () => Reflect.ownKeys(revocable({}, r => length(r, []))))
run('length revokes, keys', () => Reflect.ownKeys(revocable({}, r => length(r, ['a', 'b']))))
run('element revokes', () => Reflect.ownKeys(revocable({}, r => element(r, ['a', 'b']))))
run('trap revokes', () => Reflect.ownKeys(revocable({}, r => { r(); return ['a'] })))
run('non configurable target', () => Reflect.ownKeys(revocable(nonConfigurable(), r => length(r, ['x']))))
run('non configurable missing', () => Reflect.ownKeys(revocable(nonConfigurable(), r => length(r, []))))
run('non extensible target', () => Reflect.ownKeys(revocable(Object.preventExtensions({ a: 1 }), r => length(r, ['a']))))
run('non extensible extra key', () => Reflect.ownKeys(revocable(Object.preventExtensions({ a: 1 }), r => length(r, ['a', 'b']))))
run('getOwnPropertyNames', () => Object.getOwnPropertyNames(revocable({}, r => length(r, ['a']))))
run('getOwnPropertySymbols', () => Object.getOwnPropertySymbols(revocable({}, r => length(r, []))))
run('keys', () => Object.keys(revocable({}, r => length(r, []))))
run('keys with a key', () => Object.keys(revocable({ a: 1 }, r => length(r, ['a']))))
run('for in', () => { const k = []; for (const x in revocable({}, r => length(r, []))) k.push(x); return k })
run('revoked before', () => { const p = Proxy.revocable({}, {}); p.revoke(); return Reflect.ownKeys(p.proxy) })
out.join('\n')
