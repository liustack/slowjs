// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// Functions whose stack is deeper than the temps of the register loop, which
// fold it (engine/regvm-fold.h): parentheses nested 130, 300 and 600 deep,
// calls of 31 arguments nested 20 deep, array literals nested 20 deep, a
// ?:, && and ?. at every level (leaders deep in the stack), the order in
// which getters and valueOf run, a throw from the deepest level caught
// outside and inside the function, a loop around the expression, a for-of
// loop and a try under it (a base depth past 0), 300 captured variables
// with the stack folded, and a generator whose stack folds too
// (z2-gen-fold.js has those across their suspensions). The functions are made from source, each called
// twice from a driver: the first call converts it, the second runs it in
// the register loop.
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
  function nums(n) { return list(function (i) { return String(i); }, n).join(", "); }
  // a[0] - (a[1] - (... (a[n - 1] - a[n])))
  function minus(n) {
    return make("a", "return " + nest(function (i) { return "a[" + i + "] - ("; }, "a[" + n + "]",
                                      function () { return ")"; }, n) + ";");
  }
  var fs = [], names = [];
  function add(name, f) { names.push(name); fs.push(f); }
  add("minus 130", minus(130));
  add("minus 300", minus(300));
  add("minus 600", minus(600));
  // strings, so that the order of the operands shows
  add("concat 200", make("s", "return " + nest(function (i) { return "s[" + (i % 26) + "] + ("; }, "'!'",
                                               function () { return ")"; }, 200) + ";"));
  // g(0, 1, ..., 30, g(0, ..., 30, ... last))
  add("calls 20", make("g, last", "return " + nest(function () { return "g(" + nums(31) + ", "; }, "last",
                                                    function () { return ")"; }, 20) + ";"));
  add("method calls 20", make("o, last", "return " + nest(function () { return "o.m(" + nums(30) + ", "; },
                                                          "last", function () { return ")"; }, 20) + ";"));
  add("new 20", make("C, last", "return " + nest(function () { return "new C(" + nums(31) + ", "; }, "last",
                                                  function () { return ").v"; }, 20) + ";"));
  add("arrays 20", make("x", "return JSON.stringify(" +
                        nest(function () { return "[" + nums(31) + ", "; }, "x", function () { return "]"; }, 20) +
                        ");"));
  // a jump at every level: the leaders after it are deep in the stack
  add("cond 200", make("a, c", "return " + nest(function (i) { return "a[" + i + "] + (c[" + i + "] ? "; }, "0",
                                                function (i) { return " : -" + (199 - i) + ")"; }, 200) + ";"));
  add("and 200", make("a, c", "return " + nest(function (i) { return "a[" + i + "] + (c[" + i + "] && "; },
                                               "1", function () { return ")"; }, 200) + ";"));
  add("optional 150", make("o", "return " + nest(function (i) { return "o.v + (o.p" + (i % 3) + "?.v + "; },
                                                 "1", function () { return ")"; }, 150) + ";"));
  // getters and valueOf log the order they run in
  add("order 200", make("o, log", "return " + nest(function (i) { return "o.g" + (i % 5) + " * ("; },
                                                   "o.last", function () { return ")"; }, 200) + ";"));
  // a throw from the deepest level: the folded Arrays are freed
  add("throw 300", make("a, bad", "return " + nest(function (i) { return "a[" + i + "] + ("; }, "bad()",
                                                   function () { return ")"; }, 300) + ";"));
  add("catch 300", make("a, bad", "var r;\ntry { r = " +
                        nest(function (i) { return "a[" + i + "] + ("; }, "bad()", function () { return ")"; }, 300) +
                        "; } catch (e) { r = 'caught ' + e.message; }\nreturn r;"));
  // the expression in a loop, so its leaders are reached again
  add("loop 150", make("a, n", "var r = 0, i;\nfor (i = 0; i < n; i++)\n  r += " +
                       nest(function (j) { return "a[" + j + "] * (i + "; }, "1", function () { return ")"; }, 150) +
                       " % 1000;\nreturn r;"));
  // a base depth past 0: a for-of loop and a try under the deep expression
  add("for-of 200", make("A, a", "var r = [];\nfor (const x of A) {\n  try {\n    r.push(" +
                         nest(function (i) { return "a[" + i + "] - (x + "; }, "x", function () { return ")"; }, 200) +
                         ");\n  } catch (e) { r.push(e.message); }\n}\nreturn r.join();"));
  // 300 captured variables with the stack folded: their var_refs on the heap
  add("closures 300", make("a", "var " + list(function (i) { return "v" + i + " = " + i; }, 300).join(", ") +
                           ";\nvar f = function () { return v0 + v299; };\nreturn f() + " +
                           nest(function (i) { return "v" + i + " - ("; }, "f()", function () { return ")"; }, 200) +
                           ";"));
  var gen = make("a","return (function* () { yield " +
                 nest(function (i) { return "a[" + i + "] + ("; }, "0", function () { return ")"; }, 150) +
                 "; })().next().value;");
  return function () {
    var out = "", k, log = [];
    var a = list(function (i) { return (i * 7) % 13 + 1; }, 1000);
    var c = list(function (i) { return i % 7 !== 3; }, 1000);
    function g() { return arguments.length + ":" + arguments[arguments.length - 1]; }
    var o = { m: function () { return arguments.length + ":" + arguments[arguments.length - 1]; } };
    function C() { this.v = arguments.length + ":" + arguments[arguments.length - 1]; }
    var getters = { last: 1 }, i;
    for (i = 0; i < 5; i++)
      (function (i) {
        Object.defineProperty(getters, "g" + i, { get: function () { log.push(i); return { valueOf: function () { log.push("v" + i); return 1 + (i & 1); } }; } });
      })(i);
    var opt = { v: 1, p0: { v: 2 }, p1: null, p2: { v: 3 } };
    function bad() { throw new RangeError("deep"); }
    var args = {
      "minus": [a], "concat": ["abcdefghijklmnopqrstuvwxyz"], "calls": [g, "last"], "method calls": [o, "end"],
      "new": [C, "inner"], "arrays": ["x"], "cond": [a, c], "and": [a, c], "optional": [opt],
      "order": [getters, log], "throw": [a, bad], "catch": [a, bad], "loop": [a, 3], "for-of": [[1, 2, 3], a],
      "closures": [a]
    };
    for (k = 0; k < 2; k++) {
      fs.forEach(function (f, i) {
        var r;
        log.length = 0;
        try { r = String(f.apply(null, args[names[i].replace(/ [0-9]+$/, "")])); }
        catch (e) { r = e.name + ": " + e.message; }
        out += names[i] + " " + r + (log.length ? " " + log.join("") : "") + "\n";
      });
      out += "generator " + gen(a) + "\n";
    }
    rvExpect.apply(null, fs);
    return out;
  };
})()
