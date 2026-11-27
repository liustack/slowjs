// Copyright (c) 2026 The liustack/beastjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.
// The lines and columns of the code of a module (the pc2line table its
// compile makes) in the stacks of errors: functions, classes and arrows
// nested in it, a template and a comment across lines, and characters of
// several bytes before the places.
const got = []
const here = () => { got.push(String(new Error().stack).split("\n").slice(1).join(" / ").replace(/ +/g, " ")); return 1 }
export function outer(a) { /* é
 中 😀 */ const s = "é中😀"; here()
	function inner() {
		return here() + `
${here()}` .length
	}
	class C { static m() { return [1, 2].map((x) =>
		here() + x) } }
	return inner() + C.m().length + a
}
const v = outer(1); here()
export default class { static n() { return (() => (() => here())())() } }
import D from "./line_col_module.mjs"
D.n()
try { (function () { null.x })() } catch (e) { got.push(String(e.stack).split("\n").join(" / ").replace(/ +/g, " ")) }
globalThis.result = got.join("\n")
