// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// rvdiff: calls whose func, this or arguments are locals, arguments or this,
// mixed with temps and constants, plain and method calls, too few and too
// many arguments, calls of C functions, of functions not converted yet,
// tail calls, a call that throws and a callee that keeps its arguments.
(function () { "use strict";
var out = [];
function add(a, b) { return a + b; }
function add3(a, b, c) { return a + "," + b + "," + c; }
function keep(a, b) { return function () { return a + b; }; }
function argsof() { return arguments.length + ":" + Array.prototype.join.call(arguments, "/"); }
function thrower(a) { if (a === "boom") throw new TypeError("t" + a); return a; }
function locals(f, x, y) { var s = "s" + x; return f(x, s) + f(s, y) + f(y, y); }
function mixed(f, x) { return f(x, x + 1) + f("k", x) + add(x, f(x, "z")); }
function method(o, x) { var m = "m"; return o.f(x, m) + o.f(m, "c") + o.g(); }
function methodLocal(o, x) { var o2 = o; return o2.f(x, o2) + o2.f(x); }
function viaThis(x) { return this.f(x, this.v); }
function few(x) { return add3(x) + add3(x, x); }
function many(x, y) { return add(x, y, x, y) + argsof(x, y, "c", x); }
function cfun(s, x) { return Math.max(x, s.length) + s.indexOf(x) + String(x); }
function tail(f, x) { return f(x, x); }
// a tail call with the value of a switch under it, a counted string
function tailSwitch(f, x) { switch (x + "!") { case "1!": return f(x, "one"); case "a!": return f("a", x); default: return f(x, x + "?"); } }
function catchy(x) { try { return thrower(x); } catch (e) { return e.message; } }
function closures(x, y) { var a = keep(x, y), b = keep(y, x); x = "changed"; return a() + b() + x; }
function reassign(f, x) { var y = x; x = "new"; return f(y, x) + f(x, y); }
function nested(f, x) { return f(f(x, x), f(x, "n")); }
function selfrec(n, acc) { return n === 0 ? acc : selfrec(n - 1, acc + n); }
var obj = { v: "V", f: function (a, b) { return "[" + a + "|" + (typeof b === "object" ? "obj" : b) + "]"; }, g: function () { return this.v; } };
for (var round = 0; round < 2; round++) {
out.push(locals(add, "x", "y"), locals(add3, 1, 2));
out.push(mixed(add, 5), mixed(add, "q"));
out.push(method(obj, "a"), methodLocal(obj, "b"), viaThis.call(obj, 7));
out.push(few("f"), many("p", "q"), cfun("hello", "l"), cfun("x", 3));
out.push(tail(add, 4), tail(argsof, "t"), catchy("ok"), catchy("boom"));
out.push(tailSwitch(add, 1), tailSwitch(add, "a"), tailSwitch(argsof, "z"));
out.push(closures("c1", "c2"), reassign(add, "r"), nested(add, "w"), selfrec(50, 0));
}
for (var i = 0; i < 3; i++) out.push(locals(argsof, i, "i"));
try { locals(thrower, "boom", 1); } catch (e) { out.push(e.name + e.message); }
try { locals(null, 1, 2); } catch (e) { out.push(e.name); }
try { method({ f: 1 }, 1); } catch (e) { out.push(e.name); }
try { tailSwitch(thrower, "boom"); } catch (e) { out.push(e.name + e.message); }
try { tailSwitch(null, 1); } catch (e) { out.push(e.name); }
rvExpect(add, add3, cfun, closures, few, locals, many, method, methodLocal, mixed, nested, reassign, selfrec, tail, tailSwitch, viaThis);
rvExpect(catchy, thrower);
return out.join(";");
})()
