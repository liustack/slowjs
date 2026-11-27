// Copyright (c) 2026 The liustack/beastjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.
const out = [], print = (...a) => out.push(a.join(" "))
// A call of a name inside a with statement passes the with object as this only
// when the name is found in that object. A name a direct eval declared with var
// lives in the variables of the function around the eval: the call passes
// undefined, as for any other variable. Each line is what Node 24 prints.
function t(name, f) {
  let r;
  try { r = JSON.stringify(f()); } catch (e) { r = e.name; }
  print(name, r);
}
var me = "function () { 'use strict'; return this === undefined ? 'undefined' : this === o ? 'o' : typeof this; }";
var o = { k: 1 };
t("var of eval", () => { with (o) { eval("var f = " + me); return f(); } });
t("var of eval optional", () => { with (o) { eval("var f = " + me); return f?.(); } });
t("var of eval tagged", () => { with (o) { eval("var f = " + me); return f`x`; } });
t("var of eval in function", () => (function () { with (o) { eval("var f = " + me); return f(); } })());
t("call in eval", () => { with (o) { eval("var f = " + me); return eval("f()"); } });
t("function in with", () => { with ({}) { return (function () { eval("var f = " + me); return eval("f()"); })(); } });
t("function in with direct", () => { with ({}) { return (function () { eval("var f = " + me); return f(); })(); } });
t("parameter", () => { with (o) { return (function (a = eval("var g = " + me), b = g()) { return b; })(); } });
t("with object", () => { var p = { f: eval("(" + me.replace("=== o", "=== p") + ")") }; with (p) { return f(); } });
out.join("\n")
