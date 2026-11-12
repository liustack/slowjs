// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// rvdiff: constants with a reference count (strings from atoms, the empty
// string, floats, BigInts and strings of the constant pool) as returns,
// stores to locals, call arguments, field values and compare operands.
(function () { "use strict";
function k1() { return "abc"; }
function k2() { var s = "x"; s = "y"; return s; }
function k3(o) { o.x = "str"; return o.x; }
function k4() { return 1.5 + 2.25; }
function k5() { return 12345678901234567890n; }
function k6(a) { return a === "abc" ? 1 : 2; }
function k7() { return ""; }
function k8(f) { return f("a", "b", ""); }
function k9(a) { var s = ""; if (a) s = "yes"; else s = "no"; return s; }
function k10(a) { return a + "-" + "z"; }
function k11() { return -0.5; }
function k12(o) { o.e = ""; o.f = 2.5; return o.e + o.f; }
function cat(a, b, c) { return a + b + c; }
var out = [];
var i;
for (i = 0; i < 3; i++) {
  out.push(k1(), k2(), k3({}), k4(), String(k5()), k6("abc"), k6("ab" + "c"), k6(1), k7() === "",
    k8(cat), k9(i), k10(i), k11(), k12({}));
}
rvExpect(cat, k1, k10, k11, k12, k2, k3, k4, k5, k6, k7, k8, k9);
return out.join(",");
})()
