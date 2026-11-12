// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// rvdiff: generators, async functions and async generators whose stack folds
// in levels (engine/regvm-fold.h), suspended with it folded
// (engine/regvm-ops-gen.h): expressions nested 130, 300 and 600 deep with a
// yield or an await at a quarter, half, three quarters and the end of the
// depth, a yield* and a destructuring whose default yields or awaits in the
// middle (an iterator over the folded stack, which freezes what lies under
// it), under a try and in its finally, in the body of a for-of and of a for
// await, with closures over the locals; a next(), a throw() and a return()
// at each suspension, a rejected await, an error that escapes, and resumes
// at the bottom of a recursion of large frames, so that some run on the next
// segment of the register stack and the stack settles after them. The
// RV_FOLD_TEST builds (2 and 3) fold the same code in more levels. Each
// function is made from source and run several times, so that its later runs
// are in the register loop.
// jsrun: -result
(function () {
  "use strict";
  var out = [];
  function p(x) { out.push(String(x)); }
  var GenF = Object.getPrototypeOf(function* () {}).constructor;
  var AsyncF = Object.getPrototypeOf(async function () {}).constructor;
  var AsyncGenF = Object.getPrototypeOf(async function* () {}).constructor;
  // x + (x + (... at[k] + (... (x + last) ...))) with depth levels
  function deep(depth, at, last) {
    var s = "", i;
    for (i = 0; i < depth; i++)
      s += (at[i] || "x") + " + (";
    s += last;
    for (i = 0; i < depth; i++)
      s += ")";
    return s;
  }
  function marks(depth, a, b, c, d) {
    var at = {};
    at[depth >> 2] = a;
    at[depth >> 1] = b;
    at[(3 * depth) >> 2] = c;
    if (d)
      at[depth - 1] = d;
    return at;
  }
  function show(v) {
    return typeof v === "string" ? v.length + ":" + v.slice(0, 3) + ":" + v.slice(-3) : String(v);
  }
  // a closure over the locals, so that the frame has var_refs
  var pre = "var a1, k = 0; const bump = () => ++k;\n";
  var tryCatch = function (e) {
    return pre + "try { return " + e + "; }\n" +
      "catch (err) { log.push('caught ' + err); return 'c' + bump(); }\n" +
      "finally { log.push('fin ' + k); }";
  };

  var gens = [], gnames = [];
  function addGen(name, f) { gnames.push(name); gens.push(f); }
  [130, 300, 600].forEach(function (d) {
    addGen("try " + d, GenF("x, log, inner", tryCatch(deep(d,
      marks(d, "(yield x)", "(yield* inner(x))", "(([a1 = yield bump()] = []), a1)", "(yield 'end')"), "x"))));
  });
  addGen("plain 300", GenF("x, log, inner",
    pre + "return " + deep(300, marks(300, "(yield x)", "(yield bump())", "(yield* inner(x))"), "x") + ";"));
  addGen("finally 130", GenF("x, log, inner",
    pre + "try { log.push('body'); } finally { log.push(String(" +
      deep(130, marks(130, "(yield x)", "(yield* inner(x))", "(yield 'f')"), "x") + ").length); }\nreturn k;"));
  addGen("for-of 130", GenF("x, log, inner",
    pre + "var r = '';\nfor (const v of [1, 2]) { r += v + " +
      deep(130, marks(130, "(yield v)", "(([a1 = yield bump()] = []), a1)", "(yield* inner(x))"), "x") + "; }\nreturn r;"));
  addGen("for-of try 300", GenF("x, log, inner",
    pre + "var r = '';\nfor (const v of [1, 2]) { try { r += v + " +
      deep(300, marks(300, "(yield v)", "(yield* inner(x))", "(yield bump())"), "x") +
      "; } catch (err) { r += ' caught ' + err; } }\nreturn r;"));

  var afs = [], anames = [];
  function addAf(name, f) { anames.push(name); afs.push(f); }
  [130, 300, 600].forEach(function (d) {
    addAf("try " + d, AsyncF("x, log, q", tryCatch(deep(d,
      marks(d, "(await q)", "(await bump())", "(([a1 = await q] = []), a1)", "(await 'end')"), "x"))));
  });
  addAf("for await 130", AsyncF("x, log, q, src",
    pre + "var r = '';\nfor await (const v of src()) { r += v + " +
      deep(130, marks(130, "(await q)", "(([a1 = await v] = []), a1)", "(await bump())"), "x") + "; }\nreturn r;"));
  addAf("for-of try 130", AsyncF("x, log, q, src",
    pre + "var r = '';\nfor (const v of [1, 2]) { try { r += v + " +
      deep(130, marks(130, "(await q)", "(await v)", "(([a1 = await q] = []), a1)"), "x") +
      "; } catch (err) { r += ' caught ' + err; } }\nreturn r;"));

  var ags = [];
  [300].forEach(function (d) {
    ags.push(AsyncGenF("x, log, q, inner", tryCatch(deep(d,
      marks(d, "(yield await q)", "(yield* inner(x))", "(await (yield x))", "(yield 'end')"), "x"))));
  });

  function* inner(x) { var a = yield "i1"; yield "i2" + a; return "ir"; }
  async function* src() { yield "s1"; await 0; yield "s2"; }
  function make(g, log) { return g("ab", log, inner); }
  // every suspension: k is the one where a throw() or return() is made
  function runGen(g, mode, k) {
    var log = [], it = make(g, log), r = [], n = 0, step;
    try {
      step = it.next();
      while (!step.done && n < 40) {
        r.push(show(step.value));
        n++;
        if (n === k && mode === 1)
          step = it.throw("t" + n);
        else if (n === k && mode === 2)
          step = it.return("r" + n);
        else
          step = it.next("s" + n);
      }
      r.push("done " + show(step.value));
    } catch (e) {
      r.push("threw " + e);
    }
    return r.join(",") + " " + log.join("/");
  }
  function runAf(f, mode) {
    var log = [];
    var q = mode === 1 ? Promise.reject("rej") : Promise.resolve("Q");
    q.catch(function () {});
    return f("ab", log, q, src).then(function (v) { return show(v) + " " + log.join("/"); },
                                     function (e) { return "rejected " + e + " " + log.join("/"); });
  }
  async function runAg(g, mode, k) {
    var log = [], q = mode === 3 ? Promise.reject("rej") : Promise.resolve("Q"), r = [], n = 0, step;
    q.catch(function () {});
    var it = g("ab", log, q, inner);
    try {
      step = await it.next();
      while (!step.done && n < 40) {
        r.push(show(step.value));
        n++;
        if (n === k && mode === 1)
          step = await it.throw("t" + n);
        else if (n === k && mode === 2)
          step = await it.return("r" + n);
        else
          step = await it.next("s" + n);
      }
      r.push("done " + show(step.value));
    } catch (e) {
      r.push("threw " + e);
    }
    return r.join(",") + " " + log.join("/");
  }

  // about 50 slots a frame: a few levels fill a segment, and the frame of
  // a resume (all 127 slots when it folds) does not fit what is left
  function rec(n, it, how) {
    var v1 = n + 1, v2 = n + 2, v3 = n + 3, v4 = n + 4, v5 = n + 5, v6 = n + 6, v7 = n + 7, v8 = n + 8, v9 = n + 9, v10 = n + 10,
        v11 = n + 11, v12 = n + 12, v13 = n + 13, v14 = n + 14, v15 = n + 15, v16 = n + 16, v17 = n + 17, v18 = n + 18, v19 = n + 19, v20 = n + 20,
        v21 = n + 21, v22 = n + 22, v23 = n + 23, v24 = n + 24, v25 = n + 25, v26 = n + 26, v27 = n + 27, v28 = n + 28, v29 = n + 29, v30 = n + 30,
        v31 = n + 31, v32 = n + 32, v33 = n + 33, v34 = n + 34, v35 = n + 35, v36 = n + 36, v37 = n + 37, v38 = n + 38, v39 = n + 39, v40 = n + 40;
    var s = v1 + v2 + v3 + v4 + v5 + v6 + v7 + v8 + v9 + v10 + v11 + v12 + v13 + v14 + v15 + v16 + v17 + v18 + v19 + v20 +
        v21 + v22 + v23 + v24 + v25 + v26 + v27 + v28 + v29 + v30 + v31 + v32 + v33 + v34 + v35 + v36 + v37 + v38 + v39 + v40;
    if (n === 0) {
      if (how === 0)
        return show(it.next("Y").value);
      if (how === 1)
        return show(it.throw("t").value);
      return show(it.return("r").value);
    }
    return rec(n - 1, it, how) + (s - 40 * n - 820 === 0 ? "" : "?");
  }
  function runDeep(g, d) {
    var log = [], it = make(g, log), r = show(it.next().value);
    try {
      r += " " + show(it.next("A").value);
      r += " " + rec(d, it, d % 3);
      r += " " + show(it.next("Z").value);
      r += " " + show(it.next("W").value);
    } catch (e) {
      r += " threw " + e;
    }
    return r + " " + log.join("/");
  }

  async function main() {
    var round, i, m, k, d;
    for (round = 0; round < 2; round++) {
      for (i = 0; i < gens.length; i++) {
        p("gen " + gnames[i] + " " + runGen(gens[i], 0, 0));
        for (k = 1; k <= 5; k++)
          for (m = 1; m <= 2; m++)
            p("gen " + gnames[i] + " " + m + " at " + k + " " + runGen(gens[i], m, k));
      }
      for (i = 0; i < afs.length; i++)
        for (m = 0; m < 2; m++)
          p("af " + anames[i] + " " + m + " " + await runAf(afs[i], m));
      for (i = 0; i < ags.length; i++) {
        p("ag " + i + " " + await runAg(ags[i], 0, 0));
        p("ag " + i + " rejected " + await runAg(ags[i], 3, 0));
        for (k = 1; k <= 4; k++)
          for (m = 1; m <= 2; m++)
            p("ag " + i + " " + m + " at " + k + " " + await runAg(ags[i], m, k));
      }
    }
    for (d = 0; d <= 200; d++)
      p("deep " + d + " " + runDeep(gens[d % 2 ? 1 : 0], d));
    // the states that are garbage in cycles hold their folded stacks in
    // Arrays the old loop does not make: collected before the counts
    gc();
    rvExpect.apply(null, gens.concat(afs, ags, [rec]));
    return out.join("\n");
  }
  main().then(function (r) { globalThis.result = r; },
              function (e) { gc(); globalThis.result = "main failed " + e; });
})();
