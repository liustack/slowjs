// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// Closure variables written in the register loop (put_var_ref, set_var_ref
// and their short forms): the value from a temp, a local and a constant, of
// every kind, while the frame of the variable runs and after it returned,
// an old object freed by the write, a getter that writes the variable
// before the write of its value, and more than four closure variables.
function tryCall(f, a, b) {
  try { return f(a, b); } catch (e) { return e.constructor.name + ": " + e.message; }
}
(function () {
  "use strict";
  function cell(v0) {
    var v = v0;
    function setT(o) { v = o.x; }
    function setL(x) { v = x; }
    function setK() { v = null; }
    function setU() { v = undefined; }
    function setS(x) { return (v = x + 1); }
    function chain(x) { let w = 0; v = w = x; return w; }
    function get() { return v; }
    return { setT: setT, setL: setL, setK: setK, setU: setU, setS: setS, chain: chain, get: get };
  }
  // a write by a closure while the frame of the local runs: the frame reads
  // it after the call, and the read before the call keeps the old value
  function live(x) {
    var v = x;
    function bump(d) { v = d + v; return 1; }
    const before = v;
    const sum = v + bump(10) + v;
    bump(0.5);
    return before + ":" + sum + ":" + v;
  }
  // more than four closure variables: put_var_ref and set_var_ref with an
  // index
  function many() {
    var a = 1, b = 2, c = 3, d = 4, e = 5, f = 6;
    function w(x) { a = x; b = x; c = x; d = x; e = x; f = x + 1; return (f = f + 1); }
    function r() { return a + b + c + d + e + f; }
    return function (x) { const s = w(x); return s + "/" + r(); };
  }
  // a getter that writes the variable, then the put of what it returned
  function viaGetter() {
    var v = "start";
    const o = { get g() { v = "inner"; return "outer"; } };
    function put() { v = o.g; return v; }
    return put() + "," + v;
  }
  // an old value that is an object, freed by the write: its finalizer is
  // not seen, but its count must be right (a leak or a double free shows)
  function objects(n) {
    var held = { n: 0 };
    function put(k) { held = { n: k, prev: held.n }; }
    let i = 0;
    while (i < n) { put(i); i = 1 + i; }
    return held.n + "," + held.prev;
  }
  function counter() {
    var n = 0;
    return function () { n = 1 + n; return n; };
  }
  function floats() {
    var f = 0.5;
    function add(x) { f = x + f; return f; }
    return add(0.25) + add(1) + add(-2.5);
  }
  return function () {
    let out = "";
    let turn = 0;
    while (turn < 3) {
      const c = cell(turn);
      c.setT({ x: "t" + turn });
      out = out + c.get() + ",";
      c.setL(turn + 0.5);
      out = out + c.get() + ",";
      c.setK();
      out = out + c.get() + ",";
      c.setU();
      out = out + c.get() + ",";
      out = out + c.setS(turn) + "," + c.get() + ",";
      out = out + c.chain("ch") + "," + c.get() + ",";
      c.setT({ x: { deep: turn } });
      out = out + c.get().deep + ";";
      out = out + live(turn) + ";" + many()(turn) + ";" + viaGetter() + ";" + objects(50) + ";";
      const k = counter();
      k(); k();
      out = out + k() + ";" + floats() + ";" + tryCall(c.setT, null) + ";" + c.get().deep + "|";
      turn = 1 + turn;
    }
    return out;
  };
})()
