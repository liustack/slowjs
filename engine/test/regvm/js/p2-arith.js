// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// P2 of the register loop: mul, div, mod, and, or, xor, shl, sar with a slot,
// a temp and a constant operand, not, neg, plus, inc, dec, post_inc,
// post_dec, typeof and add_loc, on every pair of a set of values: ints at the
// ends of their range, -0, floats, strings, null, undefined, objects whose
// valueOf runs JavaScript or throws, BigInts (which throw when mixed) and a
// Symbol. Some throw with temps owned below the throwing instruction. The
// functions under test have no string constants, so that they convert
// before the constants of P1 do.
var g = 0;
(function () { "use strict";
var log = [];
var calls = { n: 0 };
var vals = [0, -0, 1, -1, 3, -7, 2147483647, -2147483648, 65536, 0.5, -1.5, NaN, Infinity,
  1e300, "", "12", "a", true, false, null, undefined,
  { valueOf: function () { calls.n = calls.n + 1; return mul(id(7), 1); } },
  { valueOf: function () { throw new RangeError("valueOf threw"); } },
  [], [5], 10n, -3n, Symbol.iterator];
function id(x) { return x; }
function mul(a, b) { return a * b; }
function div(a, b) { return a / b; }
function mod(a, b) { return a % b; }
function and(a, b) { return a & b; }
function or(a, b) { return a | b; }
function xor(a, b) { return a ^ b; }
function shl(a, b) { return a << b; }
function sar(a, b) { return a >> b; }
// temps on both sides, and a temp owned below the op when it throws
function tmul(a, b) { return id(a) + id(a) * id(b); }
function tdiv(a, b) { return id(a) + id(a) / id(b); }
function tmod(a, b) { return id(a) + id(a) % id(b); }
function tand(a, b) { return id(a) + (id(a) & id(b)); }
function tor(a, b) { return id(b) + (id(a) | id(b)); }
function txor(a, b) { return id(b) + (id(a) ^ id(b)); }
function tshl(a, b) { return id(b) + (id(a) << id(b)); }
function tsar(a, b) { return id(b) + (id(a) >> id(b)); }
// constant operands, the result into an array
function k(a, r) { r[0] = a * 3; r[1] = a * -1; r[2] = a / 2; r[3] = a % 5; r[4] = a & 255; r[5] = a | 0; r[6] = a ^ -1; r[7] = a << 3; r[8] = a >> 1; r[9] = a * -2; r[10] = a % -3; }
function un(a, r) { r[0] = ~a; r[1] = -a; r[2] = +a; r[3] = typeof a; }
function tun(a, r) { r[0] = id(a) + ~id(a); r[1] = id(a) + -id(a); r[2] = id(a) + +id(a); r[3] = typeof id(a); }
function pre(a, r) { r[0] = ++a; r[1] = --a; r[2] = a; }
function post(a, r) { r[0] = a++; r[1] = a--; r[2] = a; }
function gpost(r) { r[0] = g++; r[1] = g--; r[2] = g; }
function addloc(a, b) { var s = a; s += b; s += 1; s += 2147483647; return s; }
function addloc2(a) { var s = a; s += s; return s; }
function addloc3(a, b) { var s = a; s += id(b); return s; }
function strbuild(n, c) { var s = c; var i = 0; while (i < n) { s += c; s += i; i = i + 1; } return s; }
function show(v) {
  if (typeof v === "symbol") return "sym";
  if (typeof v === "bigint") return v + "n";
  if (Object.is(v, -0)) return "-0";
  try { return typeof v + ":" + String(v); } catch (e) { return typeof v + ":?"; }
}
function guard(f, a, b) {
  try { return show(f(a, b)); } catch (e) { return e.name + ":" + e.message; }
}
function guardr(f, a, n) {
  var r = [], s;
  r[5] = a; r[6] = a;
  try { f(a, r); s = ""; } catch (e) { s = e.name + ":" + e.message + " "; }
  for (var i = 0; i < n; i++) s = s + show(r[i]) + ",";
  return s;
}
var bins = [mul, div, mod, and, or, xor, shl, sar, tmul, tdiv, tmod, tand, tor, txor, tshl, tsar];
for (var i = 0; i < vals.length; i++) {
  var row = [];
  for (var j = 0; j < vals.length; j++) {
    var cell = [];
    for (var f = 0; f < bins.length; f++)
      cell.push(guard(bins[f], vals[i], vals[j]));
    row.push(cell.join("|"));
  }
  g = vals[i];
  row.push(guardr(k, vals[i], 11), guardr(un, vals[i], 4), guardr(tun, vals[i], 4),
    guardr(pre, vals[i], 3), guardr(post, vals[i], 3),
    guardr(gpost, vals[i], 3), guard(addloc, vals[i], vals[1]), guard(addloc, vals[i], "s"),
    guard(addloc2, vals[i]), guard(addloc3, vals[i], "t"), guard(addloc3, "t", vals[i]));
  log.push(show(vals[i]) + " => " + row.join(" "));
}
log.push(strbuild(100, "ab").length + " " + strbuild(3, "ab") + " " + strbuild(3, 1));
// ints that leave the range
log.push([mul(65536, 65536), mul(-1, 0), mul(0, -5), mul(-2147483648, -1), div(1, 0), div(-1, 0),
  div(0, -1), div(6, 3), mod(-1, 1), mod(5, -3), mod(-2147483648, -1), mod(7, 0), shl(1, 31), shl(1, 32),
  sar(-1, 33), and(-1, 2147483648), or(0.5, 1e10)].map(show).join(" "));
log.push("valueOf calls " + calls.n);
rvExpect(addloc, addloc2, addloc3, and, div, gpost, id, k, mod, mul, or, post, pre, sar, shl, strbuild, tand, tdiv, tmod, tmul, tor, tsar, tshl, tun, txor, un, xor);
return log.join("\n");
})()
