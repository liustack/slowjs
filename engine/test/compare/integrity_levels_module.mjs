// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// A module namespace is not extensible and its bindings are writable and not
// configurable: once they are initialized it is sealed and not frozen,
// sealing it works and freezing it throws. Each of these reads the bindings,
// and so does defining one, which throws a ReferenceError while the binding
// is not initialized. The module imports itself, so it can look at its own
// namespace before and after its bindings exist.
import * as me from "./integrity_levels_module.mjs"
const r = (f) => { try { return String(f()) } catch (e) { return e.name } }
const all = () => [
	r(() => Object.isExtensible(me)), r(() => Object.isSealed(me)), r(() => Object.isFrozen(me)),
	r(() => Object.seal(me) === me), r(() => Object.preventExtensions(me) === me), r(() => Object.freeze(me) === me),
	r(() => Object.isSealed(me)), r(() => Object.isFrozen(me)),
	r(() => Reflect.defineProperty(me, 'late', {})), r(() => Object.defineProperty(me, 'late', { configurable: true }) === me),
	r(() => Reflect.defineProperty(me, 'b', { value: 1 })), r(() => Reflect.defineProperty(me, 'nope', {})),
].join(' ')
globalThis.early = all()
export let late = 2
export const b = 1
export function f() {}
globalThis.after = all()
globalThis.result = 'early: ' + early + '\nafter: ' + after
