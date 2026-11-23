// Copyright (c) 2026 The liustack/tifajs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

// The rest of an object destructuring with more static keys than a list of
// them holds (33, past JS_OBJECT_LIT_MAX of engine/quickjs.c): every key
// goes into the object excludeList and the rest is copy_data_properties,
// the window op W_COPY_DATA_PROPERTIES (engine/regvm-ops-window.h), not
// COPYR. Sources: a plain object, one with a getter, a proxy, a string, a
// null source, a getter that throws, a proxy whose ownKeys throws, and a
// computed key among the others. The temps of the source and of
// excludeList are freed once on a throw.
// inject-hit: W_COPY_DATA_PROPERTIES 1
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
  function many(s) {
    const { k0, k1, k2, k3, k4, k5, k6, k7, k8, k9, k10, k11, k12, k13, k14, k15,
            k16, k17, k18, k19, k20, k21, k22, k23, k24, k25, k26, k27, k28, k29,
            k30, k31, k32, ...r } = s;
    return k0 + "/" + k32 + "|" + show(r);
  }
  function manyComputed(s, key) {
    const { k0, k1, k2, k3, k4, k5, k6, k7, k8, k9, k10, k11, k12, k13, k14, k15,
            k16, k17, k18, k19, k20, k21, k22, k23, k24, k25, k26, k27, k28, k29,
            k30, k31, k32, [key]: kk, ...r } = s;
    return k0 + "/" + kk + "|" + show(r);
  }
  function manyAssign(s) {
    var k0, k1, k2, k3, k4, k5, k6, k7, k8, k9, k10, k11, k12, k13, k14, k15,
        k16, k17, k18, k19, k20, k21, k22, k23, k24, k25, k26, k27, k28, k29,
        k30, k31, k32, r = {};
    ({ k0, k1, k2, k3, k4, k5, k6, k7, k8, k9, k10, k11, k12, k13, k14, k15,
       k16, k17, k18, k19, k20, k21, k22, k23, k24, k25, k26, k27, k28, k29,
       k30, k31, k32, ...r.rest } = s);
    return k0 + "|" + show(r.rest);
  }
  function source(i) {
    return { k0: i, k5: "five", k32: [i], x: i * 2, y: "y" + i, k33: "kept" };
  }
  for (var i = 0; i < 4; i++) {
    rec("plain", function () { return many(source(i)); });
    rec("getter", function () {
      var s = source(i);
      Object.defineProperty(s, "g", { get: function () { return "got " + i; }, enumerable: true });
      return many(s);
    });
    rec("proxy", function () {
      var log = [];
      var p = new Proxy(source(i), {
        ownKeys: function (t) { log.push("ownKeys"); return Reflect.ownKeys(t); },
        getOwnPropertyDescriptor: function (t, k) {
          log.push("desc " + String(k));
          return Reflect.getOwnPropertyDescriptor(t, k);
        },
      });
      return many(p) + " " + log.length;
    });
    rec("string", function () { return many("ab" + i); });
    rec("null", function () { return many(null); });
    rec("getter throws", function () {
      var s = source(i);
      Object.defineProperty(s, "bad", { get: function () { throw new RangeError("getter " + i); }, enumerable: true });
      return many(s);
    });
    rec("ownKeys throws", function () {
      return many(new Proxy(source(i), { ownKeys: function () { throw new TypeError("ownKeys " + i); } }));
    });
    rec("computed", function () { return manyComputed(source(i), "x"); });
    rec("computed symbol", function () {
      var s = source(i), sym = Symbol.for("q9");
      s[sym] = "sym";
      return manyComputed(s, sym) + " " + manyComputed(s, "y");
    });
    rec("assign", function () { return manyAssign(source(i)); });
  }
  rvExpect(many, manyComputed, manyAssign);
  return out.join("\n");
})()
