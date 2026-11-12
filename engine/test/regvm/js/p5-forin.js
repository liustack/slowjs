// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// for-in in the register loop (for_in_start and for_in_next,
// engine/regvm-ops-window.h): over objects, arrays, strings, prototypes,
// null and undefined, with keys deleted and added during the loop, and over
// a Proxy whose traps run JS and throw. Each test function runs twice from a
// driver the pass refuses (it catches): the first call converts it, the
// second runs it in the register loop.
(function () {
  "use strict";
  // the test functions use no string constant and no += (ops of other
  // packages), so that they convert with the window ops alone
  function keys(o) {
    var r = [];
    for (var k in o)
      r.push(k);
    return r;
  }
  // the body changes the object: f(o, k) runs at each key
  function walk(o, f) {
    var r = [];
    for (var k in o) {
      r.push(k);
      f(o, k);
    }
    return r;
  }
  // a loop in a loop, and the key read back
  function pairs(a, b) {
    var r = [];
    for (var x in a) {
      for (var y in b)
        r.push(x + y);
    }
    return r;
  }
  function stop(o, n) {
    var r = [];
    var i = 0;
    for (var k in o) {
      if (i === n)
        break;
      r.push(k);
      i = i + 1;
    }
    return r;
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
    function proto() {
      var p = { pa: 1, pb: 2 };
      var o = Object.create(p);
      o.oa = 3;
      o.pa = 4;
      Object.defineProperty(o, "hidden", { value: 5, enumerable: false });
      return o;
    }
    run("object", function () { return keys({ a: 1, b: 2, 10: 3, 2: 4, c: 5 }); });
    run("empty", function () { return keys({}); });
    run("array", function () { return keys([5, 6, 7]); });
    run("array holes", function () { return keys([1, , 3]); });
    run("array extra", function () { var a = [1, 2]; a.x = 1; return keys(a); });
    run("string", function () { return keys("abc"); });
    run("number", function () { return keys(5); });
    run("null", function () { return [keys(null), keys(undefined)]; });
    run("prototype", function () { return keys(proto()); });
    run("symbol keys", function () { var o = { a: 1 }; o[Symbol("s")] = 2; return keys(o); });
    run("delete later key", function () {
      return walk({ a: 1, b: 2, c: 3 }, function (o, k) { if (k === "a") delete o.b; });
    });
    run("delete own key", function () {
      return walk({ a: 1, b: 2, c: 3 }, function (o, k) { delete o[k]; });
    });
    run("add keys", function () {
      return walk({ a: 1, b: 2 }, function (o, k) { o["n" + k] = 1; });
    });
    run("delete from prototype", function () {
      var o = proto();
      return walk(o, function (x, k) { if (k === "oa") delete Object.getPrototypeOf(x).pb; });
    });
    run("change prototype", function () {
      return walk(proto(), function (x, k) { if (k === "oa") Object.setPrototypeOf(x, { z: 1 }); });
    });
    run("array grows", function () {
      return walk([1, 2], function (a, k) { if (k === "0") a.push(3); });
    });
    run("array shrinks", function () {
      return walk([1, 2, 3], function (a, k) { if (k === "0") a.length = 1; });
    });
    run("pairs", function () { return pairs({ a: 1, b: 2 }, [8, 9]); });
    run("break", function () { return stop({ a: 1, b: 2, c: 3 }, 2); });
    run("proxy", function () {
      var p = new Proxy({ a: 1, b: 2 }, {
        ownKeys: function (t) { L("ownKeys"); return Reflect.ownKeys(t); },
        getOwnPropertyDescriptor: function (t, k) { L("gopd " + String(k)); return Reflect.getOwnPropertyDescriptor(t, k); },
      });
      return keys(p);
    });
    run("proxy throws", function () {
      var p = new Proxy({ a: 1 }, { ownKeys: function () { throw new RangeError("no keys"); } });
      return keys(p);
    });
    run("proxy prototype", function () {
      var p = new Proxy({ pa: 1 }, {
        getPrototypeOf: function (t) { L("getPrototypeOf"); return null; },
      });
      var o = Object.create(p);
      o.own = 1;
      return keys(o);
    });
    run("body throws", function () {
      return walk({ a: 1, b: 2 }, function (o, k) { if (k === "b") throw new Error("in body"); });
    });
    rvExpect(keys, walk, pairs, stop);
    return log.join("\n");
  };
})()
