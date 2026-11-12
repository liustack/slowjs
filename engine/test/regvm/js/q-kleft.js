// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// A constant on the left of a binary op, read as an operand and not loaded
// into a temp first: KADD, KSUB and KSHL (engine/regvm-ops-basic.h and
// engine/regvm-ops-arith.h), and the ops without such a form (mul, and, or,
// xor), which load it. Every kind of value on the right, ints that overflow,
// -0, floats, strings, BigInts that mix with numbers and throw, objects
// whose valueOf runs JS (once, after the constant) or throws, Symbols, and a
// Symbol constant on the left. Results go to temps, to locals and to
// arguments.
(function () {
  "use strict";
  var log = [];
  function show(v) {
    if (typeof v === "bigint") return v + "n";
    if (typeof v === "symbol") return v.toString();
    if (v === 0 && 1 / v < 0) return "-0";
    if (typeof v === "object" && v !== null) return "[obj]";
    return String(v);
  }
  var calls = 0;
  var vals = [0, -0, 1, -1, 7, 31, 32, 33, -33, 2147483647, -2147483648, 4294967295, 0.5, -1.5,
    NaN, Infinity, -Infinity, "3", "", "x", null, undefined, true, false, 3n, -5n,
    { valueOf() { calls++; return 4; } }, { toString() { calls++; return "s"; } }, [2], [],
    Symbol("y"), { valueOf() { throw new RangeError("vo"); } }];

  function kadd(x) { return 1 + x; }
  function kaddS(x) { return "<" + x; }
  function kaddF(x) { return 0.5 + x; }
  function kaddB(x) { return 2n + x; }
  function ksub(x) { return 31 - x; }
  function ksubF(x) { return -0.25 - x; }
  function ksubZ(x) { return 0 - x; }
  function kshl(x) { return 1 << x; }
  function kshlB(x) { return 1n << x; }
  function kshlS(x) { return "2" << x; }
  function kmul(x) { return 2 * x; }
  function kmulZ(x) { return -0 * x; }
  function kmulS(x) { return "3" * x; }
  function kand(x) { return 0xff & x; }
  function kor(x) { return 0x100 | x; }
  function kxor(x) { return -1 ^ x; }
  function kmulB(x) { return 3n * x; }
  function ksym(x) { return Symbol.iterator * x; }
  function ksymA(x) { return Symbol.iterator + x; }
  // into a local and an argument, the local also on the right
  function toLocal(x) {
    var a = 5, b = "", c = 0;
    a = 1 + x; b = "k" + b; c = 31 - c; c = 1 << c;
    var d = 2 * a; x = 3 - x;
    a = 7 + a;
    return [a, b, c, d, x].map(show).join(",");
  }
  // a temp below the op when it throws
  function below(x) { return [x, 1 + x, 31 - x, 1 << x, 2 * x]; }

  var fs = [kadd, kaddS, kaddF, kaddB, ksub, ksubF, ksubZ, kshl, kshlB, kshlS, kmul, kmulZ, kmulS,
    kand, kor, kxor, kmulB, ksym, ksymA];
  for (var rep = 0; rep < 3; rep++) {
    var row = [];
    for (var i = 0; i < fs.length; i++) {
      for (var j = 0; j < vals.length; j++) {
        var c0 = calls, r;
        try { r = show(fs[i](vals[j])); } catch (e) { r = e.constructor.name + ":" + e.message; }
        row.push(r + (calls - c0 ? "@" + (calls - c0) : ""));
      }
    }
    log.push(row.join(" "));
    var row2 = [];
    for (var j = 0; j < vals.length; j++) {
      try { row2.push(toLocal(vals[j])); } catch (e) { row2.push(e.constructor.name); }
      try { row2.push(below(vals[j]).map(show).join("/")); } catch (e) { row2.push(e.constructor.name); }
    }
    log.push(row2.join(" "));
  }
  rvExpect(kadd, kaddS, kaddF, kaddB, ksub, ksubF, ksubZ, kshl, kshlB, kshlS, kmul, kmulZ, kmulS,
           kand, kor, kxor, kmulB, ksym, ksymA, toLocal, below);
  return log.join("\n");
})()
