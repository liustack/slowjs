// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// Functions whose stack folds in more than one level of Arrays
// (engine/regvm-fold.h): calls of 32 arguments nested 40 deep, in two levels
// of Arrays of 32 entries. Array destructuring under an expression 130 deep,
// with an iterator that throws (the part of the stack under its catch offset
// frozen into one Array and thawed). A deep expression under a for-in and a
// try, a throw from it caught with the for-in going on, a finally that folds
// too, and a break and a return out of a for-of from under one. A finally
// under 31 and 32 nested for-in, where the return address its gosub pushes
// needs another Array at the finally. 48 nested try and 16 nested for-of
// with calls nested 20 deep inside, the most catch
// offsets a deep stack may have (RV_FOLD_PINS_MAX, engine/regvm-func.h): the
// Arrays of 32 entries do not fit in the temps left, those of 4 in four
// levels do. The functions are made from source, each called twice from a
// driver: the first call converts it, the second runs it in the register
// loop.
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
  function close() { return ")"; }
  // g(0, 1, ..., 30, g(0, ..., 30, ... last)): 32 arguments at each level
  function calls(n, last) {
    return nest(function () { return "g(" + nums(31) + ", "; }, last, close, n);
  }
  // a[0] + (a[1] + (... inner))
  function plus(n, inner) {
    return nest(function (i) { return "a[" + i + "] + ("; }, inner, close, n);
  }
  var fs = [], names = [];
  function add(name, f) { names.push(name); fs.push(f); }
  add("calls 40", make("g, a, last", "return " + calls(40, "last") + ";"));
  // [x, y] = b deep in an expression: the iterator, its next method and its
  // catch offset above 130 entries of the stack
  add("destructure 130", make("a, b", "var x, y;\nreturn " +
                              plus(130, "([x, y] = b, x * 10 + y)") + ";"));
  add("destructure throw 130", make("a, b", "var x, y, r;\ntry { r = " +
                                    plus(130, "([x, y] = b, x * 10 + y)") +
                                    "; } catch (e) { r = 'caught ' + e.message + ' ' + x; }\nreturn r;"));
  // the enumerator of the for-in under the catch offset of the try
  add("for-in 130", make("a, o, bad", "var r = [], k;\nfor (k in o) {\n" +
                         "  try { r.push(k + ':' + " + plus(130, "o[k]()") + "); }\n" +
                         "  catch (e) { r.push(k + ' caught ' + e.message); }\n" +
                         "  finally { r.push(" + plus(130, "k.length") + "); }\n}\nreturn r.join();"));
  // a break and a return out of a for-of from under a deep expression
  add("for-of 130", make("a, A", "var r = [], x, k;\nfor (x of A) {\n" +
                         "  for (k in a) { if (" + plus(130, "x") + " < 0) break; r.push(k); if (k === '2') break; }\n" +
                         "  if (x === 2) return r.join() + ' ' + " + plus(130, "x") + ";\n}\nreturn 'no return';"));
  // a finally under 31 and 32 for-in, so that the gosub pushes its return
  // address where the layout of the depth gets another Array
  [31, 32].forEach(function (n) {
    add("finally " + n, make("a, o", "var r = 0;\n" +
                             list(function (i) { return "for (var k" + i + " in o) "; }, n).join("") +
                             "{\n  try { r += " + plus(130, "1") + "; } finally { r += k0.length; }\n}\nreturn r;"));
  });
  add("try 48", make("g, a, last, bad", "var r;\n" +
                     nest(function () { return "try { "; },
                          "r = " + calls(20, "last") + " + " + plus(130, "bad(r)") + ";",
                          function (i) { return " } catch (e) { r = (r || '') + ' ' + e.message + '" + i + "'; }"; }, 48) +
                     "\nreturn r;"));
  add("for-of 16", make("g, a, last, A", "var r = [];\n" +
                        list(function (i) { return "for (const x" + i + " of A) "; }, 16).join("") +
                        "r.push(" + calls(20, "x0 + x15") + ");\nreturn r.length + ' ' + r[0] + ' ' + r[r.length - 1];"));
  return function () {
    var out = "", k;
    var a = list(function (i) { return (i * 7) % 13 + 1; }, 1000);
    function g() { return arguments.length + ":" + arguments[arguments.length - 1]; }
    var pair = [3, 4];
    var throwing = {};
    throwing[Symbol.iterator] = function () {
      var n = 0;
      return { next: function () { if (++n > 1) throw new RangeError("second"); return { value: 7, done: false }; } };
    };
    var o = {
      one: function () { return 1; },
      bad: function () { throw new TypeError("bad"); },
      three: function () { return 3; }
    };
    function bad(r) { throw new RangeError(String(r).length); }
    var args = {
      "calls": [g, a, "last"], "destructure": [a, pair], "destructure throw": [a, throwing],
      "for-in": [a, o, bad], "for-of": [a, [1, 2, 3]], "try": [g, a, "last", bad],
      "for-of 16": [g, a, "last", [1]], "finally": [a, { x: 1 }]
    };
    for (k = 0; k < 2; k++) {
      fs.forEach(function (f, i) {
        var r, n = names[i];
        try { r = String(f.apply(null, args[n] || args[n.replace(/ [0-9]+$/, "")])); }
        catch (e) { r = e.name + ": " + e.message; }
        out += n + " " + r + "\n";
      });
    }
    rvExpect.apply(null, fs);
    return out;
  };
})()
