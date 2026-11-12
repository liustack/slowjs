// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// rvdiff: generators, async functions and async generators with so many
// captured locals that their var_refs are on the heap (CLW,
// engine/regvm-ops-wide.h). A closure made before the first yield reads and
// writes the locals across suspensions and after the end, a throw() and a
// return() resume into a try, an error escapes, and a generator that never
// ran and one left suspended are dropped. Each is made from source and run
// several times, so that its later runs are in the register loop.
// jsrun: -result
(function () {
  "use strict";
  var out = [];
  function p(x) { out.push(String(x)); }
  function names(n) {
    var r = [], i;
    for (i = 0; i < n; i++)
      r.push("v" + i);
    return r;
  }
  function decl(n) {
    return "var " + names(n).map(function (x, i) { return x + " = " + i; }).join(", ") + ";\n";
  }
  var GenF = Object.getPrototypeOf(function* () {}).constructor;
  var AsyncF = Object.getPrototypeOf(async function () {}).constructor;
  var AsyncGenF = Object.getPrototypeOf(async function* () {}).constructor;
  var N = 400, last = "v" + (N - 1);
  var sum = names(N).join(" + ");
  var gen = GenF("keep", decl(N) +
    "keep.read = function () { return " + sum + "; };\n" +
    "keep.bump = function () { v0++; " + last + "++; };\n" +
    "var got = yield keep.read();\n" +
    "keep.bump(); v1 += got;\n" +
    "try { got = yield v0 + v1; } catch (e) { v2 = 1000; got = 'caught ' + e; }\n" +
    "keep.bump();\n" +
    "try { yield got; } finally { v3 = -1; }\n" +
    "if (got === 'boom') throw new Error('out ' + v0);\n" +
    "return keep.read();");
  var af = AsyncF("keep", decl(N) +
    "keep.read = function () { return " + sum + "; };\n" +
    "keep.bump = function () { v0++; " + last + "++; };\n" +
    "await 0; keep.bump();\n" +
    "var x = await keep.read();\n" +
    "keep.bump();\n" +
    "if (keep.fail) throw new Error('af ' + v0);\n" +
    "return x + ':' + keep.read();");
  var ag = AsyncGenF("keep", decl(N) +
    "keep.read = function () { return " + sum + "; };\n" +
    "yield keep.read(); v0 += 10;\n" +
    "await 0; yield v0 + " + last + ";\n" +
    "return keep.read();");
  function runGen(mode) {
    var keep = {}, g = gen(keep), r = [];
    r.push(g.next().value);
    r.push(g.next(5).value);
    if (mode === 1) {
      r.push(g.throw("t").value);
    } else {
      r.push(g.next(mode === 2 ? "boom" : 7).value);
    }
    if (mode === 3) {
      r.push(JSON.stringify(g.return("early")));
    } else {
      try { r.push(JSON.stringify(g.next())); } catch (e) { r.push(e.message); }
    }
    keep.bump();
    r.push(keep.read());
    return r.join();
  }
  var i;
  for (i = 0; i < 4; i++) {
    p(runGen(i));
    gen({});                       // never run
    (function () { var g = gen({}); g.next(); })(); // left suspended
  }
  var pending = [];
  for (i = 0; i < 3; i++) {
    (function (i) {
      var keep = { fail: i === 2 };
      pending.push(af(keep).then(function (v) { keep.bump(); return v + "/" + keep.read(); },
                                 function (e) { return "rejected " + e.message + "/" + keep.read(); }));
      var g = ag({}), r = [];
      pending.push(g.next().then(function (a) { r.push(a.value); return g.next(); })
        .then(function (a) { r.push(a.value); return g.next(); })
        .then(function (a) { r.push(a.value); return r.join(); }));
    })(i);
  }
  Promise.all(pending).then(function (r) {
    rvExpect(gen, af, ag, runGen);
    globalThis.result = out.join(";") + " | " + r.join(";");
  });
})();
