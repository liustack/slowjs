// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// rvdiff: derived class constructors in the register loop. super(...) with
// the same new.target, the this that is not initialized until super
// returns (read before, initialized twice, never initialized), what a
// derived constructor may return, the default constructor (which passes
// its arguments on), chains of three classes, a parent that is a function
// constructor, a built-in or a bound function, a parent that throws, and
// new.target seen by the parent. The classes are defined in a function the
// register loop does not take (define_class): their constructors convert.
(function () { "use strict";
var out = [];
function p(x) { out.push(String(x)); }
var E = Error;
function F(a) { this.fa = a; this.fnt = new.target === undefined ? "none" : new.target.name; }
class A { constructor(x) { this.x = x; this.ant = new.target.name; } m() { return this.x * 2; } }
class B extends A { constructor(x) { super(x * 2); this.b = 1; } }
class Bd extends A {}
class Bb extends B { constructor(x) { super(x + 1); this.bb = this.x + this.b; } }
class Bdd extends Bd {}
class Bf extends F { constructor(a) { super(a + 1); this.k = 2; } }
class Bfd extends F {}
class Bret extends A {
  constructor(k) {
    super(1);
    if (k === 1) return 7;
    if (k === 2) return { z: 3 };
    if (k === 3) return undefined;
    if (k === 4) return null;
  }
}
class Bnos extends A { constructor(k) { if (k) super(k); } }
class Bnosret extends A { constructor(k) { if (k) return { early: k }; } }
class Btwice extends A { constructor() { super(1); super(2); } }
class Bpre extends A { constructor() { this.y = 1; super(1); } }
class Bth extends A { constructor(x) { if (x > 1) throw new E("before " + x); super(x); throw new E("after " + x); } }
function Fth() { throw new E("parent"); }
class Bpth extends Fth { constructor() { super(); this.no = 1; } }
class Bnt extends A { constructor() { super(0); this.nt = new.target === Bnt; } }
class Bm extends Map { constructor() { super(); this.set(1, 2); } }
class Ba extends Array {}
var Bound = A.bind(null, 9);
Bound.prototype = A.prototype;
class Bbound extends Bound { constructor() { super(); this.q = 1; } }
var cases = [
  function c0(x) { var q = new B(x); return q.x + "," + q.b + "," + q.ant + "," + q.m(); },
  function c1(x) { var q = new Bd(); return q.x + "," + q.ant + "," + (q instanceof Bd); },
  function c2(x) { var q = new Bb(x); return q.x + "," + q.bb + "," + q.ant; },
  function c3(x) { var q = new Bdd(); return (q instanceof Bdd) + "," + q.ant; },
  function c4(x) { var q = new Bf(x); return q.fa + "," + q.fnt + "," + q.k; },
  function c5(x) { var q = new Bfd(); return q.fa + "," + q.fnt; },
  function c6(x) { return new Bret(2).z + "," + new Bret(3).x + "," + new Bret(0).ant; },
  function c7(x) { return new Bnos(x).x + "," + new Bnosret(x).early; },
  function c8(x) { return new Bnt().nt + "," + new Bnt().ant; },
  function c9(x) { var m = new Bm(); return m.get(1) + "," + (m instanceof Map); },
  function c10(x) { var a = new Ba(); a.push(x); return a.length + "," + Array.isArray(a); },
  function c11(x) { var q = new Bbound(); return q.x + "," + q.q + "," + q.ant; },
  // the default constructor passes its arguments on
  function c12(x) { var q = new Bd(x); return q.x + "," + new Bdd(x, 2).x + "," + new Bfd(x).fa; }
];
var throwing = [
  function t0(x) { return new Bret(1); },
  function t1(x) { return new Bret(4); },
  function t2(x) { return new Bnos(0); },
  function t3(x) { return new Btwice(); },
  function t4(x) { return new Bpre(); },
  function t5(x) { return new Bth(x); },
  function t6(x) { return new Bpth(); },
  function t7(x) { var K = B; return K(x); },
  function t8(x) { var K = Bd; return K(); },
  function t9(x) { var K = Bnosret; return new K(0); }
];
var i, k, r;
for (k = 0; k < 3; k++) {
  for (i = 0; i < cases.length; i++)
    p("case " + i + ": " + cases[i](4));
  for (i = 0; i < throwing.length; i++) {
    try { r = throwing[i](k + 1); p("throwing " + i + ": no error " + r); }
    catch (e) { p("throwing " + i + ": " + e.name + ": " + e.message); }
  }
}
// from C: Reflect.construct of a derived class, new.target another class
for (k = 0; k < 3; k++) {
  r = Reflect.construct(B, [k], Bnt);
  p("reflect " + (r instanceof Bnt) + " " + r.ant + " " + r.x);
  r = Reflect.construct(Bd, [k], A);
  p("reflect default " + (Object.getPrototypeOf(r) === A.prototype) + " " + r.x);
}
rvExpect(F, A, B, Bd, Bb, Bdd, Bf, Bfd, Bret, Bnos, Bnosret, Btwice, Bpre, Bth, Fth, Bpth, Bnt, Bm, Ba, Bbound);
rvExpect(cases[0], cases[1], cases[2], cases[3], cases[4], cases[5], cases[6], cases[7], cases[8], cases[9]);
rvExpect(cases[10], cases[11], cases[12]);
rvExpect(throwing[0], throwing[1], throwing[2], throwing[3], throwing[4], throwing[5], throwing[6], throwing[7]);
rvExpect(throwing[8], throwing[9]);
return out.join("\n");
})()
