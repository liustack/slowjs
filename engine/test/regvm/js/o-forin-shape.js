// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// rvdiff: for in loops whose iterator objects (a null prototype) start from
// the empty shape the context keeps: loops of this realm and of another one
// (newRealm) over objects of both realms, one inside the other, generators of
// both realms stopped inside a loop across collections, objects with a null
// prototype that get keys, are frozen and lose keys between the loops, and a
// proxy whose traps log their order. The golden was recorded on the engine
// before the change.
(function () {
"use strict";
var out = [];
function keys(o) {
  var r = [];
  for (var k in o) r.push(k);
  return r.join(",");
}
function* gen(o) { for (var k in o) yield k; }
var other = newRealm(
  "({ keys: function (o) { var r = []; for (var k in o) r.push(k); return r.join(','); }," +
  "   gen: function* (o) { for (var k in o) yield k; }," +
  "   nul: function () { var o = Object.create(null); o.q = 1; return o; }," +
  "   obj: { x: 1, y: 2 } })");
out.push("this realm " + [keys({ a: 1 }), keys(other.obj), keys(other.nul()), keys(null)].join("|"));
out.push("other realm " + [other.keys({ a: 1 }), other.keys(other.obj), other.keys(Object.create(null)), other.keys(undefined)].join("|"));
var nested = [];
for (var a in { p: 1, q: 2 }) nested.push(a + ":" + other.keys({ r: 1, s: 2 }));
out.push("nested " + nested.join(","));
// generators of both realms, stopped inside their loops, across collections
var gs = [gen({ a: 1, b: 2 }), other.gen({ c: 1, d: 2 }), gen(other.obj), other.gen([5, 6])];
var first = gs.map(function (g) { return g.next().value; });
gc();
var made = [];
for (var i = 0; i < 20; i++) made.push(keys({ ["k" + i]: i }));
gc();
var second = gs.map(function (g) { return g.next().value; });
var done = gs.map(function (g) { return g.next().done; });
out.push("generators " + first.join(",") + " " + second.join(",") + " " + done.join(",") + " " + made.length);
// objects with a null prototype between loops
var nulls = [];
for (var j = 0; j < 5; j++) {
  var o = Object.create(null);
  nulls.push(keys(o) || "-");
  o["n" + j] = j;
  if (j % 2) Object.freeze(o);
  else delete o["n" + j];
  nulls.push(keys(o) || "-", Reflect.ownKeys(Object.create(null)).length);
}
out.push("nulls " + nulls.join(","));
// a proxy, its traps in their order
var log = [];
var p = new Proxy({ a: 1, b: 2 }, {
  ownKeys: function (t) { log.push("ownKeys"); return Reflect.ownKeys(t); },
  getOwnPropertyDescriptor: function (t, k) { log.push("gopd " + String(k)); return Reflect.getOwnPropertyDescriptor(t, k); },
  getPrototypeOf: function (t) { log.push("proto"); return Reflect.getPrototypeOf(t); }
});
out.push("proxy " + keys(p) + " [" + log.join(";") + "]");
log = [];
out.push("proxy other realm " + other.keys(p) + " [" + log.join(";") + "]");
// a loop left by a throw from a getter on the prototype chain
var thrower = Object.create({ get g() { throw new Error("g"); } });
thrower.own = 1;
var left;
try { for (var k in thrower) left = k + thrower[k] + thrower.g; } catch (e) { left = "threw " + e.message; }
out.push("left " + left);
rvExpect(keys);
return out.join("\n");
})()
