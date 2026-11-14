// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// rvdiff: an interrupt (stopAfterPolls of jsrun) in a loop that does nothing
// but call built-ins the loop answers itself (rv_fast_call in
// engine/regvm-ops-basic.h): each answer reads the interrupt flag as the call
// of the C function did, so the stop comes at the same call, and the stack
// names its line and column, which the golden recorded from the C calls.
(function () {
var log = [];
function mSpin(m, s, lim) {
  var n = 0;
  while (lim) {
    m.has(1); s.has("a"); Math.clz32(n); m.has("b");
    s.has(2); Math.clz32(lim); m.has(n); s.has(n);
    n++;
  }
  return n;
}
function mSpinBelow(m, s, lim) { return ["a", mSpin(m, s, lim)].join(); }
var m = new Map([[1, 1], ["b", 2]]), s = new Set(["a", 2]);
return function () {
  log.push(mSpinBelow(m, s, 0));
  log.push(mSpinBelow(m, s, 0));
  rvExpect(mSpin, mSpinBelow);
  stopAfterPolls(103);
  log.push(mSpinBelow(m, s, 1));
  return log.join(" ");
};
})()
