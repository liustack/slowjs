// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// let and const before their initialization in the register loop
// (get_loc_check, put_loc_check, get_var_ref_check, set_loc_uninitialized):
// the same ReferenceError at the same point, with temps owned below the
// check and the value of a put freed, and the reads after the
// initialization. A derived class constructor (get_loc_checkthis,
// put_loc_check_init) runs as the old loop runs it.
function tryCall(f, a, b) {
  try { return f(a, b); } catch (e) { return e.constructor.name + ": " + e.message; }
}
(function () {
  "use strict";
  function readEarly(c) {
    if (c) return x;
    let x = c + 1;
    return x;
  }
  function readEarlyTemp(o) {
    if (o.early) return o.a + x;
    let x = 2;
    return o.a + x;
  }
  function writeEarly(o) {
    if (o.early) x = o.v;
    let x = 3;
    x = o.v;
    return x;
  }
  function constEarly(c) {
    if (c) return k + 1;
    const k = 10;
    return k;
  }
  function closureEarly(c) {
    function g() { return v; }
    const early = tryCall(g);
    let v = c;
    return early + "," + g();
  }
  function closureWriteEarly() {
    function g(x) { v = x; return v; }
    const early = tryCall(g, 1);
    let v = 0;
    g(5);
    return early + "," + v;
  }
  // a let written by a function made before its initialization
  // (put_var_ref_check): from a temp, a local and a constant, an object
  // value freed when the check throws
  function hoistedWrite(o) {
    const e1 = tryCall(wT, o);
    const e2 = tryCall(wL, 2);
    const e3 = tryCall(wK);
    let v = 1;
    function wT(q) { v = q.x; }
    function wL(x) { v = x; }
    function wK() { v = null; }
    wT(o);
    const a = v;
    wL(3);
    const b = v;
    wK();
    return e1 + "," + e2 + "," + e3 + "," + a.k + "," + b + "," + v;
  }
  // a let in a loop body: uninitialized again at each turn
  function loopLet(n) {
    let s = 0, i = 0;
    while (i < n) {
      let y;
      if (i === 1) y = 10;
      if (y === 10) s = y + s;
      else s = 1 + s;
      i = 1 + i;
    }
    return s;
  }
  class Base { constructor(v) { this.v = v; } }
  class Derived extends Base {
    constructor(v, skip) {
      if (skip) { return; }
      super(v);
      this.w = v + 1;
    }
  }
  function derived(v, skip) { return new Derived(v, skip); }
  return function () {
    let out = "";
    let turn = 0;
    while (turn < 3) {
      out = out + tryCall(readEarly, 1) + "," + readEarly(0) + ";";
      out = out + tryCall(readEarlyTemp, { early: true, a: { s: 1 } }) + "," + readEarlyTemp({ a: 5 }) + ";";
      out = out + tryCall(writeEarly, { early: true, v: { big: turn } }) + "," + writeEarly({ v: "w" }) + ";";
      out = out + tryCall(constEarly, 1) + "," + constEarly(0) + ";";
      out = out + hoistedWrite({ x: { k: turn } }) + ";";
      out = out + closureEarly(turn) + ";" + closureWriteEarly() + ";" + loopLet(3) + ";";
      const d = derived(turn, false);
      out = out + d.v + "," + d.w + "," + tryCall(derived, 1, true) + "|";
      turn = 1 + turn;
    }
    return out;
  };
})()
