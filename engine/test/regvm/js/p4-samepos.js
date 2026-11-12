// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// Functions that are defined at the same line and column: each new Function
// has a source of its own, which starts at line 1, as every class field
// initializer is defined at 1:1. Where read has an instruction that can
// throw (its add), copy has one that cannot, at the same offset, and runs
// it more often. The throw-injection test (engine/test/regvm/inject.sh)
// counts the runs of an instruction by its function, and must tell the two
// apart, or the old loop throws in copy where the register loop threw in
// read.
(function () { "use strict";
var read = new Function("o", "'use strict'; var r = o.x; return r + 1;");
var copy = new Function("o", "'use strict'; var a = o, b = a, c = b, d = c, e = d, f = e, g = f; return g;");
function each(o, n) {
  var s = 0, i;
  for (i = 0; i < n; i++)
    s = s + copy(o).x + copy(o).x;
  return s + read(o);
}
return function () {
  var o = { get x() { return 1; } };
  var r = [each(o, 1), each(o, 2), each(o, 3)];
  rvExpect(read, copy, each);
  return r.join(" ");
};
})()
