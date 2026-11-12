// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// rvdiff: generators, async functions and async generators whose stack is
// deeper than the temps, which fold it (engine/regvm-fold.h) and suspend
// with ranges of it folded (engine/regvm-ops-gen.h): expressions nested 130
// and 300 deep with a yield or an await in the middle and at the deepest
// level, a next(), a throw() and a return() while suspended there, a
// rejected await, an error that escapes, a state dropped while suspended,
// and a yield at each depth from 28 to 70, across the edges of the ranges.
// Each is made from source and run several times, so that its later runs
// are in the register loop.
// jsrun: -result
(function () {
  "use strict";
  var out = [];
  function p(x) { out.push(String(x)); }
  var GenF = Object.getPrototypeOf(function* () {}).constructor;
  var AsyncF = Object.getPrototypeOf(async function () {}).constructor;
  var AsyncGenF = Object.getPrototypeOf(async function* () {}).constructor;
  // x + (x + (... mid + (... (x + last) ...))), depth levels, mid at level k
  function deep(depth, k, mid, last) {
    var s = "", i;
    for (i = 0; i < depth; i++)
      s += i === k ? mid + " + (" : "x + (";
    s += last;
    for (i = 0; i < depth; i++)
      s += ")";
    return s;
  }
  function tryBody(e) {
    return "try { return " + e + "; }\n" +
      "catch (err) { log.push('caught ' + err); return 'c'; }\n" +
      "finally { log.push('fin'); }";
  }
  function show(v) {
    return typeof v === "string" ? v.length + ":" + v.slice(0, 6) + ":" + v.slice(-6) : String(v);
  }

  var gens = [], afs = [], ags = [], sweep = [];
  [130, 300].forEach(function (d) {
    var mid = d >> 1;
    // a try around (base depth past 0) and none (the error escapes)
    gens.push(GenF("x, log", tryBody(deep(d, mid, "(yield x)", "(yield 'deepest')"))));
    gens.push(GenF("x, log", "return " + deep(d, mid, "(yield x)", "x") + ";"));
    afs.push(AsyncF("x, log, q", tryBody(deep(d, mid, "(await q)", "(await 'deepest')"))));
    afs.push(AsyncF("x, log, q", "return " + deep(d, mid, "(await q)", "x") + ";"));
    ags.push(AsyncGenF("x, log, q", tryBody(deep(d, mid, "(yield await q)", "(await (yield x))"))));
  });
  for (var k = 28; k <= 70; k++)
    sweep.push(GenF("x", "return " + deep(130, k, "(yield x)", "x") + ";"));

  function runGen(g, mode) {
    var log = [], it = g("ab", log), r = [];
    try {
      r.push(show(it.next().value));
      if (mode === 0) {
        r.push(show(it.next("Y").value));
        r.push(show(it.next("Z").value));
      } else if (mode === 1) {
        r.push(show(it.throw("t").value));
      } else if (mode === 2) {
        r.push(show(it.return("r").value));
      }
      // mode 3: left suspended, dropped
    } catch (e) {
      r.push("threw " + e);
    }
    return r.join(",") + " " + log.join("/");
  }

  function runAf(f, mode) {
    var log = [];
    var q = mode === 1 ? Promise.reject("rej") : Promise.resolve("Q");
    return f("ab", log, q).then(function (v) { return show(v) + " " + log.join("/"); },
                                function (e) { return "rejected " + e + " " + log.join("/"); });
  }

  async function runAg(g, mode) {
    var log = [], it = g("ab", log, mode === 1 ? Promise.reject("rej") : Promise.resolve("Q")), r = [];
    try {
      r.push(show((await it.next()).value));
      if (mode === 0) {
        r.push(show((await it.next("Y")).value));
        r.push(show((await it.next("Z")).value));
      } else if (mode === 2) {
        r.push(show((await it.return("r")).value));
      } else if (mode === 3) {
        r.push(show((await it.throw("t")).value));
      }
    } catch (e) {
      r.push("threw " + e);
    }
    return r.join(",") + " " + log.join("/");
  }

  async function main() {
    var round, i, m;
    for (round = 0; round < 3; round++) {
      for (i = 0; i < gens.length; i++)
        for (m = 0; m < 4; m++)
          p("gen " + i + " " + m + " " + runGen(gens[i], m));
      for (i = 0; i < afs.length; i++)
        for (m = 0; m < 2; m++)
          p("af " + i + " " + m + " " + await runAf(afs[i], m));
      for (i = 0; i < ags.length; i++)
        for (m = 0; m < 4; m++)
          p("ag " + i + " " + m + " " + await runAg(ags[i], m));
      for (i = 0; i < sweep.length; i++)
        p("sweep " + (28 + i) + " " + runGen(sweep[i], round === 2 ? 1 : 0));
    }
    rvExpect.apply(null, gens.concat(afs, ags, sweep));
    return out.join("\n");
  }
  main().then(function (r) { globalThis.result = r; },
              function (e) { globalThis.result = "main failed " + e; });
})();
