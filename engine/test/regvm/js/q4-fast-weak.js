// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// WeakMap.prototype.has and get and WeakSet.prototype.has answered in the
// loop (rv_fast_call in engine/regvm-ops-basic.h), against the same call
// made where the loop cannot answer it (Reflect.apply): each entry is the
// answer from the loop, then "=" when the other way gave the same, or what
// it gave. Keys that are objects, functions, symbols (registered or not),
// primitives, keys whose target was collected, the wrong this (a Map, a
// Set, the other weak class, a proxy), and a recursion that ends at the
// call depth.
(function () {
  var log = [];
  var wmhas = WeakMap.prototype.has, wmget = WeakMap.prototype.get, wshas = WeakSet.prototype.has;

  function show(v) {
    if (typeof v === "symbol") return "symbol";
    if (typeof v === "object" && v !== null) return "obj:" + v.tag;
    return String(v);
  }
  function err(e) { return "!" + e.constructor.name + ": " + e.message; }
  function stack(e) { return err(e) + " @" + String(e.stack).trim().split("\n").join("|"); }
  function other(f, t, a) {
    try { return show(Reflect.apply(f, t, a)); } catch (e) { return err(e); }
  }

  function hasM(m, k) { return m.has(k); }
  function getM(m, k) { return m.get(k); }
  function none(m) { return m.has(); }

  var a = { tag: "a" }, b = { tag: "b" }, f = function () {}, s = Symbol("s"), reg = Symbol.for("reg");
  var wm = new WeakMap([[a, { tag: "va" }], [f, 1], [s, "sym"]]), ws = new WeakSet([a, f, s]);
  var keys = [a, b, f, s, reg, Symbol("s"), {}, 1, "a", null, undefined, true, 1n];
  function row() {
    var r = [];
    keys.forEach(function (k) {
      var x, y;
      try { x = show(hasM(wm, k)); } catch (e) { x = stack(e); }
      y = other(wmhas, wm, [k]);
      r.push(x === y ? x + "=" : x + "/" + y);
      try { x = show(getM(wm, k)); } catch (e) { x = stack(e); }
      y = other(wmget, wm, [k]);
      r.push(x === y ? x + "=" : x + "/" + y);
      try { x = show(hasM(ws, k)); } catch (e) { x = stack(e); }
      y = other(wshas, ws, [k]);
      r.push(x === y ? x + "=" : x + "/" + y);
    });
    return r.join(" ") + " " + none(wm) + none(ws);
  }
  function bad() {
    var r = [];
    [[new Map([[a, 1]]), wmhas], [new Set([a]), wshas], [ws, wmhas], [wm, wshas], [new Proxy(wm, {}), wmhas],
      [{}, wmget], [ws, wmget]].forEach(function (c) {
      var o = c[0];
      o.h = c[1];
      try { r.push(show(o.h(a))); } catch (e) { r.push(stack(e)); }
      delete o.h;
    });
    return r.join(" ; ");
  }
  // keys whose target is gone: the record stays until the collector drops
  // it, and is not found
  function gone() {
    var m = new WeakMap(), t = new WeakSet(), r = [], ks = [];
    for (var i = 0; i < 50; i++) { var o = { tag: i }; m.set(o, i); t.add(o); if (i % 5 === 0) ks.push(o); }
    gc();
    for (i = 0; i < ks.length; i++) r.push(hasM(m, ks[i]) ? getM(m, ks[i]) : "x", hasM(t, ks[i]) ? 1 : 0);
    r.push(hasM(m, {}), getM(m, {}));
    return r.join(",");
  }

  for (var rep = 0; rep < 3; rep++) log.push(row(), bad(), gone());
  var depth = 0;
  try { (function rec() { depth++; wm.has(a); wm.get(a); ws.has(f); rec(); })(); } catch (e) { log.push(err(e) + " at depth " + depth); }
  rvExpect(hasM, getM, none, row, gone);
  return log.join("\n");
})()
