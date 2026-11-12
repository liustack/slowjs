// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// Functions with more constants than a byte of an instruction can name
// (engine/regvm-pass.h): 300 of every kind (ints, floats, strings, short
// BigInts, null, booleans) used in every form that takes a constant (an
// operand of add and of a compare, a store to a local and to a field, a call
// argument, a return value), 5,000, and 70,000, past a 16-bit index. The
// functions are made from source, each called twice from a driver: the first
// call converts it, the second runs it in the register loop.
(function () {
  "use strict";
  function make(body) {
    return Function("g", "o", "x", "'use strict';\n" + body);
  }
  // 300 constants: the first 256 are named by a byte, the rest are not
  var mixed = [];
  var i;
  mixed.push("var s = 0, t = '', n = 0, b = 0n, v;");
  for (i = 0; i < 60; i++) {
    mixed.push("s = s + " + (1000 + i) + ";");
    mixed.push("s = s + " + (i + 0.25) + ";");
    mixed.push("t = t + 'k" + i + "';");
    mixed.push("if (x === 'k" + i + "') n = n + 1;");
  }
  for (i = 0; i < 40; i++)
    mixed.push("b = b + " + (i + 7) + "n;");
  // past the 256th: each form once more with a new constant
  mixed.push("if (x === 'far') n = n + 100;");
  mixed.push("v = 'far store';");
  mixed.push("o.p = 'far field';");
  mixed.push("o.q = 4.75;");
  mixed.push("g(5005, 'far arg', null, true, 5.125);");
  mixed.push("if (x === 7777) return 'far return';");
  mixed.push("if (x === null) return null;");
  mixed.push("return [s, t.length, n, String(b), v];");
  var fMixed = make(mixed.join("\n"));

  // 5,000 ints and strings in a row
  var many = ["var s = 0, t = '';"];
  for (i = 0; i < 2500; i++) {
    many.push("s = s + " + (100000 + i) + ";");
    many.push("t = t + 'm" + i + "';");
  }
  many.push("return [s, t.length, t.slice(-12)];");
  var fMany = make(many.join("\n"));

  // 70,000 ints: an index past 65,535
  var huge = ["var s = 0;"];
  for (i = 0; i < 70000; i++)
    huge.push("s = s + " + (1000000 + i) + ";");
  huge.push("return s;");
  var fHuge = make(huge.join("\n"));

  return function () {
    var out = [], calls = [], obj = {}, k;
    function g() { calls.push(Array.prototype.slice.call(arguments).join(",")); }
    for (k = 0; k < 2; k++) {
      out.push(JSON.stringify(fMixed(g, obj, "k3")));
      out.push(JSON.stringify(fMixed(g, obj, "far")));
      out.push(JSON.stringify(fMixed(g, obj, 7777)));
      out.push(JSON.stringify(fMixed(g, obj, null)));
      out.push(JSON.stringify(fMany()));
      out.push(fHuge());
    }
    out.push(JSON.stringify(obj), calls.join("|"));
    rvExpect(fMixed, fMany, fHuge);
    return out.join("\n");
  };
})()
