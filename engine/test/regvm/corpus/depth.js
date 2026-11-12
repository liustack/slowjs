// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// rvdiff: the depth at which a recursion ends with a RangeError, on each path:
// pure JavaScript, a frame too large for the register stack, through
// Function.prototype.call (which the loop takes apart, so that call counts
// no level of its own), through a C function (map), through a getter,
// alternating with a function the pass refuses, and the same once the
// recursion is caught and run again.
(function () { "use strict";
var out = [];
var d = { n: 0 };
function run(name, f) {
  d.n = 0;
  var msg = "";
  try { f(0); msg = "no error"; } catch (e) { msg = e.name + ": " + e.message; }
  out.push(name + " " + d.n + " " + msg);
}
function rec(n) { d.n = n; return rec(n + 1) + 1; }
function big(n) {
  var v1 = n + 1, v2 = n + 2, v3 = n + 3, v4 = n + 4, v5 = n + 5, v6 = n + 6, v7 = n + 7, v8 = n + 8,
    v9 = n + 9, v10 = n + 10, v11 = n + 11, v12 = n + 12, v13 = n + 13, v14 = n + 14, v15 = n + 15,
    v16 = n + 16, v17 = n + 17, v18 = n + 18, v19 = n + 19, v20 = n + 20, v21 = n + 21, v22 = n + 22,
    v23 = n + 23, v24 = n + 24, v25 = n + 25, v26 = n + 26, v27 = n + 27, v28 = n + 28, v29 = n + 29,
    v30 = n + 30, v31 = n + 31, v32 = n + 32, v33 = n + 33, v34 = n + 34, v35 = n + 35, v36 = n + 36,
    v37 = n + 37, v38 = n + 38, v39 = n + 39, v40 = n + 40;
  d.n = n;
  return big(n + 1) + v1 + v40;
}
function viaCall(n) { d.n = n; return viaCall.call(null, n + 1); }
var arr = [0];
function viaMap(x) { d.n = d.n + 1; return arr.map(viaMap); }
var gobj = { get v() { return viaGet(d.n + 1); } };
function viaGet(n) { d.n = n; return gobj.v; }
function small(n) { d.n = n; return old(n + 1); }
function old(n) { d.n = n; return small(n + 1) * 1; }
function twoArgs(n, m) { d.n = n; return twoArgs(n + 1, m, m) + 1; }
function method(n) { d.n = n; return this.m(n + 1); }
var mo = { m: method };
run("rec", rec);
run("big", big);
run("call", viaCall);
run("map", viaMap);
run("getter", viaGet);
run("alternate", small);
run("extra-args", twoArgs);
run("method", function (n) { return mo.m(n); });
run("rec again", rec);
return out.join("\n");
})()
