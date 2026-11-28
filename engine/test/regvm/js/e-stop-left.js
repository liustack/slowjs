// Copyright (c) 2026 The liustack/beastjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.
// rvdiff: a stop (stopAfterPolls of jsrun) that the script ends before is
// dropped when the script and its jobs end, so that the result is the
// script's. The format that rvdiff compiles and runs after the script reads
// the interrupt flag, and a stop left for it would end the run with an
// exception the script never threw, at a place set by how often the parser
// of the day reads the flag (q3-lazyargs-stop.js met that at its injected
// throws). sum reads the flag far fewer times than the stop asks for.
(function () { "use strict";
function sum(n) {
  var s = 0;
  for (var i = 0; i < n; i++)
    s += i;
  return s;
}
return function () {
  sum(1);
  rvExpect(sum);
  stopAfterPolls(30);
  return sum(10);
};
})()
