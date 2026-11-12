// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// Functions whose stack goes past 32 entries in the register loop, whose
// temps T[32] up the pcmap names in pcmap_hi (engine/regvm-pass.h): 40
// calls nested as arguments, a method call nested 30 deep, an array of 60
// elements, a throw from the deepest call with every temp below it owned,
// 12 nested for-of loops (their catch offsets past T[32]) left by a break,
// by a throw caught inside them and by one that leaves them all, and 33
// nested try blocks with a finally each, left by a return and by a throw.
// The functions are made from source, each called twice from a driver: the
// first call converts it, the second runs it in the register loop.
(function () {
  "use strict";
  function make(params, body) {
    return Function(params, "'use strict';\n" + body);
  }
  function list(f, n) {
    var r = [], i;
    for (i = 0; i < n; i++)
      r.push(f(i));
    return r;
  }
  function nest(open, inner, close, n) {
    return list(open, n).join("") + inner + list(close, n).join("");
  }
  // g(x0, g(x1, ... g(x39, last)))
  var fCalls = make("g, last", "return " +
                    nest(function (i) { return "g(" + i + " + 0.5, "; }, "last",
                         function () { return ")"; }, 40) + ";");
  var fMethods = make("o, last", "return " +
                      nest(function (i) { return "o.m('" + i + "', "; }, "last",
                           function () { return ")"; }, 30) + ";");
  var fArray = make("a", "return [" + list(function (i) { return "a + " + i; }, 60).join(", ") + "];");
  // 12 for-of loops, each holding 3 entries
  var loops = list(function (i) { return "x" + i; }, 12);
  var fForOf = make("A, mode, log",
                    "var n = 0;\n" +
                    "out: " + loops.map(function (x) { return "for (const " + x + " of A) "; }).join("") +
                    "{\n" +
                    "  n++;\n" +
                    "  try { if (mode === 'catch' && n === 2) throw new Error('inner ' + n); }\n" +
                    "  catch (e) { log.push(e.message); }\n" +
                    "  if (mode === 'break' && n === 3) break out;\n" +
                    "  if (mode === 'throw' && n === 2) throw new RangeError('out of ' + (" +
                    loops.join(" + ") + "));\n" +
                    "  if (n === 5) return n;\n" +
                    "}\n" +
                    "return n;");
  // 33 nested try blocks with a finally each
  var fFinally = make("mode, log",
                      nest(function (i) { return "try {\n"; },
                           "if (mode === 'return') return 'returned';\n" +
                           "if (mode === 'throw') throw new Error('deep');\n",
                           function (i) { return "} finally { log.push(" + (32 - i) + "); }\n"; }, 33) +
                      "return 'end';");
  return function () {
    var out = [], k, log = [];
    function at(f) {
      try { out.push(JSON.stringify(f())); } catch (e) { out.push(e.name + ": " + e.message); }
    }
    function g(x, y) { return x + y; }
    function bad(x, y) { if (x === 39.5) throw new TypeError("deepest"); return x + y; }
    var o = { m: function (x, y) { return x + y; } };
    // a fresh iterator for each loop, whose close is logged
    function iterable(n) {
      var made = 0;
      return { [Symbol.iterator]: function () {
        var i = 0, tag = made++;
        return { next: function () { i++; return { done: i > n, value: i }; },
                 return: function () { log.push("close " + tag); return {}; } };
      } };
    }
    for (k = 0; k < 2; k++) {
      at(function () { return fCalls(g, 1); });
      at(function () { return fCalls(g, "s"); });
      at(function () { return fCalls(bad, 1); });
      at(function () { return fMethods(o, "end"); });
      at(function () { return fMethods({ m: function () { throw new Error("m"); } }, 0); });
      at(function () { return fArray(1); });
      at(function () { return fArray("a"); });
      at(function () { return fForOf([1, 2], "none", log); });
      at(function () { return fForOf([1, 2], "catch", log); });
      at(function () { return fForOf([1, 2], "break", log); });
      at(function () { return fForOf([1, 2], "throw", log); });
      at(function () { return fForOf(iterable(2), "break", log); });
      at(function () { return fForOf(iterable(2), "throw", log); });
      at(function () { return fForOf(iterable(2), "none", log); });
      at(function () { return fFinally("return", log); });
      at(function () { return fFinally("throw", log); });
      at(function () { return fFinally("none", log); });
      out.push(log.join(","));
      log.length = 0;
    }
    rvExpect(fCalls, fMethods, fArray, fForOf, fFinally);
    return out.join("\n");
  };
})()
