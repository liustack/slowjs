// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// Functions of more than 32 KB of register code (engine/regvm-pass.h,
// rv_islands): straight code, and every kind of jump across a body of more
// than 32 KB, forward and back: a loop with break and continue, if and
// else, the compare and branch of lt and ===, && and || (a branch that keeps
// its value), typeof and == null, the cases of a switch on strings, a
// catch, a finally reached by return and by the end of its try, for-of and
// for-in. The functions are made from source, each called twice from a
// driver: the first call converts it, the second runs it in the register
// loop.
(function () {
  "use strict";
  function make(params, body) {
    return Function(params, "'use strict';\n" + body);
  }
  function repeat(line, count) {
    var r = [], i;
    for (i = 0; i < count; i++)
      r.push(line);
    return r.join("\n");
  }
  // about 36 KB of register code
  var BIG = repeat("s = s + a; t = t - b; s = s + t;", 3000);
  var fStraight = make("a, b", "var s = 0, t = 0;\n" +
                       repeat("s = s + a; t = t - b; s = s + t;", 4000) +
                       "\nreturn [s, t];");
  var fLoopFirst = make("a, b, n", "var s = 0, t = 0, i;\n" +
                        "for (i = 0; i < n; i++) s = s + i;\n" +
                        repeat("s = s + a; t = t - b;", 6000) +
                        "\nreturn [s, t];");
  // a loop around the body: the branch out of it and the jump back are far,
  // so are continue and break
  var fLoop = make("a, b, n", "var s = 0, t = 0, i, k = 0;\n" +
                   "for (i = 0; i < n; i++) {\n" +
                   "  if (i === 1) continue;\n" +
                   "  k++;\n" + BIG + "\n" +
                   "  if (i === 3) break;\n" +
                   "}\n" +
                   "return [s, t, i, k];");
  // a while loop whose test is at its end
  var fDoWhile = make("a, b, n", "var s = 0, t = 0, i = 0;\n" +
                      "do {\n" + BIG + "\n i++;\n} while (i < n);\n" +
                      "return [s, t, i];");
  var fIf = make("a, b", "var s = 0, t = 0;\n" +
                 "if (a > 0) {\n" + BIG + "\n} else {\n" + BIG + "\n s = -s;\n}\n" +
                 "if (a === 2) {\n" + BIG + "\n}\n" +
                 "return [s, t];");
  // && and || whose right side is long: the branch keeps its value
  var fAndOr = make("a, b", "var s = 0, t = 0, x, y;\n" +
                    "x = a && (" + repeat("s = s + a, t = t - b, s = s + t,", 2500) + " 'right');\n" +
                    "y = b || (" + repeat("s = s + a, t = t - b, s = s + t,", 2500) + " 'right');\n" +
                    "return [s, t, x, y];");
  var fTest = make("a, b", "var s = 0, t = 0;\n" +
                   "if (typeof a === 'string') {\n" + BIG + "\n}\n" +
                   "if (b == null) {\n" + BIG + "\n}\n" +
                   "return [s, t, typeof a];");
  // the cases of a switch on strings: the targets after the first are far
  var fSwitch = make("a, b, k", "var s = 0, t = 0;\n" +
                     "switch (k) {\n" +
                     "case 'x':\n" + BIG + "\n break;\n" +
                     "case 'y':\n" + BIG + "\n s = s + 1000;\n" +
                     "case 'z':\n t = t + 7;\n break;\n" +
                     "default:\n s = -1;\n" +
                     "}\n" +
                     "return [s, t];");
  // a catch past the try, and a throw inside it from a call deep in it
  var fTry = make("a, b, f", "var s = 0, t = 0;\n" +
                  "try {\n" + BIG + "\n f(s);\n" + BIG + "\n} catch (e) {\n" +
                  " return ['caught', e.message, s, t];\n}\n" +
                  "return [s, t];");
  // a finally reached by a return at the top of its try, by a throw and by
  // the end of the try
  var fFinally = make("a, b, log", "var s = 0, t = 0;\n" +
                      "try {\n" +
                      "  if (a === 0) return 'early';\n" +
                      "  if (a === -1) throw new Error('top');\n" +
                      BIG + "\n" +
                      "} finally {\n" +
                      "  log.push('finally ' + s);\n" +
                      "}\n" +
                      "return [s, t];");
  var fForOf = make("a, b, arr", "var s = 0, t = 0, n = 0;\n" +
                    "for (const x of arr) {\n" +
                    "  if (x === 'skip') continue;\n" +
                    "  if (x === 'stop') break;\n" +
                    "  n = n + x;\n" + BIG + "\n" +
                    "}\n" +
                    "return [s, t, n];");
  var fForIn = make("a, b, o", "var s = 0, t = 0, ks = [];\n" +
                    "for (const k in o) {\n" +
                    "  if (k === 'skip') continue;\n" +
                    "  ks.push(k);\n" + BIG + "\n" +
                    "}\n" +
                    "return [s, t, ks];");
  return function () {
    var out = [], k, log = [];
    function at(f) {
      try { out.push(JSON.stringify(f())); } catch (e) { out.push(e.name + ": " + e.message); }
    }
    for (k = 0; k < 2; k++) {
      at(function () { return fStraight(1, 2); });
      at(function () { return fStraight(0.5, "x"); });
      at(function () { return fLoopFirst(3, 1, 10); });
      at(function () { return fLoop(1, 2, 0); });
      at(function () { return fLoop(1, 2, 2); });
      at(function () { return fLoop(1, 2, 10); });
      at(function () { return fDoWhile(1, 2, 3); });
      at(function () { return fIf(1, 2); });
      at(function () { return fIf(-1, 2); });
      at(function () { return fIf(2, 2); });
      at(function () { return fAndOr(1, 0); });
      at(function () { return fAndOr(0, 2); });
      at(function () { return fTest("s", null); });
      at(function () { return fTest(1, 2); });
      at(function () { return fSwitch(1, 2, "x"); });
      at(function () { return fSwitch(1, 2, "y"); });
      at(function () { return fSwitch(1, 2, "z"); });
      at(function () { return fSwitch(1, 2, "w"); });
      at(function () { return fTry(1, 2, function () {}); });
      at(function () { return fTry(1, 2, function (v) { throw new Error("at " + v); }); });
      at(function () { return fFinally(0, 2, log); });
      at(function () { return fFinally(-1, 2, log); });
      at(function () { return fFinally(1, 2, log); });
      at(function () { return fForOf(1, 2, [1, "skip", 2, 3]); });
      at(function () { return fForOf(1, 2, [1, "stop", 2]); });
      at(function () { return fForIn(1, 2, { p: 1, skip: 2, q: 3 }); });
    }
    out.push(log.join(","));
    rvExpect(fStraight, fLoopFirst, fLoop, fDoWhile, fIf, fAndOr, fTest, fSwitch,
             fTry, fFinally, fForOf, fForIn);
    return out.join("\n");
  };
})()
