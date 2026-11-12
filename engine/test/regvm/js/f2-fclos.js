// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// Closures whose index in the constants of their function is past 65,535,
// which FCLOS cannot name with its 16 bits: OLD makes them
// (engine/regvm-ops-closure.h). A function expression and an arrow, each
// reading a captured local and a closure variable, a closure called after
// the function returned, and one made in a loop. The function is made from
// source and called twice from a driver: the first call converts it, the
// second runs it in the register loop.
(function () {
  "use strict";
  var i, body = ["var x = 0, y = a[1], fs = [], k;"];
  for (i = 0; i < 65537; i++)
    body.push("x = " + i + ".5;");
  body.push("for (k = 0; k < 3; k++) fs.push(() => x + k * y);");
  body.push("return [(function () { return x + y; })(), (() => x * y)(), fs.map(function (f) { return f(); }).join(), function () { return x - y; }];");
  var f = Function("a", "'use strict';\n" + body.join("\n"));
  return function () {
    var out = "", n, r;
    for (n = 0; n < 2; n++) {
      r = f([1, 2]);
      out += r.slice(0, 3).join(" ") + " " + r[3]() + "\n";
    }
    rvExpect(f);
    return out;
  };
})()
