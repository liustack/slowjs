// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// rvdiff: shr (>>>) and pow (**) in the register loop, with a slot, a temp
// and a constant operand, on every pair of a set of values: ints at the
// ends of their range (a shift whose result is above 2^31 - 1 gives a float),
// -0, floats, strings, null, undefined, objects whose valueOf runs
// JavaScript or throws, BigInts (>>> throws on any, ** throws when mixed or
// with a negative exponent, and makes BigInts large enough to have a
// reference count) and a Symbol. Some throw with temps owned below the
// throwing instruction.
(function () { "use strict";
var log = [];
var calls = { n: 0 };
var vals = [0, -0, 1, -1, 2, 3, 31, 32, -7, 2147483647, -2147483648, 4294967295, 65536, 0.5,
  -1.5, 2.5, NaN, Infinity, -Infinity, 1e300, "", "12", "a", "-0x10", true, false, null, undefined,
  { valueOf: function () { calls.n = calls.n + 1; return shr(id(-8), 1); } },
  { valueOf: function () { throw new RangeError("valueOf threw"); } },
  [], [5], 10n, -3n, 0n, 123456789012345678901234567890n, Symbol.iterator];
function id(x) { return x; }
function shr(a, b) { return a >>> b; }
function pow(a, b) { return a ** b; }
// temps on both sides, and a temp owned below the op when it throws
function tshr(a, b) { return id(b) + (id(a) >>> id(b)); }
function tpow(a, b) { return id(b) + id(a) ** id(b); }
// constant operands: ints, a float, a string, a BigInt; the result into an array
function k(a, r) {
  r[0] = a >>> 0; r[1] = a >>> 1; r[2] = a >>> 31; r[3] = a >>> 33; r[4] = a >>> -1;
  r[5] = a ** 2; r[6] = a ** 0.5; r[7] = a ** -1; r[8] = a ** 0; r[9] = a ** "3";
  r[10] = 2 ** a; r[11] = -1 >>> a; r[12] = a >>> "2";
}
function kbig(a, r) { r[0] = a ** 3n; r[1] = 2n ** a; r[2] = a ** 100n; }
// the hash loop of emotion's murmur2, the shape that blocks MUI
function murmur(str) {
  var h = 0, kk, i = 0, len = str.length;
  for (; len >= 4; ++i, len -= 4) {
    kk = str.charCodeAt(i) & 255 | (str.charCodeAt(++i) & 255) << 8 |
      (str.charCodeAt(++i) & 255) << 16 | (str.charCodeAt(++i) & 255) << 24;
    kk = (kk & 65535) * 1540483477 + ((kk >>> 16) * 59797 << 16);
    kk ^= kk >>> 24;
    h = (kk & 65535) * 1540483477 + ((kk >>> 16) * 59797 << 16) ^
      (h & 65535) * 1540483477 + ((h >>> 16) * 59797 << 16);
  }
  h ^= h >>> 13;
  h = (h & 65535) * 1540483477 + ((h >>> 16) * 59797 << 16);
  return ((h ^ h >>> 15) >>> 0).toString(36);
}
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
  try { f(a, r); s = ""; } catch (e) { s = e.name + ":" + e.message + " "; }
  for (var i = 0; i < n; i++) s = s + show(r[i]) + ",";
  return s;
}
var bins = [shr, pow, tshr, tpow];
for (var i = 0; i < vals.length; i++) {
  var row = [];
  for (var j = 0; j < vals.length; j++) {
    var cell = [];
    for (var f = 0; f < bins.length; f++)
      cell.push(guard(bins[f], vals[i], vals[j]));
    row.push(cell.join("|"));
  }
  row.push(guardr(k, vals[i], 13), guardr(kbig, vals[i], 3));
  log.push(show(vals[i]) + " => " + row.join(" "));
}
// above 2^31 - 1 and below
log.push([shr(-1, 0), shr(-1, 1), shr(-2147483648, 0), shr(2147483647, 0), shr(-8, 28)].map(show).join(" "));
log.push(String(pow(7n, 77n)) + " " + guardr(kbig, 5n, 3) + " " + pow(2, 1024) + " " + pow(-8, 1 / 3));
var hs = [];
for (var m = 0; m < 4; m++)
  hs.push(murmur("css-" + m + "-label:Button;color:red;padding:" + m * 3 + "px;"));
log.push(hs.join(" "));
log.push("valueOf calls " + calls.n);
rvExpect(id, k, kbig, murmur, pow, shr, tpow, tshr);
return log.join("\n");
})()
