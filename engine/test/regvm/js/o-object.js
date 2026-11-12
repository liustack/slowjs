// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// rvdiff: throw_error (an assignment to the name of a function expression
// and to a const, with temps owned below it, and as the last instruction
// of a function) and the shuffles of destructuring and of
// compound assignments to elements: perm4 (o[k]++ with its value used), and
// dup1 (a computed key), swap2 and rot3l (an element as the target), whose
// patterns also need to_object (P5). The values are locals, constants and
// temps, keys run toString, and getters and setters run JavaScript or throw
// in the middle of the shuffled entries.
(function () { "use strict";
var out = [];
function id(x) { return x; }
// throw_error: the name of a function expression, a const local
var fe = function g(x) { if (x) g = 1; return typeof g; };
var fe2 = function g(x) { return id(1) + id(x) + (g = id(2)); };
var fe3 = function g(x) { g = x; return 1; };
function ro(x) { const c = x; if (x > 1) c = 2; return c; }
function ro2(x) { const c = x; if (x > 1) c++; return c * 2; }
function ro3(x) { const c = id(x); return c + id(x) + (c = id(3)); }
function last(x) { const c = 1; c = x; return c; }
// dup1: a computed key in an object pattern
function dk(o, k) { var a, b; ({ [k]: a, [id(k) + "2"]: b } = o); return a + ":" + b; }
// swap2: a computed key and an element as the target
function sw(o, k, t, i) { ({ [k]: t[i], [k + "2"]: t[id(i) + 1] } = o); return t; }
// rot3l: a named key and an element as the target
function rl(o, t, i) { ({ p: t[i], q: t[i + 1] } = o); return t; }
// perm4: the old value of an element kept
function pi(o, k) { var r = o[k]++; return r + "/" + o[k]; }
function pd(o, k) { var r = o[k]--; return r + "/" + o[k]; }
function pt(o, k) { return id(o)[id(k)]++ + id(1); }
function show(v) {
  if (typeof v === "symbol") return "sym";
  if (typeof v === "bigint") return v + "n";
  if (Object.is(v, -0)) return "-0";
  if (Array.isArray(v)) return "[" + v.map(show).join(",") + "]";
  try { return typeof v + ":" + String(v); } catch (e) { return typeof v + ":?"; }
}
function guard(f, a, b, c, d) {
  try { return show(f(a, b, c, d)); } catch (e) { return e.name + ":" + e.message; }
}
var log = [];
var key = { toString: function () { log.push("key"); return "a"; } };
var bad = { toString: function () { throw new TypeError("bad key"); } };
var src = { a: 1, a2: 2, z: 3, p: "P", q: "Q" };
var get = { get a() { log.push("get a"); return 10; }, get a2() { throw new RangeError("a2"); }, p: 1,
  get q() { log.push("get q"); return 2; } };
var setter = { set 0(v) { log.push("set0 " + v); }, set 1(v) { throw new SyntaxError("set1 " + v); } };
var frozen = Object.freeze([0, 0]);
for (var i = 0; i < 3; i++) {
  out.push(guard(ro, 1), guard(ro, 5), guard(ro2, 0), guard(ro2, 4), guard(ro3, 1), guard(last, 1));
  out.push(guard(fe, 0), guard(fe, 1), guard(fe2, 1), guard(fe3, 1));
  out.push(guard(dk, src, "a"), guard(dk, src, key), guard(dk, src, bad), guard(dk, get, "a"),
    guard(dk, null, "a"), guard(dk, { a: 5n, a2: Symbol.iterator }, "a"));
  out.push(guard(sw, src, "a", [], 0), guard(sw, src, key, [7, 7, 7], 1), guard(sw, get, "a", [], 0),
    guard(sw, src, "a", setter, 0), guard(sw, src, "a", frozen, 0), guard(sw, src, "a", null, 0));
  out.push(guard(rl, src, [], 0), guard(rl, get, [1, 2, 3], 1), guard(rl, src, setter, 0),
    guard(rl, src, frozen, 0), guard(rl, undefined, [], 0));
  out.push(guard(pi, { a: 1 }, "a"), guard(pi, { a: "7" }, key), guard(pi, [2147483647], 0),
    guard(pd, { a: 10n }, "a"), guard(pd, { a: undefined }, "a"), guard(pi, get, "a"), guard(pi, frozen, 0),
    guard(pi, { a: { valueOf: function () { throw new EvalError("vo"); } } }, "a"),
    guard(pi, null, "a"), guard(pt, { b: 1.5 }, "b"), guard(pt, { b: Symbol.iterator }, "b"));
}
out.push(log.join(","));
rvExpect(fe, fe2, fe3, pd, pi, pt);
rvExpect(dk, rl, sw, last, ro, ro2, ro3);
return out.join("\n");
})()
