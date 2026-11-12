// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// jsrun: -result
// Sloppy generators and async functions in the register loop
// (engine/regvm-ops-gen.h): a this that is coerced to an object and kept
// across a yield, a this of undefined that becomes the global object,
// stores to undeclared globals and to read only properties that fail
// without a throw, between suspensions.
(function () {
function* boxed() {
  var t = this;
  var a = yield typeof t;
  yield typeof this + ":" + (this == t) + ":" + (t + a);
  frozen.x = a; // fails without a throw in sloppy code
  yield frozen.x;
}
function* global_this() {
  var g = this;
  yield g === globalThis;
  undeclaredW3g = (yield 1) + 1;
  yield undeclaredW3g;
}
async function later(n) {
  var t = this;
  await null;
  sloppyAsyncW3g = n;
  await null;
  return typeof t + ":" + sloppyAsyncW3g;
}
var frozen = Object.freeze({ x: 7 });
function drain(g, sends) {
  var out = [], r, i = 0;
  for (;;) {
    r = g.next(sends ? sends[i++] : undefined);
    if (r.done)
      break;
    out.push(String(r.value));
  }
  return out.join(",");
}
function run() {
  var r = [], k;
  for (k = 0; k < 3; k++) {
    r.push(drain(boxed.call(5), [undefined, 10, 3]),
           drain(boxed.call("s"), [undefined, "x", 4]),
           drain(global_this(), [undefined, undefined, 41]));
  }
  return r.join(" | ");
}
var out = run();
Promise.all([later.call(2, 1), later.call(undefined, 2), later.call({}, 3)])
  .then(function (v) {
    rvExpect(boxed, global_this, later, drain);
    globalThis.result = out + " | " + v.join(" ");
  });
})();
