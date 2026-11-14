// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// rvdiff: the step of a for in loop and the read o[k] of the key it just
// gave, where Node 24 cannot be the reference: loops of this realm and of
// another one (newRealm) over objects of both, generators of both realms
// stopped inside a loop across collections while their objects change, the
// order of the traps of a proxy read in the body, a prototype set during the
// loop (whose keys QuickJS then lists, an order the specification leaves to
// the engine), objects listed from a list rather than their shape (a key
// deleted before the loop), and a loop in a with. The golden was recorded on
// the engine before the change.
(function () {
"use strict";
var out = [];
function pairs(o) {
  var r = [];
  for (var k in o) r.push(k + "=" + String(o[k]));
  return r.join(",");
}
function* gen(o) { for (var k in o) yield k + "=" + o[k]; }
var other = newRealm(
  "({ pairs: function (o) { var r = []; for (var k in o) r.push(k + '=' + String(o[k])); return r.join(','); }," +
  "   gen: function* (o) { for (var k in o) yield k + '=' + o[k]; }," +
  "   obj: { x: 1, y: 2, z: 3 }," +
  "   make: function () { return { x: 10, y: 20, z: 30 }; } })");
out.push("this realm " + [pairs({ a: 1, b: 2 }), pairs(other.obj), pairs(other.make())].join("|"));
out.push("other realm " + [other.pairs({ a: 1, b: 2 }), other.pairs(other.obj), other.pairs([7, 8])].join("|"));
// the read on an object of the other realm with the shape of the loop's
var mine = other.make(), r = [];
for (var k in other.make()) r.push(k + "=" + mine[k]);
out.push("other shape read " + r.join(","));
var nested = [];
for (var a in { p: 1, q: 2 }) nested.push(a + ":" + other.pairs({ r: a, s: 2 }));
out.push("nested " + nested.join(","));
// generators of both realms, stopped inside their loops, across collections
var o1 = { a: 1, b: 2, c: 3 }, o2 = other.make();
var gs = [gen(o1), other.gen(o2), gen(o2), other.gen(o1)];
var first = gs.map(function (g) { return g.next().value; });
gc();
delete o1.b;
o2.w = 40;
var made = [];
for (var i = 0; i < 20; i++) made.push(pairs({ ["k" + i]: i }));
gc();
var second = gs.map(function (g) { return g.next().value; });
var third = gs.map(function (g) { return g.next().value; });
var done = gs.map(function (g) { return String(g.next().done); });
out.push("generators " + first.join(",") + " " + second.join(",") + " " + third.join(",") + " " + done.join(",") + " " + made.length);
// a proxy: its traps in their order, with the reads of the body
var log = [];
var p = new Proxy({ a: 1, b: 2 }, {
  ownKeys: function (t) { log.push("ownKeys"); return Reflect.ownKeys(t); },
  getOwnPropertyDescriptor: function (t, k) { log.push("gopd " + String(k)); return Reflect.getOwnPropertyDescriptor(t, k); },
  getPrototypeOf: function (t) { log.push("proto"); return Reflect.getPrototypeOf(t); },
  get: function (t, k) { log.push("get " + String(k)); return t[k]; }
});
out.push("proxy " + pairs(p) + " [" + log.join(";") + "]");
// a proxy on the prototype chain
log = [];
var q = Object.create(p);
q.own = 0;
out.push("proxy proto " + pairs(q) + " [" + log.join(";") + "]");
// the prototype set during the loop
var o = { a: 1, b: 2, c: 3 };
r = [];
for (var k2 in o) { if (k2 === "b") Object.setPrototypeOf(o, { c: "proto", z: "pz" }); r.push(k2 + "=" + o[k2]); }
out.push("prototype set " + r.join(","));
// keys listed from a list: a key deleted before the loop
var d = { a: 1, b: 2, c: 3, e: 5 };
delete d.b;
r = [];
for (var k3 in d) { r.push(k3 + "=" + d[k3]); if (k3 === "a") delete d.c; }
out.push("listed " + r.join(","));
out.push("listed again " + pairs(d));
out.push(String(function () {
  var w = { a: 1, b: 2 }, s = [];
  return (0, eval)("(function (w, s) { with (w) { for (var k in w) s.push(k + '=' + w[k] + a); } return s.join(','); })")(w, s);
}()));
return out.join("\n");
})()
