// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// for in loops one after another and inside each other, each with an iterator
// object of its own whose empty shape (null prototype) the context now keeps:
// objects, arrays, strings, String objects, null and undefined (no turn),
// numbers, keys deleted and added during a loop, keys shadowed on the
// prototype chain, keys that are not enumerable, a loop left by break, return
// and throw, loops in loops, generators stopped inside a loop and run on
// later, and objects with a null prototype that get keys between the loops,
// which start from the same empty shape. The file is what Node 24 prints.
"use strict";
var out = [];
function show(name, f) {
  var r;
  try { r = f(); } catch (e) { r = "throws " + e.name + " " + e.message; }
  out.push(name + ": " + r);
}
function keys(o) {
  var r = [];
  for (var k in o) r.push(k);
  return r.join(",");
}

show("plain", function () { return keys({ a: 1, b: 2, c: 3 }); });
show("array", function () { return keys([5, 6, 7]); });
show("sparse array", function () { var a = [1, , 3]; a.x = 1; return keys(a); });
show("string", function () { return keys("xyz"); });
show("String object", function () { var s = new String("ab"); s.extra = 1; return keys(s); });
show("null and undefined", function () { return "[" + keys(null) + "][" + keys(undefined) + "]"; });
show("number and boolean", function () { return "[" + keys(5) + "][" + keys(true) + "]"; });
show("null prototype", function () { var o = Object.create(null); o.b = 1; o.a = 2; o[2] = 3; return keys(o); });
show("frozen null prototype", function () { var o = Object.create(null); o.k = 1; return keys(Object.freeze(o)); });

// keys changed during the loop
show("deleted during", function () {
  var o = { a: 1, b: 2, c: 3, d: 4 }, r = [];
  for (var k in o) { r.push(k); delete o.c; }
  return r.join(",");
});
show("added during", function () {
  var o = { a: 1, b: 2 }, r = [];
  for (var k in o) { r.push(k); o["n" + k] = 1; }
  return r.join(",") + " " + Object.keys(o).join(",");
});
show("deleted and added again", function () {
  var o = { a: 1, b: 2, c: 3 }, r = [];
  for (var k in o) { r.push(k); if (k === "a") { delete o.b; o.b = 5; } }
  return r.join(",");
});

// the prototype chain
show("shadowed", function () {
  var base = { a: 1, b: 2, z: 3 };
  var mid = Object.create(base); mid.b = 20; mid.m = 1;
  var top = Object.create(mid); top.a = 100; top.t = 1;
  Object.defineProperty(top, "z", { value: 0, enumerable: false });
  return keys(top);
});
show("not enumerable", function () {
  var o = { a: 1 };
  Object.defineProperty(o, "hidden", { value: 1, enumerable: false });
  Object.defineProperty(o, "shown", { value: 1, enumerable: true });
  return keys(o);
});
show("class instance", function () {
  class A { constructor() { this.x = 1; } m() {} }
  class B extends A { constructor() { super(); this.y = 2; } }
  return keys(new B());
});

// a proxy (the order of its traps differs from Node 24, see
// engine/test/regvm/js/o-forin-shape.js)
show("proxy", function () {
  var log = [];
  var p = new Proxy({ a: 1, b: 2, c: 3 }, {
    ownKeys: function (t) { log.push("ownKeys"); return Reflect.ownKeys(t); },
    getOwnPropertyDescriptor: function (t, k) { log.push("gopd " + String(k)); return Reflect.getOwnPropertyDescriptor(t, k); },
    getPrototypeOf: function (t) { log.push("proto"); return Reflect.getPrototypeOf(t); }
  });
  var r = [];
  for (var k in p) r.push(k);
  return r.join(",") + " " + (log.length > 0);
});
show("proxy that hides a key", function () {
  var p = new Proxy({ a: 1, b: 2 }, {
    getOwnPropertyDescriptor: function (t, k) { return k === "a" ? undefined : Reflect.getOwnPropertyDescriptor(t, k); }
  });
  return keys(p);
});
show("proxy whose ownKeys throws", function () {
  return keys(new Proxy({}, { ownKeys: function () { throw new Error("ownKeys"); } }));
});

// leaving a loop
function firstKey(o) { for (var k in o) return k; return "none"; }
function breakAt(o, n) { var r = [], i = 0; for (var k in o) { if (i++ === n) break; r.push(k); } return r.join(","); }
function throwAt(o, n) { var i = 0; for (var k in o) { if (i++ === n) throw new Error("at " + k); } return "done"; }
show("return", function () { return [firstKey({ q: 1, r: 2 }), firstKey({}), firstKey(null)].join(); });
show("break", function () { return breakAt({ a: 1, b: 2, c: 3 }, 2) + "|" + breakAt([1, 2, 3], 0); });
show("throw", function () { return throwAt({ a: 1, b: 2 }, 1); });
show("throw caught", function () {
  var r = [];
  for (var i = 0; i < 3; i++) { try { throwAt({ a: 1, b: 2, c: 3 }, i); } catch (e) { r.push(e.message); } }
  return r.join(",");
});
show("continue", function () { var r = []; for (var k in { a: 1, b: 2, c: 3 }) { if (k === "b") continue; r.push(k); } return r.join(","); });
show("labelled break", function () {
  var r = [];
  outer: for (var a in { x: 1, y: 2 }) { for (var b in { p: 1, q: 2 }) { if (b === "q") continue outer; if (a === "y") break outer; r.push(a + b); } }
  return r.join(",");
});

// loops in loops
show("nested", function () {
  var r = [];
  for (var a in { x: 1, y: 2 }) for (var b in [7, 8]) for (var c in "uv") r.push(a + b + c);
  return r.join(",");
});
show("recursive", function () {
  function walk(o, d) { var r = []; for (var k in o) { r.push(d + k); if (typeof o[k] === "object" && o[k]) r.push(walk(o[k], d + "-")); } return r.join(","); }
  return walk({ a: { b: { c: 1 }, d: [1, 2] }, e: null }, "");
});

// generators stopped inside a loop
show("generators", function () {
  function* gen(o) { for (var k in o) yield k; }
  var g1 = gen({ a: 1, b: 2, c: 3 }), g2 = gen(["p", "q"]), r = [];
  r.push(g1.next().value, g2.next().value);
  r.push(keys({ m: 1, n: 2 }));
  r.push(g1.next().value, g2.next().value, String(g2.next().done));
  r.push(g1.return("stop").value, String(g1.next().done));
  var g3 = gen({ z: 1, w: 2 });
  r.push(g3.next().value);
  try { g3.throw(new Error("in")); } catch (e) { r.push(e.message); }
  return r.join(",");
});
show("many generators", function () {
  function* gen(o) { for (var k in o) yield k; }
  var gs = [], r = 0;
  for (var i = 0; i < 50; i++) { var g = gen({ a: i, b: i }); g.next(); gs.push(g); }
  for (var j = 0; j < gs.length; j++) r += gs[j].next().value === "b" ? 1 : 0;
  return r;
});

// objects with a null prototype get keys between loops: the empty shape the
// iterators share stays empty
show("null prototype objects between loops", function () {
  var r = [];
  for (var i = 0; i < 3; i++) {
    var o = Object.create(null);
    r.push(keys(o) || "-");
    o["k" + i] = i;
    r.push(keys(o));
    var e = Object.create(null);
    r.push(Object.keys(e).length);
  }
  for (var k in Object.create(null)) r.push("never");
  var p = Object.create(null); p.x = 1; delete p.x; r.push(keys(p) || "-");
  return r.join(",");
});
show("iterators made in a loop", function () {
  var n = 0;
  for (var i = 0; i < 1000; i++) for (var k in { a: 1 }) n++;
  return n;
});
out.join("\n");
