// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// import() and import.meta in a module of another file, run in the register
// loop two calls deep from this module (drive, then outer and inner of
// k-modules/k-importer.mjs): the module they name is the one of the function
// that runs (JS_GetScriptOrModuleName walks the frames of the register
// loop), so "./k-imported.mjs" is found next to k-importer.mjs, not here, and
// import.meta is k-importer.mjs's.
import { meta, inner, outer } from "./k-modules/k-importer.mjs";

var log = [];
function drive(spec) {
  return outer(spec);
}
for (var i = 0; i < 3; i++) {
  var r = drive("./k-imported.mjs");
  log.push(i + ": meta of k-importer " + r[1] + ", not this one's " + (meta !== import.meta));
  r[0].then(function (m) { log.push("imported " + m.name); },
            function (e) { log.push("rejected " + e.name + ": " + e.message); });
}
globalThis.result = { toString: function () { return log.join("\n"); } };
rvExpect(drive, outer, inner);
