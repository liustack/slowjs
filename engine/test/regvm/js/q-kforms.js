// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// A constant read as an operand by define_field (DEFFK: an object literal or
// a class field whose value is a constant) and by in (INK: a constant key),
// in engine/regvm-ops-object.h. Values of every kind, a key given twice, a
// setter of the same name before it, class fields defined on a frozen this
// (which throws), and in on arrays, a Proxy, a Symbol key, an index key and
// a right side that is not an object (which throws).
(function () {
  "use strict";
  var log = [];
  function show(o) {
    return Object.getOwnPropertyNames(o).map(function (k) {
      var d = Object.getOwnPropertyDescriptor(o, k);
      var v = d.value;
      return k + "=" + (typeof v === "bigint" ? v + "n" : v === 0 && 1 / v < 0 ? "-0" : String(v)) +
        (d.writable ? "w" : "") + (d.enumerable ? "e" : "") + (d.configurable ? "c" : "");
    }).join(";");
  }
  var setterRuns = 0;
  function lit(x) {
    return { a: 1, b: "s", c: null, d: undefined, e: true, f: 2.5, g: 7n, h: -1, x: x, a: "again" };
  }
  function litSetter(x) {
    return { set a(v) { setterRuns++; }, a: 3, b: x };
  }
  function litNested(x) {
    return { o: { p: 1, q: "q" }, list: [1, "two"], k: x === 0 ? "zero" : "other" };
  }
  class Fields { x = 1; y = "y"; z = null; w; }
  class Base { constructor(frozen) { if (frozen) return Object.freeze({}); } }
  class Derived extends Base { f = 1; g = "g"; }
  function fields(frozen) {
    try { return show(frozen === 2 ? new Fields() : new Derived(frozen)); }
    catch (e) { return e.constructor.name + ":" + e.message; }
  }

  var proxyLog = [];
  var proxy = new Proxy({ a: 1 }, { has(t, k) { proxyLog.push(String(k)); return k in t; } });
  var objs = [{ a: 1, 0: 0 }, Object.create({ a: 2 }), [1, 2], proxy, function f() {}, new String("ab")];
  function ins(o) {
    return ["a" in o, 0 in o, "length" in o, "1" in o, Symbol.iterator in o, "" in o].map(Number).join("");
  }
  function insThrow(v) {
    try { return "a" in v; } catch (e) { return e.constructor.name; }
  }

  for (var rep = 0; rep < 3; rep++) {
    log.push(show(lit(rep)), show(litSetter(rep)), setterRuns, show(litNested(rep)));
    log.push(fields(false), fields(true), fields(2));
    log.push(objs.map(ins).join(","), proxyLog.join(","));
    proxyLog.length = 0;
    log.push([1, "s", null, undefined, Symbol("q"), true].map(insThrow).join(","));
  }
  rvExpect(lit, litSetter, litNested, fields, ins, insThrow);
  return log.join("\n");
})()
