// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// Private fields, methods and brands in the register loop through OLD
// (engine/regvm-ops-old.h): private_symbol, get_private_field,
// put_private_field, define_private_field, private_in, add_brand and
// check_brand, each on the right object and on a wrong one (a TypeError with
// temps owned below it), with static members, private accessors and fields
// with initializers. The functions that initialize the fields have no name a
// test can reach: the line rv-expect-op below holds them to OLD in a counting
// build.
// rv-expect-op: OLD
(function () {
  "use strict";
  var log = [];
  function L(s) { log.push(s); }
  var o = { k: "k", n: 1 };
  var seq = 0;
  function next() { return ++seq; }

  // a class made in a function each time: private_symbol, define_class
  function make(init) {
    class P {
      #x = init;
      #y;
      z = next();
      static #count = 0;
      static total = 10;
      #m() { return "m" + this.#x; }
      get #acc() { return this.#x * 2; }
      set #acc(v) { this.#x = v; }
      static #sm() { return "sm"; }
      getX() { return this.#x; }
      setX(v) { this.#x = v; return [o.k, this.#x, o.n]; }
      callM() { return this.#m(); }
      acc(v) { if (v !== undefined) this.#acc = v; return this.#acc; }
      incY() { this.#y = (this.#y || 0) + 1; return this.#y; }
      upd() { this.#x += 1; this.#x++; return this.#x; }
      static has(v) { return #x in v; }
      static hasM(v) { return #m in v; }
      static bump() { return [P.#count++, P.#sm(), P.total]; }
      static readOther(v) { return [o.k, v.#x, o.n]; }
      static writeOther(v) { v.#x = 1; return "wrote"; }
      static callOther(v) { return [o.k, v.#m(), o.n]; }
      static accOther(v) { return v.#acc; }
    }
    return P;
  }
  var P = make(5), p = new P(), q = new P();
  var getX = P.prototype.getX, setX = P.prototype.setX, callM = P.prototype.callM;
  var acc = P.prototype.acc, incY = P.prototype.incY, upd = P.prototype.upd;
  var has = P.has, hasM = P.hasM, bump = P.bump, readOther = P.readOther;
  var writeOther = P.writeOther, callOther = P.callOther, accOther = P.accOther;

  // fields with computed keys and initializers that read this and throw
  function makeFields(k) {
    return class {
      [k] = this.constructor.name;
      [k + "2"] = o.n + 1;
      a = 1;
      b = this.a + 1;
    };
  }
  function makeBadField() {
    return class { #f = (() => { throw new RangeError("field init"); })(); };
  }
  // a field that is defined twice on one object (the base returns it)
  class Ret { constructor(v) { return v; } }
  function makeTwice() {
    return class extends Ret { #t = 1; static get(v) { return v.#t; } };
  }

  return function () {
    function run(name, f) {
      for (var i = 0; i < 3; i++) {
        try {
          L(name + " " + i + ": " + JSON.stringify(f()));
        } catch (e) {
          L(name + " " + i + " threw " + e.name + ": " + e.message);
        }
      }
    }
    run("make", function () { var C = make(i => i); var c = new C(); return [C.name, c.z > 0, typeof c.getX()]; });
    run("get", function () { return [getX.call(p), getX.call(q)]; });
    run("set", function () { return setX.call(p, 8); });
    run("method", function () { return callM.call(q); });
    run("accessor", function () { return [acc.call(p), acc.call(p, 4)]; });
    run("field undefined", function () { return incY.call(q); });
    run("update", function () { return upd.call(p); });
    run("in", function () { return [has(p), has({}), hasM(p), hasM(Object.create(p))]; });
    run("in not object", function () { return has(1); });
    run("static", bump);
    run("read other", function () { return readOther({}); });
    run("read other ok", function () { return readOther(q); });
    run("write other", function () { return writeOther({ x: 1 }); });
    run("call other", function () { return callOther({}); });
    run("call other ok", function () { return callOther(p); });
    run("accessor other", function () { return accOther(5); });
    run("get on null", function () { return getX.call(null); });
    run("set on proxy", function () { return setX.call(new Proxy(p, {}), 1); });
    run("other class", function () { var Q = make(1); return [has(new Q()), readOther(new Q())]; });
    run("fields", function () { var C = makeFields("c"); var c = new C(); return [c.c, c.c2, c.a, c.b]; });
    run("field throws", function () { var C = makeBadField(); return new C(); });
    run("field twice", function () {
      var T = makeTwice(), v = {};
      new T(v);
      L("get " + T.get(v));
      return new T(v);
    });
    rvExpect(acc); // a private setter called by acc leaves its value with rot3l
    rvExpect(make, getX, setX, callM, incY, upd, has, hasM, bump, readOther, writeOther,
             callOther, accOther, makeFields, makeBadField, makeTwice);
    return log.join("\n");
  };
})()
