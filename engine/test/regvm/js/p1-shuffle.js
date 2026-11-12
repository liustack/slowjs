// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// rvdiff: the stack ops (dup, nip, swap, insert2, insert3, perm3, rot3r)
// as the compiler makes them for assignments used as values, compound
// assignments, optional chains and logical assignments, on locals, temps
// and constants, with values that have a reference count.
(function () { "use strict";
function put(o, v) { return o.x = v; }
function put2(o, v, w) { return (o.a = v).b = w; }
function chain(o, v) { o.a = o.b = o.c = v; return o.a; }
function addto(o) { o.n += 1; return o.n; }
function addto2(o, s) { return o.s += s; }
function opt(o) { return o?.a?.b; }
function optcall(o) { return o?.f?.(); }
function nul(a, b) { return a ?? b; }
function orset(o, v) { o.x ||= v; return o.x; }
function andset(o, v) { o.x &&= v; return o.x; }
function nulset(o, v) { return o.x ??= v; }
function seqs(a, b) { var x, y; x = y = a; y = b; return x + "," + y; }
function tmp(f) { return f() || f(); }
function tmp2(f, g) { return f() && g(); }
function tmp3(f) { var r = f(); return r !== null && r.x; }
var out = [];
function o1() { return { x: 1, a: { b: 2 }, n: 1, s: "s", f: function () { return "called"; } }; }
for (var round = 0; round < 2; round++) {
var o = o1();
out.push(put(o, "str"), o.x, put2(o, {}, 5).toString(), JSON.stringify(o));
out.push(chain(o1(), "c"), addto(o1()), addto2(o1(), "t"));
out.push(String(opt(null)), String(opt({})), String(opt({ a: null })), opt(o1()));
out.push(String(optcall(null)), String(optcall({})), optcall(o1()));
out.push(nul(null, 1), nul(undefined, "u"), nul(0, 2), nul("", 3));
out.push(orset({ x: 0 }, "v"), orset({ x: "k" }, "v"), andset({ x: 0 }, "v"), andset({ x: 1 }, "w"));
out.push(nulset({ x: null }, 9), nulset({ x: false }, 9), nulset({}, "z"));
out.push(seqs("p", "q"), seqs(1, 2));
}
var n = 0;
function cnt() { n = n + 1; return n > 1 ? "second" : ""; }
for (round = 0; round < 2; round++) {
out.push(tmp(cnt), n, tmp2(function () { return "a"; }, function () { return "b"; }));
out.push(String(tmp3(function () { return null; })), tmp3(function () { return { x: "x" }; }));
}
try { put(null, 1); } catch (e) { out.push(e.name); }
try { addto(undefined); } catch (e) { out.push(e.name); }
rvExpect(addto, addto2, andset, chain, nul, nulset, opt, optcall, orset, put, put2, seqs, tmp, tmp2, tmp3);
return out.join(";");
})()
