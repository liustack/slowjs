// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// rvdiff: calls between converted functions and out of the register loop:
// missing and extra arguments, methods, this, tail calls, getters and setters
// that call back, callbacks of C functions, call and apply, bound functions,
// calls of what is not a function, and a proxy.
(function () { "use strict";
var out = [];
function p(x) { out.push(String(x)); }
function two(a, b) { return a + "," + b; }
function none() { return "none"; }
function ret(a) { return a; }
function self() { return this; }
var o = { k: 3, m: function (x) { return x + this.k; }, n: function () { return this.m(1) + this.m(2); } };
p(two(1));
p(two());
p(two(1, 2, 3, 4, 5));
p(none(1, 2, 3, 4, 5, 6, 7, 8, 9));
p(ret(ret(ret(5))));
p(self());
p(o.m(4) + o.n());
p(self.call(o) === o);
p(self.apply(7) === 7);
p(two.apply(null, [8, 9, 10]));
var b = two.bind(null, "B");
p(b("c") + " " + b());
// tail calls
function tc(n, acc) { if (n < 1) return acc; return tc(n - 1, acc + n); }
p(tc(1000, 0));
function tm(n) { if (n < 1) return o.k; return o.m(n); }
p(tm(0) + " " + tm(5));
// getters and setters that call back into converted code
var g = { get v() { return ret(41) + 1; }, set v(x) { this.w = two(x, x); } };
function readv(q) { return q.v; }
function writev(q, x) { q.v = x; return q.w; }
p(readv(g) + " " + writev(g, 5));
// callbacks of C functions
function dbl(x) { return x + x; }
function cmp(a, b) { return a - b; }
p([3, 1, 2].map(dbl).join());
p([5, 3, 9, 1].sort(cmp).join());
p([1, 2, 3, 4].reduce(function (a, x) { return a + x; }, 0));
var acc = { s: 0 };
[1, 2, 3].forEach(function (x) { acc.s = acc.s + x; });
p(acc.s);
// calls of what is not a function
function callit(f) { return f(); }
function callm(q) { return q.nope(); }
try { callit(5); } catch (e) { p(e.name + ": " + e.message); }
try { callit(undefined); } catch (e) { p(e.name + ": " + e.message); }
try { callm({}); } catch (e) { p(e.name + ": " + e.message); }
try { callm(null); } catch (e) { p(e.name + ": " + e.message); }
// a proxy as a function and as an object
var pf = new Proxy(dbl, { apply: function (t, th, args) { return t(args[0]) + 1; } });
p(callit(function () { return pf(4); }));
var po = new Proxy({}, { get: function (t, k) { return k === "k" ? 9 : undefined; } });
function getk(q) { return q.k; }
p(getk(po) + " " + getk({ k: 1 }) + " " + getk("str"));
// a method of a primitive
function len(s) { return s.length; }
function upper(s) { return s.toUpperCase(); }
p(len("hello") + " " + len([1, 2]) + " " + len({ length: "x" }) + " " + upper("abc"));
// many arguments
function many(a, b, c, d, e, f, g, h) { return a + b + c + d + e + f + g + h; }
p(many(1, 2, 3, 4, 5, 6, 7, 8) + " " + many(1, 2, 3) + " " + many(1, 2, 3, 4, 5, 6, 7, 8, 9, 10));
// a function called first from C, then from JavaScript, then from C
function later(x) { return x - 1; }
p([later(3), [3].map(later)[0], later.call(null, 3)].join());
return out.join("\n");
})()
