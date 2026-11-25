// Copyright (c) 2026 The liustack/beastjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.
const out = [], print = (...a) => out.push(a.join(" "))
// A regexp is not allowed after a complete template or a private name, so the
// "/" after one is a division (upstream QuickJS 673c622). The arrow function
// parameter list is scanned ahead to tell it from a parenthesized expression,
// which is where the wrong guess made a SyntaxError.
var f = (a = `8` / 2) => a;
print(f());
class Q { #x = 10; f() { var g = (a = this.#x / 2) => a; return g(); } }
print(new Q().f());
var h = (a = `${1 + 1}` / 2, b = /x/.test('x')) => [a, b];
print(JSON.stringify(h()));
var k = (a = (`a`) / 1) => String(a);
print(k());
out.join("\n")
