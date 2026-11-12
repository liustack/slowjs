// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// for-of in the register loop (for_of_start, for_of_next, iterator_close and
// the close of an iterator by an exception, engine/regvm-ops-window.h): over
// arrays, strings, Maps, Sets, generators and iterators made by hand, with
// break, with a body that throws, and with next and return that throw. Each
// test function runs twice from a driver the pass refuses (it catches): the
// first call converts it, the second runs it in the register loop.
(function () {
  "use strict";
  // the values of an iterable, and their sum. The test functions use no
  // string constant and no += (ops of other packages), so that they convert
  // with the window ops alone
  function vals(a) {
    var r = [];
    for (var x of a)
      r.push(x);
    return r;
  }
  function sum(a) {
    var s = 0;
    for (var x of a)
      s = x + s;
    return s;
  }
  // a loop variable that is a temp of the body, and a loop in a loop
  function nested(a, b) {
    var r = [];
    for (var x of a) {
      for (var y of b)
        r.push(x + y);
    }
    return r;
  }
  // an early exit: break closes the iterator
  function first(a, n) {
    var r = [];
    var i = 0;
    for (var x of a) {
      if (i === n)
        break;
      r.push(x);
      i = i + 1;
    }
    r.push(i);
    return r;
  }
  // the body calls f, which may throw: the exception closes the iterator
  function each(a, f) {
    var r = [];
    for (var x of a)
      r.push(f(x));
    return r;
  }
  // values left on the stack under the loop when the body throws
  function under(a, f) {
    return [a, vals(a), each(a, f), a];
  }
  // return inside the loop: the iterator is closed, then the value returned
  // (nip_catch and rot3r with iterator_close)
  function find(a, v) {
    for (var x of a) {
      if (x === v)
        return [x];
    }
    return null;
  }
  // return from a loop in a loop: both iterators closed, the inner first
  function find2(a, b, v) {
    for (var x of a) {
      for (var y of b) {
        if (y === v)
          return [x, y];
      }
    }
    return null;
  }
  return function () {
    var log = [];
    function L(s) { log.push(s); }
    function iter(n, opts) {
      // an iterator by hand: next and return log and may throw
      var i = 0;
      var it = {
        next: function () {
          L("next" + i);
          if (opts.nextThrows === i)
            throw new Error("next " + i);
          i++;
          if (i > n)
            return { done: true, value: "end" };
          return { done: false, value: "v" + i };
        },
        [Symbol.iterator]: function () { return this; },
      };
      if (opts.ret)
        it.return = function () {
          L("return" + i);
          if (opts.retThrows)
            throw new Error("return " + i);
          return opts.retValue === undefined ? {} : opts.retValue;
        };
      return it;
    }
    function* gen(n) {
      try {
        for (var i = 0; i < n; i++)
          yield "g" + i;
      } finally {
        L("gen finally");
      }
    }
    function thrower(at) {
      return function (x) {
        if (x === at)
          throw new TypeError("at " + x);
        return x;
      };
    }
    function run(name, f) {
      for (var k = 0; k < 2; k++) {
        try {
          L(name + " " + k + ": " + JSON.stringify(f()));
        } catch (e) {
          L(name + " " + k + " threw " + e.name + ": " + e.message);
        }
      }
    }
    var m = new Map([["a", 1], ["b", 2]]);
    var st = new Set([1, 1, 2, 3.5, "x", null, undefined]);
    run("array", function () { return vals([1, 2.5, "s", null, undefined, {}, [3, 4]]); });
    run("empty", function () { return vals([]) + sum([]); });
    run("holes", function () { return vals([1, , 3]); });
    run("sum", function () { return sum([1, 2, 3, 0.5]); });
    run("string", function () { return vals("ab\u{1F600}c"); });
    run("map", function () { return vals(m); });
    run("map keys", function () { return vals(m.keys()) + vals(m.values()); });
    run("set", function () { return vals(st); });
    run("generator", function () { return vals(gen(3)); });
    run("nested", function () { return nested([1, 2], "xy"); });
    run("break array", function () { return first([1, 2, 3, 4], 2); });
    run("break generator", function () { return first(gen(5), 2); });
    run("break by hand", function () { return first(iter(5, { ret: true }), 1); });
    run("break no return", function () { return first(iter(5, {}), 1); });
    run("break return throws", function () { return first(iter(5, { ret: true, retThrows: true }), 1); });
    run("break return not object", function () { return first(iter(5, { ret: true, retValue: 7 }), 1); });
    run("to the end", function () { return first(iter(2, { ret: true }), 9); });
    run("body throws", function () { return each([1, 2, 3], thrower(2)); });
    run("body throws generator", function () { return each(gen(4), thrower("g1")); });
    run("body throws by hand", function () { return each(iter(4, { ret: true }), thrower("v2")); });
    run("body throws return throws", function () {
      return each(iter(4, { ret: true, retThrows: true }), thrower("v2"));
    });
    run("next throws", function () { return each(iter(4, { ret: true, nextThrows: 1 }), thrower(0)); });
    run("under", function () { return under([1, 2], thrower(2)); });
    run("not iterable", function () { return vals(5); });
    run("null", function () { return vals(null); });
    run("no next", function () { return vals({ [Symbol.iterator]: function () { return {}; } }); });
    run("next not object", function () {
      return vals({ [Symbol.iterator]: function () { return { next: function () { return 1; } }; } });
    });
    run("getter iterator", function () {
      var o = { get [Symbol.iterator]() { L("get iterator"); return function () { return [7, 8][Symbol.iterator](); }; } };
      return vals(o);
    });
    run("return in loop", function () { return [find([1, 2, 3], 2), find([1, 2], 9)]; });
    run("return in loop generator", function () { return find(gen(5), "g1"); });
    run("return in loop by hand", function () { return find(iter(5, { ret: true }), "v2"); });
    run("return in loop return throws", function () {
      return find(iter(5, { ret: true, retThrows: true }), "v2");
    });
    run("return in loop return not object", function () {
      return find(iter(5, { ret: true, retValue: 7 }), "v2");
    });
    run("return in two loops", function () { return find2(iter(3, { ret: true }), gen(4), "g2"); });
    rvExpect(vals, sum, nested, first, each, under, find, find2);
    return log.join("\n");
  };
})()
