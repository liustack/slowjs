// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// rvdiff: direct eval in the register loop. The code of a direct eval reads
// and writes the locals and the arguments of the function that runs it, its
// this and the variables around it, and a function it makes keeps them
// after the call. In sloppy code a var it declares goes into the var object
// of the function and is seen after it, in strict code it stays inside the
// eval. A spread call of eval is apply_eval. A compound assignment, an
// update or a destructuring of a name that an eval may have declared goes
// through a reference to the var object or the local. A function named
// eval that is not the eval of the realm is a plain call. A SyntaxError
// and an error thrown by the code of eval come out of the call, with the
// frames of the register loop in their stack once. The functions with eval
// here are arrow functions: e-eval-fn.js has those with an arguments object
// of their own. The aN and fN run several times, so that their later calls
// run in the register loop.
(function () {
var out = [];
function p(x) { out.push(String(x)); }
function ck(f, a, b) {
  try { return "ok " + f(a, b); } catch (e) { return e.name + ": " + e.message; }
}
function stackOf(f, a) {
  try { f(a); return "no throw"; } catch (e) {
    return String(e.stack).split("\n").filter(function (l) { return l.indexOf("at ") >= 0; })
      .map(function (l) { return l.replace(/\(.*:(\d+):\d+\)/, "line $1").trim(); }).join(" | ");
  }
}
var around = "around";

var aLocal = (k) => { var l = 10; eval("l = l + k"); return l; };
var aRead = (k) => { var l = "L"; return eval("l + k + around"); };
var aVar = (k) => { eval("var nv = k * 2"); return nv + (typeof nv); };
var aVarTwice = (k) => { eval("var nv2 = 1"); eval("nv2 = nv2 + k"); return nv2; };
var aFunc = (k) => { eval("function fd() { return 'fd' + k; }"); return fd(); };
var aClosure = (k) => { var l = k; var g = eval("(function () { return l = l + 1; })"); g(); return [g(), l].join(); };
var aStrict = (k) => { "use strict"; eval("var sv = 1"); return typeof sv + eval("k + 1"); };
var aSpread = (k) => { var l = 3; var args = ["l * k"]; return eval(...args); };
var aSpreadNone = (k) => { var args = []; return eval(...args); };
var aNotEval = (k) => { var eval = function (s) { return "not eval " + s; }; return eval("k"); };
var aShadow = (k) => { var r = []; { let k2 = k; r.push(eval("k2")); } return r.join(); };
var aLoop = (k) => { var fs = []; for (let i = 0; i < 3; i++) fs.push(eval("() => i + k")); return fs.map(function (f) { return f(); }).join(); };
var aSyntax = (k) => eval("1 +");
var aThrow = (k) => eval("throw new TypeError('from eval ' + k)");
var aNested = (k) => { var l = 1; return eval("eval('l + k')"); };
var aValue = (k) => eval(k);
var aDelete = (k) => { eval("var dv = 1"); var r = delete dv; return r + typeof dv; };
var aWithEval = (o) => { with (o) { return eval("x"); } };
var aRef = (k) => { eval("var rv = " + k); rv += 2; rv++; [rv] = [rv * 10]; for (rv in { key: 1 }) ; return rv + typeof rv; };
var aRefLocal = (k) => { var lv = k; eval(""); lv += 1; lv++; return lv; };
var aBelow = (k) => [k + "a", k + "b", eval("throw new SyntaxError('below ' + k)")];
function fThis() { return ((k) => eval("this === G ? 'global' : typeof this"))(1); }
var G = this;

for (var i = 0; i < 4; i++) {
  p(aLocal(i)); p(aRead(i)); p(aVar(i)); p(aVarTwice(i)); p(aFunc(i));
  p(aClosure(i)); p(aStrict(i)); p(aSpread(i)); p(aSpreadNone(i));
  p(aNotEval(i)); p(aShadow(i)); p(aLoop(i));
  p(ck(aSyntax, i)); p(ck(aThrow, i)); p(aNested(i));
  p(aValue(i)); p(aValue("'s' + " + i)); p(typeof aValue({}));
  p(aDelete(i)); p(aWithEval({ x: "with " + i }));
  p(aRef(i)); p(aRefLocal(i));
  p(ck(aBelow, "k" + i));
  p(fThis()); p(fThis.call({}));
}
p(stackOf(aThrow, 9));
p(stackOf(aSyntax, 9));
rvExpect(aLocal, aRead, aVar, aVarTwice, aFunc, aClosure, aStrict, aSpread, aSpreadNone,
         aNotEval, aShadow, aLoop, aSyntax, aThrow, aNested, aValue, aDelete, aWithEval, aBelow,
         aRef, aRefLocal);
return out.join(";");
})()
