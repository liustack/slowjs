// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// Array.prototype.includes and indexOf answered in the loop (rv_fast_call in
// engine/regvm-ops-basic.h), against the same call made where the loop
// cannot answer it (Reflect.apply): each entry is the answer from the loop,
// then "=" when the other way gave the same, or what it gave. An exception
// from the loop shows its stack, which the golden recorded from the C call
// holds the same. Arrays of every kind of element (NaN, -0, strings that
// are atoms and strings that are not, long strings, ropes, BigInts,
// objects), holes, a length past the count, a getter for an index on
// Array.prototype, array-likes, typed arrays, proxies, a string as this,
// every kind of fromIndex (one that converts is logged), and compares of
// strings of one length that add up past a step.
(function () {
  var log = [];
  var inc = Array.prototype.includes, idx = Array.prototype.indexOf;
  var realmArr = newRealm("(function () { return [1, 'a', NaN]; })");
  var convLog = [];

  function show(v) {
    if (typeof v === "number" && v === 0 && 1 / v < 0) return "-0";
    if (typeof v === "symbol" || typeof v === "bigint") return typeof v;
    if (typeof v === "object" && v !== null) return "obj";
    return String(v);
  }
  function err(e) { return "!" + e.constructor.name + ": " + e.message; }
  function stack(e) { return err(e) + " @" + String(e.stack).trim().split("\n").join("|"); }
  function other(f, t, a) {
    try { return show(Reflect.apply(f, t, a)); } catch (e) { return err(e); }
  }

  function inc1(a, x) { return a.includes(x); }
  function inc2(a, x, i) { return a.includes(x, i); }
  function idx1(a, x) { return a.indexOf(x); }
  function idx2(a, x, i) { return a.indexOf(x, i); }
  function incNone(a) { return a.includes(); }
  function idxNone(a) { return a.indexOf(); }
  function idxStmt(a, x) { a.indexOf(x); return 1; }

  var o = { o: 1 }, sym = Symbol("s");
  var long1 = "L".repeat(70000), long2 = "L".repeat(69999) + "L";
  function rope() { return "abcdefghij".repeat(40) + "klmnopqrst".repeat(40); }
  var arrays = [
    ["empty", []],
    ["ints", [1, 2, 3, 2, 1]],
    ["floats", [1.5, -0, 0, NaN, 2]],
    ["strings", ["a", "b", "ab" + "c", "é", "", "abc"]],
    ["mixed", [1, "1", null, undefined, true, o, sym, 1n, 2n ** 70n, NaN, -0]],
    ["long", [long1, "x", rope()]],
    ["holes", [1, , 3, , 5]],
    ["undefineds", [undefined, undefined]],
    ["long length", (function () { var a = [1, 2]; a.length = 6; return a; })()],
    ["realm", realmArr()],
    ["frozen", Object.freeze([1, "a", NaN])],
    ["subclass", (function () { class S extends Array {} return S.from([1, "a", NaN]); })()],
  ];
  var finds = [1, 1.0, 2, "1", -0, 0, NaN, "a", "abc", "a" + "bc", "é", "", undefined, null, true, o, { o: 1 }, sym,
    1n, 2n ** 70n, long2, rope(), 3, 5];
  var froms = [0, 1, -1, -2, 3, 100, -100, 1.5, "1", undefined, null, NaN, Infinity, -Infinity, 2147483647,
    -2147483648, { valueOf() { convLog.push("v"); return 1; } }];
  function row(name, a) {
    var r = [];
    finds.forEach(function (x) {
      var s, t;
      try { s = show(inc1(a, x)); } catch (e) { s = stack(e); }
      t = other(inc, a, [x]);
      r.push(s === t ? s + "=" : s + "/" + t);
      try { s = show(idx1(a, x)); } catch (e) { s = stack(e); }
      t = other(idx, a, [x]);
      r.push(s === t ? s + "=" : s + "/" + t);
      idxStmt(a, x);
    });
    var f = [];
    froms.forEach(function (i) {
      var s, t;
      try { s = show(inc2(a, a.length ? a[0] : 1, i)) + show(idx2(a, a[a.length - 1], i)); } catch (e) { s = stack(e); }
      t = other(inc, a, [a.length ? a[0] : 1, i]) + other(idx, a, [a[a.length - 1], i]);
      f.push(s === t ? s + "=" : s + "/" + t);
    });
    return name + ": " + r.join(" ") + " | " + f.join(" ") + " | " + show(incNone(a)) + show(idxNone(a));
  }
  function odd() {
    var r = [], saved;
    Object.defineProperty(Array.prototype, 1, { get() { convLog.push("g"); return "got"; }, configurable: true });
    try { var h = [0, , 2]; r.push(inc1(h, "got"), idx1(h, "got"), inc1([0], "got")); } finally { delete Array.prototype[1]; }
    Object.prototype.inc = inc; Object.prototype.idx = idx;
    try {
      var likes = [{ length: 3, 0: "a", 1: NaN, 2: "c" }, { length: -1 }, "abc", new Uint8Array([1, 2, 3]),
        new Proxy([1, 2, 3], { get(t, k) { convLog.push("p" + String(k)); return t[k]; } }),
        (function () { return arguments; })(1, NaN, 3), Object.create(Array.prototype)];
      likes.forEach(function (x) {
        var s;
        try { s = show(x.inc(NaN)) + show(x.inc("c")) + show(x.idx(2)) + show(x.idx("b")); } catch (e) { s = stack(e); }
        r.push(s);
      });
      try { r.push(show(inc.call(null, 1))); } catch (e) { r.push(stack(e)); }
      var g = idx;
      try { r.push(show(g(1))); } catch (e) { r.push(stack(e)); }
      try { r.push(show([1, 2].idx(1, Symbol("f")))); } catch (e) { r.push(stack(e)); }
      try { r.push(show([1, 2].inc(1, 1n))); } catch (e) { r.push(stack(e)); }
      try { r.push(show([].inc(1, Symbol("f")))); } catch (e) { r.push(stack(e)); }
    } finally { delete Object.prototype.inc; delete Object.prototype.idx; }
    saved = Array.prototype.includes;
    Array.prototype.includes = function (x) { return "replaced" + show(x); };
    try { r.push(inc1([1], 1)); } finally { Array.prototype.includes = saved; }
    Array.prototype.indexOf = inc;
    try { r.push(idx1([1], 1)); } finally { Array.prototype.indexOf = idx; }
    var ri = Object.getPrototypeOf(realmArr()).indexOf, a = [7, 8];
    a.ri = ri;
    r.push(a.ri(8), a.ri(9));
    return r.join(" ");
  }
  // strings of one length: the compares add up past a step, and the search
  // gives up to the call
  function budget() {
    var a = [], r = [];
    for (var i = 0; i < 12; i++) a.push("q".repeat(9999) + String.fromCharCode(65 + i));
    var miss = "q".repeat(9999) + "z", hit = "q".repeat(9999) + "K", early = "q".repeat(9999) + "B";
    r.push(inc1(a, miss), idx1(a, hit), idx1(a, early), inc2(a, hit, 11), idx2(a, hit, -2));
    var many = [];
    for (i = 0; i < 5000; i++) many.push("k" + i);
    r.push(idx1(many, "k" + 4999), inc1(many, "k5000"), idx1(many, "k" + 17));
    var nums = [];
    for (i = 0; i < 20000; i++) nums.push(i * 0.5);
    r.push(idx1(nums, 9999.5), inc1(nums, -1), idx2(nums, 3, 7));
    return r.join(",");
  }

  for (var rep = 0; rep < 2; rep++) {
    arrays.forEach(function (x) { log.push(row(x[0], x[1])); });
    log.push(odd(), budget(), convLog.join(""));
    convLog.length = 0;
  }
  var depth = 0, held = [1, 2, 3];
  try { (function rec() { depth++; held.indexOf(3); held.includes(9); rec(); })(); } catch (e) { log.push(err(e) + " at depth " + depth); }
  rvExpect(inc1, inc2, idx1, idx2, incNone, idxNone, idxStmt, budget);
  return log.join("\n");
})()
