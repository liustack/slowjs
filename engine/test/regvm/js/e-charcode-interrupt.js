// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// rvdiff: an interrupt (stopAfterPolls of jsrun) in a loop that does nothing
// but call charCodeAt, which the loop answers itself without a call
// (rv_fast_call in engine/regvm-ops-basic.h), so that no poll comes from the
// calls: the loop's own counts still bring the stop, and the stack names
// the line of the loop. Were a stop lost, the script would never end.
(function () {
var log = [];
function cSpin(s, lim) {
  var n = 0;
  while (lim) {
    s.charCodeAt(0); s.charCodeAt(1); s.charCodeAt(2); s.charCodeAt(3);
    s.charCodeAt(4); s.charCodeAt(5); s.charCodeAt(6); s.charCodeAt(7);
    n++;
  }
  return n;
}
function cSpinBelow(s, lim) { return ["a", cSpin(s, lim)].join(); }
var s = "abcdefgh";
return function () {
  log.push(cSpinBelow(s, 0));
  log.push(cSpinBelow(s, 0));
  rvExpect(cSpin, cSpinBelow);
  stopAfterPolls(100);
  log.push(cSpinBelow(s, 1));
  return log.join(" ");
};
})()
