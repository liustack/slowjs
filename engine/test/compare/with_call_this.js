// Copyright (c) 2026 The liustack/beastjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.
const out = [], print = (...a) => out.push(a.join(" "))
// A call of a name that a with object holds passes that object as this, in an
// optional call, a plain call and a tagged template alike. The compiler decides
// at parse time whether a with statement can hold the name, and it has to see
// the with statements around a direct eval, whose code finds them among its
// closure variables. Each line is what Node 24 prints, but those marked spec:.
var o = { f() { return this === o; } };
function t(name, f) {
  let r;
  try { r = JSON.stringify(f()); } catch (e) { r = e.name; }
  print(name, r);
}
t("eval optional", () => { with (o) { return eval("f?.()"); } });
t("eval plain", () => { with (o) { return eval("f()"); } });
t("eval tagged", () => { with (o) { return eval("f`x`"); } });
t("eval strict", () => { with (o) { return eval("'use strict'; [f?.(), f()]"); } });
t("eval function", () => { with (o) { return eval("(function () { return [f?.(), f()]; })()"); } });
t("eval arrow", () => { with (o) { return eval("(() => [f?.(), f()])()"); } });
t("eval in eval", () => { with (o) { return eval("eval('[f?.(), f()]')"); } });
t("function eval", () => { with (o) { return (function () { return eval("[f?.(), f()]"); })(); } });
t("eval class", () => { with (o) { return eval("(class { static m() { return [f?.(), f()]; } }).m()"); } });
t("eval no with", () => { var f = o.f; return eval("[f?.(), f()]"); });
t("strict function", () => { with (o) { return (function () { "use strict"; return [f?.(), f()]; })(); } });
// spec: the assignment finds the object once, before the right side runs,
// and sets the property again after the right side deleted it (SetMutableBinding
// of an object environment record). Node 24 sets a global variable instead.
t("with assign spec:", () => { var p = { v: 1 }; with (p) { v += (delete p.v, 1); } var r = [p.v, typeof globalThis.v]; delete globalThis.v; return r; });
t("eval assign spec:", () => { var p = { v: 1 }; with (p) { eval("v += (delete p.v, 1)"); } var r = [p.v, typeof globalThis.v]; delete globalThis.v; return r; });
t("eval increment", () => { var p = { v: 1 }; with (p) { eval("v++; v += 2; [v] = [v * 2]"); } return p.v; });
out.join("\n")
