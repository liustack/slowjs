// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// rvdiff: switches on strings whose CASES has a table (rv_cases_table,
// engine/regvm-ops-cmp.h): 4, 8, 31, 64 and 70 cases, a case given twice,
// the last case right before its body (a case of flag 0), cases that are not
// constants among constant ones, a case of an index string ("1"), which
// keeps the chain, and keys of every kind: string atoms, strings made at run
// time of the same length as a case, a rope equal to a long case, numbers,
// booleans, null, undefined, a symbol, BigInts and objects. Then the atom
// table grows and the collector runs, and the same switches run again.
(function () { "use strict";
function cases(n, prefix, body) {
  var src = "\"use strict\"; return function (s) { switch (s) {", i;
  for (i = 0; i < n; i++)
    src += " case \"" + prefix + i + "\": return " + body(i) + ";";
  src += " } return -1; };";
  return Function(src)();
}
var t4 = cases(4, "a", function (i) { return i; });
var t8 = cases(8, "b", function (i) { return i * 2; });
var t31 = cases(31, "c", function (i) { return i % 5; });
var t64 = cases(64, "d", function (i) { return i % 7; });
var t70 = cases(70, "e", function (i) { return i % 3; });
function dup(s) {
  switch (s) {
  case "p": return 1;
  case "q": return 2;
  case "p": return 3;
  case "r": return 4;
  case "q": return 5;
  case "s": return 6;
  }
  return 0;
}
function last(s) {
  var r = 0;
  switch (s) {
  case "u": r = 1; break;
  case "v": r = 2; break;
  case "w": r = 3; break;
  case "x":
    r = 4;
  }
  return r;
}
function fall(s) {
  var r = "";
  switch (s) {
  case "f1": r += "1";
  case "f2": r += "2";
  case "f3": r += "3"; break;
  case "f4": r += "4";
  default: r += "d";
  case "f5": r += "5";
  }
  return r;
}
var calls = 0;
function k(x) { calls++; return x; }
function mixed(s) {
  switch (s) {
  case "m1": return 1;
  case "m2": return 2;
  case "m3": return 3;
  case "m4": return 4;
  case k("m5"): return 5;
  case "m6": return 6;
  case "m7": return 7;
  case "m8": return 8;
  case "m9": return 9;
  }
  return 0;
}
function index(s) {
  switch (s) {
  case "x0": return 0;
  case "1": return 1;
  case "x2": return 2;
  case "x3": return 3;
  case "x4": return 4;
  }
  return -1;
}
var lstr = "L".repeat(400) + "end";
var lng = Function("\"use strict\"; return function (s) { switch (s) {" +
  " case \"LLLL\": return 1; case \"LLLLL\": return 2; case \"LLLLLL\": return 3;" +
  " case \"" + lstr + "\": return 4; case \"LLLLLLL\": return 5; } return 0; };")();
function rope(n) {
  var s = "";
  for (var i = 0; i < n; i++)
    s += "L";
  return s + "end";
}
var fns = [t4, t8, t31, t64, t70, dup, last, fall, mixed, index, lng];
function keys() {
  return ["a0", "a3", "a4", "b7", "b" + 7, "b8", "c0", "c30", "c" + "30", "c31",
    "d0", "d33", "d63", "d64", "e0", "e63", "e64", "e69", "e70",
    "p", "q", "r", "s", "t", "u", "v", "w", "x", "y",
    "f1", "f2", "f3", "f4", "f5", "f6",
    "m1", "m4", "m5", "m" + 5, "m6", "m9", "m10",
    "x0", "1", "x2", "x4", "x5",
    "LLLL", "LLL" + "L", "LLLLLLL", lstr, rope(400), rope(401), "L".repeat(403),
    "", "A0", "a00",
    0, 1, -0, 0.5, NaN, true, false, null, undefined, Symbol("a0"), 1n, 2n ** 80n,
    {}, ["a0"], { toString: function () { return "a0"; } }];
}
function run() {
  var ks = keys(), out = [], i, j;
  for (i = 0; i < ks.length; i++) {
    var row = [];
    for (j = 0; j < fns.length; j++)
      row.push(fns[j](ks[i]));
    out.push(row.join(" "));
  }
  return out.join("\n");
}
var first = run(), again = run();
// the atom table grows (a resize moves its array) and the collector runs
var o = {}, i;
for (i = 0; i < 30000; i++)
  o["grow" + i] = i;
o = null;
gc();
var third = run();
rvExpect(t4, t8, t31, t64, t70, dup, last, fall, mixed, index, lng, run);
return [first, again === first, third === first, "calls " + calls].join("\n");
})()
