// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// rvdiff: exceptions thrown at different depths inside converted functions
// and caught by a function the pass refuses (try and catch), errors of the
// engine (a field of undefined, a call of a non function, a write to a frozen
// object), and the values left when a throw passes through frames with owned
// temps.
(function () { "use strict";
var out = [];
function p(x) { out.push(String(x)); }
function boom(x) { throw new Error("boom " + x); }
function d1(x) { return boom(x) + 1; }
function d2(x) { return d1(x + 1) + d1(x + 2); }
function d3(x) { var o = { a: x }; return o.a + d2(x); }
function field(q) { return q.x.y; }
function field2(q) { return field(q) + field(q); }
function frozen(q) { q.v = 1; return q; }
function obj(n) { return { n: n, s: "x" + n }; }
function many(n) { var a = obj(n), b = obj(n + 1), c = obj(n + 2); return a.s + b.s + c.s + field(n); }
function catcher(f, x) {
  try { return "ok " + f(x); } catch (e) { return e.name + ": " + e.message; }
}
for (var i = 0; i < 5; i++) {
  p(catcher(d1, i));
  p(catcher(d2, i));
  p(catcher(d3, i));
}
p(catcher(field, undefined));
p(catcher(field, null));
p(catcher(field, {}));
p(catcher(field, { x: { y: 5 } }));
p(catcher(field2, { x: null }));
p(catcher(frozen, Object.freeze({})));
p(catcher(frozen, {}).v);
p(catcher(many, 1));
// a throw from a getter and from valueOf inside an add
var g = { get x() { throw new TypeError("getter"); } };
function addv(a, b) { return a + b; }
p(catcher(field, g));
p(catcher(function (x) { return addv(x, { valueOf: function () { return boom("v"); } }); }, 1));
// a throw caught, then the same functions again
p(catcher(d3, 10) + " " + d3.length);
// the error object and its stack frames' names, without line numbers
function names(f, x) {
  try { f(x); } catch (e) { return String(e.stack).split("\n").map(function (l) { return l.replace(/\(.*\)/, ""); }).join("/"); }
  return "no throw";
}
p(names(d3, 1));
gc();
return out.join("\n");
})()
