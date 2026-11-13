// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// The rest of an object destructuring (COPYR, engine/regvm-ops-copy.h): a
// site that sees one shape, one that sees six in turn, sources the cache
// does not keep (a class, a symbol key, a getter, a deleted key), computed
// keys left out, and the throws of a rest: a null source, a getter that
// throws, a proxy whose ownKeys throws. The result is the same whichever
// path made it, and the temps of the source and of excludeList are freed
// once on a throw.
// inject-hit: COPYR 1
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
  function show(o) {
    var ks = Reflect.ownKeys(o), s = [];
    for (var i = 0; i < ks.length; i++)
      s.push(String(ks[i]) + "=" + String(o[ks[i]]));
    return s.join(",");
  }

  function one(s) {
    const { a, ...r } = s;
    return a + "|" + show(r);
  }
  rec("one", function () {
    var t = "";
    for (var i = 0; i < 5; i++)
      t += one({ a: i, b: i * 2, c: "x" + i }) + ";";
    return t;
  });

  // six shapes at one site, many times over: the site keeps four, then
  // makes each result on the slow path
  function poly(s) {
    const { k, ...r } = s;
    return k + ":" + show(r);
  }
  var shapes = [
    function (i) { return { k: i, a: 1 }; },
    function (i) { return { a: 1, k: i, b: 2 }; },
    function (i) { return { b: 2, c: 3, k: i }; },
    function (i) { return { k: i }; },
    function (i) { return { d: [i], e: { f: i }, k: i, g: "g" }; },
    function (i) { var o = Object.create(null); o.k = i; o.z = 26; return o; },
  ];
  rec("poly", function () {
    var t = "";
    for (var n = 0; n < 30; n++)
      t += poly(shapes[n % shapes.length](n)) + ";";
    return t;
  });

  class P { constructor() { this.k = 1; this.p = 2; } }
  rec("kinds", function () {
    var sym = Symbol("s"), o = { k: 0, a: 1, b: 2 };
    delete o.a;
    var g = { k: 0, get h() { return "got"; }, j: 3 };
    var w = { k: 0, x: 1 };
    w[sym] = "sym";
    return [poly(new P()), poly(o), poly(g), poly(w), poly([7, 8]),
            poly("str"), poly(5)].join(";");
  });

  rec("computed", function () {
    var key = "b", t = "";
    for (var i = 0; i < 3; i++) {
      const { [key]: x, a, ...r } = { a: i, b: "B", c: "C" };
      t += x + a + show(r) + ";";
    }
    return t;
  });

  rec("null", function () {
    const { a, ...r } = null;
    return show(r);
  });
  rec("undefined", function () {
    return one(undefined);
  });
  rec("getter throws", function () {
    return one({ a: 1, get b() { throw new Error("from b"); } });
  });
  rec("ownKeys throws", function () {
    return one(new Proxy({ a: 1 }, {
      ownKeys: function () { throw new Error("from ownKeys"); },
    }));
  });
  rec("frozen", function () {
    var f = Object.freeze({ a: 1, b: 2 });
    var t = one(f) + one(f);
    const { a, ...r } = f;
    r.c = 3;
    return t + Object.isFrozen(r) + show(r);
  });

  return out.join("\n");
})();
