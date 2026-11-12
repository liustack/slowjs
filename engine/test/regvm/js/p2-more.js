// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// rvdiff: the P2 ops with the constants of P1: a regexp literal (REGEXP, its
// pattern and bytecode constants), string and float constants as operands
// of the arithmetic and element ops, BigInts large enough to have a
// reference count (constants and results), and a Symbol.toPrimitive that
// changes the other operand while the op converts the first one.
(function () { "use strict";
var out = [];
function re(s) { var r = /a(b+)c/g; return r.test(s) + ":" + r.lastIndex; }
function re2(s) { return s.replace(/[aeiou]/g, "*"); }
function fl(x) { return x * 2.5 + x / 0.5 - x % 1.5; }
function str(x) { return x * "3" + (x | "8") + ("4" << x); }
function big(x) { return x * 123456789012345678901234567890n + 98765432109876543210987654321n; }
function bigops(x, y) { return (x * y) % 1000000000000000000000007n + (x << 70n) + (-x) + (x & y) + (x ^ y); }
function bigmix(x) { return x * 2; }
function el(a, k) { return a["k" + k] + a[k]; }
function prim(a, b) { return a * b; }
function prim2(a, b) { return a - b + (a | b); }
var arr = { k1: "one", 1: "uno", k2: 2.5, 2: "dos" };
for (var i = 0; i < 3; i++) {
  out.push(re("xabbbc"), re("ac"), re2("education"));
  out.push(fl(3), fl(-1.25), fl("2"), str(2), str(-1));
  out.push(String(big(3n)), String(big(-123456789012345678901234567890n)));
  out.push(String(bigops(1234567890123456789012345678901234567890n, 98765432109876543210n)));
  out.push(bigmix(1.5), bigmix(-3));
  try { bigmix(5n); } catch (e) { out.push(e.name); }
  out.push(el(arr, 1), el(arr, 2));
  var o = { v: 3 };
  var t = { [Symbol.toPrimitive]: function () { o.v = 10; return 4; } };
  out.push(prim(t, o.v), prim(o, t), o.v);
  var log = [];
  var p = { [Symbol.toPrimitive]: function (h) { log.push("p" + h); q.n = 100; return 6; } };
  var q = { n: 1, [Symbol.toPrimitive]: function (h) { log.push("q" + h); return this.n; } };
  out.push(prim2(p, q), log.join(","));
  try { prim({ [Symbol.toPrimitive]: function () { throw new RangeError("tp"); } }, 2); } catch (e) { out.push(e.name + e.message); }
  try { prim(1n, 1); } catch (e) { out.push(e.name); }
}
rvExpect(big, bigmix, bigops, el, fl, prim, prim2, re, re2, str);
return out.join(";");
})()
