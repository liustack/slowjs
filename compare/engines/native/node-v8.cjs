// Copyright (c) 2026 The liustack/tifajs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

// node --jitless node-v8.cjs <dir>: runs the V8 benchmark suite version 7 in
// <dir> (internal/arewefastyet/v8-v7) on Node and prints the scores as JSON.
// It takes the steps of qjs-v8.js and of runV8 in cmd/jsbench: each file
// run as a global script in one context (vm.runInThisContext, as
// node-ssr.cjs runs its bundles), then BenchmarkSuite.RunSuites.
const fs = require("fs");
const vm = require("vm");
const dir = process.argv[2];
const files = ["base.js", "richards.js", "deltablue.js", "crypto.js", "raytrace.js", "earley-boyer.js", "regexp.js", "splay.js", "navier-stokes.js"];
for (const f of files) vm.runInThisContext(fs.readFileSync(dir + "/" + f, "utf8"), { filename: f });
const res = {};
BenchmarkSuite.RunSuites({
  NotifyResult: (name, result) => { res[name] = String(result); },
  NotifyError: (name, error) => { res[name] = "error: " + error; },
  NotifyScore: (score) => { res.Score = String(score); },
});
fs.writeSync(1, JSON.stringify(res) + "\n");
