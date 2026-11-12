// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// The drops the register loop runs in another instruction (rv_dup_branch in
// engine/regvm-ops-cmp.h): a && b, a || b and their chains, where
// the branch frees the left value when it does not jump (JFND, JTND), with
// left values that have a reference count (objects, strings, BigInts) and
// that convert to a boolean by running JavaScript (none do: ToBoolean runs
// none, an object is true), in every place a value is used; and stores to
// closure variables whose value is dropped after, at a join or before a
// return (these keep their FREE).
(function () {
  "use strict";
  var log = [];
  function show(v) {
    if (typeof v === "bigint") return v + "n";
    if (typeof v === "symbol") return v.toString();
    if (typeof v === "function") return "fn " + v.name;
    if (typeof v === "object" && v !== null) {
      try { return JSON.stringify(v); } catch (e) { return "[obj]"; }
    }
    if (v === 0 && 1 / v < 0) return "-0";
    return String(v);
  }
  function id(x) { return x; }
  var vals = [0, -0, 1, NaN, "", "s", null, undefined, true, false, 0n, 5n,
    123456789012345678901234567890n, {}, [], { a: 1 }, Symbol.iterator, function g() {}];

  function and2(a, b) { var r = a && b; return r; }
  function or2(a, b) { var r = a || b; return r; }
  function andT(o) { return id(o.a) && id(o.b); }
  function orT(o) { return id(o.a) || id(o.b); }
  function chain(o) { return o.a && o.b && o.c || o.d || id(o.e) && o.f; }
  function inArgs(o) { return [id(o.a && o.b), id(o.a || o.b), o.a && o.b || o.c]; }
  function inCond(o) {
    var n = 0;
    if (o.a && o.b) n += 1;
    if (o.a || o.b) n += 2;
    while (o.a && o.k-- > 0) n += 4;
    return n + (o.a && o.b ? 8 : 0);
  }
  function inLoop(list) {
    var acc = [];
    for (var i = 0; i < list.length; i++) {
      var x = list[i];
      acc.push(x && x.v || "none", x.w && x.w.z);
    }
    return acc;
  }
  function nullish(o) { return [o.a ?? o.b, o.a?.b, o.a?.b ?? "d", o.a ?? o.b ?? o.c]; }
  // a getter for the left value: it runs once, before the branch
  var count = 0;
  var getterObj = { get a() { count++; return { left: count }; }, b: "right" };
  function viaGetter(o) { return o.a && o.b; }

  // stores to closure variables
  var c1 = null, c2 = 0, c3 = "";
  function set1(v) { c1 = v; }
  function set2(v) { c1 = v.x; }
  function set3(v) { c2 += v; }
  function set4(v) { c3 = c3 || v; }
  function set5(v) { (c1 = v); return; }
  function set6(v) { c1 = c2 = v; }
  function set7(v) { c2++; c1 = { v: v }; }
  function set8(v) { if (v) { c1 = v; } else { c1 = [v]; } }

  for (var rep = 0; rep < 2; rep++) {
    var row = [];
    for (var i = 0; i < vals.length; i++) {
      for (var j = 0; j < vals.length; j += 5) {
        row.push(show(and2(vals[i], vals[j])), show(or2(vals[i], vals[j])));
        var o = { a: vals[i], b: vals[j], c: { c: 1 }, d: "d", e: vals[j], f: [i], k: 2, w: vals[i] };
        row.push(show(andT(o)), show(orT(o)), show(chain(o)), show(inArgs(o)), inCond(o));
        row.push(show(nullish(o)));
      }
    }
    log.push(row.join(" "));
    log.push(show(inLoop([{ v: 1, w: { z: 2 } }, null, { v: 0, w: 0 }, { v: "s", w: { z: null } }, ""])));
    log.push(show(viaGetter(getterObj)) + " " + count);
    var objs = [];
    for (var k = 0; k < 3; k++) {
      set1({ k: k }); objs.push(show(c1));
      set2({ x: [k] }); objs.push(show(c1));
      set3(k); objs.push(c2);
      set4("s" + k); objs.push(c3);
      set5(k + "x"); objs.push(show(c1));
      set6({ six: k }); objs.push(show(c1), show(c2));
      c2 = 0;
      set7(k); objs.push(show(c1), c2);
      set8(k); objs.push(show(c1));
    }
    log.push(objs.join(","));
  }
  rvExpect(and2, or2, andT, orT, chain, inArgs, inCond, inLoop, nullish, viaGetter,
           set1, set2, set3, set4, set5, set6, set7, set8);
  return log.join("\n");
})()
