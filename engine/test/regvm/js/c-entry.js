// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// rvdiff: the ways into a constructor of the register loop that pass
// through C: Reflect.construct with a new.target whose prototype is read
// through a proxy (in order with the arguments), is not an object, or
// throws; a proxy around a constructor, with and without a construct trap;
// bound functions, bound twice; new.target that is a bound function or a
// proxy; Function.prototype.apply style spread (new X(...a)), and
// constructors called back from a built-in.
(function () { "use strict";
var out = [];
var log = [];
function p(x) { out.push(String(x)); }
var E = Error;
function P(a, b) { log.push("P"); this.a = a; this.b = b; this.nt = new.target === undefined ? "u" : typeof new.target; }
P.prototype.sum = function () { return this.a + this.b; };
class C { constructor(x) { log.push("C"); this.x = x; } }
class D extends C { constructor(x) { super(x); this.d = new.target === D; } }
function NoProto() { this.k = 1; }
var protoLog = new Proxy(function () {}, {
  get: function (t, k) { log.push("get " + String(k)); return k === "prototype" ? P.prototype : t[k]; }
});
var protoThrow = new Proxy(function () {}, {
  get: function (t, k) { throw new E("no prototype"); }
});
var protoNum = function () {};
protoNum.prototype = 5;
var trapped = new Proxy(P, { construct: function (t, args, nt) { log.push("trap"); return new t(args[0], 100); } });
var plain = new Proxy(P, {});
var plainC = new Proxy(D, {});
var B1 = P.bind(null, 1), B2 = B1.bind(null, 2), BD = D.bind(null, 3);
function viaSpread(a) { return new P(...a).sum(); }
function viaCb(xs) { return xs.map(function (x) { return new P(x, x).sum(); }).join(","); }
var i, r;
for (i = 0; i < 3; i++) {
  log = [];
  r = Reflect.construct(P, [i, 1], protoLog);
  p("proxy nt " + r.sum() + " " + r.nt + " " + log.join(","));
  log = [];
  try { Reflect.construct(P, [i], protoThrow); } catch (e) { p("proxy throws " + e.message + " " + log.join(",")); }
  log = [];
  r = Reflect.construct(P, [i, 2], protoNum);
  p("not an object " + (Object.getPrototypeOf(r) === Object.prototype) + " " + r.a + " " + log.join(","));
  log = [];
  r = Reflect.construct(D, [i], protoLog);
  p("derived proxy nt " + r.x + " " + r.d + " " + log.join(","));
  log = [];
  r = new trapped(i);
  p("trap " + r.sum() + " " + log.join(","));
  log = [];
  r = new plain(i, 3);
  p("plain proxy " + r.sum() + " " + r.nt + " " + log.join(","));
  r = new plainC(i);
  p("plain proxy class " + r.x + " " + r.d);
  r = new B2();
  p("bound " + r.sum() + " " + r.nt + " " + (r instanceof P));
  r = new BD();
  p("bound class " + r.x + " " + r.d);
  try { r = Reflect.construct(P, [i, 4], B1); p("bound nt " + r.sum()); } catch (e) { p("bound nt " + e.name + ": " + e.message); }
  r = Reflect.construct(NoProto, [], NoProto);
  p("noproto " + r.k);
  p("spread " + viaSpread([i, 5]) + " " + viaSpread([i]));
  p("cb " + viaCb([i, i + 1]));
}
rvExpect(P, C, D, NoProto, viaSpread, viaCb);
return out.join("\n");
})()
