// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.
// rvdiff: o.name (GETF, GETFL, GETFT and the read of a method before CALLM)
// on arrays and arguments objects, whose methods are on the prototype, at
// sites that have read before: a method of Array.prototype replaced, deleted,
// made an accessor whose getter logs, made read-only, an own property of the
// array over it, the prototype of the array changed or null, a subclass of
// Array with and without its own method, an array that stops being fast, a
// frozen array, an array with a named key, a key added to Array.prototype and
// to Object.prototype, a proxy of an array, strict and sloppy arguments,
// typed arrays, and arrays of another realm. Each read is checked against
// Reflect.get or against what the script set, a getter writes to a log, and
// each test runs three times so that the later runs read through a warm site.
// inject-hit: GETF 1
(function () {
var out = [], log = [], bad = 0;
// out and log are filled by index: the tests delete and replace Array.prototype.push
function show(v) {
  if (typeof v === "function") return "fn " + v.name;
  if (v !== null && typeof v === "object") return "obj";
  if (typeof v === "symbol") return "sym";
  return typeof v + " " + String(v);
}
function guard(f, o) {
  try { return show(f(o)); } catch (e) { return "throws " + e.name + " " + e.message; }
}
// one site each: GETF into a temp, GETFL into a local, GETFT on a temp
function rdPush(o) { return o.push; }
function rdPushL(o) { var v; v = o.push; return v; }
function rdPushT(o) { return [o][0].push; }
function rdHas(o) { return o.hasOwnProperty; }
function rdStr(o) { return o.toString; }
function rdZz(o) { return o.zz; }
function rdLen(o) { return o.length; }
function callJoin(o) { return o.join("-"); }
function callPush(o) { return o.push(9); }
function check(name, o, rd, want) {
  var a = guard(rd, o);
  var ok = want === undefined ? true : a === want;
  if (!ok) bad++;
  out[out.length] = name + " " + a + (ok ? "" : " MISMATCH want " + want);
}
function all(name, o) {
  var w = show(Reflect.get(Object(o), "push", o));
  check(name + " .push", o, rdPush, w);
  check(name + " .push local", o, rdPushL, w);
  check(name + " .push temp", o, rdPushT, w);
  check(name + " .hasOwnProperty", o, rdHas, show(Reflect.get(Object(o), "hasOwnProperty", o)));
  check(name + " .toString", o, rdStr, show(Reflect.get(Object(o), "toString", o)));
}
var AP = Array.prototype, push0 = AP.push, join0 = AP.join, toString0 = Object.prototype.toString;

function run(round) {
  out[out.length] = "-- round " + round;
  // plain arrays of several lengths and kinds of element
  var a = [1, 2, 3];
  all("array", a);
  all("empty", []);
  all("strings", ["x", "y"]);
  all("new Array", new Array(4));
  check("join", a, callJoin, "string 1-2-3");
  check("push call", [1], callPush, "number 2");
  check("length", a, rdLen, "number 3");
  // a method of Array.prototype replaced, then put back
  AP.join = function joinR() { return "replaced " + this.length; };
  check("join replaced", a, callJoin, "string replaced 3");
  AP.join = join0;
  check("join back", a, callJoin, "string 1-2-3");
  // deleted, made a getter with side effects, made read-only, put back
  delete AP.push;
  all("push deleted", a);
  check("push deleted call", a, callPush);
  Object.defineProperty(AP, "push", { get: function () { log[log.length] = "get push"; return push0; }, configurable: true });
  var n0 = log.length;
  all("push getter", a);
  check("push getter call", [1, 2], callPush, "number 3");
  check("getter calls", log.length - n0, function (n) { return n; }, "number 5");
  Object.defineProperty(AP, "push", { value: push0, writable: false, enumerable: false, configurable: true });
  all("push read-only", a);
  Object.defineProperty(AP, "push", { value: push0, writable: true, enumerable: false, configurable: true });
  all("push back", a);
  // an own property of the array over the method
  var b = [4, 5];
  all("before own", b);
  b.push = function ownPush() { return "own"; };
  all("own push", b);
  check("own push call", b, callPush, "string own");
  delete b.push;
  all("own push gone", b);
  check("own push gone call", b, callPush, "number 3");
  // a named key on the array, frozen and sealed arrays
  var c = [1];
  c.extra = "e";
  all("named key", c);
  all("frozen", Object.freeze([1, 2]));
  all("sealed", Object.seal([1, 2]));
  // an array that stops being fast
  var s = [1, 2, 3];
  all("fast", s);
  s[100000] = 1;
  all("not fast", s);
  check("not fast call", s, callPush, "number 100002");
  var h = [1, 2];
  delete h[0];
  all("hole", h);
  // the prototype of the array changed or null
  var p = [1, 2];
  all("before set proto", p);
  Object.setPrototypeOf(p, { push: function protoPush() { return "proto"; } });
  all("set proto", p);
  check("set proto call", p, callPush, "string proto");
  Object.setPrototypeOf(p, null);
  all("null proto", p);
  Object.setPrototypeOf(p, AP);
  all("proto back", p);
  // subclasses of Array, with and without their own method
  class Plain extends Array {}
  class Own extends Array { push(v) { return "sub " + v; } }
  var pl = new Plain(), ow = new Own();
  pl.push(1);
  all("subclass", pl);
  all("subclass own", ow);
  check("subclass own call", ow, callPush, "string sub 9");
  check("subclass call", pl, callPush, "number 2");
  Plain.prototype.push = function plainPush() { return "plain"; };
  all("subclass gains", pl);
  check("subclass gains call", pl, callPush, "string plain");
  delete Plain.prototype.push;
  all("subclass loses", pl);
  // keys added to Array.prototype and Object.prototype
  check("zz none", a, rdZz, "undefined undefined");
  AP.zz = "on array proto";
  check("zz array proto", a, rdZz, "string on array proto");
  delete AP.zz;
  check("zz gone", a, rdZz, "undefined undefined");
  Object.prototype.zz = "on object proto";
  check("zz object proto", a, rdZz, "string on object proto");
  delete Object.prototype.zz;
  check("zz gone again", a, rdZz, "undefined undefined");
  Object.prototype.toString = function strR() { return "str"; };
  all("toString replaced", a);
  Object.prototype.toString = toString0;
  all("toString back", a);
  // a proxy of an array, and an object whose prototype is one
  var px = new Proxy([1, 2], { get: function (t, k) { log[log.length] = "trap " + String(k); return t[k]; } });
  n0 = log.length;
  all("proxy", px);
  check("proxy call", px, callJoin, "string 1-2");
  all("under proxy", Object.create(px));
  check("trap calls", log.length - n0, function (n) { return n; }, "number 20");
  // arguments, strict and sloppy, and arguments given an Array method
  var st = (function () { "use strict"; return arguments; })(1, 2);
  var sl = (function (x, y) { return arguments; })(1, 2);
  all("strict arguments", st);
  all("sloppy arguments", sl);
  check("strict arguments length", st, rdLen, "number 2");
  check("sloppy arguments length", sl, rdLen, "number 2");
  Object.prototype.push = function objPush() { return "obj push"; };
  all("arguments with object push", st);
  check("arguments push call", st, callPush, "string obj push");
  delete Object.prototype.push;
  all("arguments push gone", st);
  st.push = push0;
  check("arguments own push call", st, callPush, "number 3");
  // typed arrays
  var ta = new Int8Array([1, 2, 3]), fa = new Float64Array(2);
  all("typed", ta);
  all("typed float", fa);
  check("typed join", ta, callJoin, "string 1-2-3");
  check("typed length", ta, rdLen, "number 3");
  Int8Array.prototype.join = function i8Join() { return "i8"; };
  check("typed join own proto", ta, callJoin, "string i8");
  delete Int8Array.prototype.join;
  check("typed join back", ta, callJoin, "string 1-2-3");
  // many receivers in turn at one site
  var turns = [a, pl, ow, st, sl, ta, px, { push: function objOwn() { return "o"; } }, "str", 7];
  for (var k = 0; k < 3; k++)
    for (var i = 0; i < turns.length; i++)
      all("turn " + i, turns[i]);
  // arrays of another realm, read by code of this realm and the other way round
  if (typeof newRealm === "function") {
    var other = newRealm("({ rd: function (o) { return o.push; }, join: function (o) { return o.join('+'); }," +
      " make: function () { return [7, 8]; }, setJoin: function (f) { Array.prototype.join = f; } })");
    var theirs = other.make();
    all("realm array", theirs);
    check("realm code mine", a, other.rd, show(push0));
    check("realm code theirs", theirs, other.rd, show(Reflect.get(theirs, "push")));
    check("realm join", theirs, callJoin, "string 7-8");
    check("realm code join", a, other.join, "string 1+2+3");
    var j1 = Reflect.get(theirs, "join");
    other.setJoin(function otherJoin() { return "other"; });
    check("realm join replaced", theirs, callJoin, "string other");
    check("realm join here", a, callJoin, "string 1-2-3");
    other.setJoin(j1);
    check("realm join back", theirs, callJoin, "string 7-8");
  }
}
for (var round = 0; round < 3; round++)
  run(round);
out[out.length] = bad ? bad + " MISMATCH" : "all match";
return out.join("\n");
})()
