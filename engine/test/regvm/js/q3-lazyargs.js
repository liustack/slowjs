// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// The arguments object of a strict function that only reads
// arguments.length and arguments[key] is made only when a read needs it
// (ARGLEN and ARGGET, engine/regvm-ops-args.h): an index of an argument is
// read from the frame, any other key builds the object once into its local,
// and every later read of the call goes to that object. Each function here
// is such a function, called with fewer, as many and more arguments than it
// has parameters.
// inject-hit: ARGGET 1
// inject-hit: ARGLEN 1
(function () {
  "use strict";
  var out = [];
  function show(v) {
    switch (typeof v) {
    case "symbol": return "symbol";
    case "function": return "function";
    case "object": return v === null ? "null" : "object";
    case "number": return Object.is(v, -0) ? "-0" : String(v);
    default: return typeof v + ":" + String(v);
    }
  }
  function rec(label, f) {
    var r;
    try {
      r = f();
    } catch (e) {
      r = "throws " + e.name + " " + e.message;
    }
    out.push(label + ": " + r);
  }
  var sym = Symbol("k");
  var keyObj;

  // every kind of key, with each count of arguments
  function keys(a, b) {
    var r = [];
    r.push(arguments.length);
    r.push(show(arguments[0]), show(arguments[1]), show(arguments[2]));
    r.push(show(arguments[arguments.length]), show(arguments[arguments.length + 1]));
    r.push(show(arguments[-1]), show(arguments[1.0]), show(arguments[-0]), show(arguments[0.5]));
    r.push(show(arguments["0"]), show(arguments["1"]), show(arguments["length"]));
    r.push(show(arguments[sym]), show(arguments[keyObj]), show(arguments[Symbol.iterator]));
    r.push(show(arguments[NaN]), show(arguments[2147483648]), show(arguments[4294967295]));
    r.push(arguments.length);
    return r.join(" ");
  }
  keyObj = { toString: function () { return "1"; } };
  rec("keys 0", function () { return keys(); });
  rec("keys 1", function () { return keys("x"); });
  rec("keys 2", function () { return keys("x", 2); });
  rec("keys 3", function () { return keys("x", 2, null); });
  rec("keys 5", function () { return keys(1, 2, 3, 4, 5); });

  // a key whose toString reads arguments.length of the same call
  function keyReadsOwn() {
    var n = arguments.length, seen = -1;
    var k = { toString: function () { seen = n; return "0"; } };
    var v = arguments[k];
    return show(v) + " seen " + seen + " len " + arguments.length;
  }
  rec("key toString", function () { return keyReadsOwn("p", "q"); });

  // a getter on Object.prototype for a missing index keeps this and writes
  // to it: the later reads of the call see the write
  var kept = [];
  function missing(a) {
    var r = show(arguments[3]);
    r += " " + show(arguments[0]) + " " + show(arguments[1]) + " " + arguments.length;
    return r + " " + show(arguments[5]);
  }
  rec("getter for a missing index", function () {
    Object.defineProperty(Object.prototype, 3, {
      configurable: true,
      get: function () {
        kept.push(this);
        this[0] = "written";
        this[5] = "five";
        Object.defineProperty(this, "length", { value: 42 });
        return "got " + typeof this + " " + Object.prototype.toString.call(this);
      }
    });
    try {
      return missing("a", "b");
    } finally {
      delete Object.prototype[3];
    }
  });
  rec("kept", function () {
    var a = kept[0];
    return kept.length + " " + a.length + " " + a[0] + " " + a[1] + " " + a[5] + " " +
      Object.prototype.toString.call(a);
  });

  // length redefined through the kept reference, then read
  var lenRef;
  function lenRedefined(a) {
    var r = [arguments.length];
    r.push(show(arguments[9]));
    r.push(arguments.length);
    r.push(show(arguments[0]));
    return r.join(" ");
  }
  rec("length redefined", function () {
    Object.defineProperty(Object.prototype, 9, {
      configurable: true,
      get: function () {
        lenRef = this;
        Object.defineProperty(this, "length", { get: function () { return "getter length"; }, configurable: true });
        Object.defineProperty(this, 0, { get: function () { return "getter 0"; }, configurable: true });
        return "nine";
      }
    });
    try {
      return lenRedefined("z", "y", "x");
    } finally {
      delete Object.prototype[9];
    }
  });

  // more arguments than parameters, fewer, through apply with a long array
  function sum(a, b) {
    var t = 0;
    for (var i = 0; i < arguments.length; i++)
      t += arguments[i];
    return t + "/" + arguments.length + "/" + show(arguments[arguments.length - 1]);
  }
  rec("sum none", function () { return sum(); });
  rec("sum fewer", function () { return sum(1); });
  rec("sum more", function () { return sum(1, 2, 3, 4); });
  var long = [];
  for (var i = 0; i < 3000; i++)
    long.push(i);
  rec("sum apply", function () { return sum.apply(null, long); });
  rec("sum spread", function () { return sum(...long.slice(0, 300)); });

  // a function read as React's createElement reads its children
  function children(type, props) {
    var n = arguments.length - 2, kids;
    if (n === 1) {
      kids = arguments[2];
    } else if (n > 1) {
      kids = Array(n);
      for (var c = 0; c < n; c++)
        kids[c] = arguments[c + 2];
    }
    return type + " " + JSON.stringify(kids);
  }
  rec("children 0", function () { return children("div", null); });
  rec("children 1", function () { return children("div", null, "a"); });
  rec("children 3", function () { return children("div", null, "a", 1, [2]); });

  // the same function on the stack twice: an outer call that built its
  // object and an inner one that did not, then the other way round
  function twice(depth, tag) {
    var r = [];
    if (depth === 0)
      r.push(show(arguments[7]));
    if (depth < 2)
      r.push(twice(depth + 1, tag + depth));
    r.push(depth + ":" + arguments.length + ":" + show(arguments[1]) + ":" + show(arguments[2]));
    if (depth === 2)
      r.push(show(arguments["2"]));
    return r.join(" ");
  }
  rec("twice", function () { return twice(0, "t"); });
  rec("twice extra", function () { return twice(0, "t", "e"); });

  // a throw from a getter on the slow read, in a try, and a read after it
  function throwing(a) {
    var r = [];
    try {
      r.push(show(arguments[4]));
    } catch (e) {
      r.push("caught " + e.message);
    }
    r.push(arguments.length, show(arguments[0]), show(arguments[4]));
    return r.join(" ");
  }
  rec("throwing getter", function () {
    var n = 0;
    Object.defineProperty(Object.prototype, 4, {
      configurable: true,
      get: function () {
        if (n++ === 0)
          throw new Error("first " + typeof this);
        return "second";
      }
    });
    try {
      return throwing("only");
    } finally {
      delete Object.prototype[4];
    }
  });

  // functions that are not lazy: an escape, a write to a parameter, an
  // arrow that reads arguments, the object kept, arguments of a default
  // parameter
  function escapes(a) { return [].slice.call(arguments).join(","); }
  function writes(a) { a = "w"; return arguments[0] + " " + arguments.length; }
  function arrow(a) { var f = () => arguments[0]; return f() + " " + arguments.length; }
  function keeps(a) { var x = arguments; return x[0] + " " + x.length; }
  function defaults(a = "d") { return show(arguments[0]) + " " + arguments.length; }
  function method(a) { return arguments[0](); }
  rec("escapes", function () { return escapes(1, 2, 3); });
  rec("writes", function () { return writes("orig", 2); });
  rec("arrow", function () { return arrow("v", 1); });
  rec("keeps", function () { return keeps("k", 1); });
  rec("defaults", function () { return defaults() + " | " + defaults("given"); });
  rec("method", function () { return method(function () { return "this " + show(this); }); });

  // the object is made by every key that is not an index of an argument
  function compare(a) {
    var x = arguments[1];
    var y = arguments["x"];
    return show(x) + " " + show(y) + " " + arguments.length;
  }
  rec("compare", function () { return compare(1, 2); });
  rec("loop", function () {
    var r = [];
    for (var i = 0; i < 5; i++)
      r.push(compare(i, i * 2, i * 3));
    return r.join(" | ");
  });
  return out.join("\n");
})();
