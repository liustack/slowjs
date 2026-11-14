// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// The spread of an object literal (COPYS, engine/regvm-ops-copy.h): into an
// empty target and into one with keys, a key of the source that the target
// has, a site that sees six shapes in turn, sources the cache does not keep
// (a class, a symbol, a getter, an array, a string), and the throws of a
// spread: a getter that throws, a proxy whose ownKeys throws, a target with
// a getter whose key the source has. The result is the same whichever path
// made it, and the three temps of the spread are freed once on a throw.
// inject-hit: COPYS 1
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

  function around(s) { return { a: "A", ...s, z: "Z" }; }
  function empty(s) { return { ...s }; }
  rec("one", function () {
    var t = "";
    for (var i = 0; i < 5; i++)
      t += show(around({ x: i, a: i * 2 })) + "|" + show(empty({ x: i })) + ";";
    return t;
  });

  var shapes = [
    function (i) { return { x: i }; },
    function (i) { return { a: i, x: 1 }; },
    function (i) { return { y: 2, z: i }; },
    function (i) { return {}; },
    function (i) { return { d: [i], e: { f: i }, g: "g" }; },
    function (i) { var o = Object.create(null); o.k = i; return o; },
  ];
  rec("poly", function () {
    var t = "";
    for (var n = 0; n < 30; n++)
      t += show(around(shapes[n % shapes.length](n))) + ";";
    return t;
  });

  class P { constructor() { this.x = 1; } }
  rec("kinds", function () {
    var sym = Symbol("s"), w = { x: 0 };
    w[sym] = "sym";
    return [around(new P()), around(w), around({ get x() { return "got"; } }),
            around([7, 8]), around("st"), around(5), around(null)].map(show).join(";");
  });

  rec("getter throws", function () {
    return show(around({ x: 1, get y() { throw new Error("from y"); } }));
  });
  rec("ownKeys throws", function () {
    return show(around(new Proxy({ x: 1 }, {
      ownKeys: function () { throw new Error("from ownKeys"); },
    })));
  });
  rec("target getter", function () {
    var t = "";
    for (var i = 0; i < 3; i++) {
      var o = { get a() { return "g"; }, ...{ a: i } };
      t += show(o) + ";";
    }
    return t;
  });
  rec("frozen", function () {
    var f = Object.freeze({ x: 1, a: 2 });
    var r = around(f);
    r.w = 3;
    return show(around(f)) + " " + Object.isFrozen(r) + " " + show(r);
  });

  return out.join("\n");
})();
