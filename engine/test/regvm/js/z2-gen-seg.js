// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// rvdiff: generators whose stack folds (engine/regvm-fold.h), suspended with
// ranges of it folded and resumed at the bottom of a recursion of large
// frames, at every depth up to past the largest segment of the register
// stack, so that some resumes run on the next segment (rv_gen_resume) and
// the stack settles after them (engine/regvm-entry.h): a next(), a throw()
// and a return() at the bottom, then the generator goes on or ends at the
// top, where the segments were stepped back and settled. Each is made from
// source and run many times, so that its later runs are in the register
// loop.
(function () {
  "use strict";
  var out = [];
  var GenF = Object.getPrototypeOf(function* () {}).constructor;
  function deep(depth, k, mid, last) {
    var s = "", i;
    for (i = 0; i < depth; i++)
      s += i === k ? mid + " + (" : "x + (";
    s += last;
    for (i = 0; i < depth; i++)
      s += ")";
    return s;
  }
  // 60 locals, so that the frame of a resume needs more room than a frame
  // of rec leaves at the top of a segment
  var locals = "var " + Array.from({ length: 60 }, function (_, i) { return "a" + i + " = " + i; }).join(", ") + ";\n";
  function body(e) {
    return locals + "try { return " + e + "; }\n" +
      "catch (err) { log.push('caught ' + err); return 'c'; }\n" +
      "finally { log.push('fin ' + (" + Array.from({ length: 60 }, function (_, i) { return "a" + i; }).join(" + ") + ")); }";
  }
  function show(v) {
    return typeof v === "string" ? v.length + ":" + v.slice(-4) : String(v);
  }
  var gens = [GenF("x, log", body(deep(130, 65, "(yield x)", "(yield 'deepest')")))];
  // about 50 slots a frame, so that a few levels fill a segment
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
  var d, i, log, it, r;
  for (d = 0; d <= 400; d++) {
    for (i = 0; i < gens.length; i++) {
      log = [];
      it = gens[i]("ab", log);
      r = show(it.next().value);
      try {
        r += " " + rec(d, it, d % 3);
        r += " " + show(it.next("Z").value);
        r += " " + show(it.next("W").done);
      } catch (e) {
        r += " threw " + e;
      }
      out.push(d + " " + i + " " + r + " " + log.join("/"));
    }
  }
  rvExpect(gens[0], rec);
  return out.join("\n");
})();
