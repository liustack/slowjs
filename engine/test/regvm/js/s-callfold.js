// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// rvdiff: calls whose last arguments are locals, arguments, this or
// constants that the call copies into its window itself: the same slot
// twice, a slot that also takes the result, a string constant, undefined,
// one to six of them, a slot written by an argument before it, a captured
// local, the arguments local, more and fewer arguments than parameters,
// the first call of a function not converted yet, a generator, C functions,
// call and bind, results dropped and tail calls, a callee that throws, a
// call of what is not a function, the line and column of both in the stack,
// and a deep recursion that moves to a new segment of the register stack.
(function () { "use strict";
var out = [];
function show() { var s = []; for (var i = 0; i < arguments.length; i++) s.push(typeof arguments[i] === "object" ? "o" : String(arguments[i])); return s.join("/"); }
function two(a, b) { return a + "," + b; }
function four(a, b, c, d) { return [a, b, c, d].join(","); }
function six(a, b, c, d, e, f) { return [a, b, c, d, e, f].join(","); }
function boom(a, b) { if (b === "x") throw new RangeError("boom " + a); return a; }
function same(a) { return two(a, a) + show(a, a, a); }
function intoSource(a, b) { a = two(b, a); b = two(a, b); return a + "|" + b; }
function strings(a) { return two("str", a) + two(a, "s" + "t") + show(a, "", "long string constant") + four(a, a, "k", "k"); }
function undef(a) { return two(a, undefined) + show(undefined, a, null, true, 1.5); }
function counts(a, b, c) { return show() + show(a) + two(a, b) + four(a, b, c, a) + six(a, b, c, a, b, c) + show(a, b, c, a, b, c, 7); }
function written(a) { var b = a; return two(b, b = "w") + two(b, (a = "z", a)) + a + b; }
function captured(a) { var c = a; var g = function () { c = "g"; return c; }; return two(c, g()) + c; }
function args(a) { return two(a, arguments.length) + show(a, arguments[0]); }
function moreFewer(a, b) { return two(a) + two(a, b, a, b) + four(b) + boom(a, b, a); }
function fresh(a, b) { function inner(x, y) { return x + "+" + y; } return inner(a, b) + inner(b, a); }
function* gen(a, b) { yield a; yield b; }
function useGen(a, b) { var g = gen(a, b), r = []; for (var v of g) r.push(v); return r.join(":"); }
function cfuns(a, s) { return Math.max(a, 3, s.length) + s.indexOf(a) + String(a) + parseInt("12", 10); }
function wrapped(a, b) { var f = two.bind(null), h = two; return f(a, b) + h.call(null, a, b) + two.call(undefined, b, a); }
function drop(a, b) { var n = []; function put(x, y) { n.push(x + y); } put(a, b); put(b, "d"); four(a, b, a, b); return n.join(); }
function tail(a, b) { return two(b, a); }
function tailK(a) { return four(a, "t", undefined, 3); }
var obj = { v: "V", m: function (a, b) { return this.v + a + b; } };
function viaThis(a) { return two(this.v, a) + show(this, a); }
function throwsIn(a) { try { return boom(a, "x"); } catch (e) { return e.message + " " + e.stack.split("\n").slice(0, 2).join(" "); } }
function notFn(a) { var f = a; try { return f(a, "x"); } catch (e) { return e.name + " " + e.stack.split("\n")[1]; } }
function stackIn(a, b) { return new Error(a + b).stack.split("\n").slice(0, 3).join(" "); }
function stackAt(a) { return stackIn(a, "s"); }
function rec(n, a, b) { return n === 0 ? a + b : rec(n - 1, a, b); }
for (var round = 0; round < 2; round++) {
  out.push(same("s"), same(1), intoSource("a", "b"), strings("q"), undef(0));
  out.push(counts(1, "b", 3), written("r"), captured("c"), args("a"), moreFewer("m", "n"));
  out.push(fresh(1, 2), useGen("y", 4), cfuns(5, "xyz5"), wrapped("w", 1), drop("p", "q"));
  out.push(tail("t", 1), tailK(9), obj.m("o", 2), viaThis.call(obj, "h"), throwsIn("t"));
  out.push(notFn(3), stackAt("e"), rec(3000, "deep", round));
}
rvExpect(same, intoSource, strings, undef, counts, written, captured, args, moreFewer, fresh, useGen, cfuns, wrapped, drop, tail, tailK, viaThis, throwsIn, notFn, stackIn, stackAt, rec);
rvExpect(two, four, six, show, boom);
return out.join("\n");
})()
