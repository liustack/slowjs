// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// rvdiff: stack traces and exceptions through constructor frames of the
// register loop: the stack of an Error made in a constructor called by new
// from the loop, from C (Reflect.construct, a bound function, a callback),
// and through super(...); an exception thrown in a constructor, caught in
// the caller and three frames up, with temps owned below each new, and
// the error of a derived constructor seen in its stack.
(function () { "use strict";
var out = [];
function p(x) { out.push(String(x)); }
var E = Error;
function lines(s) { return s.split("\n").slice(0, 6).join(" | "); }
function S1(k) {
  this.k = k;
  this.stack = lines(new E("s1").stack);
}
function S2(k) { this.inner = new S1(k + 1); this.stack = lines(new E("s2").stack); }
function T1(k) { if (k > 0) throw new E("t1 " + k); this.k = k; }
function T2(k) { this.v = 1 + new T1(k).k; }
function T3(k) { this.v = [1, 2, new T2(k).v]; }
class A { constructor(k) { this.k = k; this.stack = lines(new E("a").stack); } }
class B extends A { constructor(k) { super(k + 1); this.bstack = lines(new E("b").stack); } }
class Bt extends A { constructor(k) { super(k); if (k) throw new E("bt " + k); } }
class Bu extends A { constructor(k) { if (k) super(k); } }
var o = { k: 1 };
function viaNew(k) { return new S2(k).inner.stack + " # " + new S2(k).stack; }
function viaClass(k) { var q = new B(k); return q.stack + " # " + q.bstack; }
function catchHere(k) {
  try { return o.k + new T3(k).v[2]; } catch (e) { return "caught " + e.message + " " + lines(e.stack); }
}
function thrower(k) { return o.k + new T3(k).v[2]; }
function mid(k) { return o.k + thrower(k); }
function top3(k) {
  try { return mid(k); } catch (e) { return "caught3 " + e.message + " " + lines(e.stack); }
}
function derivedThrow(k) {
  try { return new Bt(k).k; } catch (e) { return "caughtd " + e.message + " " + lines(e.stack); }
}
function derivedUninit(k) {
  try { return new Bu(0).k; } catch (e) { return "caughtu " + e.name + " " + lines(e.stack); }
}
var BS = S1.bind(null, 7);
function viaBound(k) { return new BS().stack; }
var i;
for (i = 0; i < 3; i++) {
  p("new " + viaNew(i));
  p("class " + viaClass(i));
  p("catch " + catchHere(i));
  p("top3 " + top3(i));
  p("derived " + derivedThrow(i));
  p("uninit " + derivedUninit(i));
  p("bound " + viaBound(i));
  p("reflect " + Reflect.construct(S1, [i]).stack);
  p("reflect class " + Reflect.construct(B, [i]).bstack);
  p("map " + [i].map(function cb(k) { return new S1(k).stack; })[0]);
}
rvExpect(S1, S2, T1, T2, T3, A, B, Bt, Bu, viaNew, viaClass, catchHere, thrower, mid, top3);
rvExpect(derivedThrow, derivedUninit, viaBound, lines);
return out.join("\n");
})()
