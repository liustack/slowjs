// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// Constants the pass once refused: a string literal of digits, whose atom is
// an index and has no string (push_atom_value), and a property read or
// written on a constant ("abc".length, "a,b".split(","), null.x, a store
// to a string in strict and sloppy code), each throwing too with temps owned
// below it. Each target runs at least twice from the driver: the first call
// converts it, the next ones run in the register loop.
(function () {
  var log = [];
  function L(s) { log.push(s); }
  var o = { k: "k", n: 1 };
  function digits() { return ["0", "12", "2147483647", "0" + "12", "-1", "1.5"]; }
  function digitsKey(x) { var t = {}; t["7"] = x; return [t[7], "7" in t, Object.keys(t)[0] === "7"]; }
  function digitsTwice() { return "12" === "1" + "2" && "12" == 12; }
  function strField() { return [o.k, "abc".length, "abc".foo, o.n]; }
  function strMethod() { return [o.k, "a,b".split(","), "x".concat("y", "z"), o.n]; }
  function numMethod() { return [(1.5).toFixed(2), (255).toString(16), true.toString()]; }
  function nullField() { return [o.k, null.foo, o.n]; }
  function undefLength() { return [o.k, undefined.length, o.n]; }
  function undefMethod() { return [o.k, undefined.f(), o.n]; }
  function strictStore() { "use strict"; var a = [o.k]; "abc".x = 1; return a; }
  function sloppyStore() { var a = [o.k]; "abc".x = 1; "abc"[0] = "z"; return [a, "abc".x]; }
  function nullStore() { var a = [o.k]; null.x = 1; return a; }
  // a getter on String.prototype runs JS with the constant as this
  Object.defineProperty(String.prototype, "kGetter", {
    get: function () { L("getter on " + this); return this.length; },
    configurable: true
  });
  function strGetter() { return [o.k, "abcd".kGetter, o.n]; }
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
    run("digits", digits);
    run("digits key", function () { return digitsKey(3); });
    run("digits twice", digitsTwice);
    run("string field", strField);
    run("string method", strMethod);
    run("number method", numMethod);
    run("null field", nullField);
    run("undefined length", undefLength);
    run("undefined method", undefMethod);
    run("strict store", strictStore);
    run("sloppy store", sloppyStore);
    run("null store", nullStore);
    run("string getter", strGetter);
    delete String.prototype.kGetter;
    rvExpect(digits, digitsKey, digitsTwice, strField, strMethod, numMethod, nullField,
             undefLength, undefMethod, strictStore, sloppyStore, nullStore, strGetter);
    return log.join("\n");
  };
})()
