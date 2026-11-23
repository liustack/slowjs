// Copyright (c) 2026 The liustack/beastjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

// hermes -Xhermes-internal-test-methods [-Xjit] hermes-v8.js -- <src|hbc> <dir>: runs the V8 benchmark suite
// version 7 on Hermes and prints the scores as JSON, taking the steps of
// qjs-v8.js and of runV8 in cmd/jsbench: each file of <dir>
// (internal/arewefastyet/v8-v7) run as a global script in one runtime, then
// BenchmarkSuite.RunSuites.
//   src: <dir> holds the .js files. Each is read with hermescli.loadFile, and
//        run with an indirect eval, so that it runs as a global script and the
//        engine compiles it from source.
//   hbc: <dir> holds the same files compiled ahead by
//        `hermesc -O -emit-binary -out <name>.hbc <name>.js`. Each is run with
//        hermescli.loadHBC.
// The Hermes shell has print, and the hermescli object (loadFile, loadHBC,
// getScriptArgs) only with -Xhermes-internal-test-methods. It has no file
// system module, so these are the only I/O.
var args = hermescli.getScriptArgs();
var mode = args[0], dir = args[1];
var files = ["base", "richards", "deltablue", "crypto", "raytrace", "earley-boyer", "regexp", "splay", "navier-stokes"];
var geval = eval;
function decode(u8) {
  var s = "";
  for (var i = 0; i < u8.length; i += 8192) s += String.fromCharCode.apply(null, u8.subarray(i, i + 8192));
  return s;
}
for (var i = 0; i < files.length; i++) {
  if (mode === "hbc") hermescli.loadHBC(hermescli.loadFile(dir + "/" + files[i] + ".hbc"));
  else geval(decode(hermescli.loadFile(dir + "/" + files[i] + ".js")));
}
var __res = {};
BenchmarkSuite.RunSuites({
  NotifyResult: function (name, result) { __res[name] = String(result); },
  NotifyError: function (name, error) { __res[name] = "error: " + error; },
  NotifyScore: function (score) { __res.Score = String(score); }
});
print(JSON.stringify(__res));
