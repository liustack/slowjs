// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// String.prototype.indexOf, includes, startsWith and endsWith answered in the
// loop (rv_fast_call in engine/regvm-ops-basic.h), against the same call
// made where the loop cannot answer it (Reflect.apply): each entry is the
// answer from the loop, then "=" when the other way gave the same, or what
// it gave. An exception from the loop shows its stack, which the golden
// recorded from the C call holds the same. 8-bit and 16-bit strings both
// ways, empty strings, needles of 1, 2 to 8 and more characters, a needle
// longer than the string, every kind of position (one that converts is
// logged), a rope as this or as the needle, a needle that is not a string
// (a number, an object whose toString is logged, a RegExp, a Symbol), a
// String object as this, null and undefined as this, and searches long
// enough that the search reads the interrupt flag (the call is made).
(function () {
  var log = [];
  var sidx = String.prototype.indexOf, sinc = String.prototype.includes;
  var ssw = String.prototype.startsWith, sew = String.prototype.endsWith;
  var convLog = [];

  function show(v) { return String(v); }
  function err(e) { return "!" + e.constructor.name + ": " + e.message; }
  function stack(e) { return err(e) + " @" + String(e.stack).trim().split("\n").join("|"); }
  function other(f, t, a) {
    try { return show(Reflect.apply(f, t, a)); } catch (e) { return err(e); }
  }

  function idx1(s, x) { return s.indexOf(x); }
  function idx2(s, x, i) { return s.indexOf(x, i); }
  function inc1(s, x) { return s.includes(x); }
  function inc2(s, x, i) { return s.includes(x, i); }
  function sw1(s, x) { return s.startsWith(x); }
  function sw2(s, x, i) { return s.startsWith(x, i); }
  function ew1(s, x) { return s.endsWith(x); }
  function ew2(s, x, i) { return s.endsWith(x, i); }
  function none(s) { return [s.indexOf(), s.includes(), s.startsWith(), s.endsWith()].join(); }
  function stmt(s, x) { s.indexOf(x); s.includes(x); return 1; }

  var hays = ["", "a", "abc", "abcabcabd", "hello world, hello", "aé€x€", "日本語の日本", "x".repeat(300) + "needle" + "y".repeat(300),
    "ab".repeat(5000) + "abc", "Āabcabc"];
  var needles = ["", "a", "b", "c", "ab", "abc", "abd", "bca", "hello", "world", "é", "€", "€x", "日本", "の日", "needle",
    "y".repeat(299), "xy", "zz", "ababababababc", "abcabcabdx", "Ā", "Āa"];
  var poss = [0, 1, 2, -1, -5, 3, 100, undefined];
  var fns = [[idx1, idx2, sidx], [inc1, inc2, sinc], [sw1, sw2, ssw], [ew1, ew2, sew]];
  function row(h) {
    var r = [];
    needles.forEach(function (x) {
      fns.forEach(function (f) {
        var s, t;
        try { s = show(f[0](h, x)); } catch (e) { s = stack(e); }
        t = other(f[2], h, [x]);
        r.push(s === t ? s : s + "/" + t);
        poss.forEach(function (p) {
          try { s = show(f[1](h, x, p)); } catch (e) { s = stack(e); }
          t = other(f[2], h, [x, p]);
          r.push(s === t ? s : s + "/" + t);
        });
      });
      stmt(h, x);
    });
    return r.join(" ") + " " + none(h);
  }
  function odd() {
    var r = [], h = "abc" + "def".repeat(200), rope = "ab".repeat(300) + "cd".repeat(300);
    var things = [1, 12, null, undefined, true, { toString() { convLog.push("t"); return "c"; } }, ["a", "b"],
      1.5, -0, NaN, 1n];
    var pos = [1.5, "1", null, NaN, Infinity, -Infinity, 2147483648, { valueOf() { convLog.push("v"); return 1; } }];
    fns.forEach(function (f) {
      things.forEach(function (x) {
        try { r.push(show(f[0]("1truenull12a,bc1.50NaN1undefined", x))); } catch (e) { r.push(stack(e)); }
      });
      pos.forEach(function (p) {
        try { r.push(show(f[1]("abcabc", "c", p))); } catch (e) { r.push(stack(e)); }
      });
      [/a/, Symbol("s")].forEach(function (x) {
        try { r.push(show(f[0]("abc", x))); } catch (e) { r.push(stack(e)); }
      });
      try { r.push(show(f[0](rope, "dc")), show(f[0](h, rope)), show(f[0](rope, rope))); } catch (e) { r.push(stack(e)); }
      try { r.push(show(f[0](new String("abc"), "b"))); } catch (e) { r.push(stack(e)); }
    });
    Object.prototype.si = sidx;
    try {
      [5, true, { toString() { return "xbx"; } }].forEach(function (t) {
        try { r.push(show(t.si("b"))); } catch (e) { r.push(stack(e)); }
      });
    } finally { delete Object.prototype.si; }
    var g = sinc;
    try { r.push(show(g("a"))); } catch (e) { r.push(stack(e)); }
    var saved = String.prototype.indexOf;
    String.prototype.indexOf = function (x) { return "replaced" + x; };
    try { r.push(idx1("abc", "b")); } finally { String.prototype.indexOf = saved; }
    String.prototype.indexOf = String.prototype.lastIndexOf;
    try { r.push(idx1("abcb", "b")); } finally { String.prototype.indexOf = saved; }
    return r.join(" ");
  }
  // searches whose work passes a step: the flag would be read, so the call
  // is made
  function long() {
    var big = "a".repeat(200000) + "b", r = [];
    r.push(idx1(big, "b"), idx1(big, "aab"), idx1(big, "aaaaaaaaab"), idx1(big, "c"), inc1(big, "ab"), ew1(big, "ab"),
      sw1(big, "aaa"), idx2(big, "b", 199990), idx1(big, "a".repeat(70000)), inc1(big, "a".repeat(70000) + "b"),
      sw1(big, "a".repeat(70000)), idx1("€".repeat(100000) + "x", "x"));
    return r.join(",");
  }

  for (var rep = 0; rep < 2; rep++) {
    hays.forEach(function (h) { log.push(row(h)); });
    log.push(odd(), long(), convLog.join(""));
    convLog.length = 0;
  }
  var depth = 0;
  try { (function rec() { depth++; "abc".indexOf("c"); "abc".includes("z"); rec(); })(); } catch (e) { log.push(err(e) + " at depth " + depth); }
  rvExpect(idx1, idx2, inc1, inc2, sw1, sw2, ew1, ew2, none, stmt, long);
  return log.join("\n");
})()
