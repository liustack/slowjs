// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// rvdiff: calls of the loop through Function.prototype.call and bound
// functions with no bound arguments, which the loop takes apart and makes
// as the call they stand for: f.call(t, ...), call.bind(f) (the pattern of
// core-js), call.call(call, f, ...), chains of bound functions (one of
// 100000, taken apart with no recursion), f.call() with no arguments, more
// and fewer arguments than parameters, a function that reads its
// arguments, a C function, a generator, a class constructor; and those it
// leaves as they were: a bound function with bound arguments, a proxy, a
// target that is not callable, a function and a call of another realm;
// new.target in each, and the stack of an Error made through them.
(function () { "use strict";
var out = [];
function p(x) { out.push(String(x)); }
var E = Error;
var call = Function.prototype.call;
var ucall = call.bind(call);
function lines(s) { return s.split("\n").slice(0, 5).join(" | "); }
function add(a, b) { return (this === undefined ? "u" : this.k) + ":" + a + ":" + b; }
function nt() { return new.target === undefined ? "u" : typeof new.target; }
function argn() { return arguments.length + ":" + Array.prototype.join.call(arguments, ","); }
function st(k) { return k + " " + lines(new E("st").stack); }
function thr(k) { throw new E("thr " + k); }
function* gen(a) { yield this.k + a; }
class C { constructor() { this.c = 1; } }
var sloppy = Function("return this === globalThis");
var o = { k: 7 };
var b1 = add, i;
for (i = 0; i < 50; i++)
  b1 = b1.bind(o);
var deep = add;
for (i = 0; i < 100000; i++) {
  deep = deep.bind(o);
  Object.defineProperty(deep, "name", { value: "" });
}
var bst = st.bind(null), bst2 = bst.bind(null);
var bargs = add.bind(o, 1);
var bcall = call.bind(st, null, 3);
var prox = new Proxy(add, {});
var proxCall = new Proxy(call, {});
var other = newRealm('"use strict"; ({' +
  ' f: function (a) { return [a, Object.getPrototypeOf([]) === Array.prototype]; },' +
  ' call: Function.prototype.call,' +
  ' st: function (k) { return new Error("o").stack.split("\\n").slice(0, 4).join(" | "); } })');
function viaCall(k) { return add.call(o, k, k + 1); }
function viaUcall(k) { return ucall(add, o, k, k + 2); }
function viaCallCall(k) { return call.call(call, add, o, k, 3); }
function viaChain(k) { return b1(k, 4); }
function viaDeep(k) { try { return deep(k, 5); } catch (e) { return e.name; } }
function noArgs() { return add.call() + " " + nt.call() + " " + sloppy.call() + " " + argn.call(); }
function oneArg() { return add.call(o) + " " + argn.call(o); }
function moreArgs(k) { return add.call(o, k, 2, 3, 4) + " " + argn.call(o, k, 2, 3, 4, 5) + " " + ucall(argn, o, k); }
function cfun(k) { return call.call(Math.max, null, k, 9, 2) + " " + ucall(Math.min, null, k, -1); }
function viaGen(k) { return gen.call(o, k).next().value; }
function newTarget(k) { return nt.call(o) + nt.bind(o)() + ucall(nt, o) + " " + typeof new (nt.bind(o))() + " " + (new (nt.bind(null))() instanceof nt); }
function stacks(k) {
  return [st.call(null, k), ucall(st, null, k), bst(k), bst2(k), call.call(call, st, null, k)].join(" # ");
}
function kept(k) {
  return [bargs(k), call.call(prox, o, k, 5), proxCall.call(add, o, k, 6), bcall(),
          add.bind(o, k)(8)].join(" # ");
}
function thrown(k) {
  var r = [];
  try { thr.call(null, k); } catch (e) { r.push(lines(e.stack)); }
  try { ucall(thr, null, k); } catch (e) { r.push(lines(e.stack)); }
  try { call.call(JSON.parse, null, "{"); } catch (e) { r.push(e.name + " " + lines(e.stack)); }
  return r.join(" # ");
}
function notCallable(k) {
  var r = [];
  try { call.call(k); } catch (e) { r.push(e.name + ": " + e.message); }
  try { call.call({}, null); } catch (e) { r.push(e.name + ": " + e.message); }
  try { ucall(undefined, null); } catch (e) { r.push(e.name + ": " + e.message); }
  try { C.call(o); } catch (e) { r.push(e.name + ": " + e.message); }
  try { ucall(C, o); } catch (e) { r.push(e.name + ": " + e.message); }
  return r.join(" # ");
}
function realms(k) {
  return [other.f.call(null, k), ucall(other.f, null, k), other.call.call(add, o, k, 1),
          other.f.bind(null)(k), other.st.call(null, k)].join(" # ");
}
for (i = 0; i < 3; i++) {
  p("call " + viaCall(i));
  p("ucall " + viaUcall(i));
  p("callcall " + viaCallCall(i));
  p("chain " + viaChain(i));
  p("deep " + viaDeep(i));
  p("noargs " + noArgs());
  p("onearg " + oneArg());
  p("more " + moreArgs(i));
  p("cfun " + cfun(i));
  p("gen " + viaGen(i));
  p("newtarget " + newTarget(i));
  p("stacks " + stacks(i));
  p("kept " + kept(i));
  p("thrown " + thrown(i));
  p("notcallable " + notCallable(i));
  p("realms " + realms(i));
}
rvExpect(add, nt, argn, st, thr, viaCall, viaUcall, viaCallCall, viaChain, viaDeep, noArgs, oneArg, moreArgs);
rvExpect(cfun, viaGen, newTarget, stacks, kept, thrown, notCallable, realms);
return out.join("\n");
})()
