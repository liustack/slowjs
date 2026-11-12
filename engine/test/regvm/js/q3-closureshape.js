// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// A closure is made at the shape it ends with, length, name and, for a
// function with one, prototype (js_closure_object, engine/quickjs.c), when
// a closure of its kind made before is on that shape still. Each kind
// of closure is made many times here, and each closure must have its own
// values, the keys in their order with their flags, a prototype of its own
// made on the first read, and stay a constructor or not, as the first one.
(function () {
  "use strict";
  var out = [];
  function rec(label, f) {
    var r;
    try {
      r = f();
    } catch (e) {
      r = "throws " + e.name + " " + e.message;
    }
    out.push(label + ": " + r);
  }
  function desc(o, k) {
    var d = Object.getOwnPropertyDescriptor(o, k);
    if (!d)
      return k + "=none";
    return k + "=" + ("value" in d ? (typeof d.value === "object" ? "object" : String(d.value)) : "accessor") +
      (d.writable ? "w" : "") + (d.enumerable ? "e" : "") + (d.configurable ? "c" : "");
  }
  function shape(f) {
    return Reflect.ownKeys(f).map(function (k) { return desc(f, k); }).join(" ");
  }

  // the same function expression made again and again
  function make(i) {
    return function named(a, b) { return i + a + b; };
  }
  function makeArrow(i) { return (a) => i + a; }
  function makeAsync(i) { return async function (a, b, c) { return i; }; }
  function makeGen(i) { return function* gen(a) { yield i; }; }
  var obj = { method(a) { return a; } };
  rec("first", function () { return shape(make(0)); });
  rec("many", function () {
    var fs = [], r = [];
    for (var i = 0; i < 20; i++)
      fs.push(make(i));
    for (i = 0; i < 20; i += 7)
      r.push(fs[i](1, 2) + ":" + shape(fs[i]));
    r.push(fs[3].prototype === fs[4].prototype);
    r.push(fs[3].prototype.constructor === fs[3]);
    r.push(Object.getPrototypeOf(fs[5].prototype) === Object.prototype);
    return r.join(" | ");
  });
  rec("arrow", function () {
    var r = [];
    for (var i = 0; i < 5; i++) {
      var f = makeArrow(i);
      r.push(f(10) + ":" + shape(f));
    }
    try { new (makeArrow(1))(); r.push("constructs"); } catch (e) { r.push(e.name); }
    return r.join(" | ");
  });
  rec("async", function () {
    var r = [];
    for (var i = 0; i < 4; i++)
      r.push(shape(makeAsync(i)));
    return r.join(" | ");
  });
  rec("generator", function () {
    var r = [];
    for (var i = 0; i < 4; i++) {
      var g = makeGen(i);
      r.push(g().next().value + ":" + shape(g) + ":" + (g.prototype === makeGen(i).prototype));
    }
    return r.join(" | ");
  });
  rec("method", function () { return shape(obj.method); });
  rec("new", function () {
    var r = [];
    for (var i = 0; i < 4; i++) {
      var C = make(i);
      var o = new C();
      r.push((o instanceof C) + " " + (Object.getPrototypeOf(o) === C.prototype));
    }
    return r.join(" | ");
  });

  // a closure changed after it was made, and the next one as the first
  rec("changed", function () {
    var f = make(1);
    f.prototype = 5;
    delete f.name;
    Object.defineProperty(f, "length", { value: 9 });
    f.extra = 1;
    var g = make(2);
    return shape(f) + " / " + shape(g) + " / " + g.prototype.constructor.name;
  });
  rec("frozen", function () {
    var f = make(1);
    Object.freeze(f);
    var g = make(2);
    g.prototype.x = 1;
    return Object.isFrozen(f) + " " + Object.isFrozen(g) + " " + shape(g) + " " + ("x" in make(3).prototype);
  });

  // Function.prototype with a key of its own does not change the keys of a
  // closure
  rec("proto key", function () {
    Function.prototype.extra = "fp";
    var f = make(1);
    var r = shape(f) + " " + f.extra;
    delete Function.prototype.extra;
    return r + " " + make(2).extra;
  });

  // names that are numbers and computed names
  rec("names", function () {
    var o = { 1: function () {}, ["k" + 2]: function () {}, [Symbol.iterator]: function () {} };
    return [o[1].name, o.k2.name, o[Symbol.iterator].name].join(",") + " " + shape(o[1]);
  });

  // another realm makes its own closures at its own shapes
  if (typeof newRealm === "function") {
    rec("realm", function () {
      var mk = newRealm("(function (i) { return function f(a) { return i; }; })");
      var f = mk(1), g = mk(2);
      return shape(f) + " " + (Object.getPrototypeOf(f) === Function.prototype) + " " +
        (Object.getPrototypeOf(f.prototype) === Object.prototype) + " " + (f.prototype !== g.prototype);
    });
  }
  return out.join("\n");
})();
