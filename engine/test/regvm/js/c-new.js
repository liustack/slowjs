// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// rvdiff: constructors that run in the register loop. new X(...) of a
// function constructor and of a class, from a function of the register loop
// and from C, with what the constructor returns (an object, a primitive,
// null, a function), new.target read inside (directly and through an
// arrow), missing and extra arguments, a constructor that throws with temps
// owned below the new, one that recurses through new, and a class
// constructor called without new. The classes are defined here, in a
// function the register loop does not take (define_class): their
// constructors and methods convert. Each runs several times, so that the
// later calls run in the register loop.
(function () { "use strict";
var out = [];
function p(x) { out.push(String(x)); }
var E = Error, undef, sym = Symbol("s");
function P(a, b) { this.a = a; this.b = b; }
P.prototype.sum = function () { return this.a + this.b; };
function R(kind) {
  this.x = 1;
  if (kind === 1) return { y: 2 };
  if (kind === 2) return 5;
  if (kind === 3) return null;
  if (kind === 4) return dbl;
  if (kind === 5) return sym;
  if (kind === 6) return "str";
  if (kind === 7) return undef;
}
function Th(x) { if (x) throw new E("boom " + x); this.ok = 1; }
function dbl(x) { return x + x; }
function NT(x) { this.same = new.target === NT; this.t = typeof new.target; this.x = x; }
function NTU() { return new.target; }
function NTA() { var f = () => new.target; this.f = f; }
function Fa(a) { this.n = arguments.length; this.s = arguments[1]; this.a = a; }
function Fr(a, ...r) { this.n = r.length; this.a = a; }
function* GN(x) { yield new.target; yield x; }
function Deep(n) { this.v = n > 0 ? new Deep(n - 1).v + 1 : 0; }
function Many(a, b, c, d, e, f) { var s = a + b, t = c + d, u = e + f; this.v = s + t + u; }
function Loc() { var a = 1, b = 2, c = 3; this.v = a + b + c; }
function Big(n) {
  var v1 = n + 1, v2 = n + 2, v3 = n + 3, v4 = n + 4, v5 = n + 5, v6 = n + 6, v7 = n + 7, v8 = n + 8,
    v9 = n + 9, v10 = n + 10, v11 = n + 11, v12 = n + 12;
  this.v = v1 + v2 + v3 + v4 + v5 + v6 + v7 + v8 + v9 + v10 + v11 + v12;
}
class C { constructor(x) { this.x = x; } get v() { return this.x + 1; } m() { return this.x * 2; } }
class Cn {}
class Cnt { constructor() { this.nt = new.target === Cnt; } }
var o = { k: 10, s: "str", f: 1.5 };
var cases = [
  // plain functions: constants, a local, an argument, a temp, missing and
  // extra arguments, no parentheses
  function c0(x) { var q = new P(1, 2); return q.a + q.b + q.sum(); },
  function c1(x) { var a = x, K = P; return new K(a, o.k).sum(); },
  function c2(x) { return new P().a + "," + new P(x).b + "," + new P(1, 2, 3, 4).sum(); },
  function c3(x) { var q = new P; return q.a; },
  function c4(x) { return new P(o.k, o.k + x).sum(); },
  function c5(x) { return new P(new P(x, 1).sum(), new P(2, x).sum()).sum(); },
  function c6(x) { return new P(o.f, o.s).sum() + "," + new P(null, undef).a + "," + new P(o, o).a.k; },
  // what the constructor returns
  function c7(x) {
    return new R(0).x + "," + new R(1).y + "," + new R(2).x + "," + new R(3).x + "," +
      new R(4)(3) + "," + new R(5).x + "," + new R(6).x + "," + new R(7).x;
  },
  // new.target, read directly and through an arrow, and in a plain call
  function c8(x) { var q = new NT(x); return q.same + q.t + q.x; },
  function c9(x) { return (new NTU() === NTU) + "," + (NTU() === undef) + "," + (new NTA().f() === NTA); },
  // frames of several sizes, recursion through new
  function c10(x) { return new Many(1, 2, 3, 4, 5, x).v + new Loc().v + new Big(x).v; },
  function c11(x) { return new Deep(x * 10).v; },
  // classes
  function c12(x) { var q = new C(x); return q.v + q.m() + (q instanceof C); },
  function c13(x) { return (new Cn() instanceof Cn) + "," + new Cnt().nt; },
  // the prototype of the object made comes from new.target
  function c14(x) { var q = new P(x, 1); return (Object.getPrototypeOf(q) === P.prototype) + "," + q.sum(); },
  // new in a loop
  function c15(x) { var s = 0; for (var k = 0; k < 50; k++) s = s + new P(k, x).sum(); return s; },
  // a constructor that reads its arguments, with fewer, as many and more
  // than it has
  function c16(x) {
    var q = new Fa(), r = new Fa(x), s = new Fa(x, 7, 8), t = new Fr(x), u = new Fr(x, 1, 2);
    return [q.n, q.a, r.n, r.a, s.n, s.s, t.n, u.n, u.a].join(" ");
  },
  // new.target in a generator, which is never constructed
  function c17(x) { var g = GN(x), a = g.next().value, b = g.next().value; return [a === undef, b].join(" "); }
];
// each throws, through a temp owned below the new where it can
var throwing = [
  function t0(x) { return o.k + new Th(x).ok; },
  function t1(x) { return dbl(o.k, new Th(x)); },
  function t2(x) { return new P(o.k, new Th(x)).a; },
  function t3(x) { var K = C; return K(x); },
  function t4(x) { var K = Cn; return o.k + K(); },
  function t5(x) { return o.k + new P(o.k, x.nope.nope).a; },
  function t6(x) { var f = dbl.bind(null); return new Th(x, f()); },
  function t7(x) { return new R(x).nope.nope; }
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
// from C: Reflect.construct, with new.target another function
for (k = 0; k < 3; k++) {
  r = Reflect.construct(P, [1, 2], NT);
  p("reflect " + (Object.getPrototypeOf(r) === NT.prototype) + " " + r.sum);
  r = Reflect.construct(NT, [k], P);
  p("reflect nt " + r.same + " " + r.t + " " + (r instanceof P));
  r = Reflect.construct(C, [k], Cnt);
  p("reflect class " + (r instanceof Cnt) + " " + r.x);
}
rvExpect(P, R, Th, NT, NTU, NTA, Deep, Many, Loc, Big, C, Cnt, dbl, Fa, Fr);
rvExpect(cases[0], cases[1], cases[2], cases[3], cases[4], cases[5], cases[6], cases[7]);
rvExpect(cases[8], cases[9], cases[10], cases[11], cases[12], cases[13], cases[14], cases[15], cases[16], cases[17]);
rvExpect(throwing[0], throwing[1], throwing[2], throwing[3], throwing[4], throwing[5], throwing[6], throwing[7]);
return out.join("\n");
})()
