// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// String called as a function answered in the loop (rv_fast_call in
// engine/regvm-ops-basic.h) for a string, against the same call made where
// the loop cannot answer it (Reflect.apply): each entry is the answer from
// the loop, then "=" when the other way gave the same, or what it gave. An
// exception from the loop shows its stack, which the golden recorded from
// the C call holds the same. Strings of both widths, empty and long ones,
// ropes, every other kind of value (an object whose conversion is logged,
// a Symbol, one that throws), no argument and extra ones, new String, the
// String of another realm, a String replaced, and a recursion that ends at
// the call depth.
(function () {
  var log = [];
  var S = String, realmS = newRealm("(function () { return String; })")();
  var convLog = [];

  function show(v) { return typeof v + ":" + (typeof v === "symbol" ? "sym" : String(v).slice(0, 40)); }
  function err(e) { return "!" + e.constructor.name + ": " + e.message; }
  function stack(e) { return err(e) + " @" + String(e.stack).trim().split("\n").join("|"); }
  function other(a) {
    try { return show(Reflect.apply(S, undefined, a)); } catch (e) { return err(e); }
  }

  function s1(x) { return String(x); }
  function s2(x) { return String(x, 1, 2); }
  function s0() { return String(); }
  function sStmt(x) { String(x); return 1; }
  function same(x) { return String(x) === x; }

  var long = "z".repeat(70000);
  function rope() { return "abcdefghij".repeat(40) + "klmnopqrst".repeat(40); }
  var vals = ["", "a", "abc", "é€日本", "\u0001x", long, rope(), "1" + 2, 1, -0, 1.5, NaN, 1n, true, null, undefined,
    {}, [1, 2], { toString() { convLog.push("t"); return "obj"; } }, { valueOf() { return 1; } }, function f() {},
    Symbol("s"), Symbol.for("r"), new String("boxed"), Object.create(null)];
  function row() {
    var r = [];
    vals.forEach(function (x) {
      var s, t;
      try { s = show(s1(x)); } catch (e) { s = stack(e); }
      t = other([x]);
      r.push((s === t ? s + "=" : s + "/" + t));
      try { s = show(s2(x)); } catch (e) { s = stack(e); }
      t = other([x, 1, 2]);
      r.push((s === t ? "=" : s + "/" + t));
      try { sStmt(x); } catch (e) { r.push("stmt" + err(e)); }
    });
    return r.join(" ") + " | " + show(s0()) + " " + same("abc") + same(long) + " " + convLog.join("");
  }
  function odd() {
    var r = [];
    try { r.push(s1({ toString() { throw new Error("conv"); } })); } catch (e) { r.push(stack(e)); }
    try { r.push(s1(Object.create(null))); } catch (e) { r.push(stack(e)); }
    r.push(typeof new String("a"), new String("ab").length, show(realmS("x")), realmS("x") === "x");
    var saved = String;
    String = function (x) { return "replaced" + x; };
    try { r.push(s1("a")); } finally { String = saved; }
    String = realmS;
    try { r.push(s1("a"), s1(1)); } finally { String = saved; }
    var o = { S: S };
    r.push(o.S("method"), show(S.call(o, "c")), show(S.apply(null, ["d"])), show(S.bind(null)("e")));
    return r.join(" ");
  }

  for (var rep = 0; rep < 2; rep++) {
    convLog.length = 0;
    log.push(row(), odd());
  }
  var depth = 0;
  try { (function rec() { depth++; String("a"); String(long); rec(); })(); } catch (e) { log.push(err(e) + " at depth " + depth); }
  rvExpect(s1, s2, s0, sStmt, same);
  return log.join("\n");
})()
