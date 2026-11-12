// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// The relational compares (<, <=, > and >=) of two numbers that are not both
// ints, made in place by the compares into a bool and the compare jumps
// (engine/regvm-ops-cmp.h), as registers and as constants: NaN on each side,
// -0 against 0, an int against a float above 2^53 and floats around an int.
// Then operands that are not numbers, which take rv_binop_slow: a BigInt, a
// string, a boolean, null, undefined and an object with valueOf.
(function () {
  "use strict";
  var vals = [0, -0, 1, -1, 2147483647, -2147483648, 0.5, -0.5, 1.0000000001,
              2147483648, 9007199254740992, 9007199254740993, 18014398509481984,
              -9007199254740994, 1e308, -1e308, Infinity, -Infinity, NaN];
  function lt(a, b) { return a < b; }
  function lte(a, b) { return a <= b; }
  function gt(a, b) { return a > b; }
  function gte(a, b) { return a >= b; }
  function jlt(a, b) { if (a < b) return 1; return 0; }
  function jlte(a, b) { if (a <= b) return 1; return 0; }
  function jgt(a, b) { if (a > b) return 1; return 0; }
  function jgte(a, b) { if (a >= b) return 1; return 0; }
  function jnlt(a, b) { if (!(a < b)) return 1; return 0; }
  function jngte(a, b) { if (!(a >= b)) return 1; return 0; }
  var ops = [lt, lte, gt, gte, jlt, jlte, jgt, jgte, jnlt, jngte];
  var out = [];
  for (var o = 0; o < ops.length; o++) {
    var row = "";
    for (var i = 0; i < vals.length; i++)
      for (var j = 0; j < vals.length; j++)
        row += +ops[o](vals[i], vals[j]);
    out.push(ops[o].name + ":" + row);
  }
  // a constant on the right: a float, -0, an int above a float, 2^53 + 2
  function ks(a) {
    var r = [a < 0.5, a <= -0, a > 2147483648, a >= 9007199254740994, a < 3, a >= -1.5];
    if (a < 1.5) r.push("j1"); else r.push("n1");
    if (a >= 9007199254740994) r.push("j2"); else r.push("n2");
    if (a > -0) r.push("j3"); else r.push("n3");
    return r.map(function (x) { return x === true ? 1 : x === false ? 0 : x; }).join("");
  }
  for (var i = 0; i < vals.length; i++)
    out.push("k " + vals[i] + ":" + ks(vals[i]));
  // a loop with a float bound, as a layout loop makes it
  var n = 0;
  for (var x = 0.25; x < 100.5; x += 0.75) n++;
  for (var y = 100; y >= 0.5; y -= 1.5) n++;
  out.push("loop " + n);
  // not numbers: the slow side, rv_binop_slow
  var others = [10n, -3n, 2n ** 70n, "2", "abc", "", true, null, undefined,
                { valueOf: function () { return 2.5; } }];
  var nums = [2.5, NaN, -0, 9007199254740993, 3];
  for (var o = 0; o < ops.length; o++) {
    var row = "";
    for (var i = 0; i < others.length; i++)
      for (var j = 0; j < nums.length; j++)
        row += +ops[o](others[i], nums[j]) + "" + +ops[o](nums[j], others[i]);
    out.push("slow " + ops[o].name + ":" + row);
  }
  out.push("bigint " + [lt(1n, 2n), gte(2n ** 64n, 2n ** 64n), gt("b", "a"), lt("10", "9")].join(","));
  return out.join("\n");
})();
