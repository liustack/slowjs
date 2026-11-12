// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// The exceptions of the register loop (engine/regvm-ops-except.h): throw of a
// temp, a local, an argument, this and a constant, throw_error, a catch with
// temps owned above its catch offset when a callee throws, finally on return,
// on throw, on break and continue, rethrow from catch, nested try, a throw
// caught three frames up, a getter that throws inside a slow side, and the
// stack of each. The functions under test use only what the pass takes; the
// helpers that make strings, objects and errors run in the old loop.
(function () { "use strict";
var E = Error, TE = TypeError;
var log = [];
function p(x) { log.push(String(x)); }
function boom(m) { throw new E("boom " + m); }
function mkobj(v) { return { e: v, n: 1, inner: { m: v } }; }
var getter = { get x() { throw new TE("getter " + log.length); } };
function note(x) { log.push("note " + x); return x; }

// throw of each kind of operand
function throwArg(x) { throw x; }
function throwLoc(x) { var y = x; y = 1 + y; throw y; }
function throwTemp(o) { throw o.e; }
function throwK(x) { throw 7; }
function throwU(x) { throw void 0; }
function throwThis() { throw this; }
// throw_error: an assignment to a const
function throwRO(x) { const c = 1; c = 2; return c; }
// a catch, with temps owned above its catch offset when the callee throws
function catchTemps(o, f) {
  var r = 0;
  try { r = o.n + o.inner.m + f(o.e) + o.n; } catch (e) { r = e; }
  return r;
}
// the exception in the catch's temp, then a local
function catchLocal(f, x) {
  var got;
  try { f(x); got = 0; } catch (e) { got = e; }
  return got;
}
// finally on the way out: return, throw and fall through
function finRet(f, x) {
  var n = 0;
  try { n++; return f(x); } finally { n = 10 + n; note(n); }
}
function finThrow(f, x) {
  try { f(x); } finally { note(x); }
  return 1;
}
function finCatch(f, x) {
  var r = 0;
  try { r = f(x); } catch (e) { r = 100; } finally { r++; }
  return r;
}
// return from a catch through finally, and a throw out of finally
function catchRetFin(f, x) {
  try { return f(x); } catch (e) { return 5; } finally { note(x); }
}
function throwFromFin(f, x) {
  try { return f(x); } finally { throwArg(x + 1000); }
}
// rethrow, and nested try
function rethrow(f, x) {
  try { f(x); } catch (e) { throw e; }
  return 0;
}
function nested(f, x) {
  var r = 0;
  try {
    try { r = f(x); } catch (e) { r = 1; throwArg(x + 1); }
  } catch (e2) { r = e2 + r; }
  return r;
}
// a finally inside a finally, and a catch inside a finally
function finFin(f, x) {
  var r = 0;
  try {
    try { r = f(x); } finally { r++; note(r); }
  } finally {
    try { f(x + 1); } catch (e) { r = e + r; }
  }
  return r;
}
// break and continue through finally
function loopFin(f, n) {
  var i = 0, s = 0;
  while (i < n) {
    try {
      i++;
      if (i === 2) continue;
      if (i === 4) break;
      s = f(i) + s;
    } finally { s = 100 + s; }
  }
  return s;
}
// a try in a loop, caught each turn
function loopCatch(f, n) {
  var i = 0, s = 0;
  while (i < n) {
    i++;
    try { s = f(i) + s; } catch (e) { s = e + s; }
  }
  return s;
}
// a throw caught three frames up, from callees in the loop
function up0(x) { return throwArg(x) + 1; }
function up1(x) { return up0(x) + 1; }
function up2(x) { return up1(x) + 1; }
function sup0(x) { return boom(x) + 1; }
function sup1(x) { return sup0(x) + 1; }
function sup2(x) { return sup1(x) + 1; }
function up3(x) {
  try { return up2(x); } catch (e) { return e + 1; }
}
// a getter that throws inside the slow side of get_field, and an error of
// the engine (a field of undefined)
function viaGetter(o) {
  try { return o.x + 1; } catch (e) { return e; }
}
function deep(o) { return o.x.y; }
function ident(x) { return x; }
function stack(f, x) {
  try { f(x); } catch (e) { return e.stack; }
  return "no throw";
}
function msg(e) { return e instanceof E ? e.name + ": " + e.message : e; }
var cases = [
  function () { return catchLocal(throwArg, 3); },
  function () { return catchLocal(throwLoc, 3); },
  function () { return catchLocal(throwTemp, mkobj(9)); },
  function () { return catchLocal(throwK, 0); },
  function () { return catchLocal(throwU, 0); },
  function () { return catchLocal(function (v) { return throwThis.call(v); }, 4); },
  function () { return msg(catchLocal(throwRO, 0)); },
  function () { return catchTemps(mkobj(2), throwArg); },
  function () { return catchTemps(mkobj(2), ident); },
  function () { return msg(catchTemps(mkobj(2), boom)); },
  function () { return finRet(ident, 5); },
  function () { return catchLocal(function (x) { return finRet(throwArg, x); }, 6); },
  function () { return catchLocal(function (x) { return finThrow(throwArg, x); }, 7); },
  function () { return finThrow(ident, 7); },
  function () { return finCatch(ident, 8) + " " + finCatch(throwArg, 8); },
  function () { return catchRetFin(ident, 9) + " " + catchRetFin(throwArg, 9); },
  function () { return catchLocal(function (x) { return throwFromFin(ident, x); }, 10); },
  function () { return catchLocal(function (x) { return throwFromFin(throwArg, x); }, 11); },
  function () { return catchLocal(function (x) { return rethrow(throwArg, x); }, 12); },
  function () { return nested(ident, 13) + " " + nested(throwArg, 13); },
  function () { return finFin(ident, 14) + " " + catchLocal(function (x) { return finFin(throwArg, x); }, 14); },
  function () { return loopFin(ident, 10); },
  function () { return catchLocal(function (n) { return loopFin(throwArg, n); }, 10); },
  function () { return loopCatch(throwArg, 5) + " " + loopCatch(ident, 5); },
  function () { return up3(20); },
  function () { return msg(viaGetter(getter)) + " " + viaGetter({ x: 1 }); },
  function () { return msg(viaGetter(null)); },
  function () { return msg(catchLocal(deep, {})); },
  function () { return stack(sup2, 1); },
  function () { return stack(boom, 2); },
  function () { return stack(throwRO, 3); },
  function () { return stack(deep, {}); },
  function () { return stack(deep, undefined); },
  function () { return stack(function (o) { return catchTemps(o, boom); }, mkobj(1)); },
  function () { return stack(function (x) { return finThrow(boom, x); }, 4); },
  function () { return stack(function (x) { return rethrow(boom, x); }, 5); },
];
return function () {
  var round, i;
  for (round = 0; round < 3; round++) {
    for (i = 0; i < cases.length; i++)
      p(i + ": " + cases[i]());
  }
  rvExpect(throwArg, throwLoc, throwTemp, throwK, throwU, throwThis, throwRO,
           catchTemps, catchLocal, finRet, finThrow, finCatch, catchRetFin,
           throwFromFin, rethrow, nested, finFin, loopFin, loopCatch, up0, up1,
           up2, up3, sup0, sup1, sup2, viaGetter, deep, ident, stack);
  return log.join("\n");
};
})()
