// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// qjs --std qjs-v8.js <dir>: runs the V8 benchmark suite version 7 in <dir>
// (internal/arewefastyet/v8-v7) on C QuickJS and prints the scores as JSON.
// It takes the steps runV8 of cmd/jsbench takes: each file evaluated as a
// global script in one context, then BenchmarkSuite.RunSuites.
var dir = scriptArgs[1];
var files = ["base.js", "richards.js", "deltablue.js", "crypto.js", "raytrace.js", "earley-boyer.js", "regexp.js", "splay.js", "navier-stokes.js"];
for (var i = 0; i < files.length; i++) std.evalScript(std.loadFile(dir + "/" + files[i]));
var __res = {};
BenchmarkSuite.RunSuites({
  NotifyResult: function (name, result) { __res[name] = String(result); },
  NotifyError: function (name, error) { __res[name] = "error: " + error; },
  NotifyScore: function (score) { __res.Score = String(score); }
});
print(JSON.stringify(__res));
