// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// x++ and x-- on locals, arguments and temps, on every kind of value: they
// convert to a number (or a BigInt) first, unlike x + 1. Each function is
// called once with an int first, so that it converts, then with the rest.
(function () { "use strict";
  var B = { n: 0 };
  function inc(p) { var v = p; B.n = B.n + 1; v++; return v; }
  function dec(p) { var v = p; B.n = B.n + 1; v--; return v; }
  function pinc(p) { var v = p; B.n = B.n + 1; var w = v++; return [w, v]; }
  function pdec(p) { var v = p; B.n = B.n + 1; var w = v--; return [w, v]; }
  function preinc(p) { var v = p; B.n = B.n + 1; return [++v, v]; }
  function predec(p) { var v = p; B.n = B.n + 1; return [--v, v]; }
  function arg(p) { B.n = B.n + 1; p++; return p; }
  function loop(p) { var v = p; B.n = B.n + 1; for (var i = 0; i < 3; i++) v++; return v; }
  var fs = [inc, dec, pinc, pdec, preinc, predec, arg, loop];
  var calls = 0;
  var vals = [
    "1", "", " 2 ", "x", "0x10", "-0", 1.5, -1, 0, -0, 2147483647, -2147483648,
    NaN, Infinity, null, undefined, true, false, 1n, -1n,
    { valueOf: function () { calls++; return "1"; } },
    { valueOf: function () { calls++; return 2n; } },
    { toString: function () { calls++; return "3"; } },
    [5], [], Symbol("s"),
  ];
  function show(v) {
    if (Array.isArray(v)) return "[" + v.map(show).join(",") + "]";
    if (typeof v === "number" && v === 0 && 1 / v < 0) return "-0";
    if (typeof v === "symbol") return "symbol";
    return typeof v + ":" + String(v);
  }
  var out = [];
  for (var k = 0; k < fs.length; k++) fs[k](1);
  for (k = 0; k < fs.length; k++) {
    for (var j = 0; j < vals.length; j++) {
      var r;
      try { r = show(fs[k](vals[j])); } catch (e) { r = e.name + ": " + e.message; }
      out.push(fs[k].name + " " + j + " " + r);
    }
  }
  out.push("valueOf and toString calls " + calls);
  return out.join("\n");
})()
