// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// rvdiff: switches on strings, run by the case chain of the register loop:
// each kind of key (an atom, a string made at run time, a number, null, an
// object), fall through, default first and last, a key that is an index
// ("1"), and a chain longer than one dispatch takes (its bodies return few
// numbers, so that the constants fit the table of the pass, and it is
// expected only with the fused ops: without them each case string is a
// constant, 80 of them, over the 64 of the table).
(function () { "use strict";
function sw(s) {
  switch (s) {
  case "a": return 1;
  case "bb": return 2;
  case "ccc": return 3;
  case "": return 4;
  default: return 0;
  }
}
function sw2(s) {
  var r = 0;
  switch (s) {
  case "x": r = r + 1;
  case "y": r = r + 2; break;
  case "z": r = 10; break;
  }
  return r;
}
function sw3(s) {
  switch (s) {
  default: return "d";
  case "p": return "P";
  case "q": return "Q";
  }
}
function sw4(s) {
  switch (s) {
  case "1": return "one";
  case "2": return "two";
  case "x": return "ex";
  }
  return "none";
}
var src = "\"use strict\"; return function (s) { switch (s) {", i;
for (i = 0; i < 80; i++)
  src += " case \"k" + i + "\": return " + (i % 7) + ";";
src += " } return -1; };";
var big = Function(src)();
var keys = ["a", "bb", "ccc", "", "b" + "b", "c".repeat(3), "x", "y", "z", "p", "q", "1", "2",
  1, 2, null, undefined, { toString: function () { return "a"; } }, "k0", "k" + 63, "k64", "k79", "k80"];
var out = [];
for (i = 0; i < keys.length; i++)
  out.push(sw(keys[i]), sw2(keys[i]), sw3(keys[i]), sw4(keys[i]), big(keys[i]));
rvExpect(sw, sw2, sw3, sw4);
if (rvFusedOps)
  rvExpect(big);
return out.join(",");
})()
