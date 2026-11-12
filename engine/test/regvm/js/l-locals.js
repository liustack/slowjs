// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// Functions of more than 128 locals, arguments and this together, whose
// first slots no byte operand can name (engine/regvm-ops-wide.h): reads and
// stores of far locals and arguments, set_loc, inc_loc, dec_loc and add_loc
// of every kind of value (a valueOf that runs and throws, strings), let and
// const in their TDZ with the error of the old loop, an assignment to a let
// whose value is read (set_loc_check, near F too), a let in a loop
// (set_loc_uninitialized), far locals captured by closures, this, and a
// named function expression that reads itself, set_loc_uninitialized in a
// function of more than 256 constants, and set_loc_check at the deepest
// point of the stack. The functions are made from source, each called twice
// from a driver: the first call converts it, the second runs it in the
// register loop. Each reads all of its locals at its end: the compiler drops
// a local that is never read.
(function () {
  "use strict";
  function names(prefix, n) {
    var r = [], i;
    for (i = 0; i < n; i++)
      r.push(prefix + i);
    return r;
  }
  // "var v0 = 0, v1 = 1, ...": v0 is the farthest from F
  function decl(kind, prefix, n) {
    return kind + " " + names(prefix, n).map(function (x, i) { return x + " = " + i; }).join(", ") + ";\n";
  }
  function sum(prefix, n) {
    return "(" + names(prefix, n).join(" + ") + ")";
  }
  var fLocals = Function("a", "b", "'use strict';\n" + decl("var", "v", 200) +
    "v0 = a; v1 = b; v199 = 7; v2 = v0 + v1;\n" +
    "v3 = v4 = v0;\n" +                    // set_loc
    "v5 = 'x'; v5 += v0; v5 += 1.5;\n" +   // add_loc of a string
    "v6 = 1; v6++; v6++; v6--;\n" +        // inc_loc and dec_loc
    "v7 = 2147483647; v7++;\n" +           // past an int
    "v8 = 10; v8 += v199; v8 += 0.25;\n" +
    "v9 = b; v9++;\n" +                    // inc_loc of an object (valueOf)
    "v10 = { valueOf: function () { return 100; } }; v10 += v0;\n" +
    "v198 = v0; v197 = v2;\n" +
    "return [v0, v1, v2, v3, v4, v5, v6, v7, v8, v9, v10, v197, v198, v199, v100, " +
    sum("v", 200) + "];");
  // 200 parameters: every argument is far
  var fArgs = Function(names("p", 200).join(", "), "'use strict';\n" +
    "p0 = p0 + 1; p1 = p199; p2++; p3 += 'y';\n" +
    "return [p0, p1, p2, p3, p100, p198, p199];");
  // let and const: the checks of the TDZ on far locals
  var fTdz = Function("which", "'use strict';\n" +
    "var log = [];\n" +
    "if (which === 0) log.push(q0);\n" +   // get_loc_check before the let
    "if (which === 1) q0 = 5;\n" +         // put_loc_check before the let
    "if (which === 2) log.push(c0);\n" +
    decl("let", "q", 150) +
    "const c0 = 9;\n" +
    "q0 = 1; q1 = q0 + 1;\n" +
    "if (which === 3) c0 = 10;\n" +       // assignment to a const
    "log.push(q0, q1, c0, " + sum("q", 150) + ");\n" +
    "return log;");
  // an assignment to a let whose value is read: set_loc_check, which
  // throws before the let, near F and far from it
  var fSetNear = Function("c", "'use strict';\n" +
    "if (c) return (z = 1);\n" +
    "let z = 2;\n" +
    "return (z = 3) + z;");
  var fSetFar = Function("c", "'use strict';\n" +
    "if (c) return (z = 1);\n" +
    "let z = 2;\n" + decl("var", "v", 200) +
    "return (z = 3) + z + " + sum("v", 200) + ";");
  // a let in a loop, captured by a closure made at each turn: each turn has
  // its own binding (set_loc_uninitialized, close_loc). The loop comes
  // before the vars, so that w is far.
  var fLoopLet = Function("n", "'use strict';\n" +
    "var fs = [], i, t = 0;\n" +
    "for (i = 0; i < n; i++) { let w = i * 2; fs.push(function () { return w; }); }\n" +
    decl("var", "v", 200) +
    "for (i = 0; i < fs.length; i++) t = t + fs[i]();\n" +
    "return [t, " + sum("v", 200) + "];");
  // far locals captured by a closure that writes them
  var fCaptured = Function("'use strict';\n" + decl("var", "v", 200) +
    "v0 = 1; v150 = 2;\n" +
    "var set = function (x) { v0 = x; v150 = x + 1; };\n" +
    "set(10);\n" +
    "v1 = v0 + v150;\n" +
    "set(20);\n" +
    "return [v0, v1, v150, " + sum("v", 200) + "];");
  // this, far below F
  var fThis = Function("'use strict';\n" + decl("var", "v", 200) +
    "v0 = this; return [typeof v0, this === v0, this ? this.k : 0, " + sum("v", 200) + "];");
  // a named function expression that reads itself
  var fSelf = Function("'use strict';\n" +
    "return function self(n) {\n" + decl("var", "v", 200) +
    "v0 = n; if (v0 > 0) return self(v0 - 1) + 1; return " + sum("v", 200) + "; };")();
  // a let in a block of a function with more than the 256 constants a byte
  // names: set_loc_uninitialized of a far local takes its constant past
  // them, and the TDZ still holds
  var fUninitK = Function("bad", "'use strict';\nvar s = 0;\n" +
    names("", 260).map(function (x, i) { return "s += " + (1000 + i) + ";"; }).join("") + "\n" +
    "{ if (bad) return q; let q = 'q'; s += q; }\n" + decl("var", "v", 130) +
    "return s + " + sum("v", 130) + ";");
  // an assignment to a let whose value is read, as the last argument of a
  // call that takes the stack to its deepest: set_loc_check there, near F
  // and far from it, before the let (it throws) and after it
  function zeros(n) {
    var r = [], i;
    for (i = 0; i < n; i++)
      r.push(i);
    return r.join(", ");
  }
  function deepSet(n, far) {
    return Function("g", "c", "'use strict';\n" +
      "if (c) return g(" + zeros(n) + ", (z = {}));\n" +
      "let z = 0;\n" + (far ? decl("var", "v", 200) : "") +
      "return g(" + zeros(n) + ", (z = [z])) + z" + (far ? " + " + sum("v", 200) : "") + ";");
  }
  var fDeepSet = [], n;
  // n = 30 and 122: the stack at 32 and at 124, the most there is. A
  // function with far locals has a scratch slot past its stack: 121 there.
  for (n = 26; n <= 30; n++)
    fDeepSet.push(deepSet(n, false), deepSet(n, true));
  for (n = 118; n <= 122; n++)
    fDeepSet.push(deepSet(n, false), deepSet(Math.min(n, 121), true));
  return function () {
    var out = [], k, obj = { valueOf: function () { return 41; } };
    var bad = { valueOf: function () { throw new RangeError("valueOf"); } };
    function at(f) {
      try { out.push(JSON.stringify(f())); } catch (e) { out.push(e.name + ": " + e.message); }
    }
    for (k = 0; k < 2; k++) {
      at(function () { return fLocals(3, obj); });
      at(function () { return fLocals("s", 4); });
      at(function () { return fLocals(1, bad); });
      at(function () { return fArgs(1, 2, 3, 4); });
      at(function () { return fArgs.apply(null, names("a", 200)); });
      at(function () { return fArgs.apply(null, names("b", 210)); });
      at(function () { return fTdz(0); });
      at(function () { return fTdz(1); });
      at(function () { return fTdz(2); });
      at(function () { return fTdz(3); });
      at(function () { return fTdz(4); });
      at(function () { return fSetNear(true); });
      at(function () { return fSetNear(false); });
      at(function () { return fSetFar(true); });
      at(function () { return fSetFar(false); });
      at(function () { return fLoopLet(5); });
      at(function () { return fCaptured(); });
      at(function () { return fThis.call({ k: 3 }); });
      at(function () { return fThis.call(undefined); });
      at(function () { return fSelf(3); });
      at(function () { return fUninitK(false); });
      at(function () { return fUninitK(true); });
      fDeepSet.forEach(function (f) {
        at(function () { return f(function () { return arguments.length + ":" + arguments[arguments.length - 1]; }, true); });
        at(function () { return f(function () { return arguments.length + ":" + arguments[arguments.length - 1]; }, false); });
      });
    }
    rvExpect(fLocals, fArgs, fTdz, fSetNear, fSetFar, fLoopLet, fCaptured, fThis, fSelf, fUninitK);
    rvExpect.apply(null, fDeepSet);
    return out.join("\n");
  };
})()
