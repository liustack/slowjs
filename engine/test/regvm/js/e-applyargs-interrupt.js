// Copyright (c) 2026 The liustack/tifajs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.
// rvdiff: an interrupt (stopAfterPolls of jsrun) that comes in a function
// called by f.apply(this, arguments) from the register loop (CALLMA,
// engine/regvm-ops-args.h): the loop reads the flag before it makes the
// call and the callee polls in its loop, so the stop is thrown there, and
// its stack has the frame of apply under the callee, as the call of apply
// out of the loop had.
// rv-expect-op: CALLMA
(function () {
function spin(a, b) {
  var n = 0;
  while (a) n++;
  return n + b;
}
function fwd() { return spin.apply(this, arguments); }
return function () {
  var r = fwd(0, 1);
  rvExpect(fwd, spin);
  stopAfterPolls(50);
  return r + fwd(1, 2);
};
})()
