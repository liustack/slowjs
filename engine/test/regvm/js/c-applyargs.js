// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// rvdiff: f.apply(t, arguments) in a function whose arguments object is
// not built yet (CALLMA, engine/regvm-ops-args.h): the loop calls f with
// the frame's own arguments when apply is Function.prototype.apply of this
// realm and f a function of bytecode of this realm, and the stack of an
// Error made or thrown in f keeps its "at apply (native)" line. Sloppy
// functions (a mapped object, the parameters as they are now) and strict
// ones, 0 to 300 arguments, more and fewer than the parameters, the result
// kept, dropped and returned, a forward through a forward, one in a
// constructor (the pattern of Prototype's Class.create); and those left to
// apply: another realm's apply or target, a C function, a bound function,
// a proxy, a class constructor, a value that is not a function, apply
// replaced, read through a getter, deleted or shadowed on f, and the
// object built before the call.
// rv-expect-op: CALLMA
// inject-hit: CALLMA 1
(function () { "use strict";
var out = [];
function p(x) { out.push(String(x)); }
var E = Error;
function lines(s) { return s.split("\n").slice(0, 6).join(" | "); }
function S(params, body) {
  return Function("E", "lines", "return function (" + params + ") {" + body + "}")(E, lines);
}
function show() {
  var r = [this === undefined ? "u" : this === globalThis ? "g" : typeof this === "object" ? this.k : String(this),
           arguments.length];
  for (var i = 0; i < arguments.length && i < 6; i++)
    r.push(arguments[i]);
  return r.join(":");
}
function st(k) { return k + " " + lines(new E("st").stack); }
function thr(k) { throw new E("thr " + k); }
function nt() { return new.target === undefined ? "u" : "new"; }
class C { constructor() { this.c = 1; } }
var o = { k: "o" };

// the forwards: sloppy, strict, with parameters
var fwd = S("", "return this.f.apply(this.t, arguments);");
var fwdS = function () { return this.f.apply(this.t, arguments); };
var fwd3 = S("a, b, c", "return this.f.apply(this.t, arguments);");
var fwd3S = function (a, b, c) { return this.f.apply(this.t, arguments); };
// the parameters written before the forward: the mapped object has the new
// values below the count of arguments
var written = S("a, b", "a = 'A'; if (arguments.length > 3) b = 'B'; return this.f.apply(this.t, arguments);");
var viaClosure = S("a", "var set = function (v) { a = v; }; set('closure'); return this.f.apply(this.t, arguments);");
// the result dropped, kept, a forward through a forward
var dropped = S("", "this.f.apply(this.t, arguments); return 'dropped';");
var twice = S("", "var r = this.f.apply(this.t, arguments); return r + '/' + this.f.apply(this.t, arguments);");
var inner = { f: show, t: o };
var outer = { f: function () { return fwd.apply(inner, arguments); }, t: null };
// Class.create of Prototype: the constructor forwards to initialize
function Klass() { this.initialize.apply(this, arguments); }
Klass.prototype.initialize = function (a, b, c) { this.v = [a, b, c, arguments.length].join(","); };
var KlassS = S("", "this.initialize.apply(this, arguments);");
KlassS.prototype.initialize = Klass.prototype.initialize;

function call(name, w, args) {
  var r;
  try { r = w.apply({ f: show, t: o }, args); } catch (e) { r = "throws " + e.name + ": " + e.message; }
  p(name + " " + r);
}
function calls(name, fn, t) {
  var self = { f: fn, t: t };
  [[], ["x"], ["x", "y"], [1, 2, 3], [1, 2, 3, 4, 5, 6, 7]].forEach(function (a) {
    [fwd, fwdS, fwd3, fwd3S].forEach(function (w, i) {
      var r;
      try { r = w.apply(self, a); } catch (e) { r = "throws " + e.name + ": " + e.message; }
      p(name + " " + i + " " + a.length + " " + r);
    });
  });
}
calls("show", show, o);
calls("show undefined", show, undefined);
calls("show 7", show, 7);
calls("arrow", () => "arrow", o);
calls("nt", nt, o);
calls("bound", show.bind(o, "b"), null);
calls("proxy", new Proxy(show, {}), o);
calls("class", C, o);
calls("max", Math.max, null);
calls("not callable", {}, o);
calls("number", 5, o);
calls("generator", function* g(a) { yield a; }, o);
var big = [];
for (var i = 0; i < 300; i++)
  big.push(i);
call("300", fwd, big);
call("300 strict", fwdS, big);
var self = { f: show, t: o };
p("written " + written.call(self, 1) + " # " + written.call(self, 1, 2) + " # " + written.call(self, 1, 2, 3, 4));
p("written none " + written.call(self));
p("closure " + viaClosure.call(self, "x") + " # " + viaClosure.call(self));
p("dropped " + dropped.call(self, 1, 2));
p("twice " + twice.call(self, 1, 2));
p("nested " + outer.f("n1", "n2"));
p("klass " + new Klass(1, 2).v + " # " + new Klass(1, 2, 3, 4).v + " # " + new KlassS().v);

// stacks: made and thrown in the callee, through two forwards
var stSelf = { f: st, t: null };
p("stack " + fwd.call(stSelf, "s1"));
p("stack strict " + fwdS.call(stSelf, "s2"));
p("stack nested " + fwd.call({ f: function (k) { return fwdS.call(stSelf, k); }, t: null }, "s3"));
try { fwd.call({ f: thr, t: null }, "t1"); } catch (e) { p("thrown " + lines(e.stack)); }
try { fwd3S.call({ f: C, t: null }); } catch (e) { p("class " + e.name + " " + lines(e.stack)); }
try { fwd.call({ f: JSON.parse, t: null }, "{"); } catch (e) { p("c function " + e.name + " " + lines(e.stack)); }

// another realm: its apply, its function
var other = newRealm('({ apply: Function.prototype.apply,' +
  ' f: function () { return [arguments.length, Object.getPrototypeOf([]) === Array.prototype].join(":"); },' +
  ' st: function () { return new Error("o").stack.split("\\n").slice(0, 4).join(" | "); } })');
var oapp = S("", "var g = function () { return arguments.length + ':' + this.k; }; g.apply = this.apply; return g.apply(this.t, arguments);");
p("other apply " + oapp.call({ apply: other.apply, t: o }, 1, 2));
p("other target " + fwd.call({ f: other.f, t: null }, 1, 2, 3));
p("other stack " + fwd.call({ f: other.st, t: null }));

// apply changed: replaced, a getter, deleted, shadowed on the callee
var FP = Function.prototype, app = FP.apply;
var self2 = { f: show, t: o };
FP.apply = function (t, a) { return "replaced " + a.length; };
p(fwd.call(self2, 1, 2) + " # " + fwdS.call(self2, 1));
FP.apply = app;
var reads = 0;
Object.defineProperty(FP, "apply", { configurable: true, get: function () { reads++; return app; } });
p("getter " + fwd.call(self2, 1) + " " + fwdS.call(self2, 1, 2) + " reads " + reads);
Object.defineProperty(FP, "apply", { configurable: true, writable: true, value: app });
delete FP.apply;
try { fwd.call(self2, 1); } catch (e) { p("deleted " + e.name + ": " + e.message); }
FP.apply = app;
var own = function () { return "own"; };
own.apply = function (t, a) { return "shadowed " + a.length; };
p(fwd.call({ f: own, t: o }, 1, 2, 3));
// the object built before the forward, changed through it
var built = S("a", "var x = arguments; x[0] = 'changed'; x[3] = 'three'; return this.f.apply(this.t, arguments);");
p("built " + built.call(self2, 1) + " # " + built.call(self2, 1, 2));
var builtS = function (a) { var n = arguments.length; var x = arguments; return this.f.apply(this.t, arguments) + ":" + (x === arguments) + ":" + n; };
p("built strict " + builtS.call(self2, 1, 2));
// the depth a recursion through the forward reaches, sloppy and strict
var depth = { n: 0 };
var recur = S("n", "this.n = n; n++; return this.r.apply(this, arguments);");
var recurS = function (n) { this.n = n; return recurS.apply(this, [n + 1]); };
var recurF = function (n) { this.n = n; return recurF2.apply(this, arguments); };
var recurF2 = function (n) { return recurF.call(this, n + 1); };
[["sloppy", recur], ["array", recurS], ["strict", recurF]].forEach(function (e) {
  depth.n = 0;
  depth.r = e[1];
  try { e[1].call(depth, 0); } catch (x) { p("depth " + e[0] + " " + depth.n + " " + x.name + ": " + x.message); }
});
// apply named with a string and an argument named arguments
var named = S("", "return this.f['apply'](this.t, arguments);");
p("named " + named.call(self2, "q"));

rvExpect(fwd, fwdS, fwd3, fwd3S, written, viaClosure, dropped, twice, Klass, KlassS, built, named);
return out.join("\n");
})()
