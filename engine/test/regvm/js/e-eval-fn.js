// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// rvdiff: direct eval in a function with an arguments object of its own,
// and the locals an eval or a closure can change while the function runs.
// The code of eval reads and writes arguments (mapped to the parameters in
// sloppy code, not in strict code), a rest parameter and this. A local
// that an eval, a closure or a mapped arguments object writes in the middle
// of an expression is read again after it, in an arrow function, a
// function, and under with. The functions run several times, so that their
// later calls run in the register loop.
(function () {
var out = [];
function p(x) { out.push(String(x)); }
var aLazy = (k) => { var x = 1; var r = x + eval("x = 5, 0") + x; return r + "/" + x; };
var aLazy2 = (k) => { var x = k; return [x, eval("x++"), x].join(); };
var aLazyArg = (a) => { return [a, eval("a = 9"), a].join(); };
function fLazy(a) { var x = a; return [x, a, eval("x = 7; a = 8"), x, a, arguments[0]].join(); }
function fArgs(a, b) { eval("arguments[0] = 'm'"); return [a, b, arguments.length, arguments[1]].join(); }
function fArgsStrict(a, b) { "use strict"; eval("arguments[0] = 'm'"); return [a, b, arguments.length].join(); }
function fArgsRead(a) { return eval("arguments.length + ':' + a"); }
function fRest(a, ...r) { return eval("r.length + a"); }
function fThis2() { return eval("typeof this"); }
function wLazy(o) { var x = 1; with (o) { return [x, (o.f(), 0), x].join(); } }
var o = { f: function () { } };
function wLazy2(o) { var x = 1; var g = function () { x = 5; }; with (o) { return x + (g(), 0) + x; } }
function cLazy() { var x = 1; var g = function () { x = 5; }; return x + (g(), 0) + x; }
function mLazy(a) { var args = arguments; return [a, (args[0] = "z", 0), a].join(); }
function mLazyEval(a) { return [a, eval("arguments[0] = 'e'"), a].join(); }
for (var i = 0; i < 3; i++) {
  p(aLazy(i)); p(aLazy2(i)); p(aLazyArg(i)); p(fLazy(i)); p(fArgs(i, 2)); p(fArgsStrict(i));
  p(fArgsRead(i, 4)); p(fRest(i, 1, 2)); p(fThis2()); p(fThis2.call(5)); p(wLazy(o));
  p(wLazy2(o)); p(cLazy()); p(mLazy(i)); p(mLazyEval(i));
}
rvExpect(aLazy, aLazy2, aLazyArg, fLazy, fArgs, fArgsStrict, fArgsRead, fRest, fThis2, wLazy,
         wLazy2, cLazy, mLazy, mLazyEval);
return out.join(";");
})()
