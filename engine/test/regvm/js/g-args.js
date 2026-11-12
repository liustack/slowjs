// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// jsrun: -result
// Generators and async functions that read their arguments in the register
// loop (engine/regvm-ops-gen.h, frame-args.md): the arguments past the
// parameters are below func at the first resume only, where the prologue
// makes the arguments object and rest, and are gone at the first
// suspension. The arguments object, rest and a mapped arguments object
// (sloppy code, aliased to the parameters) are read and written across
// later suspensions, with more, fewer and as many arguments as parameters.
(function () {
function* mapped(a, b) {
  arguments[0] = "m";
  yield a + ":" + arguments.length;
  a = "n";
  yield arguments[0] + arguments[2];
  b = arguments[3];
  yield b;
}
function* plain(a) {
  "use strict";
  var args = arguments;
  yield args.length;
  a = 7;
  yield args[0] + ":" + args[2] + ":" + a;
}
function* rest(a, ...more) {
  yield more.length;
  yield a + more.join("-");
}
async function wait(a, ...more) {
  var args = arguments;
  await null;
  more.push(args.length);
  await null;
  return a + more.join("+") + args[args.length - 1];
}
async function* agen(a) {
  var args = arguments;
  yield args.length;
  await null;
  yield args[1] + a;
}
function drain(g) {
  var out = [], r;
  while (!(r = g.next()).done)
    out.push(String(r.value));
  return out.join(",");
}
function run() {
  var r = [], k;
  for (k = 0; k < 3; k++) {
    r.push(drain(mapped(1, 2, 3, 4)), drain(mapped(1)), drain(mapped(1, 2)),
           drain(plain(1, 2, 3)), drain(plain()), drain(rest(1, 2, 3, 4)),
           drain(rest(1)));
  }
  return r.join(" | ");
}
var out = run();
var g = agen("x", "y", "z"), seen = [];
function step(r) {
  if (r.done)
    return seen.join(",");
  seen.push(r.value);
  return g.next().then(step);
}
Promise.all([wait(1, 2, 3), wait(), wait(1), g.next().then(step)])
  .then(function (v) {
    rvExpect(mapped, plain, rest, wait, agen, drain, step);
    globalThis.result = out + " | " + v.join(" ");
  });
})();
