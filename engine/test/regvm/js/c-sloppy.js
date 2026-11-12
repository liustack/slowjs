// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// rvdiff: sloppy constructors in the register loop. A sloppy function's this
// is coerced at its push_this, and in a construct call it is the object
// made, so the coercion copies it. new of a sloppy constructor from a sloppy
// and from a strict function of the loop, and from C (Reflect.construct);
// new.target in a sloppy constructor; a sloppy constructor called without
// new (its this is the global object); and a store to a frozen this, which
// a sloppy constructor ignores and a strict one throws on, by the mode of
// the constructor and not of the frame the loop was entered with. This file
// is sloppy, the strict functions say "use strict" in their bodies. Each
// runs several times, so that the later calls run in the register loop. ck
// catches and stays in the old loop.
(function () {
var out = [];
function p(x) { out.push(String(x)); }
function ck(f, a) {
  try { return "ok " + f(a); } catch (e) { return e.name + ": " + e.message; }
}
var G = this;
function S(a, b) { this.a = a; this.b = b; this.g = this === G; }
S.prototype.sum = function () { return this.a + this.b; };
function SN(x) { this.t = typeof new.target; this.same = new.target === SN; this.x = x; }
function SR(kind) { this.x = 1; if (kind === 1) return { y: 2 }; if (kind === 2) return 7; }
function SF(x) { Object.freeze(this); this.y = x; return this; }
function TF(x) { "use strict"; Object.freeze(this); this.y = x; return this; }
function SG(x) { this.sgx = x; return typeof this; }
var cases = [
  function s0(x) { var q = new S(x, 2); return q.sum() + "," + q.g + "," + (q instanceof S); },
  function s1(x) { "use strict"; var q = new S(x, 3); return q.sum() + "," + q.g; },
  function s2(x) { var q = new SN(x); return q.t + "," + q.same + "," + q.x; },
  function s3(x) { return new SR(0).x + "," + new SR(1).y + "," + new SR(2).x; },
  function s4(x) { var q = new SF(x); return q.y + "," + Object.isFrozen(q); },
  function s5(x) { var r = SG(x); var v = G.sgx; delete G.sgx; return r + "," + v; },
  function s6(x) { "use strict"; return new S(new S(x, 1).sum(), new SN(x).x).sum(); }
];
var i, k, r;
for (k = 0; k < 3; k++) {
  for (i = 0; i < cases.length; i++)
    p("case " + i + ": " + cases[i](k + 4));
  p("strict frozen: " + ck(function t0(x) { return new TF(x).y; }, k));
  p("sloppy frozen: " + ck(function t1(x) { "use strict"; return new SF(x).y; }, k));
  r = Reflect.construct(S, [k, 1], SN);
  p("reflect " + (Object.getPrototypeOf(r) === SN.prototype) + " " + r.a + " " + r.g);
  r = Reflect.construct(SN, [k], S);
  p("reflect nt " + r.t + " " + r.same + " " + (r instanceof S));
  p("reflect frozen " + ck(function (x) { return Reflect.construct(TF, [x]).y; }, k) +
    " " + ck(function (x) { return Reflect.construct(SF, [x]).y; }, k));
}
rvExpect(S, SN, SR, SF, TF, SG, cases[0], cases[1], cases[2], cases[3], cases[4], cases[5], cases[6]);
return out.join("\n");
})()
