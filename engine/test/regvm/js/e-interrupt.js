// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// rvdiff: an interrupt (stopAfterPolls of jsrun) read where a with_* op jumps
// to the code after it, as the old loop reads it there: the value it found
// is in its temp then, and the caller has a temp of its own below the call.
// The loop reads the flag nine times at the with statements for each time at
// its jump back, so the stop comes at one of them, and the stack names its
// line.
(function () {
var log = [];
function wSpin(o, lim, k) {
  var n = 0;
  while (lim) {
    log.length = [k + "t", n++].length;
    with (o) { x; }
    with (o) { x; }
    with (o) { x; }
    with (o) { x; }
    with (o) { x; }
    with (o) { x; }
    with (o) { x; }
    with (o) { x; }
    with (o) { x; }
  }
  return n;
}
function wSpinBelow(o, lim, k) { return [k + "a", wSpin(o, lim, k)].join(); }
var o = { x: {} };
return function () {
  log.push(wSpinBelow(o, 0, "a"));
  log.push(wSpinBelow(o, 0, "b"));
  rvExpect(wSpin, wSpinBelow);
  stopAfterPolls(100);
  log.push(wSpinBelow(o, 1, "c"));
  return log.join(" ");
};
})()
