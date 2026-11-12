// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// P2 of the register loop: inc_loc and dec_loc (x++ and x-- on a local, the
// value unused) are ToNumeric(x) + 1 and - 1, not x + 1: strings, numeric
// strings, objects whose valueOf gives a string, a number, a BigInt or
// throws, BigInts, booleans, null, undefined, a Symbol, floats, and ints at
// the ends of their range.
(function () { "use strict";
var B = { n: 0 };
var vals = [0, 1, -1, 2147483646, 2147483647, -2147483647, -2147483648, 0.5, -0, NaN, Infinity,
  "1", "a", "", " 12 ", "0x10", "1e3", true, false, null, undefined,
  { valueOf: function () { return "1"; } }, { valueOf: function () { return 5; } },
  { valueOf: function () { return 1n; } }, { valueOf: function () { throw new RangeError("valueOf threw"); } },
  1n, 0n, -1n, 9007199254740993n, Symbol.iterator];
function inc(p) { var v = p; B.n = B.n + 1; v++; return v; }
function dec(p) { var v = p; B.n = B.n + 1; v--; return v; }
function inc2(p) { var v = p; B.n = B.n + 1; v++; v++; return v; }
function dec2(p) { var v = p; B.n = B.n + 1; v--; v--; return v; }
function loop(p, n) { var v = p, i = 0; B.n = B.n + 1; while (i < n) { v++; v--; v++; i++; } return v; }
function show(v) {
  if (typeof v === "symbol") return "sym";
  if (typeof v === "bigint") return v + "n";
  if (Object.is(v, -0)) return "-0";
  try { return typeof v + ":" + String(v); } catch (e) { return typeof v + ":?"; }
}
function guard(f, a, b) {
  try { return show(f(a, b)); } catch (e) { return e.name + ":" + e.message; }
}
var log = [];
inc(1); dec(1); inc2(1); dec2(1); loop(1, 1);
for (var i = 0; i < vals.length; i++)
  log.push(show(vals[i]) + " => " + [guard(inc, vals[i]), guard(dec, vals[i]), guard(inc2, vals[i]),
    guard(dec2, vals[i]), guard(loop, vals[i], 3)].join(" "));
log.push("calls " + B.n);
rvExpect(dec, dec2, inc, inc2, loop);
return log.join("\n");
})()
