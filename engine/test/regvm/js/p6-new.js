// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// rvdiff: new X(...) in a function of the register loop (call_constructor, a
// call out of the loop): plain functions, classes, derived classes, bound
// functions, built-ins, a constructor that returns an object or a
// primitive, one that throws with temps owned below the window, one that
// calls back into the register loop, new.target, and new of what is not a
// constructor. The functions named cN and tN are the ones the register loop
// takes: they use only the ops it has, and read the constructors through
// closure variables. Each runs several times, so that the later calls run in
// the register loop.
(function () { "use strict";
var out = [];
function p(x) { out.push(String(x)); }
function j() { return Array.prototype.join.call(arguments, ","); }
var M = Map, S = Set, E = Error, TE = TypeError, Pr = Promise, A = Array, D = Date,
  U8 = Uint8Array, RE = RegExp, O = Object, N = Number, St = String, AB = ArrayBuffer,
  WM = WeakMap, Px = Proxy, Bi = BigInt, Sy = Symbol, MathO = Math, JSONO = JSON;
function P(a, b) { this.a = a; this.b = b; }
P.prototype.sum = function () { return this.a + this.b; };
function R(kind) { this.x = 1; if (kind === 1) return { y: 2 }; if (kind === 2) return 5; if (kind === 3) return null; if (kind === 4) return dbl; }
function Th(x) { if (x) throw new E("boom " + x); this.ok = 1; }
function CB(f, v) { this.v = f(v); }
function dbl(x) { return x + x; }
function NT() { this.same = new.target === NT; }
class C { constructor(x) { this.x = x; } get v() { return this.x + 1; } }
class Dd extends C { constructor(x) { super(x * 2); this.d = 1; } }
class Dn extends C {}
class Dbad extends C { constructor(k) { super(1); if (k === 1) return 7; if (k === 2) return { z: 3 }; } }
class Dnos extends C { constructor() { if (0) super(); } }
class Base { constructor() { this.nt = new.target.name; } }
class Sub extends Base {}
var Cv = C, Ddv = Dd, Dnv = Dn, Dbadv = Dbad, Dnosv = Dnos, Basev = Base, Subv = Sub, NTv = NT;
var BP = P.bind(null, "bound");
var o = { k: 10, s: "str", f: 1.5, m: function () { this.mm = 1; }, arrow: () => 2, valueOf: function () { return dbl(21); } };
var arrow = (x) => x;
function exec(res) { o.ran = 1; res(1); }
function* gen() { yield 1; }
async function af() {}
var mstr = "m", tstr = "t", abstr = "ab", aplus = "a+", caab = "caab", one = "1";
var cases = [
  // plain functions, every kind of operand: a constant, a local, an
  // argument, a temp, missing and extra arguments, no parentheses
  function c0(x) { var q = new P(1, 2); return q.a + q.b + q.sum(); },
  function c1(x) { var a = x, K = P; return new K(a, o.k).sum(); },
  function c2(x) { return j(new P().a, new P(x).b, new P(1, 2, 3, 4).sum()); },
  function c3(x) { var q = new P; return q.a; },
  function c4(x) { return new P(o.k, o.k + x).sum(); },
  function c5(x) { return new P(new P(x, 1).sum(), new P(2, x).sum()).sum(); },
  // floats, strings, objects, null, undefined and a valueOf that runs JS
  function c6(x) { return j(new P(o.f, o.s).sum(), new P(null, void 0).a, new P(o, o).a.k, new P(o, 1).sum()); },
  // what the constructor returns
  function c7(x) { return j(new R(0).x, new R(1).y, new R(2).x, new R(3).x, new R(4)(3)); },
  // classes and derived classes
  function c8(x) { return new Cv(x).v + new Ddv(x).v + new Ddv(x).d + new Dnv(x).x; },
  function c9(x) { var q = new Dbadv(2); return q.z; },
  function c10(x) { return j(new Subv().nt, new Basev().nt); },
  // bound functions
  function c11(x) { var q = new BP(x); return q.a + q.b; },
  // new.target inside the constructor
  function c12(x) { return new NTv().same; },
  // the constructor calls back into the register loop
  function c13(x) { return new CB(dbl, x).v + new CB(arrow, 3).v; },
  // built-ins
  function c14(x) { var m = new M(); m.set(1, x); return m.get(1) + new S().size; },
  function c15(x) { var e = new E(mstr); return j(e.message, new TE(tstr).name, new E(x).message); },
  function c16(x) { return new A(3).length + new A(1, 2, 3).length + new A(x).length; },
  function c17(x) { return new D(0).getTime() + new U8(4).length + new AB(8).byteLength; },
  function c18(x) { return j(new RE(aplus).test(caab), new O(1).valueOf(), new N(5).valueOf(), new St(abstr).length); },
  function c19(x) { var w = new WM(), k = new O(); w.set(k, x); return w.get(k); },
  function c20(x) { var q = new Px(new O(), new P(1, 2)); return q.a; },
  function c21(x) { var q = new Pr(exec); return o.ran + x; },
  function c22(x) { return new O(new P(x, 1)).sum(); },
  function c23(x) { var m = o.m; return new m().mm; },
  // new in a loop
  function c24(x) { var s = 0; for (var k = 0; k < 100; k++) s = s + new P(k, x).sum(); return s; }
];
// each throws, through a temp owned below the new where it can
var throwing = [
  function t0(x) { return o.k + new Th(x).ok; },
  function t1(x) { return dbl(o.k, new Th(x)); },
  function t2(x) { return new P(o.k, new Th(x)).a; },
  function t3(x) { return o.k + new Dbadv(1); },
  function t4(x) { return new Dnosv(); },
  function t5(x) { return o.k + new arrow(1); },
  function t6(x) { var f = o.arrow; return new f(); },
  function t7(x) { return new x(); },
  function t8(x) { var f = 5; return new f(); },
  function t9(x) { var f = MathO.max; return new f(1, 2); },
  function t10(x) { var f = Bi; return new f(1); },
  function t11(x) { var f = Sy; return new f(); },
  function t12(x) { var f = gen; return new f(); },
  function t13(x) { var f = af; return new f(); },
  function t14(x) { var f = null; return new f(); },
  function t15(x) { var f = o; return new f(); },
  function t16(x) { var f = JSONO.parse; return new f(one); },
  function t17(x) { var f = Cv; return f(1); },
  function t18(x) { var f = Dnv; return f(1); },
  function t19(x) { return o.k + new P(o.k, x.nope.nope).a; }
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
rvExpect(dbl, exec, p);
return out.join("\n");
})()
