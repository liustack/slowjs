// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// rvdiff: return f(...) (tail_call, tail_call_method) with values left under
// the call on the stack, as a switch leaves its value there while its cases
// run: a string switch on an argument (the shape of React's insertion mode
// function), a switch on a computed value, nested switches (two values
// under the call), a method call, a callee that is a built-in or not
// converted, and a callee that throws with the values still under it. Each
// sN runs several times, so that its later calls run in the register loop.
(function () { "use strict";
var out = [];
function p(x) { out.push(String(x)); }
function J(a, b, c, d) { return { m: a, v: b, s: c, t: d }; }
function g(x) { return x + 1; }
function boom(x) { if (x > 1) throw new TypeError("boom " + x); return x; }
var o = { k: "b", n: 2, f: function (x) { return this.n + x; } };
var mx = Math.max;

function s1(n, e, t) {
  var l = n.s & -25;
  switch (e) {
  case "noscript": return J(2, null, l | 1, null);
  case "select": return J(2, t.value != null ? t.value : t.dv, l, null);
  case "svg": return J(4, null, l, null);
  case "thead": case "tbody": return J(7, null, l, null);
  case "head": if (2 > n.m) return J(3, null, l, null); break;
  case "html": if (n.m === 0) return J(1, null, l, null);
  }
  return 6 <= n.m || 2 > n.m ? J(2, null, l, null) : n.v !== null || n.s !== l ? J(n.m, n.v, l, null) : n;
}
function s2(x) {
  switch (x.k) {
  case "a": return g(1);
  case "b": return g(x.n);
  default: return g(0);
  }
}
function s3(a, b) {
  switch (a) {
  case 1:
    switch (b) {
    case 2: return g(a + b);
    default: return g(b);
    }
  default: return g(a);
  }
}
function s4(k, x) { switch (k) { case 1: return o.f(x); case 2: return mx(x, 7); } return 0; }
function s5(k, x) { switch (k + "") { case "1": return boom(x); } return -1; }

var i, k, r, n = { s: 31, m: 3, v: null };
var names = ["noscript", "select", "svg", "thead", "tbody", "head", "html", "div"];
for (k = 0; k < 3; k++) {
  for (i = 0; i < names.length; i++) {
    r = s1(n, names[i], { value: k, dv: 9 });
    p("s1 " + names[i] + ": " + (r === n ? "n" : JSON.stringify(r)));
  }
  p("s1 head small: " + JSON.stringify(s1({ s: 8, m: 1, v: 0 }, "head", {})));
  p("s2: " + s2({ k: "a" }) + " " + s2(o) + " " + s2({ k: "z" }));
  p("s3: " + s3(1, 2) + " " + s3(1, 5) + " " + s3(4, 2));
  p("s4: " + s4(1, k) + " " + s4(2, k) + " " + s4(3, k));
  for (i = 0; i < 3; i++) {
    try { p("s5: " + s5(1, i)); } catch (e) { p("s5 threw " + e.name + ": " + e.message); }
  }
}
rvExpect(s1, s2, s3, s4, s5);
return out.join("\n");
})()
