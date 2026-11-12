// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// Object and array literals and spreads in the register loop (object_lit,
// array_from, append, copy_data_properties, apply, engine/regvm-ops-window.h):
// values from locals, arguments, constants and temps of every kind, computed
// keys, getters, spreads of arrays, strings, Sets, generators and objects,
// spread calls and constructors, and iterables and getters that throw. Each
// test function runs twice from a driver the pass refuses (it catches): the
// first call converts it, the second runs it in the register loop.
(function () {
  "use strict";
  // the test functions use no string constant, global or += (ops of other
  // packages) where they can, so that most convert with the window ops alone
  // object_lit with values of every kind of operand
  function lit(a, b) {
    var x = a + 1;
    return { a: a, b: b, c: 1, d: b, e: x, f: a + b, g: null, h: undefined, i: 2.5 };
  }
  function lit2(o) {
    return { k: o, n: { m: o.v } };
  }
  function big(a) {
    return { p0: a, p1: a, p2: a, p3: a, p4: a, p5: a, p6: a, p7: a, p8: a, p9: a,
             q0: 0, q1: 1, q2: 2, q3: 3, q4: 4, q5: 5, q6: 6, q7: 7, q8: 8, q9: 9 };
  }
  // array_from
  function arr(a, b) {
    var x = a + b;
    return [a, b, 1, b, x, null, undefined, 2.5, [a], {}];
  }
  function none() {
    return [];
  }
  // a value that throws while the literal is built: the values before it
  // are owned temps below the call
  function partial(a, f) {
    return [a, a + 1, { v: a }, f(a), a];
  }
  function partialObj(a, f) {
    return { a: a, b: a + 1, c: f(a), d: a };
  }
  // append: spreads in an array literal
  function spread(a) {
    return [...a];
  }
  function spread2(a, b) {
    return [0, ...a, 1, ...b, 2];
  }
  // copy_data_properties: spreads in an object literal and the rest of a
  // destructuring
  function ospread(o) {
    return { z: 0, ...o, y: 1 };
  }
  function rest(o) {
    var { a, ...r } = o;
    return [r, a];
  }
  // computed keys and getters in a literal
  function computed(k, v) {
    return { [k]: v, [v]: 1, get g() { return k; }, set s(w) { }, m() { return v; } };
  }
  // apply: spread calls and constructors
  function call(f, a) {
    return f(...a);
  }
  function callMethod(o, a) {
    return o.m(1, ...a);
  }
  function construct(C, a) {
    return new C(...a);
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
    function* gen() { yield 1; yield 2; }
    function thrower(x) { throw new RangeError("value " + x); }
    var bad = { [Symbol.iterator]: function () { throw new Error("no iterator"); } };
    var badNext = { [Symbol.iterator]: function () { return { next: function () { throw new Error("next"); } }; } };
    run("lit", function () { return JSON.stringify(lit(1, 2)) + JSON.stringify(lit("a", "b")); });
    run("lit objects", function () { return JSON.stringify(lit2({ v: [1] })); });
    run("big", function () { return JSON.stringify(big(7)); });
    run("same shape", function () { return Object.keys(lit(1, 2)).join() === Object.keys(lit(3, 4)).join(); });
    run("arr", function () { return JSON.stringify(arr(1, 2)) + JSON.stringify(arr("x", "y")); });
    run("none", function () { return JSON.stringify(none()) + none().length; });
    run("partial", function () { return JSON.stringify(partial(1, thrower)); });
    run("partial obj", function () { return JSON.stringify(partialObj(1, thrower)); });
    run("partial ok", function () { return JSON.stringify(partial(1, function (x) { return x * 10; })); });
    run("spread array", function () { return JSON.stringify(spread([1, 2, , 4])); });
    run("spread string", function () { return JSON.stringify(spread("a\u{1F600}b")); });
    run("spread set", function () { return JSON.stringify(spread(new Set([1, 2, 2]))); });
    run("spread generator", function () { return JSON.stringify(spread(gen())); });
    run("spread map", function () { return JSON.stringify(spread2(new Map([[1, 2]]), "xy")); });
    run("spread bad", function () { return JSON.stringify(spread(bad)); });
    run("spread bad next", function () { return JSON.stringify(spread2([1], badNext)); });
    run("spread number", function () { return JSON.stringify(spread(5)); });
    run("ospread", function () { return JSON.stringify(ospread({ a: 1, z: 9, y: 8 })); });
    run("ospread string", function () { return JSON.stringify(ospread("ab")); });
    run("ospread null", function () { return JSON.stringify(ospread(null)); });
    run("ospread getter", function () {
      return JSON.stringify(ospread({ get a() { L("getter"); return 5; } }));
    });
    run("ospread getter throws", function () {
      return JSON.stringify(ospread({ get a() { throw new Error("getter"); } }));
    });
    run("rest", function () { return rest({ a: 1, b: 2, c: 3 }); });
    run("rest null", function () { return rest(null); });
    run("computed", function () {
      var o = computed("k", 3);
      return JSON.stringify(o) + o.g + o.m() + Object.keys(o).join();
    });
    run("computed symbol", function () { var s = Symbol("q"); return computed(s, 1)[s]; });
    run("call", function () { return call(function (a, b, c) { return a + b + c; }, [1, 2, 3]); });
    run("call string", function () { return call(function () { return arguments.length; }, "abcd"); });
    run("call method", function () {
      return callMethod({ k: 5, m: function (a, b) { return this.k + a + b; } }, [2]);
    });
    run("call throws", function () { return call(thrower, [4]); });
    run("call not function", function () { return call(5, []); });
    run("call bad spread", function () { return call(thrower, bad); });
    run("construct", function () { return JSON.stringify(construct(Array, [3])) + construct(Date, [0]).getTime(); });
    run("construct class", function () {
      class P { constructor(a, b) { this.s = a + b; } }
      return construct(P, [1, 2]).s;
    });
    run("construct not constructor", function () { return construct(function* () {}, []); });
    rvExpect(lit, lit2, big, arr, none, partial, partialObj, spread, spread2, ospread, rest,
             computed, call, callMethod, construct);
    return log.join("\n");
  };
})()
