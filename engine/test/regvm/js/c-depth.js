// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// rvdiff: the depth at which a recursion through constructors ends with a
// RangeError, when the constructors run in the register loop: a function
// constructor that constructs itself, one with a large frame, a class, a
// derived class whose super(...) recurses, and calls and constructs mixed,
// each run twice.
(function () { "use strict";
var out = [];
var d = { n: 0 };
function run(name, f) {
  d.n = 0;
  var msg = "";
  try { f(0); msg = "no error"; } catch (e) { msg = e.name + ": " + e.message; }
  out.push(name + " " + d.n + " " + msg);
}
function X(n) { d.n = n; this.v = new X(n + 1); }
function XB(n) {
  var v1 = n + 1, v2 = n + 2, v3 = n + 3, v4 = n + 4, v5 = n + 5, v6 = n + 6, v7 = n + 7, v8 = n + 8,
    v9 = n + 9, v10 = n + 10, v11 = n + 11, v12 = n + 12, v13 = n + 13, v14 = n + 14, v15 = n + 15,
    v16 = n + 16, v17 = n + 17, v18 = n + 18, v19 = n + 19, v20 = n + 20, v21 = n + 21, v22 = n + 22,
    v23 = n + 23, v24 = n + 24, v25 = n + 25, v26 = n + 26, v27 = n + 27, v28 = n + 28, v29 = n + 29,
    v30 = n + 30, v31 = n + 31, v32 = n + 32, v33 = n + 33, v34 = n + 34, v35 = n + 35, v36 = n + 36,
    v37 = n + 37, v38 = n + 38, v39 = n + 39, v40 = n + 40;
  d.n = n;
  this.v = new XB(n + 1) + v1 + v40;
}
class K { constructor(n) { d.n = n; this.v = new Kv(n + 1); } }
var Kv = K;
class Base { constructor(n) { d.n = n; if (n >= 0) this.v = new Sub(n + 1); } }
class Sub extends Base { constructor(n) { super(n); } }
function mixed(n) { d.n = n; return new M(n + 1); }
function M(n) { this.v = mixed(n + 1); }
run("new", function (n) { return new X(n); });
run("new big", function (n) { return new XB(n); });
run("new class", function (n) { return new K(n); });
run("super", function (n) { return new Sub(n); });
run("mixed", mixed);
run("new again", function (n) { return new X(n); });
run("super again", function (n) { return new Sub(n); });
run("mixed again", mixed);
rvExpect(X, XB, K, Base, Sub, mixed, M);
return out.join("\n");
})()
