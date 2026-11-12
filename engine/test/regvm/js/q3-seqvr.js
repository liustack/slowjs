// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// A strict equality whose right operand is a variable of an enclosing
// function, read right before the compare, and a jump on it: the jump reads
// the variable in place (JSEQVF in engine/regvm-ops-cmp.h). Every pair of a
// set of values, with the left operand a local and a temp, the variable
// changed between runs, as === and !== in an if, as a value, and with the
// variable on the left, which takes the plain jump. Strings that are ropes
// go to the slow side.
(function () {
  "use strict";
  var out = [];
  var long1 = "", long2 = "";
  for (var i = 0; i < 40; i++) { long1 += "abcdefgh"; long2 += "abcdefgh"; }
  var obj = {}, sym = Symbol("s");
  var values = [0, -0, 1, 2147483647, 2147483648, 1.5, NaN, Infinity,
                "abc", "ab" + "c".repeat(1), "", long1, long2, long1 + "x",
                obj, {}, sym, Symbol("s"), undefined, null, true, false,
                10n, 10n * 1n, [1]];
  function make() {
    var V;
    function set(v) { V = v; }
    function jeq(x) { if (x === V) return 1; return 0; }
    function jne(x) { if (x !== V) return 0; return 1; }
    function jnot(x) { if (!(x === V)) return 0; return 1; }
    function val(x) { return x === V ? 1 : 0; }
    function left(x) { if (V === x) return 1; return 0; }
    function id(x) { return x; }
    function tmp(x) { if (id(x) === V) return 1; return 0; }
    function loop(x) { var n = 0; for (var k = 0; k < 3; k++) { if (x === V) n++; } return n; }
    return { set: set, fns: [jeq, jne, jnot, val, left, tmp, loop] };
  }
  var m = make();
  for (var a = 0; a < values.length; a++) {
    m.set(values[a]);
    var row = [];
    for (var b = 0; b < values.length; b++) {
      var s = "";
      for (var f = 0; f < m.fns.length; f++)
        s += m.fns[f](values[b]);
      row.push(s);
    }
    out.push(a + ": " + row.join(" "));
  }
  // the variable changes while the same function runs
  var c = make(), seen = [];
  for (var j = 0; j < 6; j++) {
    c.set(j % 3 === 0 ? "abc" : j);
    seen.push(c.fns[0]("abc") + "" + c.fns[0](j));
  }
  out.push("changes " + seen.join(","));
  return out.join("\n");
})();
