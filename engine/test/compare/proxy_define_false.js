// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// A store to a proxy with no set trap whose defineProperty trap returns false:
// the [[Set]] of the proxy returns false, so a store in strict code throws a
// TypeError and one in sloppy code goes on, as for a read-only property.
// Reflect.set returns false. The file is what Node 24 prints.
const out = []
function run(name, f) {
  let r
  try { r = f() } catch (e) { r = 'throws ' + e.name }
  out.push(name + ': ' + String(r))
}
const refuse = () => new Proxy({}, { defineProperty() { return false } })
const refuseArr = () => new Proxy([], { defineProperty() { return false } })
function strictField(o) { 'use strict'; o.x = 1; return 'ok' }
function sloppyField(o) { o.x = 1; return 'ok' }
function strictElem(o, k) { 'use strict'; o[k] = 1; return 'ok' }
function sloppyElem(o, k) { o[k] = 1; return 'ok' }
function strictInherit(o) { 'use strict'; const c = Object.create(o); c.x = 1; return Object.hasOwn(c, 'x') }
for (let i = 0; i < 3; i++) {
  run('strict field ' + i, () => strictField(refuse()))
  run('sloppy field ' + i, () => sloppyField(refuse()))
  run('strict element ' + i, () => strictElem(refuseArr(), 0))
  run('sloppy element ' + i, () => sloppyElem(refuseArr(), 0))
  run('strict computed key ' + i, () => strictElem(refuse(), 'k'))
  run('strict ordinary ' + i, () => strictField({}))
}
run('strict store under a proxy prototype', () => strictInherit(refuse()))
run('Reflect.set', () => Reflect.set(refuse(), 'x', 1))
run('Reflect.set with receiver', () => Reflect.set({}, 'x', 1, refuse()))
run('strict compound', () => { 'use strict'; const p = refuse(); p.n += 1; return 'ok' })
run('target left alone', () => { const t = {}; const p = new Proxy(t, { defineProperty() { return false } }); sloppyField(p); return Object.hasOwn(t, 'x') })
run('strict array push', () => Array.prototype.push.call(refuseArr(), 1))
// The target already has the property: the store finds it writable on the
// target and redefines its value on the receiver, the proxy, whose trap
// refuses. The same holds in strict code as for a new property.
const refuseHas = () => new Proxy({ x: 0 }, { defineProperty() { return false } })
const refuseHasArr = () => new Proxy([0, 0], { defineProperty() { return false } })
for (let i = 0; i < 2; i++) {
  run('strict field, existing ' + i, () => strictField(refuseHas()))
  run('sloppy field, existing ' + i, () => sloppyField(refuseHas()))
  run('strict element, existing ' + i, () => strictElem(refuseHasArr(), 0))
  run('sloppy element, existing ' + i, () => sloppyElem(refuseHasArr(), 0))
}
run('Reflect.set, existing', () => Reflect.set(refuseHas(), 'x', 1))
run('Reflect.set with receiver, existing', () => Reflect.set({}, 'x', 1, refuseHas()))
run('Object.assign, existing', () => { Object.assign(refuseHas(), { x: 1 }); return 'ok' })
run('array fill, existing', () => { Array.prototype.fill.call(refuseHasArr(), 1); return 'ok' })
run('strict array push, length', () => Array.prototype.push.call(new Proxy([], { defineProperty(t, k, d) { return k !== 'length' && Reflect.defineProperty(t, k, d) } }), 1))
const home = { m() { 'use strict'; super.x = 1; return 'ok' }, s() { super.x = 1; return 'ok' } }
run('strict super store, existing', () => home.m.call(refuseHas()))
run('sloppy super store, existing', () => home.s.call(refuseHas()))
run('strict store in with, existing', () => { const p = refuseHas(); with (p) { return (function () { 'use strict'; x = 1; return 'ok' })() } })
out.join('\n')
