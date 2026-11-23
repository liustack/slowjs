// Copyright (c) 2026 The liustack/tifajs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

// A closure is one block: the object, its properties and its closure
// variables (js_new_function_object, engine/quickjs.c). Closures with no,
// one and many captured variables (past the size of a small block), by
// value and by reference, whose properties outgrow the block (added,
// deleted, redefined), class constructors and methods, generators and async
// functions, closures in cycles that a collection frees, and the names that
// set_name gives a function with none of its own, in place.
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
      return String(k) + "=none";
    return String(k) + "=" + ("value" in d ? (typeof d.value === "object" ? "object" : String(d.value)) : "accessor") +
      (d.writable ? "w" : "") + (d.enumerable ? "e" : "") + (d.configurable ? "c" : "");
  }
  function shape(f) {
    return Reflect.ownKeys(f).map(function (k) { return desc(f, k); }).join(" ");
  }

  rec("no capture", function () {
    var fs = [], t = 0;
    for (var i = 0; i < 20; i++)
      fs.push(() => 3);
    fs.forEach(function (f) { t += f(); });
    return t + " " + shape(fs[7]);
  });
  rec("by value and by reference", function () {
    var fs = [];
    for (let i = 0; i < 10; i++) {
      let w = i;
      const k = i * 2;
      fs.push({ get: () => w + k, set: (v) => { w = v; } });
    }
    fs[5].set(100);
    return fs[5].get() + " " + fs[6].get() + " " + fs[9].get();
  });
  rec("many captured", function () {
    function make(s) {
      var a0 = s, a1 = s + 1, a2 = s + 2, a3 = s + 3, a4 = s + 4, a5 = s + 5, a6 = s + 6, a7 = s + 7,
        a8 = s + 8, a9 = s + 9, b0 = s + 10, b1 = s + 11, b2 = s + 12, b3 = s + 13, b4 = s + 14,
        b5 = s + 15, b6 = s + 16, b7 = s + 17, b8 = s + 18, b9 = s + 19, c0 = s + 20, c1 = s + 21,
        c2 = s + 22, c3 = s + 23, c4 = s + 24, c5 = s + 25, c6 = s + 26, c7 = s + 27, c8 = s + 28,
        c9 = s + 29, d0 = s + 30, d1 = s + 31, d2 = s + 32, d3 = s + 33, d4 = s + 34;
      var f = function () {
        return a0 + a1 + a2 + a3 + a4 + a5 + a6 + a7 + a8 + a9 + b0 + b1 + b2 + b3 + b4 + b5 + b6 +
          b7 + b8 + b9 + c0 + c1 + c2 + c3 + c4 + c5 + c6 + c7 + c8 + c9 + d0 + d1 + d2 + d3 + d4;
      };
      var g = () => { a0++; return a0; };
      return [f, g];
    }
    var r = [], i, p;
    for (i = 0; i < 5; i++) {
      p = make(i);
      r.push(p[0]() + p[1]() + p[0]());
    }
    p[0].extra = 1;
    p[0].more = 2;
    return r.slice(0, 3).join() + " " + r[4] + " " + shape(p[0]);
  });
  rec("properties outgrow the block", function () {
    var fs = [], i, s = [];
    for (i = 0; i < 10; i++) {
      let x = i;
      const f = () => x;
      f.a = i;
      f.b = i + 1;
      f.c = i + 2;
      if (i % 2)
        delete f.b;
      if (i % 3 === 0)
        Object.defineProperty(f, "name", { value: "n" + i });
      if (i % 5 === 0)
        delete f.length;
      fs.push(f);
    }
    for (i = 0; i < 10; i += 3)
      s.push(fs[i]() + ":" + fs[i].name + ":" + shape(fs[i]));
    return s.join(" | ");
  });
  rec("prototype read and replaced", function () {
    var fs = [], i;
    for (i = 0; i < 6; i++) {
      let x = i;
      fs.push(function (a) { this.v = a + x; });
    }
    fs[3].prototype.m = function () { return this.v; };
    fs[4].prototype = { m: function () { return -1; } };
    return new fs[3](1).m() + " " + new fs[4](1).m() + " " + shape(fs[5]) + " " + shape(fs[3]);
  });
  rec("classes", function () {
    var r = [], i;
    for (i = 0; i < 5; i++) {
      let x = i;
      class A {
        constructor(y) { this.s = x + y; }
        get g() { return this.s * x; }
        m() { return () => this.s + x; }
        static k() { return x; }
      }
      class B extends A {
        constructor() { super(1); this.t = x; }
      }
      r.push(new B().m()() + A.k() + new A(2).g);
    }
    return r.join();
  });
  rec("generators and async", function () {
    var r = [], i;
    for (i = 0; i < 3; i++) {
      let x = i;
      const g = function* () { yield x; yield x + 1; };
      const it = g();
      r.push(it.next().value + it.next().value);
      const af = async () => x;
      r.push(typeof af().then);
    }
    return r.join() + " " + shape(function* () {});
  });
  rec("cycles", function () {
    var t = 0, i, k;
    for (k = 0; k < 2; k++) {
      for (i = 0; i < 50; i++) {
        const o = { x: i };
        o.a = () => o.x;
        o.b = function () { return o.a() + 1; };
        o.self = o;
        t += o.b();
      }
      gc();
    }
    return t;
  });
  rec("names from set_name", function () {
    const anon = function () {};
    const arrow = () => 1;
    let later;
    later = () => 2;
    const o = { a: () => 1, b: function () {}, c: function named() {}, ["d" + 1]: () => 1, e: async () => 1, f: function* () {} };
    const p = { m() {}, get g() { return 1; } };
    var r = [anon.name, arrow.name, later.name, o.a.name, o.b.name, o.c.name, o.d1.name, o.e.name, o.f.name,
      p.m.name, Object.getOwnPropertyDescriptor(p, "g").get.name];
    return r.join(",") + " " + shape(o.a) + " " + shape(o.b) + " " + shape(anon);
  });
  rec("names made many times", function () {
    var r = [], i;
    for (i = 0; i < 8; i++) {
      const o = { first: () => i, second: function () { return i; } };
      if (i % 4 === 0)
        r.push(o.first.name + o.second.name + o.first() + shape(o.second));
    }
    return r.join(" ");
  });
  rec("name in an error stack", function () {
    const thrower = () => { throw new Error("x"); };
    try {
      thrower();
    } catch (e) {
      return String(e.stack).indexOf("at thrower") >= 0;
    }
    return "no throw";
  });
  return out.join("\n") + "\n";
})()
