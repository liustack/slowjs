// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// rvdiff: recursions deeper than one segment of the register stack (256 KiB),
// so that calls in the loop and from C start new segments and return across
// them: arguments, this and locals in a frame at a segment start, fewer
// arguments than parameters, an exception thrown at the bottom and caught at
// the top (the unwinding steps back over each segment), a closure made deep
// and called after its frame returned, a callback of a builtin (an entry from
// C at a segment edge), a getter, and the same again so that the spare
// segment is used, a frame with extra arguments, and one with more of them
// than a segment holds. Every local is read, so that the frames keep them.
(function () { "use strict";
var out = [];
// about 70 slots a frame: 3000 levels are 3.3 MB, 13 segments
function big(n, a, b) {
  var v1 = n + 1, v2 = n + 2, v3 = n + 3, v4 = n + 4, v5 = n + 5, v6 = n + 6, v7 = n + 7, v8 = n + 8, v9 = n + 9, v10 = n + 10, v11 = n + 11, v12 = n + 12, v13 = n + 13, v14 = n + 14, v15 = n + 15, v16 = n + 16, v17 = n + 17, v18 = n + 18, v19 = n + 19, v20 = n + 20, v21 = n + 21, v22 = n + 22, v23 = n + 23, v24 = n + 24, v25 = n + 25, v26 = n + 26, v27 = n + 27, v28 = n + 28, v29 = n + 29, v30 = n + 30, v31 = n + 31, v32 = n + 32, v33 = n + 33, v34 = n + 34, v35 = n + 35, v36 = n + 36, v37 = n + 37, v38 = n + 38, v39 = n + 39, v40 = n + 40, v41, v42, v43, v44, v45, v46, v47, v48, v49, v50, v51, v52, v53, v54, v55, v56, v57, v58, v59, v60;
  if (b !== undefined || [v41, v42, v43, v44, v45, v46, v47, v48, v49, v50, v51, v52, v53, v54, v55, v56, v57, v58, v59, v60].some(function (x) { return x !== undefined; }))
    return -1;
  var s = v1 + v2 + v3 + v4 + v5 + v6 + v7 + v8 + v9 + v10 + v11 + v12 + v13 + v14 + v15 + v16 + v17 + v18 + v19 + v20 + v21 + v22 + v23 + v24 + v25 + v26 + v27 + v28 + v29 + v30 + v31 + v32 + v33 + v34 + v35 + v36 + v37 + v38 + v39 + v40;
  if (n === 0)
    return a + s;
  return big(n - 1, a + 1) + s - 40 * n - 820;
}
// a method: this at the segment start
var o = {
  k: 3,
  m: function (n) {
    var v1 = n + 1, v2 = n + 2, v3 = n + 3, v4 = n + 4, v5 = n + 5, v6 = n + 6, v7 = n + 7, v8 = n + 8, v9 = n + 9, v10 = n + 10, v11 = n + 11, v12 = n + 12, v13 = n + 13, v14 = n + 14, v15 = n + 15, v16 = n + 16, v17 = n + 17, v18 = n + 18, v19 = n + 19, v20 = n + 20, v21 = n + 21, v22 = n + 22, v23 = n + 23, v24 = n + 24, v25 = n + 25, v26 = n + 26, v27 = n + 27, v28 = n + 28, v29 = n + 29, v30 = n + 30;
    var s = v1 + v2 + v3 + v4 + v5 + v6 + v7 + v8 + v9 + v10 + v11 + v12 + v13 + v14 + v15 + v16 + v17 + v18 + v19 + v20 + v21 + v22 + v23 + v24 + v25 + v26 + v27 + v28 + v29 + v30;
    if (n === 0)
      return this.k;
    return this.m(n - 1) + this.k + s - 30 * n - 465;
  }
};
// throws at the bottom, caught at the top
function thrower(n) {
  var v1 = n + 1, v2 = n + 2, v3 = n + 3, v4 = n + 4, v5 = n + 5, v6 = n + 6, v7 = n + 7, v8 = n + 8, v9 = n + 9, v10 = n + 10, v11 = n + 11, v12 = n + 12, v13 = n + 13, v14 = n + 14, v15 = n + 15, v16 = n + 16, v17 = n + 17, v18 = n + 18, v19 = n + 19, v20 = n + 20, v21 = n + 21, v22 = n + 22, v23 = n + 23, v24 = n + 24, v25 = n + 25, v26 = n + 26, v27 = n + 27, v28 = n + 28, v29 = n + 29, v30 = n + 30;
  var s = v1 + v2 + v3 + v4 + v5 + v6 + v7 + v8 + v9 + v10 + v11 + v12 + v13 + v14 + v15 + v16 + v17 + v18 + v19 + v20 + v21 + v22 + v23 + v24 + v25 + v26 + v27 + v28 + v29 + v30;
  if (n === 0)
    throw new Error("bottom " + s);
  return thrower(n - 1) + s;
}
function catcher(n) {
  try {
    return thrower(n);
  } catch (e) {
    return e.message;
  }
}
// a closure over a local of a frame deep in the stack, called after
var keep = [];
function closer(n) {
  var v1 = n + 1, v2 = n + 2, v3 = n + 3, v4 = n + 4, v5 = n + 5, v6 = n + 6, v7 = n + 7, v8 = n + 8, v9 = n + 9, v10 = n + 10, v11 = n + 11, v12 = n + 12, v13 = n + 13, v14 = n + 14, v15 = n + 15, v16 = n + 16, v17 = n + 17, v18 = n + 18, v19 = n + 19, v20 = n + 20, v21 = n + 21, v22 = n + 22, v23 = n + 23, v24 = n + 24, v25 = n + 25, v26 = n + 26, v27 = n + 27, v28 = n + 28, v29 = n + 29, v30 = n + 30;
  var c = n * 2 + v1 + v2 + v3 + v4 + v5 + v6 + v7 + v8 + v9 + v10 + v11 + v12 + v13 + v14 + v15 + v16 + v17 + v18 + v19 + v20 + v21 + v22 + v23 + v24 + v25 + v26 + v27 + v28 + v29 + v30;
  if (n % 500 === 0)
    keep.push(function () { return c + v30; });
  if (n === 0)
    return 0;
  return closer(n - 1) + 1;
}
// through a builtin: each level enters the loop from C
function viaMap(n) {
  var v1 = n + 1, v2 = n + 2, v3 = n + 3, v4 = n + 4, v5 = n + 5, v6 = n + 6, v7 = n + 7, v8 = n + 8, v9 = n + 9, v10 = n + 10, v11 = n + 11, v12 = n + 12, v13 = n + 13, v14 = n + 14, v15 = n + 15, v16 = n + 16, v17 = n + 17, v18 = n + 18, v19 = n + 19, v20 = n + 20, v21 = n + 21, v22 = n + 22, v23 = n + 23, v24 = n + 24, v25 = n + 25, v26 = n + 26, v27 = n + 27, v28 = n + 28, v29 = n + 29, v30 = n + 30;
  var s = v1 + v2 + v3 + v4 + v5 + v6 + v7 + v8 + v9 + v10 + v11 + v12 + v13 + v14 + v15 + v16 + v17 + v18 + v19 + v20 + v21 + v22 + v23 + v24 + v25 + v26 + v27 + v28 + v29 + v30;
  if (n === 0)
    return s;
  return [n - 1].map(viaMap)[0] + 1;
}
// through a getter
var g = {};
Object.defineProperty(g, "d", { get: function () { return getDeep(this.n - 1); } });
function getDeep(n) {
  var v1 = n + 1, v2 = n + 2, v3 = n + 3, v4 = n + 4, v5 = n + 5, v6 = n + 6, v7 = n + 7, v8 = n + 8, v9 = n + 9, v10 = n + 10, v11 = n + 11, v12 = n + 12, v13 = n + 13, v14 = n + 14, v15 = n + 15, v16 = n + 16, v17 = n + 17, v18 = n + 18, v19 = n + 19, v20 = n + 20, v21 = n + 21, v22 = n + 22, v23 = n + 23, v24 = n + 24, v25 = n + 25, v26 = n + 26, v27 = n + 27, v28 = n + 28, v29 = n + 29, v30 = n + 30;
  var s = v1 + v2 + v3 + v4 + v5 + v6 + v7 + v8 + v9 + v10 + v11 + v12 + v13 + v14 + v15 + v16 + v17 + v18 + v19 + v20 + v21 + v22 + v23 + v24 + v25 + v26 + v27 + v28 + v29 + v30;
  if (n <= 0)
    return s;
  g.n = n;
  return g.d + 1;
}
// reads its arguments, called with more than it declares: the frame keeps
// the extra ones below func, also at a segment start
function argy(n) {
  var v1 = n + 1, v2 = n + 2, v3 = n + 3, v4 = n + 4, v5 = n + 5, v6 = n + 6, v7 = n + 7, v8 = n + 8, v9 = n + 9, v10 = n + 10, v11 = n + 11, v12 = n + 12, v13 = n + 13, v14 = n + 14, v15 = n + 15, v16 = n + 16, v17 = n + 17, v18 = n + 18, v19 = n + 19, v20 = n + 20, v21 = n + 21, v22 = n + 22, v23 = n + 23, v24 = n + 24, v25 = n + 25, v26 = n + 26, v27 = n + 27, v28 = n + 28, v29 = n + 29, v30 = n + 30;
  var s = v1 + v2 + v3 + v4 + v5 + v6 + v7 + v8 + v9 + v10 + v11 + v12 + v13 + v14 + v15 + v16 + v17 + v18 + v19 + v20 + v21 + v22 + v23 + v24 + v25 + v26 + v27 + v28 + v29 + v30;
  if (n === 0)
    return arguments.length + s;
  return argy(n - 1, n, "x" + n) + arguments.length - 3 + (arguments[1] === n + 1 ? 1 : 0);
}
// more arguments than it declares, not read: the extra ones kept all the same
function extra(n, a) {
  var v1 = n + 1, v2 = n + 2, v3 = n + 3, v4 = n + 4, v5 = n + 5, v6 = n + 6, v7 = n + 7, v8 = n + 8, v9 = n + 9, v10 = n + 10, v11 = n + 11, v12 = n + 12, v13 = n + 13, v14 = n + 14, v15 = n + 15, v16 = n + 16, v17 = n + 17, v18 = n + 18, v19 = n + 19, v20 = n + 20, v21 = n + 21, v22 = n + 22, v23 = n + 23, v24 = n + 24, v25 = n + 25, v26 = n + 26, v27 = n + 27, v28 = n + 28, v29 = n + 29, v30 = n + 30;
  var s = v1 + v2 + v3 + v4 + v5 + v6 + v7 + v8 + v9 + v10 + v11 + v12 + v13 + v14 + v15 + v16 + v17 + v18 + v19 + v20 + v21 + v22 + v23 + v24 + v25 + v26 + v27 + v28 + v29 + v30;
  if (n === 0)
    return a + s;
  return extra(n - 1, a + 1, {}, [n], "y") + s - 30 * n - 465;
}
// more arguments than a segment holds, from C (apply), at the top and deep
// in the stack: a segment made large enough
function many() {
  return arguments.length + ":" + arguments[arguments.length - 1];
}
function deepMany(n, list) {
  var v1 = n + 1, v2 = n + 2, v3 = n + 3, v4 = n + 4, v5 = n + 5, v6 = n + 6, v7 = n + 7, v8 = n + 8, v9 = n + 9, v10 = n + 10, v11 = n + 11, v12 = n + 12, v13 = n + 13, v14 = n + 14, v15 = n + 15, v16 = n + 16, v17 = n + 17, v18 = n + 18, v19 = n + 19, v20 = n + 20, v21 = n + 21, v22 = n + 22, v23 = n + 23, v24 = n + 24, v25 = n + 25, v26 = n + 26, v27 = n + 27, v28 = n + 28, v29 = n + 29, v30 = n + 30;
  var s = v1 + v2 + v3 + v4 + v5 + v6 + v7 + v8 + v9 + v10 + v11 + v12 + v13 + v14 + v15 + v16 + v17 + v18 + v19 + v20 + v21 + v22 + v23 + v24 + v25 + v26 + v27 + v28 + v29 + v30;
  if (n === 0)
    return many.apply(null, list) + " " + many(...list) + " " + s;
  return deepMany(n - 1, list);
}
var long = [];
for (var i = 0; i < 40000; i++)
  long.push(i);
for (var r = 0; r < 2; r++) {
  out.push("big " + big(3000, 7));
  out.push("method " + o.m(2500));
  out.push("throw " + catcher(2500));
  keep = [];
  out.push("closer " + closer(2500) + " " + keep.map(function (f) { return f(); }).join(","));
  out.push("map " + viaMap(1500));
  out.push("getter " + getDeep(1200));
  out.push("argy " + argy(2500, 1, 2));
  out.push("extra " + extra(2500, 5, 6, 7));
  out.push("many " + many.apply(null, long) + " " + deepMany(1200, long));
}
// more than one segment was used with the loop on, and stepping back
// freed all but one spare (rvStat 7 and 8 are 0 with the loop off). Both
// are read in both loops, so that the throw-injection test (inject.sh)
// runs the same instructions in each.
var made = rvStat(7), held = rvStat(8), none = made === 0, lots = made >= 10;
out.push("segments " + (none || lots) + " " + (held <= 2));
rvExpect(big, o.m, thrower, catcher, closer, viaMap, getDeep, argy, extra, many, deepMany);
return out.join("\n");
})()
