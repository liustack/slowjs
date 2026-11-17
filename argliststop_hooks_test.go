// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build testhooks

package slowjs // import "github.com/liustack/slowjs"

import "testing"

// argListStops are calls with 65,534 arguments, the most a call takes, and
// spreads of a long array into an array literal: the copy of the elements
// into the list of arguments (build_arg_list), into the array of a spread
// (js_append_array) and into arguments or a rest parameter in the register
// loop each ran 6 or 7 polls without a read, and a call went through two or
// three of them: up to 9 to 23 polls saw the request outside the freeing
// of the list. The spread of 2^21 elements into an array literal ran 198.
var argListStops = []stopCase{
	{"f(...a)", `var a = Array(65534).fill(1); function f() {}`, `f(...a)`},
	{"apply", `var a = Array(65534).fill(1); function f() {}`, `f.apply(null, a)`},
	{"Reflect.apply", `var a = Array(65534).fill(1); function f() {}`, `Reflect.apply(f, null, a)`},
	{"apply of an array-like", `var a = {length: 65534}; for (var i = 0; i < 65534; i++) a[i] = i; function f() {}`, `f.apply(null, a)`},
	{"apply of a sparse array", `var a = []; a[65533] = 1; function f() {}`, `f.apply(null, a)`},
	{"new F(...a)", `var a = Array(65534).fill(1); function F() {}`, `new F(...a)`},
	{"super(...r)", `var a = Array(65534).fill(1); class B { constructor() {} } ` +
		`class D extends B { constructor(...r) { super(...r) } }`, `new D(...a)`},
	{"arguments", `var a = Array(65534).fill(1); function f() { return arguments.length }`, `f(...a)`},
	{"mapped arguments", `var a = Array(65534).fill(1); function f(x) { var o = arguments; return o.length }`, `f(...a)`},
	{"rest parameter", `var a = Array(65534).fill(1); function f(...r) { return r.length }`, `f(...a)`},
	{"String.fromCharCode.apply", `var a = Array(65534).fill(65)`, `String.fromCharCode.apply(null, a)`},
	{"Math.max.apply", `var a = Array(65534).fill(1)`, `Math.max.apply(null, a)`},
	{"Math.max(...a)", `var a = Array(65534).fill(1)`, `Math.max(...a)`},
	{"push.apply", `var a = Array(65534).fill(1)`, `[].push.apply([], a)`},
	{"bound function", `var a = Array(32767).fill(1); function f() {} var g = f.bind(null, ...a)`, `g(...a)`},
	{"spread of objects", `var a = Array.from({length: 65534}, () => ({})); function f() {}`, `f(...a)`},
	{"array literal", `var a = Array(1 << 21).fill(1)`, `[...a].length`},
	{"array literal after an element", `var a = Array(1 << 21).fill(1)`, `[0, ...a].length`},
}

// TestArgListStops: a stop that comes anywhere in a call with a long list
// of arguments ends it within stepStopPolls polls that saw the request:
// each copy of the elements reads the flag every JS_COPY_POLL_KEYS
// (engine/quickjs.c). What a callee does with its arguments in one pass,
// as Math.max, push and a bound function do, reads none, and is at most
// 65,534 steps: up to 7 polls. The freeing of the list (free_arg_list,
// rv_free_extra and rv_free_entry_extra) is not counted with the polls of
// the call, as the freeing of an object (dstCleanup): it cannot stop, and
// polls as long as the list.
func TestArgListStops(t *testing.T) {
	testStops(t, argListStops, 16, stepStopPolls, "free_arg_list", "rv_free_extra", "rv_free_entry_extra")
}
