// Copyright (c) 2026 The liustack/tifajs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.
// rvdiff: a chained assignment into a local, l = x[k] = v and l = o.f = v,
// with put_loc and set_loc after it: the local and the element get the same
// value, a setter or a proxy sees the value it is given, the local named in
// the target itself (l = l[k] = v), a throw of the put with a catch around
// (the local keeps its old value) and without one, values with a reference
// count, and a local that a closure reads (kept in the old order).
(function () { "use strict";
var log = [];
function el(x, k, v) { var l; l = x[k] = v; return l + "|" + x.join(); }
function elset(x, k, v) { var l; return (l = x[k] = v) + "|" + l + "|" + x.join(); }
function fld(o, v) { var l = 0; l = o.f = v; return l + "|" + o.f; }
function fldset(o, v) { var l; var r = (l = o.f = v); return r + "|" + l; }
function self(x) { var l = x; l = l[0] = [9]; return JSON.stringify(l) + "|" + JSON.stringify(x); }
function selfkey(k) { var x = [0, 1, 2]; k = x[k] = k + 1; return k + "|" + x.join(); }
function sum(x, n) { var i, last = 0, s = 0; for (i = 0; i < n; i++) { last = x[i] = (last + i) * 2; s += last; } return s + "|" + x.join(); }
function caught(x, v) { var l = "old"; try { l = x[0] = v; } catch (e) { return e.name + "|" + l; } return "none|" + l; }
function caughtf(o, v) { var l = "old"; try { l = o.f = v; } catch (e) { return e.name + "|" + l; } return "none|" + l; }
function uncaught(x, v) { var l = "old"; l = x[0] = v; return l; }
function captured(x, v) { var l = "old", g = function () { return l; }; try { l = x[0] = v; } catch (e) { return e.name + "|" + g(); } return g(); }
function objs(n) { var a = [], o, l, i; for (i = 0; i < n; i++) { o = {}; l = o.v = { i: i }; a.push(l === o.v); } return a.join(); }
var setter = {
    set f(v) { log.push("set f=" + v); },
    get f() { return "got"; },
};
var prox = new Proxy([], {
    set: function (t, k, v) { log.push("set " + String(k) + "=" + v); t[k] = v; return true; },
});
var frozen = Object.freeze([1]);
var ro = Object.freeze({ f: 1 });
var out = [], round;
for (round = 0; round < 3; round++) {
    out.push(el([1, 2], 1, "v"), el([], 2, 3n), elset([0], 0, { x: 1 }.x), fld({}, "s"), fldset({}, [1, 2]));
    out.push(self([1]), selfkey(1), sum([0, 0, 0, 0], 4));
    log = [];
    out.push(fld(setter, "w"), el(prox, 0, "p"), log.join());
    out.push(caught(frozen, 2), caughtf(ro, 2), caught(null, 2), caught([5], 6));
    try { uncaught(frozen, 2); } catch (e) { out.push("uncaught " + e.name); }
    out.push(captured(frozen, 2), captured([1], 3), objs(3));
}
rvExpect(el, elset, fld, fldset, self, selfkey, sum, caught, caughtf, uncaught, captured, objs);
return out.join(";");
})()
