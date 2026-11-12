// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// rvdiff: an interrupt (stopAfterPolls of jsrun) asked while the loop takes
// apart a chain of 100000 bound functions (rv_unwrap_call), which reads the
// flag at each one: the stop comes in the call of the chain, in viaDeep,
// not in the loop that spins after it.
(function () {
var o = { k: 1 };
function f(a) { return this.k + a; }
var deep = f, i;
for (i = 0; i < 100000; i++) {
  deep = deep.bind(o);
  Object.defineProperty(deep, "name", { value: "" });
}
function viaDeep(k) { return deep(k); }
return function () {
  var r = viaDeep(1), n = 0;
  rvExpect(f, viaDeep);
  stopAfterPolls(50);
  r += viaDeep(2);
  while (r) n++;
  return r + " " + n;
};
})()
