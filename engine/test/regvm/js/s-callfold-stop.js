// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// rvdiff: an interrupt (stopAfterPolls of jsrun) in a loop of a call whose
// last arguments, a local, an argument and a string constant, the call
// copies into its window itself: the stop leaves no reference behind,
// whether it comes at the entry of the call or at the jump back.
(function () {
function pair(a, b, c) { return a.length + b + c.length; }
function spin(a, m) {
  var s = "s" + a, n = 0, i = 0;
  for (; i !== m; i++) n = pair(s, a, "constant") + n & 0xffff;
  return n;
}
return function () {
  var r = spin(1, 10);
  rvExpect(pair, spin);
  stopAfterPolls(50);
  return r + spin(2, -1);
};
})()
