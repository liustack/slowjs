// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// The keys of a module namespace are bindings of the module. Listing them
// with their values, or only the enumerable ones, throws for a binding not
// yet initialized, and the names alone do not. The module imports itself, so
// it can look at its own namespace before and after its bindings exist.
import * as me from "./own_keys_module.mjs"
const r = (f) => { try { return JSON.stringify(f()) } catch (e) { return e.name } }
const forIn = () => { const ks = []; for (const k in me) ks.push(k); return ks }
globalThis.early = [
	r(() => Object.getOwnPropertyNames(me)), r(() => Reflect.ownKeys(me).map(String)),
	r(() => Object.keys(me)), r(() => Object.values(me)), r(() => Object.entries(me)),
	r(forIn), r(() => ({ ...me })), r(() => Object.assign({}, me)),
].join(' ')
export let late = 2
export const b = 1
export function f() {}
globalThis.after = [
	r(() => Object.getOwnPropertyNames(me)), r(() => Object.keys(me)), r(() => Object.values(me).map(String)),
	r(() => Object.entries(me).map(String)), r(forIn), r(() => Object.keys({ ...me })), r(() => Object.keys(Object.assign({}, me))),
].join(' ')
globalThis.result = 'early: ' + early + '\nafter: ' + after
