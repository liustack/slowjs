// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// rvdiff: special_object in a function of the register loop: the function
// itself (a named function expression that reads its name) and new.target,
// which is undefined in every frame of the register loop, since new runs the
// constructor in the old loop. Called directly, as a method, through call,
// apply and bind, from a C function, with new, and recursively, several times
// each so that the later calls run in the register loop.
(function () { "use strict";
var out = [];
function p(x) { out.push(String(x)); }
var A = Array, undef;
var count = function f(n) { if (n < 1) return 0; return f(n - 1) + 1; };
var self = function s(x) { return s; };
var selfUse = function u(x) { var g = u; return g.k + x; };
selfUse.k = 10;
function nt() { return new.target; }
function nt2(x) { var t = new.target; if (t === undef) return x + 1; this.y = x; }
var both = function b(x) { var t = new.target; if (t === b) return x; if (t === undef) return b; return 0; };
var o = { m: nt, n: nt2, b: both };
var i, r;
for (i = 0; i < 3; i++) {
  p("count " + count(10) + " " + count(0) + " " + count(1000));
  p("self " + (self() === self) + " " + (self.call(null, 1) === self) + " " + ([1].map(self)[0] === self));
  p("selfUse " + selfUse(1) + " " + selfUse("s"));
  p("nt " + nt() + " " + o.m() + " " + nt.call(o) + " " + nt.apply(null, []) + " " + (new nt() === nt));
  p("nt2 " + nt2(1) + " " + o.n(2) + " " + new nt2(3).y + " " + [4].map(nt2)[0] + " " + nt2.bind(null, 5)() + " " + new (nt2.bind(null, 6))().y);
  r = new both(7);
  p("both " + (both(1) === both) + " " + (o.b(2) === both) + " " + (typeof r) + " " + (Reflect.construct(both, [8], nt) instanceof nt));
}
rvExpect(nt, nt2, p);
return out.join("\n");
})()
