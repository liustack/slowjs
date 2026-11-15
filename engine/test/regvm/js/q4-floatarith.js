// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// Add, sub and mul of two numbers that are not both ints, which rv_arith_num
// and rv_bin_other do (engine/regvm-ops-basic.h and engine/regvm-ops-arith.h),
// in every form: both operands in registers into a temp (ADD, SUB, MUL), into
// a local that held a string or an array, whose old value is freed
// (ADDL, SUBL), with a constant operand (ADDK, SUBK, MULK) and both (ADDKL,
// SUBKL). The results go into temps that a call below them owns, so that a
// throw there unwinds them. Then values that are not numbers, which go on to
// rv_arith_other and rv_bin_other, and a throwing valueOf.
(function () {
  "use strict";
  var vals = [0.5, -1.5, -0, NaN, Infinity, 1e308, 9007199254740992, 2.5, 3, -7];
  function fmt(v) {
    return v === 0 && 1 / v < 0 ? "-0" : String(v);
  }
  function id(x) { return x; }
  function add(a, b) { return a + b; }
  function sub(a, b) { return a - b; }
  function mul(a, b) { return a * b; }
  function addl(a, b) { var r = "s" + a; r = a + b; return r; }
  function subl(a, b) { var r = [a]; r = a - b; return r; }
  function ks(a) {
    var r = [a], s = "k" + a;
    r = a + 0.25;
    s = a - 1e300;
    return [a + 1.5, a - 0.5, a * 2.5, a * -0, r, s].map(fmt).join(",");
  }
  // temps owned below the op: id() calls around it
  function tmp(a, b) { return id(a) + (id(a) * id(b) - id(b)) + id(b + a); }
  var out = [];
  var fs = [add, sub, mul, addl, subl, tmp];
  for (var f = 0; f < fs.length; f++) {
    var row = [];
    for (var i = 0; i < vals.length; i++)
      for (var j = 0; j < vals.length; j++)
        row.push(fmt(fs[f](vals[i], vals[j])));
    out.push(fs[f].name + ":" + row.join(","));
  }
  for (var i = 0; i < vals.length; i++)
    out.push("k " + fmt(vals[i]) + ":" + ks(vals[i]));
  // a loop of floats, warm
  var s = 0.5, t = 1.5;
  for (var k = 0; k < 300; k++) {
    s = s + k * 0.5;
    t = t * 1.0001 - s * 1e-9;
    s = s - t;
  }
  out.push("loop " + fmt(s) + " " + fmt(t));
  // not two floats: the shared tails and the slow side
  var others = ["1.5", true, null, undefined, { valueOf: function () { return 0.25; } }];
  var row = [];
  for (var i = 0; i < others.length; i++)
    row.push(fmt(add(others[i], 0.5)), fmt(sub(0.5, others[i])), fmt(mul(others[i], -2.5)));
  out.push("slow:" + row.join(","));
  var bad = { valueOf: function () { throw new RangeError("valueOf"); } };
  try { addl(0.5, bad); out.push("throw: none"); }
  catch (e) { out.push("throw:" + e.constructor.name); }
  try { mul(1n, 0.5); out.push("bigint: none"); }
  catch (e) { out.push("bigint:" + e.constructor.name); }
  return out.join("\n");
})();
