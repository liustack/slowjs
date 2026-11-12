// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// delete, to_object and set_name in the register loop
// (engine/regvm-ops-window.h): delete of own, missing, inherited and
// non-configurable properties (a TypeError in strict mode), of elements and
// of Proxy traps, destructuring of values that are not objects, and the
// names of anonymous functions. Each test function runs twice from a driver
// the pass refuses (it catches): the first call converts it, the second runs
// it in the register loop.
(function () {
  "use strict";
  // del and delUnder use no string constant and no "in" (ops of other
  // packages), so that they convert with the window ops alone
  function del(o, k) {
    return delete o[k];
  }
  function delField(o) {
    return [delete o.p, o.p];
  }
  // a delete with temps below it on the stack
  function delUnder(o, k) {
    return [o.x, delete o[k], o.x];
  }
  // to_object: destructuring
  function pick(v) {
    var { length, x } = v;
    return [length, x];
  }
  function pickArr(v) {
    var [a, b] = v;
    return [a, b];
  }
  // set_name: the name of an anonymous function, here closures over the
  // captured local v
  function names(v) {
    var f = function () { return v; };
    var g = () => v;
    var h = function () { return f() + g(); };
    return f.name + "," + g.name + "," + h.name + "," + h() + "," + (function () {}).name + "|";
  }
  // and of an anonymous class (define_class, which no package takes yet)
  function className() {
    var C = class { };
    return C.name;
  }
  return function () {
    var log = [];
    function L(s) { log.push(s); }
    function run(name, f) {
      for (var k = 0; k < 2; k++) {
        try {
          L(name + " " + k + ": " + JSON.stringify(f()));
        } catch (e) {
          L(name + " " + k + " threw " + e.name + ": " + e.message);
        }
      }
    }
    var frozen = Object.freeze({ p: 1 });
    run("delete own", function () { var o = { a: 1, b: 2 }; return [del(o, "a"), "a" in o, JSON.stringify(o)]; });
    run("delete missing", function () { return del({ a: 1 }, "z"); });
    run("delete inherited", function () { return del(Object.create({ a: 1 }), "a"); });
    run("delete element", function () { var a = [1, 2, 3]; return del(a, 1) + JSON.stringify(a) + a.length; });
    run("delete number key", function () { return del({ 1: "x" }, 1); });
    run("delete symbol", function () { var s = Symbol(); var o = {}; o[s] = 1; return del(o, s); });
    run("delete field", function () { return delField({ p: 5 }); });
    run("delete frozen", function () { return delField(frozen); });
    run("delete length", function () { return del([1], "length"); });
    run("delete non-configurable", function () {
      var o = {};
      Object.defineProperty(o, "q", { value: 1, configurable: false });
      return del(o, "q");
    });
    run("delete key object", function () {
      return del({ k: 1 }, { toString: function () { L("toString"); return "k"; } });
    });
    run("delete key throws", function () {
      return del({ k: 1 }, { toString: function () { throw new Error("key"); } });
    });
    run("delete of null", function () { return del(null, "a"); });
    run("delete of string", function () { return del("abc", "0") + del("abc", "x"); });
    run("delete proxy", function () {
      var p = new Proxy({ a: 1 }, { deleteProperty: function (t, k) { L("trap " + k); return delete t[k]; } });
      return del(p, "a");
    });
    run("delete proxy false", function () {
      var p = new Proxy({ a: 1 }, { deleteProperty: function () { return false; } });
      return del(p, "a");
    });
    run("delete proxy throws", function () {
      var p = new Proxy({}, { deleteProperty: function () { throw new Error("trap"); } });
      return delUnder(p, "a");
    });
    run("delete under", function () { return delUnder({ x: 1 }, "x"); });
    run("pick string", function () { return pick("abc"); });
    run("pick number", function () { return pick(5); });
    run("pick object", function () { return pick({ length: 2, x: 3 }); });
    run("pick getter", function () { return pick({ get x() { L("get x"); return 9; } }); });
    run("pick null", function () { return pick(null); });
    run("pick undefined", function () { return pick(undefined); });
    run("pick array", function () { return pickArr([1, 2, 3]) + pickArr("xy"); });
    run("pick array null", function () { return pickArr(null); });
    run("names", function () { return names(1); });
    run("class name", function () { return className(); });
    rvExpect(del, delField, delUnder, pick, pickArr, names);
    // when the class ops are taken: rvExpect(className);
    return log.join("\n");
  };
})()
