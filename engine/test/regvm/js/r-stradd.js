// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// a + b of two strings in each form of the register loop's add: ADD into a
// temp, ADDL into a local or an argument, ADDK and ADDKL with a constant on
// the right, KADD with one on the left, into a temp and into a local
// (engine/regvm-ops-basic.h, rv_add_str). Strings of both widths, ropes, the
// empty string, the left one being the string of the local it goes into,
// and a sum past the longest string (JS_STRING_LEN_MAX, ropes doubled, so
// that nothing large is allocated): the error is the engine's, an
// InternalError, where Node throws a RangeError at about half that length,
// and the variable keeps the string it had. Then a string with an operand
// that is not a string in each form, which goes to the generic add.
// inject-hit: ADD 1
// inject-hit: ADDL 1
// inject-hit: ADDK 1
// inject-hit: ADDKL 1
// inject-hit: KADD 1
(function () { "use strict";
var log = [];
function err(e) { return e.constructor.name + ": " + e.message; }
function add(a, b) { return a + b; }
function addl(a, b) { var t = "x"; t = a + b; return t; }
function addself(a, b) { var t = a; t = t + b; return t; }
function addk(a) { return a + "<k>"; }
function addkl(a) { var t = a; t = t + "</k>"; return t; }
function kadd(b) { return "<k>" + b; }
function kaddl(b) { var t = b; t = "<k>" + t; return t; }
function sum(s) {
  var h = 0, i;
  for (i = 0; i < s.length; i++) h = (h * 31 + s.charCodeAt(i)) | 0;
  return s.length + ":" + h;
}
var long = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ_-";
var vals = ["", "a", long, "中" + long, long + long + long + long + long + long + long + long + long,
  "r".repeat(9000) + "s".repeat(9000)];
var row = [], i, j;
for (i = 0; i < vals.length; i++) {
  for (j = 0; j < vals.length; j++) {
    row.push(sum(add(vals[i], vals[j])) === sum(vals[i] + vals[j]) &&
      sum(addl(vals[i], vals[j])) === sum(vals[i].concat(vals[j])) &&
      sum(addself(vals[i], vals[j])) === sum(vals[i].concat(vals[j])) ? "." : i + "," + j);
  }
  row.push(sum(addk(vals[i])) + "/" + sum(addkl(vals[i])) + "/" + sum(kadd(vals[i])) + "/" + sum(kaddl(vals[i])));
}
log.push("forms " + row.join(" "));
// past the longest string: 2^20 doubled 10 times is 2^30, one more than it
var big = "x".repeat(1 << 20), n = 0;
try {
  for (n = 0; n < 12; n++) big = add(big, big);
} catch (e) { log.push("add " + n + " " + big.length + " " + err(e)); }
// 2^30 - 2 characters, ropes that share one string of 2^20
var piece = "x".repeat(1 << 20), r = piece, big2 = "x".repeat((1 << 20) - 2);
for (n = 20; n < 30; n++) { big2 = r + big2; if (n < 29) r = r + r; }
log.push("max " + big2.length);
var tries = [
  function () { return add(big2, "ab"); },
  function () { return addl(big2, "ab"); },
  function () { return addself(big2, big2); },
  function () { return addk(big2); },
  function () { return addkl(big2); },
  function () { return kadd(big2); },
  function () { return kaddl(big2); },
];
for (i = 0; i < tries.length; i++) {
  try { log.push("long " + i + " " + tries[i]().length); } catch (e) { log.push("long " + i + " " + err(e)); }
}
// the same error caught in the function that holds the destination: a local
// keeps the string it had, after ADDL, ADDL into its own left operand (the
// right one from a call: t = t + b of two locals is ADDLOC), ADDKL and KADD
// into a local, and the operands of ADD, ADDK and KADD into a temp are still
// there
function id(x) { return x; }
function same(x, y) { return x === y ? "kept" : "LOST " + typeof x; }
var held = [
  function (a, b) { var t = "old"; try { t = a + b; } catch (e) { return err(e) + " " + same(t, "old"); } return "none"; },
  function (a, b) { var t = a; try { t = t + id(b); } catch (e) { return err(e) + " " + same(t, a); } return "none"; },
  function (a) { var t = "old"; try { t = a + "</k>"; } catch (e) { return err(e) + " " + same(t, "old"); } return "none"; },
  function (b) { var t = "old"; try { t = "<k>" + b; } catch (e) { return err(e) + " " + same(t, "old"); } return "none"; },
  function (a, b) { var x = a, y = b; try { return (a + b).length; } catch (e) { return err(e) + " " + same(a, x) + " " + same(b, y); } },
  function (a) { var x = a; try { return (a + "<k>").length; } catch (e) { return err(e) + " " + same(a, x); } },
  function (b) { var y = b; try { return ("<k>" + b).length; } catch (e) { return err(e) + " " + same(b, y); } },
];
for (i = 0; i < held.length; i++) {
  held[i]("a", "b");
  held[i]("a", "b");
  log.push("held " + i + " " + held[i](big2, big2));
}
rvExpect.apply(null, held);
big = big2 = r = piece = undefined;
// an operand that is not a string: the generic add, which converts it
var conv = [];
var o = { toString: function () { conv.push("toString"); return "O"; }, valueOf: function () { conv.push("valueOf"); return "V"; } };
var others = [1, 1.5, true, null, undefined, 3n, o, new String("so"), [1, 2]];
row = [];
for (i = 0; i < others.length; i++)
  row.push(add("s", others[i]) + "|" + addl(others[i], "s") + "|" + addk(others[i]) + "|" + kadd(others[i]) + "|" + kaddl(others[i]));
log.push("others " + row.join(" ") + " " + conv.join(","));
try { add("s", Symbol("x")); } catch (e) { log.push("symbol " + e.constructor.name); }
try { kaddl(Symbol("x")); } catch (e) { log.push("symbol k " + e.constructor.name); }
rvExpect(add, addl, addself, addk, addkl, kadd, kaddl);
return log.join("\n");
})()
