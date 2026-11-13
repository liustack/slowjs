// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// Array.isArray and Array.prototype.push answered in the loop (rv_fast_call
// in engine/regvm-ops-basic.h), against the same call made where the loop
// cannot answer it (Reflect.apply) on a copy made the same way: each line is
// the answer and what the array holds after it, from the loop, then "=" when
// the other way gave the same, or what it gave. An exception from the loop
// shows its stack, which the golden recorded from the C call holds the same.
// push: an empty array and one at the end of its room, no, one or several
// arguments, a frozen, sealed or non-extensible array, a read-only length, a
// length past the count, holes, a subclass, an array of another realm, a
// setter for an index on Array.prototype or Object.prototype, an array-like
// object, a string, a proxy, an arguments object, a length near 2^32, and a
// recursion that ends at the call depth. isArray: every kind of value, a
// proxy, a revoked proxy and a proxy of a proxy.
(function () {
  var log = [];
  var push = Array.prototype.push, isArray = Array.isArray;
  var realmArr = newRealm("(function () { return [1, 2]; })");
  var trap = [];

  function show(v) {
    if (typeof v === "number" && v === 0 && 1 / v < 0) return "-0";
    if (typeof v === "symbol" || typeof v === "bigint") return typeof v;
    if (typeof v === "object" && v !== null) return Array.isArray(v) ? "[" + v.length + "]" : "obj";
    return String(v);
  }
  function err(e) { return "!" + e.constructor.name + ": " + e.message; }
  function state(a) {
    if (a === null || a === undefined || typeof a !== "object") return show(a);
    var k = Object.getOwnPropertyNames(a).filter(function (x) { return x !== "p"; });
    return k.map(function (x) {
      var d = Object.getOwnPropertyDescriptor(a, x);
      return x + "=" + ("value" in d ? show(d.value) : "acc");
    }).join(" ") + (Object.isExtensible(a) ? "" : " noext");
  }
  function stack(e) { return err(e) + " @" + String(e.stack).trim().split("\n").join("|"); }

  function push0(a) { return a.p(); }
  function push1(a, x) { return a.p(x); }
  function push2(a, x, y) { return a.p(x, y); }
  function push3(a, x) { return a.p(x, x, x); }
  function pushStmt(a, x) { a.push(x); return 1; }
  function pushM(a, x) { return a.push(x); }

  function room(n) { var a = []; for (var i = 0; i < n; i++) a.push(i); return a; }
  var makers = [
    ["empty", function () { return []; }],
    ["one", function () { return [7]; }],
    ["lit3", function () { return [1, 2, 3]; }],
    ["room8", function () { return room(8); }],
    ["room7", function () { return room(7); }],
    ["room12", function () { return room(12); }],
    ["holes", function () { return [1, , 3]; }],
    ["new Array(3)", function () { return new Array(3); }],
    ["long length", function () { var a = [1, 2]; a.length = 5; return a; }],
    ["short length", function () { var a = [1, 2, 3, 4]; a.length = 2; return a; }],
    ["frozen", function () { return Object.freeze([1, 2]); }],
    ["sealed", function () { return Object.seal([1, 2]); }],
    ["noext", function () { return Object.preventExtensions([1, 2]); }],
    ["readonly length", function () { var a = [1, 2]; Object.defineProperty(a, "length", { writable: false }); return a; }],
    ["subclass", function () { class S extends Array {} return S.from([1, 2]); }],
    ["realm", function () { return realmArr(); }],
    ["array-like", function () { return { length: 2, 0: "a", 1: "b" }; }],
    ["array-like no length", function () { return { 0: "a" }; }],
    ["array-like big", function () { return { length: 2 ** 53 - 1 }; }],
    ["proxy", function () { return new Proxy([1, 2], { set(t, k, v) { trap.push("set " + String(k)); t[k] = v; return true; } }); }],
    ["arguments", function () { return (function () { return arguments; })(1, 2); }],
    ["typed", function () { return new Uint8Array(2); }],
    ["near 2^32", function () { var a = [1]; a.length = 2 ** 32 - 1; return a; }],
    ["near 2^31", function () { var a = []; a.length = 2 ** 31 - 1; return a; }],
    ["self", function () { var a = [0]; a.self = a; return a; }],
  ];
  var vals = [1, -0, "s", null, undefined, { o: 1 }, Symbol("x"), 2n, [1, 2]];
  function one(f, mk, args) {
    var a = mk(), b = mk(), r, o;
    try { r = show(f.apply(null, [a].concat(args))) + " " + state(a); } catch (e) { r = stack(e) + " " + state(a); }
    try { o = show(Reflect.apply(push, b, args)) + " " + state(b); } catch (e) { o = err(e) + " " + state(b); }
    return o === r ? r + " =" : r + " /= " + o;
  }
  function pushRows() {
    var out = [];
    Object.defineProperty(Object.prototype, "p", { value: push, writable: true, configurable: true });
    makers.forEach(function (m) {
      var row = [one(push0, m[1], []), one(push2, m[1], [1, "two"]), one(push3, m[1], [{}])];
      vals.forEach(function (v) { row.push(one(push1, m[1], [v])); });
      out.push(m[0] + ": " + row.join(", ") + " " + trap.join(","));
      trap.length = 0;
    });
    delete Object.prototype.p;
    return out.join("\n");
  }
  function pushProto() {
    var r = [], a;
    Object.defineProperty(Array.prototype, 2, { set(v) { trap.push("proto set " + show(v)); }, configurable: true });
    try {
      a = [0, 1]; r.push(pushM(a, "x"), state(a)); a = []; r.push(pushM(a, "y"), state(a));
    } finally { delete Array.prototype[2]; }
    a = [0, 1]; r.push(pushM(a, "x"), state(a));
    Object.defineProperty(Object.prototype, 1, { set(v) { trap.push("objproto set " + show(v)); }, configurable: true });
    try { a = [0]; r.push(pushM(a, "z"), state(a)); } finally { delete Object.prototype[1]; }
    Object.defineProperty(Array.prototype, 0, { value: "ro", writable: false, configurable: true });
    try { a = []; r.push(pushM(a, "w")); } catch (e) { r.push(stack(e)); } finally { delete Array.prototype[0]; }
    String.prototype.p = push;
    try { r.push(show("abc".p(1))); } catch (e) { r.push(stack(e)); } finally { delete String.prototype.p; }
    var g = push;
    try { r.push(show(g(1))); } catch (e) { r.push(stack(e)); }
    var saved = Array.prototype.push;
    Array.prototype.push = function (x) { return "replaced" + show(x); };
    try { r.push(pushM([1], 2)); } finally { Array.prototype.push = saved; }
    Array.prototype.push = push.bind(a = [9]);
    try { r.push(pushM([1], 2), state(a)); } finally { Array.prototype.push = saved; }
    r.push(trap.join(","));
    trap.length = 0;
    return r.join(" ; ");
  }
  function pushMany() {
    var a = [], s = 0;
    for (var i = 0; i < 3000; i++) s += pushM(a, i & 1 ? { i: i } : i);
    var b = [];
    for (i = 0; i < 100; i++) pushStmt(b, a);
    return s + " " + a.length + " " + b.length + " " + show(a[2999]) + " " + (b[99] === a);
  }

  function isA(x) { return Array.isArray(x); }
  function isF(x) { return isArray(x); }
  function isNone() { return Array.isArray(); }
  function isExtra(x) { return Array.isArray(x, 1); }
  var revocable = Proxy.revocable([], {});
  revocable.revoke();
  var isVals = [[], [1], new Array(5), {}, "abc", 1, null, undefined, true, Symbol("q"), 1n, new Proxy([], {}),
    new Proxy({}, {}), new Proxy(new Proxy([], {}), {}), (function () { return arguments; })(), new Uint8Array(1),
    realmArr(), Object.create(Array.prototype), revocable.proxy, function () {}, Array.prototype];
  function isRow(f) {
    return isVals.map(function (v) {
      var r;
      try { r = show(f(v)); } catch (e) { r = stack(e); }
      var o = (function () { try { return show(Reflect.apply(isArray, Array, [v])); } catch (e) { return err(e); } })();
      return o === r ? r + " =" : r + " /= " + o;
    }).join(", ") + " " + isNone() + " " + isExtra([]);
  }

  for (var rep = 0; rep < 2; rep++) {
    log.push(pushRows(), pushProto(), pushMany(), isRow(isA), isRow(isF));
  }
  var depth = 0, held = [];
  try { (function rec() { depth++; held.push({ d: depth }); Array.isArray(held); rec(); })(); } catch (e) { log.push(err(e) + " at depth " + depth + " " + held.length); }
  held.length = 0;
  rvExpect(push0, push1, push2, push3, pushStmt, pushM, pushMany, isA, isF, isNone, isExtra);
  return log.join("\n");
})()
