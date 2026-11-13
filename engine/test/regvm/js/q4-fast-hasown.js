// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// Object.hasOwn and Object.prototype.hasOwnProperty answered in the loop
// (rv_fast_call in engine/regvm-ops-basic.h), against the same call made
// where the loop cannot answer it (Reflect.apply): each entry is the answer
// from the loop, then "=" when the other way gave the same, or what it
// gave. An exception from the loop shows its stack, which the golden
// recorded from the C call holds the same. Objects of every kind (plain,
// with many properties, with deleted ones, frozen, with accessors, arrays,
// functions, String objects, arguments, typed arrays, proxies whose traps
// are logged, the global object, objects of another realm, primitives,
// null and undefined), keys of every kind (atoms, numeric strings, ints,
// negative and large numbers, symbols, objects whose conversion is logged,
// in the order each built-in converts, strings made at run time whose atom
// exists or not), a replaced built-in, and a
// recursion that ends at the call depth.
var fastHasOwnGlobal = 1;
(function () {
  var log = [];
  var hasOwn = Object.hasOwn, hop = Object.prototype.hasOwnProperty;
  var realmObj = newRealm("(function () { return { a: 1, 2: 2 }; })");
  var realmHasOwn = newRealm("(function () { return Object.hasOwn; })")();
  var convLog = [];

  function show(v) { return String(v); }
  function err(e) { return "!" + e.constructor.name + ": " + e.message; }
  function stack(e) { return err(e) + " @" + String(e.stack).trim().split("\n").join("|"); }
  function other(f, t, a) {
    try { return show(Reflect.apply(f, t, a)); } catch (e) { return err(e); }
  }

  function ho(o, k) { return Object.hasOwn(o, k); }
  function hp(o, k) { return o.hasOwnProperty(k); }
  function hoNone(o) { return Object.hasOwn(o); }
  function hpNone(o) { return o.hasOwnProperty(); }
  function hoStmt(o, k) { Object.hasOwn(o, k); o.hasOwnProperty(k); return 1; }

  var sym = Symbol("s"), proto = { inherited: 1 };
  var many = {};
  for (var i = 0; i < 40; i++) many["k" + i] = i;
  delete many.k3; delete many.k20;
  var acc = Object.create(proto, { g: { get() { convLog.push("get"); return 1; }, enumerable: false } });
  acc.own = 1; acc[sym] = 2; acc[7] = 3; acc["-1"] = 4; acc["1.5"] = 5;
  var objs = [
    ["plain", { a: 1, b: undefined, 0: 0, 4294967294: 1 }],
    ["many", many],
    ["acc", acc],
    ["frozen", Object.freeze({ a: 1, 1: 2 })],
    ["null proto", Object.assign(Object.create(null), { a: 1, hasOwnProperty: hop })],
    ["array", [1, , 3]],
    ["function", function named(x) {}],
    ["string obj", new String("ab")],
    ["arguments", (function () { return arguments; })(1, 2)],
    ["typed", new Uint8Array(3)],
    ["proxy", new Proxy({ a: 1 }, { getOwnPropertyDescriptor(t, k) { convLog.push("p" + String(k)); return Reflect.getOwnPropertyDescriptor(t, k); } })],
    ["global", globalThis],
    ["realm", realmObj()],
    ["error", new Error("x")],
    ["map", new Map()],
  ];
  var keys = ["a", "b", "c", "k0", "k3", "k20", "k39", "own", "g", "inherited", "length", "name", "prototype", "message",
    "fastHasOwnGlobal", "hasOwnProperty", "", "0", "1", "2", "7", "-1", "1.5", "4294967294", "4294967295",
    0, 1, 2, 7, -1, 1.5, -0, 4294967294, 2147483647, 2147483648, sym, Symbol("s"),
    null, undefined, true, 1n, { toString() { convLog.push("k"); return "a"; } },
    // strings made at run time: their atom exists, or not (the call makes it)
    "k" + (1 + 2), "o" + "wn", "len" + "gth", "1" + "0", "0" + "7", "4294967" + "294", "4294967" + "295", "-" + "1",
    "zz" + "q" + "unseen", "has" + "OwnProperty", "fastHas" + "OwnGlobal"];
  function row(name, o) {
    var r = [];
    keys.forEach(function (k) {
      var s, t;
      try { s = show(ho(o, k)); } catch (e) { s = stack(e); }
      t = other(hasOwn, Object, [o, k]);
      r.push(s === t ? s + "=" : s + "/" + t);
      if (typeof o.hasOwnProperty === "function") {
        try { s = show(hp(o, k)); } catch (e) { s = stack(e); }
        t = other(hop, o, [k]);
        r.push(s === t ? s + "=" : s + "/" + t);
        hoStmt(o, k);
      }
    });
    return name + ": " + r.join(" ") + " | " + hoNone(o) + hpNone(o) + " " + convLog.join("");
  }
  function odd() {
    var r = [];
    convLog.length = 0;
    // the order of the conversions: hasOwn the object then the key,
    // hasOwnProperty the key then the object
    [1, "ab", true, sym, 1n, null, undefined].forEach(function (p) {
      ["0", "length", "x", { toString() { convLog.push("k"); return "0"; } }].forEach(function (k) {
        try { r.push(show(ho(p, k))); } catch (e) { r.push(stack(e)); }
        try { r.push(show(hop.call(p, k))); } catch (e) { r.push(stack(e)); }
        Object.prototype.hp2 = hop;
        try { if (p != null) r.push(show(p.hp2(k))); } catch (e) { r.push(stack(e)); } finally { delete Object.prototype.hp2; }
      });
    });
    try { r.push(show(ho(null, { toString() { convLog.push("never"); return "a"; } }))); } catch (e) { r.push(stack(e)); }
    try { r.push(show(hop.call(null, { toString() { convLog.push("first"); return "a"; } }))); } catch (e) { r.push(stack(e)); }
    try { r.push(show(ho({}, { toString() { throw new Error("key"); } }))); } catch (e) { r.push(stack(e)); }
    try { r.push(show(ho({}, { [Symbol.toPrimitive]() { return sym; } }))); } catch (e) { r.push(stack(e)); }
    var saved = Object.hasOwn;
    Object.hasOwn = function (o, k) { return "replaced" + String(k); };
    try { r.push(ho({ a: 1 }, "a")); } finally { Object.hasOwn = saved; }
    Object.hasOwn = realmHasOwn;
    try { r.push(ho({ a: 1 }, "a"), ho({ a: 1 }, "b")); } finally { Object.hasOwn = saved; }
    var hpSaved = Object.prototype.hasOwnProperty;
    Object.prototype.hasOwnProperty = function (k) { return "mine" + String(k); };
    try { r.push(hp({ a: 1 }, "a")); } finally { Object.prototype.hasOwnProperty = hpSaved; }
    var o = { a: 1 };
    r.push(ho(o, "a"));
    delete o.a;
    r.push(ho(o, "a"));
    o.a = 2;
    Object.defineProperty(o, "b", { value: 1 });
    r.push(ho(o, "a"), ho(o, "b"), hp(o, "b"));
    return r.join(" ") + " " + convLog.join("");
  }

  for (var rep = 0; rep < 2; rep++) {
    objs.forEach(function (x) { convLog.length = 0; log.push(row(x[0], x[1])); });
    log.push(odd());
  }
  var depth = 0, held = { a: 1 };
  try { (function rec() { depth++; Object.hasOwn(held, "a"); held.hasOwnProperty("b"); rec(); })(); } catch (e) { log.push(err(e) + " at depth " + depth); }
  rvExpect(ho, hp, hoNone, hpNone, hoStmt);
  return log.join("\n");
})()
