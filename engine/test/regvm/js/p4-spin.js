// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// A loop that starts at the first instruction of its function jumps back to
// offset 0 of the old bytecode: an interrupt read at that jump is named at
// the pc before its target, which is -1, the place where the function is
// defined. spin converts at its first call, which run makes from the
// register loop, so it spins there.
(function () { "use strict";
function spin(o) {
  for (;;) {
    o.n = o.n + 1;
  }
}
function run(go) {
  if (go)
    return spin({ n: 0 });
  return 0;
}
return function () {
  run(false);
  run(false);
  rvExpect(run);
  stopAfterPolls(50);
  return run(true);
};
})()
