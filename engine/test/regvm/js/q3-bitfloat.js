// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// The six bit ops (&, |, ^, <<, >>, >>>) of two numbers that are not both
// ints, done in place by rv_bin_other (engine/regvm-ops-arith.h): a float
// above 2^31, a negative float, floats out of the int32 range and past 2^53,
// NaN, the infinities and -0, on either side, as registers and as
// constants. Then operands that are not numbers, which take rv_bin_slow: a
// string, a boolean, null, undefined, an object with valueOf, a BigInt pair
// and a BigInt mixed with a number (which throws).
(function () {
  "use strict";
  var vals = [0, 1, -1, 7, 31, 32, 2147483647, -2147483648,
              2147483653, 4294967295, 4294967296, 4294967301, -3.5, -2147483649.7,
              -4294967297, 1.9, -0.5, -0, 9007199254740993, 1e20, -1e20, 1.5e300,
              NaN, Infinity, -Infinity];
  function fmt(v) {
    return v === 0 && 1 / v < 0 ? "-0" : String(v);
  }
  function and(a, b) { return a & b; }
  function or(a, b) { return a | b; }
  function xor(a, b) { return a ^ b; }
  function shl(a, b) { return a << b; }
  function sar(a, b) { return a >> b; }
  function shr(a, b) { return a >>> b; }
  var ops = [and, or, xor, shl, sar, shr];
  var out = [];
  for (var o = 0; o < ops.length; o++) {
    var row = [];
    for (var i = 0; i < vals.length; i++)
      for (var j = 0; j < vals.length; j++)
        row.push(fmt(ops[o](vals[i], vals[j])));
    out.push(ops[o].name + ":" + row.join(","));
  }
  // a constant on the right (ANDK and the others), a float in the register
  function ks(a) {
    return [a & 4294967295, a | 2147483653, a ^ -3.5, a << 33.7, a >> 4294967297, a >>> -1,
            a & 0xffff, a ^ 0x80000000, a >>> 0].map(fmt).join(",");
  }
  for (var i = 0; i < vals.length; i++)
    out.push("k " + fmt(vals[i]) + ":" + ks(vals[i]));
  // a hash of uint32 floats, as MUI's class names make it
  function hash(s) {
    var h = 2166136261;
    for (var i = 0; i < s.length; i++) {
      h ^= s.charCodeAt(i);
      h = (h * 16777619) >>> 0;
    }
    return h;
  }
  out.push("hash " + hash("css-1a2b3c") + " " + hash("MuiButton-root") + " " + hash(""));
  // not numbers: the slow side, rv_bin_slow
  var others = ["12", "x", true, null, undefined, { valueOf: function () { return 2147483653; } }];
  for (var o = 0; o < ops.length; o++) {
    var row = [];
    for (var i = 0; i < others.length; i++)
      row.push(fmt(ops[o](others[i], 3.5)), fmt(ops[o](-3.5, others[i])));
    out.push("slow " + ops[o].name + ":" + row.join(","));
  }
  out.push("bigint " + [and(12n, 10n), or(12n, 10n), xor(12n, 10n), shl(3n, 4n), sar(-256n, 4n)].join(","));
  try { xor(1n, 2.5); out.push("mixed: no throw"); }
  catch (e) { out.push("mixed:" + e.constructor.name); }
  try { shr(8n, 1n); out.push("shr bigint: no throw"); }
  catch (e) { out.push("shr bigint:" + e.constructor.name); }
  return out.join("\n");
})();
