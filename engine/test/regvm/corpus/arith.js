// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// rvdiff: add, sub, lt and strict_eq of every pair of a set of values, in
// functions the register pass converts (strict, with no captured locals), and
// inc_loc and dec_loc at the ends of the int range. Objects with valueOf and
// toString call back into JavaScript, and some of them throw.
(function () { "use strict";
var log = [];
var calls = { n: 0 };
var vals = [0, -0, 1, -1, 2147483647, -2147483648, 1073741824, 0.5, -1.5, NaN, Infinity, -Infinity,
  1e300, 5e-324, "", "1", "a", "10", "2", true, false, null, undefined,
  { valueOf: function () { calls.n = calls.n + 1; return 7; } },
  { toString: function () { calls.n = calls.n + 1; return "s"; }, valueOf: undefined },
  { valueOf: function () { throw new RangeError("valueOf threw"); } },
  [], [1], [1, 2], 10n, Symbol.iterator];
function add(a, b) { return a + b; }
function sub(a, b) { return a - b; }
function lt(a, b) { return a < b; }
function seq(a, b) { return a === b; }
function addk(a) { return a + 1; }
function subk(a) { return a - 1; }
function kadd(a) { return "k" + a; }
function ltk(a) { if (a < 3) return 1; return 0; }
function seqk(a) { if (a === "1") return 1; return 0; }
function show(v) {
  if (typeof v === "symbol") return "sym";
  if (typeof v === "bigint") return v + "n";
  if (Object.is(v, -0)) return "-0";
  try { return typeof v + ":" + String(v); } catch (e) { return typeof v + ":?"; }
}
function guard(f, a, b) {
  try { return show(f(a, b)); } catch (e) { return e.name + ":" + e.message; }
}
for (var i = 0; i < vals.length; i++) {
  var row = [];
  for (var j = 0; j < vals.length; j++)
    row.push(guard(add, vals[i], vals[j]) + "|" + guard(sub, vals[i], vals[j]) + "|" +
      guard(lt, vals[i], vals[j]) + "|" + guard(seq, vals[i], vals[j]));
  row.push(guard(addk, vals[i]) + "|" + guard(subk, vals[i]) + "|" + guard(kadd, vals[i]) + "|" +
    guard(ltk, vals[i]) + "|" + guard(seqk, vals[i]));
  log.push(show(vals[i]) + " => " + row.join(" "));
}
function incs(n) { var x = n; x++; x++; return x; }
function decs(n) { var x = n; x--; x--; return x; }
log.push([2147483646, 2147483647, -2147483648, 0.5, "1", null].map(function (v) {
  return show(incs(v)) + "/" + show(decs(v));
}).join(" "));
// a loop of adds that leaves the int range half way
function sum(n, start) { var s = start, i = 0; while (i < n) { s = s + 1073741823; i = i + 1; } return s; }
log.push(sum(5, 0) + " " + sum(5, -2147483648) + " " + sum(3, 0.25) + " " + sum(2, "x"));
log.push("valueOf calls " + calls.n);
return log.join("\n");
})()
