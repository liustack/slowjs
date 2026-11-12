// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// Functions whose frame would be larger than an int8_t slot names: the
// closure state of hundreds of captured locals under the temps, with a call
// of many arguments above it, so that the temps would start past slot 127.
// Their var_refs go on the heap (CLW, engine/regvm-ops-wide.h). The sizes
// around the point where they start to, and a frame far below it. Each is
// made from source and called twice from a driver: the first call converts
// it, the second runs it in the register loop. A closure reads and writes
// every local, so that each is captured, and is kept past the frame: it
// reads the values the frame left, after a return and after a throw.
(function () {
  "use strict";
  function names(prefix, n) {
    var r = [], i;
    for (i = 0; i < n; i++)
      r.push(prefix + i);
    return r;
  }
  function decl(prefix, n) {
    return "var " + names(prefix, n).map(function (x, i) { return x + " = " + i; }).join(", ") + ";\n";
  }
  // n captured locals, and a call of m arguments, each an object
  function make(n, m) {
    var objs = [], i;
    for (i = 0; i < m; i++)
      objs.push("{ i: " + i + " }");
    return Function("g", "keep", "'use strict';\n" + decl("v", n) +
      "keep.read = function () { return " + names("v", n).join(" + ") + "; };\n" +
      "keep.bump = function () { v0++; v" + (n - 1) + "++; };\n" +
      "keep.bump();\n" +
      "return [keep.read(), g(" + objs.join(", ") + "), v0, v" + (n - 1) + "];");
  }
  var fs = [make(200, 30), make(240, 8), make(60, 30)], n;
  for (n = 205; n <= 225; n += 2)
    fs.push(make(n, 8));
  return function () {
    var out = [], k;
    function g() {
      if (arguments[0].i === 0 && g.thr)
        throw new RangeError("g");
      return arguments.length + ":" + arguments[arguments.length - 1].i;
    }
    function at(f) {
      try { out.push(JSON.stringify(f())); } catch (e) { out.push(e.name + ": " + e.message); }
    }
    for (k = 0; k < 2; k++) {
      fs.forEach(function (f) {
        var keep = {};
        g.thr = false;
        at(function () { return f(g, keep); });
        keep.bump();
        at(function () { return keep.read(); });
        g.thr = true;
        at(function () { return f(g, keep); });
        keep.bump();
        at(function () { return keep.read(); });
      });
    }
    rvExpect.apply(null, fs);
    return out.join("\n");
  };
})()
