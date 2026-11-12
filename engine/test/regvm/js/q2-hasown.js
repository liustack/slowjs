// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// Object.prototype.hasOwnProperty.call(o, k) answered without its two calls
// (js_has_own_call in engine/quickjs.c, from RV_CALL_OUT in
// engine/regvm-ops-basic.h), and every case where the calls are made: keys
// that are not atoms yet, numbers that are not index atoms, Symbols, objects
// that convert, exotic objects (arrays, a Proxy, String objects, arguments),
// a primitive this, too few arguments, another function called through
// call, call itself replaced, and calls that throw.
(function () {
  "use strict";
  var log = [];
  var has = Object.prototype.hasOwnProperty;
  var sym = Symbol("s");
  function show(v) { return typeof v === "symbol" ? v.toString() : String(v); }
  var proxyLog = [];
  var proxy = new Proxy({ p: 1 }, {
    getOwnPropertyDescriptor(t, k) { proxyLog.push(show(k)); return Reflect.getOwnPropertyDescriptor(t, k); }
  });
  var objs = [
    { a: 1, b: undefined, 0: "z", 7: 7 },
    Object.create({ a: "proto" }),
    [1, 2, 3],
    proxy,
    new String("str"),
    function f() {},
    Object.freeze({ a: 1 }),
  ];
  objs[1][sym] = 1;
  var keys = ["a", "b", "z", "length", "0", 0, 7, 1, -1, 4294967295, 1.5, "1.5", sym, "",
    "dynamic" + "Key", { toString() { return "a"; } }, null, undefined, true, "toString"];

  function viaHas(o, k) { return has.call(o, k); }
  function viaChain(o, k) { return Object.prototype.hasOwnProperty.call(o, k); }
  function viaMethod(o, k) { return o.hasOwnProperty(k); }
  function stmt(o, k) { has.call(o, k); return 1; }
  function inLoop(o) {
    var n = 0;
    for (var k in o) if (has.call(o, k)) n++;
    return n;
  }
  function few(o) {
    var r = [has.call(o), has.call(o, "a", "extra")];
    try { r.push(has.call()); } catch (e) { r.push(e.constructor.name); }
    return r.join(",");
  }
  function other(o) { return [Object.prototype.propertyIsEnumerable.call(o, "a"), Object.keys.call(null, o).length].join(","); }
  function prim(k) {
    var r = [];
    for (var v of [1, "str", true, sym]) {
      try { r.push(has.call(v, k)); } catch (e) { r.push(e.constructor.name); }
    }
    try { r.push(has.call(null, k)); } catch (e) { r.push(e.constructor.name); }
    try { r.push(has.call(undefined, k)); } catch (e) { r.push(e.constructor.name); }
    return r.join(",");
  }
  function throwsKey(o) {
    try { return has.call(o, { toString() { throw new RangeError("key"); } }); }
    catch (e) { return e.constructor.name; }
  }
  // Function.prototype.call replaced, then put back
  function replaced(o) {
    var saved = Function.prototype.call, r;
    Function.prototype.call = function () { return "replaced"; };
    try { r = has.call(o, "a"); } finally { Function.prototype.call = saved; }
    return r + " " + has.call(o, "a");
  }
  // deep recursion through it: the call depth is checked as the calls do
  function deep(n, o) {
    if (n === 0) return has.call(o, "a");
    return deep(n - 1, o);
  }

  for (var rep = 0; rep < 3; rep++) {
    var row = [];
    for (var i = 0; i < objs.length; i++) {
      for (var j = 0; j < keys.length; j++) {
        var o = objs[i], k = keys[j];
        row.push(+viaHas(o, k), +viaChain(o, k), +viaMethod(o, k), stmt(o, k));
      }
      row.push(inLoop(objs[i]), few(objs[i]), other(objs[i]));
    }
    log.push(row.join(""));
    log.push(prim("a"), prim(0), throwsKey({}), replaced({ a: 1 }), deep(50, { a: 1 }));
    log.push(proxyLog.join(","));
    proxyLog.length = 0;
  }
  try { (function rec() { has.call({}, "a"); rec(); })(); } catch (e) { log.push(e.constructor.name); }
  rvExpect(viaHas, viaChain, viaMethod, stmt, inLoop, few, other, prim, throwsKey, replaced, deep);
  return log.join("\n");
})()
