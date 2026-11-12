// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// The length of a string read by GETF in the register loop, which answers it
// in the handler (engine/regvm-ops-basic.h): strings of 8-bit and 16-bit
// characters, the empty string, ropes made by concatenation, a constant, a
// string in a temp, String objects and objects with a length of their own,
// other names on strings, length on values that are not strings, and
// a getter put on the length of String.prototype, which it refuses.
(function () { "use strict";
var log = [];
function len(s) { return s.length; }
function lent(s) { return String(s).length; }
function lenk() { return "abc".length; }
function lenl(s) { var n = s.length; return n; }
function other(s) { return s.big + "|" + s.at + "|" + typeof s.charAt; }
function guard(f, v) {
  try { return String(f(v)); } catch (e) { return e.name; }
}
var rope = "", i;
for (i = 0; i < 200; i++) rope = rope + "ab" + i;
var wide = "中文" + rope + "😀";
var vals = ["", "a", "abc", "中", "😀", rope, wide, rope + rope, new String("xyz"),
  { length: 7 }, [1, 2, 3], function (a, b) {}, 5, 1.5, true, null, undefined, 10n, Symbol("s")];
var row = [];
for (i = 0; i < vals.length; i++)
  row.push(guard(len, vals[i]) + "/" + guard(lenl, vals[i]));
log.push("len " + row.join(" "));
row = [];
for (i = 0; i < 6; i++) row.push(guard(lent, vals[i]));
log.push("temp " + row.join(" ") + " k " + lenk());
log.push("other " + other("abc") + " " + other(rope));
try { Object.defineProperty(String.prototype, "length", { get: function () { return -1; } }); } catch (e) { log.push("define " + e.name); }
log.push("proto getter " + len("abcd") + " " + len(rope) + " " + len(new String("ab")));
var sum = 0;
for (i = 0; i < 1000; i++) sum = sum + len(i % 2 ? rope : "xy");
log.push("sum " + sum);
rvExpect(len, lent, lenk, lenl, other, guard);
return log.join("\n");
})()
