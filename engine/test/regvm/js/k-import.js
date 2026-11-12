// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// import() in the register loop through OLD (engine/regvm-ops-old.h): the
// promise it makes, a specifier whose conversion runs JS or throws, options
// that are not an object, all with temps owned below.
(function () {
  "use strict";
  var log = [];
  function L(s) { log.push(s); }
  var o = { k: "k", n: 1 };
  function imp(spec) { return [o.k, import(spec), o.n]; }
  function impOpt(spec, opt) { return [o.k, import(spec, opt), o.n]; }
  return function () {
    function run(name, f) {
      for (var i = 0; i < 3; i++) {
        try {
          var r = f();
          L(name + " " + i + ": " + r[0] + " " + Object.prototype.toString.call(r[1]) + " " + r[2]);
          r[1].then(function (v) { L(name + " resolved"); }, function (e) { L(name + " rejected " + e.name); });
        } catch (e) {
          L(name + " " + i + " threw " + e.name + ": " + e.message);
        }
      }
    }
    run("string", function () { return imp("./no-such-module-k.js"); });
    run("object", function () { return imp({ toString: function () { L("toString"); return "./no-such-k.js"; } }); });
    run("throws", function () { return imp({ toString: function () { throw new Error("spec"); } }); });
    run("options", function () { return impOpt("./x.js", 5); });
    rvExpect(imp, impOpt);
    return log.join("\n");
  };
})()
