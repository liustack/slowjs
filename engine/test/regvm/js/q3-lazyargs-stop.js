// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// A stop (stopAfterPolls of jsrun) in a getter that a read of the arguments
// object runs right after ARGGET built the object into its local (see
// q3-lazyargs.js): the object is whole in the local when the getter spins,
// the stop passes the catch and the finally of the function, and the frame
// frees it on the way out. Were a catch to take the stop, the function
// would return and the output would be the log. The log is a string, as an
// array would meet the getter on Object.prototype at its index 5.
(function () { "use strict";
var log = "";
function g(a) {
  try {
    log += " " + arguments.length + " " + arguments[0];
    log += " " + arguments[5];
    return arguments[0] + arguments.length;
  } catch (e) {
    return "caught";
  } finally {
    log += " finally " + arguments.length;
  }
}
return function () {
  log += " " + g("a", "b");
  Object.defineProperty(Object.prototype, 5, {
    configurable: true,
    get: function () { var i = 0; while (this.length > 0) i++; return i; }
  });
  rvExpect(g);
  stopAfterPolls(100);
  log += " " + g("c");
  return log;
};
})()
