// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// Map.prototype.get answered in the loop (rv_fast_call in
// engine/regvm-ops-basic.h), against the same call made where the loop
// cannot answer it (Reflect.apply): each line is the answer from the loop,
// then "=" when the other way gave the same, or what it gave. An exception
// from the loop shows its stack, which the golden recorded from the C call
// holds the same. Values that are objects, strings and undefined, every kind
// of key, the wrong this, a WeakMap, a replaced or other realm's get, keys
// whose search gives up, and a recursion that ends at the call depth with a
// value that has a reference (the answer is freed, mem-end shows a leak).
(function () {
  var log = [];
  var mget = Map.prototype.get;
  var realmMap = newRealm("new Map([[1, 'one'], ['k', {}]])");

  function show(v) {
    if (typeof v === "number" && v === 0 && 1 / v < 0) return "-0";
    if (typeof v === "symbol" || typeof v === "bigint") return typeof v;
    if (typeof v === "object" && v !== null) return "obj:" + v.tag;
    if (typeof v === "function") return "fn";
    return String(v);
  }
  function err(e) { return "!" + e.constructor.name + ": " + e.message; }
  function other(f, t, a) {
    try { return show(Reflect.apply(f, t, a)); } catch (e) { return err(e); }
  }
  function cmp(r, f, t, a) {
    var o = other(f, t, a);
    return o === r ? r + " =" : r + " /= " + o;
  }
  function stack(e) { return err(e) + " @" + String(e.stack).trim().split("\n").join("|"); }

  function getM(m, k) { return m.get(k); }
  function getStmt(m, k) { m.get(k); return 1; }
  function getNone(m) { return m.get(); }
  function getExtra(m, k) { return m.get(k, 1, 2); }
  var sym = Symbol("k"), obj = { tag: "key" }, fn = function () {};
  var big = "y".repeat(70000);
  function rope() { return "abcdefghij".repeat(30) + "klmnopqrst".repeat(30); }
  var keys = [0, -0, 1, -1, 1.5, NaN, 2147483648, Infinity, "0", "1", "k", "abc", "ab" + "c", "", "é", big,
    "y".repeat(69999) + "y", rope(), sym, Symbol("k"), obj, {}, fn, 1n, 2n ** 70n, true, false, null, undefined];
  function fill(m) {
    m.set(0, { tag: "zero" }); m.set(1.5, "s1.5"); m.set(NaN, NaN); m.set("abc", { tag: "abc" });
    m.set("é", -0); m.set(big, "big"); m.set(sym, sym); m.set(obj, obj); m.set(1n, 1n); m.set(2n ** 70n, "b70");
    m.set(true, undefined); m.set(null, null); m.set(undefined, "u"); m.set(rope(), "rope"); m.set(fn, fn);
    return m;
  }
  class SubMap extends Map {}
  function getRow(m) {
    var row = [];
    for (var i = 0; i < keys.length; i++) {
      var r;
      try { r = show(getM(m, keys[i])); } catch (e) { r = stack(e); }
      row.push(cmp(r, mget, m, [keys[i]]));
      getStmt(m, keys[i]);
    }
    return row.join(", ");
  }
  function getBad() {
    var r = [];
    var odd = [["set", new Set([1])], ["weakmap", new WeakMap([[obj, 1]])], ["proxy", new Proxy(new Map([[1, 1]]), {})],
      ["prototype", Map.prototype], ["plain", {}], ["array", [1]], ["realm map", realmMap]];
    odd.forEach(function (o) {
      var v;
      try { o[1].g = mget; v = show(o[1].g(1)) + show(o[1].g("k")); delete o[1].g; } catch (e) { delete o[1].g; v = stack(e); }
      r.push(o[0] + ": " + cmp(v, mget, o[1], [1]));
    });
    var g = mget;
    try { r.push(show(g(1))); } catch (e) { r.push(stack(e)); }
    Number.prototype.g = mget;
    try { r.push(show((5).g(1))); } catch (e) { r.push(stack(e)); } finally { delete Number.prototype.g; }
    return r.join(" ; ");
  }
  function getLong() {
    var m = new Map(), r = [];
    for (var i = 0; i < 8; i++) m.set("z".repeat(30000) + i, i);
    r.push(getM(m, "z".repeat(30000) + "x"), getM(m, "z".repeat(30000) + "5"), getM(m, rope()), getNone(m),
      getNone(new Map([[undefined, "u"]])), getExtra(m, "z".repeat(30000) + "2"));
    var many = new Map();
    for (i = 0; i < 4096; i++) many.set("k" + i, i * 2);
    var s = 0;
    for (i = 0; i < 4200; i += 7) s += getM(many, "k" + i) | 0;
    r.push(s);
    return r.join(",");
  }
  function getOdd() {
    var saved = Map.prototype.get, m = fill(new Map()), r = [];
    Map.prototype.get = function (k) { return "replaced" + show(k); };
    try { r.push(getM(m, 1)); } finally { Map.prototype.get = saved; }
    Map.prototype.get = new Proxy(mget, { apply(t, th, a) { return "p" + show(Reflect.apply(t, th, a)); } });
    try { r.push(getM(m, 0)); } finally { Map.prototype.get = saved; }
    r.push(show(getM(realmMap, 1)), show(getM(realmMap, "k")), show(getM(realmMap, 2)));
    m.rg = Object.getPrototypeOf(realmMap).get;
    r.push(show(m.rg(0)), show(m.rg(99)));
    delete m.rg;
    r.push(show(getM(new SubMap([[1, "sub"]]), 1)));
    return r.join(",");
  }

  var m0 = fill(new Map()), sm = fill(new SubMap());
  for (var rep = 0; rep < 3; rep++) {
    log.push("map: " + getRow(m0), "submap: " + getRow(sm), "empty: " + getRow(new Map()), getBad(), getLong(),
      getOdd());
  }
  var depth = 0, held = new Map([[1, { tag: "held" }]]);
  try { (function rec() { depth++; held.get(1); rec(); })(); } catch (e) { log.push(err(e) + " at depth " + depth); }
  rvExpect(getM, getStmt, getNone, getExtra, getRow, getLong);
  return log.join("\n");
})()
