// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// Closures made in the register loop (fclosure) and the frames whose locals
// they capture: a closure that outlives its frame, one that sees later
// writes of the local, one made and called in the same frame, captured
// arguments and missing ones, two closures of one local, a frame left by an
// exception with captured locals and temps owned below the throw, and a
// recursion past the register stack, where calls go back to the old loop.
function tryCall(f, a, b) {
  try { return f(a, b); } catch (e) { return e.constructor.name + ": " + e.message; }
}
(function () {
  "use strict";
  function counter(start) {
    let n = start;
    function get() { return n; }
    n = 1 + n;
    return get;
  }
  function pair(a, b) {
    function f() { return a + b; }
    a = 10 + a;
    return f;
  }
  function missing(a, b) {
    function f() { return b; }
    return f;
  }
  function sameFrame(x) {
    let y = x + 1;
    function f() { return y + x; }
    y = 1 + y;
    return f() + f();
  }
  function shared(x) {
    let v = x;
    function a() { return v; }
    function b() { return v + 100; }
    v = 1 + v;
    return function () { return a() + b(); };
  }
  function nested(x) {
    let v = x;
    function mk() { return function () { return v; } };
    v = 5 + v;
    return mk;
  }
  // the local read on the stack before a call that a closure of it makes:
  // the read is the value before the call
  function readBefore(o) {
    let v = o.s;
    function f() { return v; }
    return v + f();
  }
  function strs(s, q) {
    let t = s + q;
    function f() { return t + s; }
    t = q + t;
    return f;
  }
  function leaves(o, holder) {
    let v = o.a;
    function g() { return v; }
    holder.g = g;
    v = 1 + v;
    return o.a + o.b.c;
  }
  function rec(n) {
    let v = n;
    function f() { return v; }
    if (n < 1)
      return f;
    const g = rec(n - 1);
    return function () { return f() + g(); };
  }
  function deep(n) {
    let v = n;
    function f() { return v; }
    if (n < 1)
      return f();
    return deep(n - 1) + f() - n;
  }
  function churn(n) {
    let s = 0, i = 0;
    while (i < n) { s = i + s; i = 1 + i; }
    return s;
  }
  // returns of a constant and of nothing from a frame with captured locals
  function retK(h) {
    var v = 3;
    function g() { return v; }
    h.g = g;
    v = 4;
    return 7;
  }
  function retU(h) {
    var v = 5;
    function g() { return v; }
    h.g = g;
    v = 6;
  }
  function tail(x) {
    let v = x;
    function f() { return v + 1; }
    return f();
  }
  return function () {
    let out = "";
    let turn = 0;
    while (turn < 3) {
      const c = counter(turn);
      const p = pair(1, 2);
      const m = missing(1);
      const sh = shared(turn);
      const nf = nested(turn)();
      const s = strs("a", "!");
      const holder = {};
      const r = rec(20);
      out = out + c() + "," + p() + "," + m() + "," + sameFrame(turn) + "," + sh() + "," +
        nf() + "," + readBefore({ s: "x" }) + "," + s() + "," + tail(turn) + ";";
      out = out + tryCall(leaves, { a: 7, b: undefined }, holder) + "," + churn(50) + "," + holder.g() + ";";
      out = out + tryCall(leaves, { a: "s", b: { c: "t" } }, holder) + "," + holder.g() + ";";
      const h2 = {};
      out = out + retK(h2) + "," + h2.g() + ",";
      out = out + retU(h2) + "," + h2.g() + ";";
      out = out + r() + "," + deep(3000) + "|";
      turn = 1 + turn;
    }
    return out;
  };
})()
