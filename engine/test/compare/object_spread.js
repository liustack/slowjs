// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// The spread of an object literal, `{a, ...s, b}`: the keys of the result,
// in order, with their values and flags, whatever the source and the
// target before the spread are. Each site runs a few times, so that it
// runs from what an earlier run kept as well. The output is what Node 24
// prints.

function show(o) {
  var ks = Reflect.ownKeys(o), s = [];
  for (var i = 0; i < ks.length; i++) {
    var d = Object.getOwnPropertyDescriptor(o, ks[i]);
    var f = (d.enumerable ? "e" : "") + (d.writable ? "w" : "") +
            (d.configurable ? "c" : "") + ("get" in d ? "g" : "");
    var v = "get" in d ? "" : String(d.value);
    s.push(String(ks[i]) + "=" + v + (f === "ewc" ? "" : "/" + f));
  }
  var p = Object.getPrototypeOf(o);
  return "{" + s.join(",") + "}" + (p === Object.prototype ? "" : p === null ? " null" : " proto");
}

var out = [];

function rec(label, f) {
  var r;
  try {
    r = f();
  } catch (e) {
    r = "throws " + e.name;
  }
  out.push(label + ": " + r);
}

function three(label, make, f) {
  rec(label, function () {
    var t = [];
    for (var i = 0; i < 3; i++)
      t.push(show(f(make(i))));
    return t.join(" ");
  });
}

// the sites: an empty target, keys before, keys before and after, two sources
function empty(s) { return { ...s }; }
function before(s) { return { a: "A", z: "Z", ...s }; }
function around(s) { return { a: "A", ...s, z: "Z" }; }
function twice(s) { return { a: "A", ...s, ...s, m: "M" }; }
function nullproto(s) { return { __proto__: null, a: "A", ...s }; }
function protoset(s) { return { __proto__: { p: 1 }, a: "A", ...s }; }
function getter(s) { return { get a() { return "got"; }, ...s }; }
function computed(s, k) { return { [k]: "K", a: "A", ...s }; }

var sites = { empty: empty, before: before, around: around, twice: twice,
              nullproto: nullproto, protoset: protoset, getter: getter };

var sym = Symbol("s"), sym2 = Symbol("t");
var sources = {
  "plain": function (i) { return { x: i, y: "y" + i }; },
  "overwrite": function (i) { return { z: i, a: "a" + i, q: 1 }; },
  "empty": function (i) { return {}; },
  "null": function (i) { return null; },
  "undefined": function (i) { return undefined; },
  "number": function (i) { return i; },
  "boolean": function (i) { return true; },
  "array": function (i) { return [i, "b", , "d"]; },
  "index keys": function (i) { return { b: 1, 2: "two", 0: "zero", a: i }; },
  "symbol": function (i) { var o = { x: i }; o[sym] = "S"; return o; },
  "symbol first": function (i) { var o = {}; o[sym2] = "T"; o.x = i; return o; },
  "not enumerable": function (i) {
    var o = { x: i, a: "hidden" };
    Object.defineProperty(o, "a", { enumerable: false });
    return o;
  },
  "read only": function (i) {
    return Object.defineProperty({ x: i }, "a", { value: "ro", enumerable: true });
  },
  "getter": function (i) {
    return { x: i, get a() { return "g" + i; }, y: 2 };
  },
  "getter adds": function (i) {
    var o = { get x() { o.late = 1; return "gx"; }, a: i };
    return o;
  },
  "getter deletes": function (i) {
    var o = { get x() { delete o.a; return "gx"; }, a: i, b: 2 };
    return o;
  },
  "deleted": function (i) { var o = { x: i, a: 1, y: 2 }; delete o.a; return o; },
  "frozen": function (i) { return Object.freeze({ x: i, a: "f" }); },
  "sealed": function (i) { return Object.seal({ a: "s", x: i }); },
  "own __proto__": function (i) {
    return JSON.parse('{"x": ' + i + ', "__proto__": {"evil": 1}}');
  },
  "null prototype": function (i) {
    var o = Object.create(null); o.a = "n"; o.x = i; return o;
  },
  "class": function (i) {
    class C { constructor() { this.x = i; this.a = "c"; } m() {} }
    return new C();
  },
  "arguments": function (i) { return (function () { return arguments; })(i, "b"); },
  "function": function (i) { var f = function () {}; f.x = i; return f; },
  "error": function (i) { var e = new Error("m" + i); e.x = i; return e; },
  "map": function (i) { var m = new Map([[1, 2]]); m.x = i; return m; },
  "proxy": function (i) {
    return new Proxy({ x: i, a: "p" }, {
      ownKeys: function (t) { return ["a", "x"]; },
    });
  },
  "stack": function (i) { return { stack: "s" + i, x: i }; },
  "many": function (i) {
    var o = {};
    for (var k = 0; k < 40; k++)
      o["k" + k] = k + i;
    return o;
  },
  "many overwrite": function (i) {
    var o = { a: i };
    for (var k = 0; k < 70; k++)
      o["k" + k] = k;
    o.z = i;
    return o;
  },
};

for (var sn in sites)
  for (var on in sources)
    three(sn + " / " + on, sources[on], sites[sn]);

three("computed", sources.plain, function (s) { return computed(s, "x"); });
three("computed a", sources.overwrite, function (s) { return computed(s, "a"); });

// a target with a key that is not writable or not enumerable before the
// spread, by a literal that defines it
rec("target defined", function () {
  var t = [];
  for (var i = 0; i < 3; i++) {
    var o = { a: 1, ...Object.defineProperty({}, "b", { value: 2, enumerable: true }) };
    t.push(show({ ...o, ...{ a: i, b: i } }));
  }
  return t.join(" ");
});

// one site, many shapes in turn: it keeps some of them and makes the
// others the slow way, each result right
rec("poly", function () {
  var makers = [
    function (i) { return { x: i }; },
    function (i) { return { y: i, x: i }; },
    function (i) { return { a: i }; },
    function (i) { return { z: i, b: 1 }; },
    function (i) { return { c: i, d: 2, e: 3 }; },
    function (i) { return { a: i, z: 9, k: 0 }; },
    function (i) { return { m: [i] }; },
  ];
  var t = [];
  for (var n = 0; n < 50; n++)
    t.push(show(around(makers[n % makers.length](n))));
  return t.slice(0, 7).join(" ") + " ... " + t.slice(43).join(" ");
});

// targets of other shapes at one site: the second source of a spread
rec("targets", function () {
  var t = [];
  for (var n = 0; n < 12; n++) {
    var first = n % 3 == 0 ? { a: n } : n % 3 == 1 ? { z: n, a: 1 } : {};
    t.push(show({ ...first, ...{ a: "A" + n, x: n } }));
  }
  return t.join(" ");
});

// a kept shape whose object then changes: the result follows the object
rec("changed", function () {
  var t = [];
  for (var n = 0; n < 8; n++) {
    var s = { x: n, a: "s" };
    if (n == 4) s.y = 1;
    if (n == 5) delete s.x;
    if (n == 6) Object.defineProperty(s, "a", { enumerable: false });
    if (n == 7) Object.defineProperty(s, "x", { get: function () { return "g"; }, enumerable: true });
    t.push(show(before(s)));
  }
  return t.join(" ");
});

// a target that is not extensible: a spread into it throws
rec("not extensible", function () {
  var o = Object.preventExtensions({ a: 1 });
  return show(Object.assign(o, {})) + " " + show({ ...o });
});

// the values are shared, not copied
rec("same values", function () {
  var inner = { n: 1 }, s = { v: inner, a: [1] };
  var r = around(s), r2 = around(s);
  r.v.n = 2;
  return (r.v === inner) + " " + (r2.a === s.a) + " " + inner.n;
});

// the result is a new object each time, and can grow and lose keys
rec("result", function () {
  var s = { x: 1, y: 2 };
  var r1 = around(s), r2 = around(s);
  r1.w = 3;
  delete r2.x;
  return (r1 !== r2) + " " + show(r1) + " " + show(r2) + " " + show(around(s));
});

// a setter on Object.prototype does not run: the keys are defined
rec("setter on prototype", function () {
  var hit = 0;
  Object.defineProperty(Object.prototype, "x", {
    set: function (v) { hit++; }, configurable: true,
  });
  try {
    var t = [];
    for (var i = 0; i < 3; i++)
      t.push(show(around({ x: i })) + show(empty({ x: i })));
    return t.join(" ") + " " + hit;
  } finally {
    delete Object.prototype.x;
  }
});

// a hot loop over two shapes
rec("hot", function () {
  var a = { x: 1, y: 2 }, b = { y: 3, w: 4, x: 5 }, sum = 0, r;
  for (var i = 0; i < 2000; i++) {
    r = { k: i, ...(i & 1 ? a : b), z: i };
    sum += r.x + r.y + r.k + r.z;
  }
  return sum + " " + show(r);
});

out.join("\n");
