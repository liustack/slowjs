// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// rvdiff: lte, gt, gte, eq, neq, strict_neq and the tests (is_null,
// is_undefined, is_undefined_or_null, the typeof tests, lnot), each as a
// bool and fused with its branch, with the constant on either side, on every
// pair of a set of values. Some values convert by calling back into
// JavaScript, one of them throws.
(function () { "use strict";
var vals = [0, -0, 1, -1, 2147483647, 0.5, NaN, Infinity, "", "1", "a", "10", "b",
  true, false, null, undefined,
  { valueOf: function () { return 7; } },
  { toString: function () { return "s"; }, valueOf: undefined },
  { valueOf: function () { throw new RangeError("valueOf threw"); } },
  10n, Symbol.iterator, [1], function () {}];
function lt(a, b) { return a < b; }
function lte(a, b) { return a <= b; }
function gt(a, b) { return a > b; }
function gte(a, b) { return a >= b; }
function eq(a, b) { return a == b; }
function neq(a, b) { return a != b; }
function seq(a, b) { return a === b; }
function sneq(a, b) { return a !== b; }
function lteb(a, b) { if (a <= b) return 1; return 0; }
function gtb(a, b) { if (a > b) return 1; return 0; }
function gteb(a, b) { if (a >= b) return 1; return 0; }
function eqb(a, b) { if (a == b) return 1; return 0; }
function neqb(a, b) { if (a != b) return 1; return 0; }
function sneqb(a, b) { if (a !== b) return 1; return 0; }
var two = [lt, lte, gt, gte, eq, neq, seq, sneq, lteb, gtb, gteb, eqb, neqb, sneqb];
function k1(a) { return 3 < a; }
function k2(a) { return "b" >= a; }
function k3(a) { if ("1" > a) return 1; return 0; }
function k4(a) { return null == a; }
function k5(a) { return a !== null; }
function k6(a) { if (undefined != a) return 1; return 0; }
function k7(a) { return a <= 2.5; }
function k8(a) { if (a === "a") return 1; return 0; }
function k9(a) { return "a" !== a; }
function isn(a) { return a === null; }
function isu(a) { return a === undefined; }
function isun(a) { return a == null; }
function isnb(a) { if (a === null) return 1; return 0; }
function isub(a) { if (a !== undefined) return 1; return 0; }
function isunb(a) { if (a == undefined) return 1; return 0; }
function tf(a) { return typeof a === "function"; }
function tu(a) { return typeof a === "undefined"; }
function ts(a) { return typeof a === "string"; }
function tfb(a) { if (typeof a === "function") return 1; return 0; }
function tub(a) { if (typeof a !== "undefined") return 1; return 0; }
function tnb(a) { if (typeof a == "number") return 1; return 0; }
function not(a) { return !a; }
function notb(a) { if (!a) return 1; return 0; }
function nott(a) { return !0; }
function notf(a) { return !1; }
function notnot(a) { return !!a; }
function and(a, b) { return a !== null && b; }
function or(a, b) { return a || b; }
function ornull(a) { return a == null || a === 0; }
var one = [k1, k2, k3, k4, k5, k6, k7, k8, k9, isn, isu, isun, isnb, isub, isunb,
  tf, tu, ts, tfb, tub, tnb, not, notb, nott, notf, notnot];
function show(v) {
  if (v === true) return "t";
  if (v === false) return "f";
  if (typeof v === "symbol") return "y";
  if (typeof v === "bigint") return "n";
  if (typeof v === "object" && v !== null) return "o";
  if (typeof v === "function") return "F";
  return String(v);
}
var out = [], i, j, f, r;
for (f = 0; f < two.length; f++) {
  for (i = 0; i < vals.length; i++) {
    for (j = 0; j < vals.length; j++) {
      try { r = show(two[f](vals[i], vals[j])); } catch (e) { r = "<" + e.name + ">"; }
      out.push(r);
    }
  }
  out.push("|");
}
for (f = 0; f < one.length; f++) {
  for (i = 0; i < vals.length; i++) {
    try { r = show(one[f](vals[i])); } catch (e) { r = "<" + e.name + ">"; }
    out.push(r);
  }
  out.push("|");
}
for (i = 0; i < vals.length; i++) {
  for (j = 0; j < vals.length; j++) {
    try { r = show(and(vals[i], vals[j])) + show(or(vals[i], vals[j])); } catch (e) { r = "<" + e.name + ">"; }
    out.push(r);
  }
  out.push(show(ornull(vals[i])));
}
rvExpect(and, eq, eqb, gt, gtb, gte, gteb, isn, isnb, isu, isub, isun, isunb, k1, k2, k3, k4, k5, k6, k7, k8, k9, lt, lte, lteb, neq, neqb, not, notb, notf, notnot, nott, or, ornull, seq, show, sneq, sneqb, tf, tfb, tnb, ts, tu, tub);
return out.join("");
})()
