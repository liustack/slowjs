// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// String.prototype.charCodeAt answered in the loop (rv_fast_call in
// engine/regvm-ops-basic.h) for a string or a rope made flat and an index
// that is an int, and every case where the call is made: a rope not flat
// yet, a String object or another primitive as this, null and undefined as
// this, an index that is not an int (a double, a string, an object that
// converts, a Symbol), out of range or negative, a missing argument, extra
// arguments, charCodeAt of another realm, charCodeAt replaced, wrapped in a
// Proxy or bound, and deep recursion through it.
(function () {
  "use strict";
  var log = [];
  var cc = String.prototype.charCodeAt;
  var realmCc = newRealm("String.prototype.charCodeAt");
  var convLog = [];
  var conv = { valueOf() { convLog.push("v"); return 2; } };
  var strObj = { toString() { convLog.push("s"); return "xyz"; } };
  var idx = [0, 1, 2, 5, -1, 3, 4, 1.5, -0, 2.9, "1", "", NaN, Infinity, -Infinity,
    2147483647, -2147483648, 4294967296, 4294967297, null, true, undefined, conv];

  function wide() { return "aé€😀"; }
  function rope(k) {
    var a = "abcdefghijklmnopqrstuvwxyz".repeat(20), b = "0123456789".repeat(60);
    return k ? a + b : b + "€" + a;
  }
  function one(s, i) { return s.charCodeAt(i); }
  function stmt(s, i) { s.charCodeAt(i); return 1; }
  function none(s) { return s.charCodeAt(); }
  function extra(s) { return s.charCodeAt(1, 2, 3); }
  function sum(s) {
    var n = 0;
    for (var i = 0; i < s.length; i++) n += s.charCodeAt(i);
    return n;
  }
  function throws(f) {
    try { return f(); } catch (e) { return e.constructor.name + (e instanceof TypeError ? "" : "!"); }
  }
  function prims() {
    var m = { cc: cc };
    Number.prototype.cc = cc;
    Boolean.prototype.cc = cc;
    Symbol.prototype.cc = cc;
    try {
      return [(53).cc(0), (1.5).cc(1), true.cc(2), throws(function () { return Symbol("q").cc(0); }),
        new String("abc").charCodeAt(1), m.cc.call(strObj, 0), throws(function () { return m.cc.call(null, 0); }),
        throws(function () { return cc(0); }), [1, 2].join.call({ length: 2, 0: "a" }, "").charCodeAt(0)].join(",");
    } finally {
      delete Number.prototype.cc;
      delete Boolean.prototype.cc;
      delete Symbol.prototype.cc;
    }
  }
  function badIndex(s) {
    return [throws(function () { return s.charCodeAt(Symbol("i")); }),
      throws(function () { return s.charCodeAt(1n); }),
      throws(function () { return s.charCodeAt({ valueOf() { throw new RangeError("r"); } }); })].join(",");
  }
  function realm(s) {
    var o = { cc: realmCc };
    return [o.cc.call(s, 1), throws(function () { return realmCc.call(undefined, 0); })].join(",");
  }
  function viaRealm(s, i) { return s.rc(i); }
  function replaced(s) {
    var r;
    String.prototype.charCodeAt = function (i) { return "replaced" + i; };
    try { r = s.charCodeAt(1); } finally { String.prototype.charCodeAt = cc; }
    return r + " " + s.charCodeAt(1);
  }
  function proxied(s) {
    var calls = 0;
    String.prototype.charCodeAt = new Proxy(cc, { apply(t, th, a) { calls++; return Reflect.apply(t, th, a); } });
    var r;
    try { r = [s.charCodeAt(0), s.charCodeAt(9), calls]; } finally { String.prototype.charCodeAt = cc; }
    return r.join(",");
  }
  function bound(s) {
    var b = cc.bind("bound"), o = { b: b, c: cc.bind(s, 2) };
    return [b(1), o.b(0), o.c(), o.c(7), b(99)].join(",");
  }
  function deep(n, s) {
    if (n === 0) return s.charCodeAt(1);
    return deep(n - 1, s);
  }

  var strs = ["", "a", "abc", wide(), "x".repeat(70000)];
  for (var rep = 0; rep < 3; rep++) {
    var row = [];
    for (var i = 0; i < strs.length; i++) {
      for (var j = 0; j < idx.length; j++) {
        row.push(one(strs[i], idx[j]), stmt(strs[i], idx[j]));
      }
      row.push(none(strs[i]), extra(strs[i]), sum(strs[i]), badIndex(strs[i]));
    }
    log.push(row.join(" "));
    // a rope: the first call makes it flat, the ones after find it flat
    for (var k = 0; k < 2; k++) {
      var r = rope(k), first = [one(r, 0), one(r, 519), one(r, 520), one(r, 600), one(r, 601), one(r, r.length - 1),
        one(r, r.length), one(r, -1), one(r, 1.5), none(r)];
      log.push(first.join(",") + " " + sum(r) + " " + sum(rope(k) + rope(1 - k)));
    }
    log.push(prims(), realm("qāz"), replaced("abc"), proxied("abcdefghij"), bound("cde"), deep(50, "一二"));
    String.prototype.rc = realmCc;
    try { log.push([viaRealm("abc", 0), viaRealm("abc", 5), viaRealm(rope(1), 2)].join(",")); } finally { delete String.prototype.rc; }
    log.push(convLog.join(""));
    convLog.length = 0;
  }
  try { (function rec() { "x".charCodeAt(0); rec(); })(); } catch (e) { log.push(e.constructor.name); }
  rvExpect(one, stmt, none, extra, sum, badIndex, prims, realm, viaRealm, replaced, proxied, bound, deep);
  return log.join("\n");
})()
