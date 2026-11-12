// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// A result written straight to a local or an argument in the register loop
// (rv_mark_dst and rv_store in engine/regvm-pass.h): each op with one result
// stored to a local, to an argument and to a captured local, with operands
// that are the same local (x = x.f, x = x[i], x = x === y, x = -x), with
// operands in temps, with old values that have a reference count (objects,
// strings, BigInts) and with ops that throw: the local keeps its old value,
// read again in the catch. Getters and valueOf read and write a captured
// local while the op runs.
(function () {
  "use strict";
  var log = [];
  function id(x) { return x; }
  function show(v) {
    if (typeof v === "bigint") return v + "n";
    if (typeof v === "symbol") return v.toString();
    if (typeof v === "function") return "fn " + v.name;
    if (typeof v === "object" && v !== null) {
      try { return JSON.stringify(v); } catch (e) { return "[obj]"; }
    }
    if (v === 0 && 1 / v < 0) return "-0";
    return String(v);
  }
  function tryCall(f, a, b) {
    try { return show(f(a, b)); } catch (e) { return e.constructor.name + ": " + e.message; }
  }

  // binary ops: into a local, the local as an operand, temps as operands
  function bin(a, b) {
    var x = {}, r = [];
    x = a * b; r.push(x);
    x = x / b; r.push(x);
    x = a % b; r.push(x);
    x = a & b; r.push(x);
    x = x | b; r.push(x);
    x = a ^ b; r.push(x);
    x = a << b; r.push(x);
    x = x >> b; r.push(x);
    x = a >>> b; r.push(x);
    x = a ** 2; r.push(x);
    x = id(a) * id(b); r.push(x);
    x = id(a) - id(b); r.push(x);
    x = x + "s"; r.push(x);
    x = "t" + x; r.push(x);
    x = a < b; r.push(x);
    x = x == b; r.push(x);
    x = a === b; r.push(x);
    x = id(a) !== id(b); r.push(x);
    x = a >= id(b); r.push(x);
    a = a * b; r.push(a);
    b = b - a; r.push(b);
    return r.map(show).join(",");
  }
  // unary ops, typeof, !, and tests against null, undefined and a type
  function un(a) {
    var x = "old", r = [];
    x = -a; r.push(x);
    x = -x; r.push(x);
    x = +a; r.push(x);
    x = ~a; r.push(x);
    x = !a; r.push(x);
    x = !x; r.push(x);
    x = typeof a; r.push(x);
    x = typeof x; r.push(x);
    x = a === null; r.push(x);
    x = a == null; r.push(x);
    x = a === undefined; r.push(x);
    x = typeof a === "string"; r.push(x);
    x = typeof id(a) === "number"; r.push(x);
    var y = a;
    ++y; r.push(y);
    y = id(a); --y; r.push(y);
    a = -a; r.push(a);
    a = !a; r.push(a);
    return r.map(show).join(",");
  }
  // fields, elements, length, in, instanceof, object and array literals,
  // regexps, globals, closure variables
  var captured = { f: 1 };
  function obj(o, k) {
    var x = o, r = [];
    x = x.a; r.push(x);
    x = o.b.c; r.push(x);
    x = o.list; x = x.length; r.push(x);
    x = o.list; x = x[1]; r.push(x);
    x = o.list[k]; r.push(x);
    x = o.list[0]; r.push(x);
    x = id(o).a; r.push(x);
    x = k in o; r.push(x);
    x = "a" in o; r.push(x);
    x = o instanceof Object; r.push(x);
    x = {}; r.push(x);
    x = { a: k }; r.push(x);
    x = [k, k]; r.push(x);
    x = /ab+/g; r.push(x.source + x.lastIndex);
    x = Math; r.push(typeof x.max);
    x = captured; r.push(x);
    x = captured.f; r.push(x);
    o = o.a; r.push(o);
    k = o; r.push(k);
    return r.map(show).join(",");
  }
  // a computed key (to_propkey), into a local that is also the key
  function key(o, k) {
    var r = [];
    o[k] = 1;
    o[k + "x"] = k;
    for (var i = 0; i < 3; i++) { k = i; o[k] = k; }
    r.push(Object.keys(o).join("|"));
    return r.join(",");
  }
  // a captured local written by the op, read by the closure after
  function capt(o) {
    var x = 0;
    function get() { return x; }
    x = o.a; var g1 = get();
    x = o.a * 2; var g2 = get();
    x = o.a === 3; var g3 = get();
    x = typeof o; var g4 = get();
    // a captured local read into a temp (MOV) and stored: MOVL, also to itself
    var y = "old", z = { z: 1 };
    y = x; z = x; x = x;
    var g5 = get();
    x = { cap: o.a }; y = x; z = y;
    return [g1, g2, g3, g4, g5, y, z, x === z].map(show).join(",");
  }
  // a constant made at a block end and stored after (LOADK, LOADKR)
  function kjoin(c) {
    var x = { old: 1 }, y = "old", r = [];
    x = c ? 1 : 2; r.push(x);
    y = c ? "yes" : "no"; r.push(y);
    x = c && "and"; r.push(x);
    y = c || 7; r.push(y);
    return r.map(show).join(",");
  }
  // closures into a local, and a local that is a function
  function clo(n) {
    var f = null, r = [];
    f = function inner() { return n + 1; };
    r.push(f());
    f = () => n * 2;
    r.push(f());
    f = function inner2() { return f === inner2; };
    r.push(f());
    return r.map(show).join(",");
  }
  // let and const: get_loc_check, get_var_ref_check into a local
  function tdz(o) {
    var x = "old";
    let y = o.v;
    x = y; var r1 = x;
    function g() { return z; }
    let z = o.w;
    x = g(); var r2 = x;
    x = (function () { return y; })(); var r3 = x;
    return [r1, r2, r3].map(show).join(",");
  }
  function tdzThrow(o) {
    var x = "old";
    try { x = later; } catch (e) { return x + " " + e.constructor.name; }
    let later = o;
    return x;
  }
  function tdzThrowRef(o) {
    var x = "old";
    function g() { x = later; }
    try { g(); } catch (e) { return x + " " + e.constructor.name; }
    let later = o;
    return x;
  }
  // ops that throw: the local keeps its old value, a temp below is freed
  function throws(o, b) {
    var x = { keep: 1 }, r = [];
    try { x = o.a.b; } catch (e) { r.push(show(x) + " " + e.constructor.name); }
    try { x = o.a[b]; } catch (e) { r.push(show(x) + " " + e.constructor.name); }
    try { x = b in o.a; } catch (e) { r.push(show(x) + " " + e.constructor.name); }
    try { x = o instanceof o; } catch (e) { r.push(show(x) + " " + e.constructor.name); }
    try { x = b * 2n; } catch (e) { r.push(show(x) + " " + e.constructor.name); }
    try { x = -b; } catch (e) { r.push(show(x) + " " + e.constructor.name); }
    try { x = b < 1; } catch (e) { r.push(show(x) + " " + e.constructor.name); }
    try { x = b + 1; } catch (e) { r.push(show(x) + " " + e.constructor.name); }
    try { x = b - 1; } catch (e) { r.push(show(x) + " " + e.constructor.name); }
    try { x = id("x") + (o.a * b); } catch (e) { r.push(show(x) + " " + e.constructor.name); }
    try { x = undefinedGlobal; } catch (e) { r.push(show(x) + " " + e.constructor.name); }
    try { x = o.getter; } catch (e) { r.push(show(x) + " " + e.constructor.name); }
    return r.join(";");
  }
  // a getter and valueOf that read and write a captured local while the op
  // that writes it runs
  function reentry() {
    var x = "start", r = [];
    var o = {
      get g() { r.push("in " + show(x)); x = "set in getter"; return "got"; },
      valueOf() { r.push("vo " + show(x)); x = { set: "in valueOf" }; return 5; }
    };
    x = o.g; r.push(show(x));
    x = o * 2; r.push(show(x));
    x = { a: 1 }; x = o - x.a; r.push(show(x));
    x = -o; r.push(show(x));
    x = o < 10; r.push(show(x));
    return r.join(",");
  }
  // old values with a reference count, freed when written over, many times
  function churn(n) {
    var s = "", o = null, b = 1n, t = 0;
    for (var i = 0; i < n; i++) {
      o = { i: i, s: s };
      s = o.s + i;
      s = s + "-";
      b = b * 3n;
      t = s.length;
      o = o.i;
    }
    return [s.length, show(o), show(b % 1000n), t].join(",");
  }
  // a let of the enclosing function read before and after its initialization
  function readLate(o) {
    var x = "old";
    try { x = lateLet; } catch (e) { return x + " " + e.constructor.name; }
    x = lateLet;
    return show(x) + o;
  }
  function sloppyGlobal(o) {
    var x = 1;
    x = globalThis.Math.abs(o.v);
    x = x.toString;
    return typeof x;
  }

  log.push(bin(7, 3));
  log.push(bin(-8, 2));
  log.push(bin(2.5, 0.5));
  log.push(bin("12", 5));
  log.push(bin(10n, 3n));
  log.push(tryCall(bin, 1n, 2));
  log.push(un(5));
  log.push(un(-0));
  log.push(un("str"));
  log.push(un(null));
  log.push(un(undefined));
  log.push(tryCall(un, 7n));
  log.push(tryCall(un, Symbol("s")));
  log.push(un({ valueOf() { return 4; } }));
  log.push(obj({ a: { z: 1 }, b: { c: "c" }, list: [10, 20, 30] }, 2));
  log.push(obj({ a: "A", b: { c: [1] }, list: "xyz" }, "a"));
  log.push(tryCall(obj, { a: 1, b: null, list: [] }, 0));
  log.push(key({}, "k"));
  log.push(key([], 5));
  log.push(capt({ a: 3 }));
  log.push(capt({ a: "s" }));
  log.push(kjoin(1), kjoin(0), kjoin(""), kjoin({}));
  for (var rep = 0; rep < 2; rep++) {
    log.push(clo(4 + rep));
    log.push(tdz({ v: [rep], w: "w" }));
    log.push(tdzThrow(rep));
    log.push(tdzThrowRef(rep));
  }
  log.push(throws({ a: null }, 1));
  log.push(throws({ a: {}, getter: 1 }, Symbol("y")));
  log.push(throws({ a: 3, get getter() { throw new Error("g"); } }, 1n));
  for (var rep = 0; rep < 2; rep++) {
    log.push(reentry());
    log.push(churn(50 + rep));
    log.push(sloppyGlobal({ v: -3 - rep }));
    log.push(readLate(rep));
  }
  let lateLet = { v: 2 };
  log.push(readLate(2));
  rvExpect(bin, un, obj, key, capt, kjoin, clo, tdz, tdzThrow, tdzThrowRef, throws, reentry, churn,
           sloppyGlobal, readLate);
  return log.join("\n");
})()
