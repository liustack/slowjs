// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// Package scenarios holds the scripts that the commands gcpause and interrupt
// run: each stays inside one long operation of the engine.
package scenarios

// A Scenario is a script that never ends by itself.
type Scenario struct{ Name, JS string }

// All are scripts that never end by themselves. Every one keeps the engine
// inside one long operation for as long as the operation takes, and runs it
// again and again.
var All = []Scenario{
	{"loop", `let a = 0; for (let i = 0; ; i++) { a += i * i }`},
	{"fib", `function f(n) { return n < 2 ? n : f(n - 1) + f(n - 2) } for (;;) f(27)`},
	{"regexp_backtrack", `const s = 'x'.repeat(24); for (;;) { /(x+x+)+y/.test(s) }`},
	{"regexp_scan", `const s = 'a'.repeat(1 << 26); for (;;) { /a*b/.test(s) }`},
	{"regexp_replace", `'a'.replace(/a/g, 'b'); const s = 'ab'.repeat(1 << 22); for (;;) { s.replace(/a/g, 'cc') }`},
	{"json_parse", `const s = JSON.stringify(Array.from({ length: 300000 }, (_, i) => ({ id: i, name: 'item ' + i, tags: ['a', 'b'], v: i / 3 })));
		for (;;) { JSON.parse(s) }`},
	{"json_stringify", `const o = Array.from({ length: 300000 }, (_, i) => ({ id: i, name: 'item ' + i, tags: ['a', 'b'], v: i / 3 }));
		for (;;) { JSON.stringify(o) }`},
	{"string_search", `const s = 'ab'.repeat(1 << 26); for (;;) { s.indexOf('c'); s.lastIndexOf('c'); s.includes('ba' + 'c') }`},
	{"string_repeat_split", `for (;;) { 'ab'.repeat(1 << 21).split('b').length }`},
	{"array_join", `const a = Array.from({ length: 2000000 }, (_, i) => 'item' + i); for (;;) { a.join(',') }`},
	{"sort_typed", `const a = new Float64Array(1 << 22).map(() => Math.random()); for (;;) { a.slice().sort() }`},
	{"sort_default", `const a = Array.from({ length: 500000 }, () => Math.random()); for (;;) { a.slice().sort() }`},
	{"bigint_mul", `let x = 7n ** 300000n; for (;;) { x * x }`},
	{"array_fill", `for (;;) { new Array(1 << 22).fill(0).reverse().indexOf(1) }`},
	{"normalize", `const s = 'éå'.repeat(1 << 20); for (;;) { s.normalize('NFC') }`},
}
