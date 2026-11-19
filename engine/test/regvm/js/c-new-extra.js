// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.
// rvdiff: new of a constructor given more arguments than it has parameters,
// which runs in the register loop with the extra arguments below its func,
// as a call does (engine/regvm-ops-ctor.h), and the prototype new reads from
// the function. Sloppy constructors (a mapped arguments object over the
// parameters and the extra arguments) and strict ones, rest and default
// parameters, new.target, arguments read by a closure and an arrow, what the
// constructor returns, derived classes that pass on more arguments than the
// parent has, the default constructor of a derived class, class fields,
// Prototype's Class.create (this.initialize.apply(this, arguments)),
// recursion through such a new, 40 arguments, errors thrown before, inside
// and below the constructor with their stacks, and the prototype: replaced
// by an object or a primitive, never read before, read only, a bound
// function, a proxy, and Reflect.construct with another new.target.
// rv-expect-op: NEW CRES
(function () {
var out = [];
function p(x) { out.push(String(x)); }
var E = Error, undef;
function M(a, b) {
  arguments[0] = "w"; arguments[3] = "x3"; b = "b2";
  this.v = [a, b, arguments.length, arguments[1], arguments[2], arguments[3]].join(",");
}
function S(a) { "use strict"; arguments[0] = 9; this.v = [a, arguments.length, arguments[1], arguments[arguments.length - 1]].join(","); }
function Z() { this.n = arguments.length; this.l = arguments[arguments.length - 1]; }
function Init() { this.initialize.apply(this, arguments); }
Init.prototype = { initialize: function (a, b, c) { this.s = a + b + c; } };
function NT(a) { this.same = new.target === NT; this.n = arguments.length; this.a = a; }
function R(kind) {
  this.x = 1;
  if (kind === 1) return { y: arguments[1] };
  if (kind === 2) return arguments[1];
  if (kind === 3) return null;
}
function Th(x) { if (x) throw new E("boom " + arguments[1]); this.ok = 1; }
function ThS(x) { if (x) undef.f; this.ok = 1; }
function Def(a, b = arguments.length) { this.v = a + "," + b + "," + arguments.length; }
function Rest(a, ...r) { this.v = a + "," + r.join("|"); }
function Deep(n) { this.v = n > 0 ? new Deep(n - 1, n, n).v + 1 : arguments.length; }
function Big(a) {
  var v1 = a + 1, v2 = a + 2, v3 = a + 3, v4 = a + 4, v5 = a + 5, v6 = a + 6, v7 = a + 7, v8 = a + 8,
    v9 = a + 9, v10 = a + 10, v11 = a + 11, v12 = a + 12;
  this.v = v1 + v2 + v3 + v4 + v5 + v6 + v7 + v8 + v9 + v10 + v11 + v12 + arguments[arguments.length - 1];
}
function Cap(a) {
  var self = this;
  this.f = function () { return a + "," + arguments.length; };
  this.g = () => arguments.length + "," + arguments[1];
  this.h = function () { a = "h"; };
}
function Ret(a) { return this; }
class B { constructor(a) { this.a = a; this.n = arguments.length; this.t = new.target.name; } }
class Dd extends B { constructor(a) { super(a, 1, 2); this.m = arguments.length; } }
class Dflt extends B {}
class Fld { x = 1; y = this.x + 1; constructor(a) { this.a = a + "," + arguments.length; } }
class Th2 extends B { constructor(a) { super(a, a); throw new E("derived " + arguments[1]); } }
class Bad extends B { constructor(a) { super(a); return arguments[1]; } }
var Z40 = Function("Z", "return function (i) { return new Z(" +
  Array.from({ length: 40 }, function (_, k) { return "i + " + k; }).join(", ") + "); };")(Z);
// the prototype
function Pr(a) { this.a = a; }
function Pg(a) { this.a = a; }
function Fresh(a) { this.a = a; }
function Pw(a) { this.a = a; }
Object.defineProperty(Pw, "prototype", { value: { k: "ro" }, writable: false });
var Bd = Pr.bind(null, "bound");
var seen = [];
var Px = new Proxy(Pr, { get: function (t, k) { seen.push(String(k)); return t[k]; } });
function proto(o) {
  var q = Object.getPrototypeOf(o);
  return q === Object.prototype ? "Object.prototype" : q && q.k !== undef ? "k=" + q.k : typeof q;
}
var cases = [
  function c0(x) { return new M(1, 2, 3).v + " " + new M(1, 2, 3, 4, 5).v + " " + new M(1).v; },
  function c1(x) { return new S(1, 2).v + " " + new S(1, 2, 3, x).v; },
  function c2(x) { var a = new Z(), b = new Z(x), c = new Z(x, 2, 3); return [a.n, a.l, b.n, b.l, c.n, c.l].join(" "); },
  function c3(x) { var q = new Init(x, 2, 3); return q.s + " " + (q instanceof Init); },
  function c4(x) { var q = new NT(x, 2, 3); return [q.same, q.n, q.a].join(" "); },
  function c5(x) {
    return [new R(0, 7).x, new R(1, 7).y, new R(2, 7).x, new R(3, 7).x, new R(2, { z: 9 }).z].join(" ");
  },
  function c6(x) { return new Def(x).v + " " + new Def(x, 5, 6).v + " " + new Def(x, undef, 6).v; },
  function c7(x) { return new Rest(x).v + " " + new Rest(x, 1, 2, 3).v; },
  function c8(x) { return new Deep(x * 10).v; },
  function c9(x) { return new Big(x, 1, 2, 3).v; },
  function c10(x) { var q = new Cap(x, "e1", "e2"), r = q.f(9); q.h(); return r + " " + q.g() + " " + q.f(); },
  function c11(x) { var q = new Ret(x, x); return q instanceof Ret; },
  function c12(x) {
    var d = new Dd(x, 7, 8), e = new Dflt(x, 7, 8, 9);
    return [d.a, d.n, d.m, d.t, e.a, e.n, e.t, d instanceof B].join(" ");
  },
  function c13(x) { var f = new Fld(x, 1, 2); return [f.x, f.y, f.a].join(" "); },
  function c14(x) { var q = Z40(x); return q.n + " " + q.l; },
  function c15(x) { var s = 0; for (var k = 0; k < 30; k++) s = s + new Z(k, x, k).l + new NT(k, k).n; return s; },
  // the prototype
  function c16(x) {
    var r = [];
    Pr.prototype = { k: 1 };
    r.push(proto(new Pr(x)), proto(new Pr(x, 2)));
    Pr.prototype = 5;
    r.push(proto(new Pr(x)), proto(new Pr(x, 2)));
    Pr.prototype = null;
    r.push(proto(new Pr(x, 2)));
    Pr.prototype = { k: 2 };
    r.push(proto(new Pr(x, 2)));
    return r.join(" ");
  },
  function c17(x) {
    var a = new Pg(x), r = [proto(a)];
    Pg.prototype.k = "own";
    r.push(proto(new Pg(x, 1)), Object.getPrototypeOf(a) === Pg.prototype);
    return r.join(" ");
  },
  function c18(x) { return proto(new Fresh(x, x)) + " " + (Object.getPrototypeOf(new Fresh(x)) === Fresh.prototype); },
  function c19(x) { return proto(new Pw(x)) + " " + proto(new Pw(x, 1, 2)); },
  function c20(x) { var q = new Bd(x, 1); return q.a + " " + proto(q); },
  function c21(x) { seen.length = 0; var q = new Px(x, 1); return q.a + " " + proto(q) + " " + seen.join(","); },
  function c22(x) {
    var q = Reflect.construct(Pr, [x, 1, 2], NT), r = Reflect.construct(NT, [x, 1, 2], Pg);
    return [Object.getPrototypeOf(q) === NT.prototype, r.same, r.n, Object.getPrototypeOf(r) === Pg.prototype].join(" ");
  }
];
// each throws: in an argument before the new, in the constructor, and in a
// function the constructor calls, with temps owned below the new
var throwing = [
  function t0(x) { return x + new Th(x, "a", "b").ok; },
  function t1(x) { return new Z(x, new Th(x, "c", "d"), 3).n; },
  function t2(x) { return x + new ThS(x, 1, 2).ok; },
  function t3(x) { return [x, new Th2(x, "e", "f")].length; },
  function t4(x) { return new Bad(x, 5).a; },
  function t5(x) { return new Z(x, undef.nope, 1).n; },
  function t6(x) { var q = new Init(x, 1); q.initialize = null; return new Init(x, 1, 2).s + Init.prototype.nope.nope; }
];
var i, k, r;
for (k = 0; k < 3; k++) {
  for (i = 0; i < cases.length; i++)
    p("case " + i + ": " + cases[i](4));
  for (i = 0; i < throwing.length; i++) {
    try { r = throwing[i](k + 1); p("throwing " + i + ": no error " + r); }
    catch (e) { p("throwing " + i + ": " + e.name + ": " + e.message); p(e.stack); }
  }
}
rvExpect(M, S, Z, Init, NT, R, Th, ThS, Def, Rest, Deep, Big, Cap, Ret, B, Dd, Fld, Th2, Bad, Pr, Pg, Fresh, Pw);
rvExpect(cases[0], cases[1], cases[2], cases[3], cases[4], cases[5], cases[6], cases[7], cases[8], cases[9], cases[10]);
rvExpect(cases[11], cases[12], cases[13], cases[14], cases[15], cases[16], cases[17], cases[18], cases[19], cases[20]);
rvExpect(cases[21], cases[22]);
rvExpect(throwing[0], throwing[1], throwing[2], throwing[3], throwing[4], throwing[5], throwing[6]);
return out.join("\n");
})()
