// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// Math.clz32, Map.prototype.has and Set.prototype.has answered in the loop
// (rv_fast_call in engine/regvm-ops-basic.h), against the same call made
// where the loop cannot answer it (Reflect.apply), over every kind of this
// and argument: each line is the answer from the loop, then "=" when the
// other way gave the same, or what it gave. An exception from the loop shows
// its stack, which the golden recorded from the C call holds the same.
// Also: a replaced, bound, proxied or other realm's function, a Map
// subclass, keys that are -0, NaN, ropes, long strings, symbols and objects,
// keys of a record that are ropes, chains of long keys of one length, and a
// recursion that ends where the call depth ends.
(function () {
  var log = [];
  var clz = Math.clz32, mhas = Map.prototype.has, shas = Set.prototype.has;
  var realmClz = newRealm("Math.clz32"), realmMap = newRealm("new Map([[1, 'one'], ['k', 2]])");
  var convLog = [];
  var conv = { valueOf() { convLog.push("v"); return 7; } };

  function show(v) {
    if (typeof v === "number" && v === 0 && 1 / v < 0) return "-0";
    if (typeof v === "symbol" || typeof v === "bigint") return typeof v;
    if (typeof v === "object" && v !== null) return "obj";
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
  function stack(e) {
    return err(e) + " @" + String(e.stack).trim().split("\n").join("|");
  }

  // Math.clz32 from the loop: a method of Math, a plain call, no argument,
  // extra arguments
  function clzM(x) { return Math.clz32(x); }
  function clzF(x) { return clz(x); }
  function clzNone() { return Math.clz32(); }
  function clzExtra(x) { return Math.clz32(x, 1, 2); }
  function clzStmt(x) { Math.clz32(x); return 1; }
  var nums = [0, 1, -1, 2, 3, 255, 256, 0x7fffffff, -0x80000000, 2147483648, 4294967295, 4294967296,
    4294967297, -4294967297, 1.5, 0.5, -0.5, -0, NaN, Infinity, -Infinity, 1e300, -1e300, 2 ** 53, 2 ** 64 + 2048,
    5e-324, "8", "", "x", null, undefined, true, false, conv, [], [3], {}];
  function clzRow(f) {
    var row = [];
    for (var i = 0; i < nums.length; i++) {
      var r;
      try { r = show(f(nums[i])); } catch (e) { r = stack(e); }
      row.push(cmp(r, clz, Math, [nums[i]]));
    }
    return row.join(", ");
  }
  function clzBad() {
    var row = [];
    [Symbol("s"), 1n, { valueOf() { throw new RangeError("r"); } }, Object.create(null)].forEach(function (x) {
      try { row.push(show(clzM(x))); } catch (e) { row.push(stack(e)); }
      row.push(other(clz, Math, [x]));
    });
    return row.join(" ; ");
  }
  function clzOdd() {
    var saved = Math.clz32, r = [];
    Math.clz32 = function (x) { return "replaced" + x; };
    try { r.push(clzM(1)); } finally { Math.clz32 = saved; }
    Math.clz32 = new Proxy(clz, { apply(t, th, a) { return "p" + Reflect.apply(t, th, a); } });
    try { r.push(clzM(1)); } finally { Math.clz32 = saved; }
    Math.clz32 = clz.bind(null, 5);
    try { r.push(clzM(1)); } finally { Math.clz32 = saved; }
    Math.clz32 = realmClz;
    try { r.push(clzM(1), clzM(-1), clzM(2.5), clzM(conv)); } finally { Math.clz32 = saved; }
    r.push(clzM(1), clzNone(), clzExtra(16), clzStmt(3));
    return r.join(",");
  }

  // Map.prototype.has and Set.prototype.has from the loop
  function hasM(m, k) { return m.has(k); }
  function hasStmt(m, k) { m.has(k); return 1; }
  function hasNone(m) { return m.has(); }
  function hasExtra(m, k) { return m.has(k, 1, 2); }
  var sym = Symbol("k"), obj = {}, fn = function () {};
  var big = "y".repeat(70000), big2 = "y".repeat(69999) + "y";
  var ropeA = "abcdefghij".repeat(30) + "klmnopqrst".repeat(30);
  function rope() { return "abcdefghij".repeat(30) + "klmnopqrst".repeat(30); }
  var keys = [0, -0, 1, -1, 1.0, 1.5, NaN, 0x7fffffff, 2147483648, Infinity, "0", "1", "k", "abc", "ab" + "c",
    "", "é", "€", big, big2, ropeA, rope(), sym, Symbol("k"), obj, {}, fn, 1n, 2n ** 70n, true, false, null,
    undefined];
  function fill(m) {
    m.add ? (m.add(0), m.add(1.5), m.add(NaN), m.add("abc"), m.add("é"), m.add(big), m.add(sym), m.add(obj),
      m.add(1n), m.add(2n ** 70n), m.add(true), m.add(null), m.add(undefined), m.add(rope()), m.add(fn))
      : (m.set(0, 1), m.set(1.5, 2), m.set(NaN, 3), m.set("abc", 4), m.set("é", 5), m.set(big, 6), m.set(sym, 7),
        m.set(obj, 8), m.set(1n, 9), m.set(2n ** 70n, 10), m.set(true, 11), m.set(null, 12),
        m.set(undefined, 13), m.set(rope(), 14), m.set(fn, 15));
    return m;
  }
  class SubMap extends Map {}
  class SubSet extends Set { has(k) { return "sub" + super.has(k); } }
  var holders = [
    ["map", fill(new Map()), mhas], ["set", fill(new Set()), shas], ["empty map", new Map(), mhas],
    ["submap", fill(new SubMap()), mhas], ["subset", fill(new SubSet()), shas],
  ];
  function hasRow(m, f) {
    var row = [];
    for (var i = 0; i < keys.length; i++) {
      var r;
      try { r = show(hasM(m, keys[i])); } catch (e) { r = stack(e); }
      row.push(cmp(r, f, m, [keys[i]]));
      hasStmt(m, keys[i]);
    }
    return row.join(", ");
  }
  function hasBad() {
    var r = [];
    var odd = [["map has on a set", fill(new Set()), mhas], ["set has on a map", fill(new Map()), shas],
      ["weakmap", new WeakMap([[obj, 1]]), mhas], ["proxy", new Proxy(new Map([[1, 1]]), {}), mhas],
      ["prototype", Map.prototype, mhas], ["plain", {}, mhas], ["array", [1], shas], ["string", "abc", mhas],
      ["number", 5, shas], ["null", null, mhas], ["undefined", undefined, shas], ["realm map", realmMap, mhas]];
    odd.forEach(function (o) {
      var v;
      try {
        if (o[1] === null || o[1] === undefined) v = show(o[2](1));
        else if (typeof o[1] === "object") { o[1].h = o[2]; v = show(o[1].h(1)) + show(o[1].h("k")); delete o[1].h; }
        else { var p = Object.getPrototypeOf(o[1]); p.h = o[2]; try { v = show(o[1].h(1)); } finally { delete p.h; } }
      } catch (e) { v = stack(e); }
      r.push(o[0] + ": " + cmp(v, o[2], o[1], [1]));
    });
    return r.join(" ; ");
  }
  // a rope as key, and records whose keys are ropes or long strings of one
  // length, so that the search gives up and the call is made
  function hasLong() {
    var m = new Map(), s = new Set(), r = [];
    for (var i = 0; i < 8; i++) {
      m.set("z".repeat(30000) + i, i);
      s.add("q".repeat(300) + "w".repeat(300) + i);
    }
    var probe = "z".repeat(30000) + "x", hit = "z".repeat(30000) + "5";
    r.push(hasM(m, probe), hasM(m, hit), hasM(m, "z".repeat(30000)), hasM(s, "q".repeat(300) + "w".repeat(300) + "3"),
      hasM(s, "q".repeat(600)), hasM(s, ropeA), hasNone(m), hasNone(new Set([undefined])), hasExtra(m, hit));
    var many = new Map();
    for (i = 0; i < 4096; i++) many.set("k" + i, i);
    for (i = 0; i < 4200; i += 97) r.push(hasM(many, "k" + i) ? 1 : 0);
    return r.join(",");
  }
  function hasOdd() {
    var saved = Map.prototype.has, m = fill(new Map()), r = [];
    Map.prototype.has = function (k) { return "replaced" + show(k); };
    try { r.push(hasM(m, 1)); } finally { Map.prototype.has = saved; }
    Map.prototype.has = new Proxy(mhas, { apply(t, th, a) { return "p" + Reflect.apply(t, th, a); } });
    try { r.push(hasM(m, 0)); } finally { Map.prototype.has = saved; }
    Map.prototype.has = mhas.bind(m, 0);
    try { r.push(hasM(new Map(), 7)); } finally { Map.prototype.has = saved; }
    r.push(hasM(realmMap, 1), hasM(realmMap, "k"), hasM(realmMap, 2));
    var rm = Object.getPrototypeOf(realmMap).has;
    m.rh = rm;
    r.push(m.rh(0), m.rh(99));
    delete m.rh;
    return r.join(",");
  }
  function deep(n, m) {
    if (n === 0) return m.has(1) && Math.clz32(n) === 32;
    return deep(n - 1, m);
  }

  for (var rep = 0; rep < 3; rep++) {
    log.push(clzRow(clzM), clzRow(clzF), clzBad(), clzOdd());
    holders.forEach(function (h) { log.push(h[0] + ": " + hasRow(h[1], h[2])); });
    log.push(hasBad(), hasLong(), hasOdd(), deep(100, new Map([[1, 1]])), convLog.join(""));
    convLog.length = 0;
  }
  var depth = 0;
  try { (function rec() { depth++; Math.clz32(depth); new Map().has(1); rec(); })(); } catch (e) { log.push(err(e) + " at depth " + depth); }
  rvExpect(clzM, clzF, clzNone, clzExtra, clzStmt, clzRow, hasM, hasStmt, hasNone, hasExtra, hasRow, deep);
  return log.join("\n");
})()
