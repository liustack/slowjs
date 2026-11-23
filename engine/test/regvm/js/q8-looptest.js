// Copyright (c) 2026 The liustack/beastjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.
// rvdiff: the test of a for or while loop read again at the end of the loop:
// tests on locals, arguments, closure variables, constants and fields, the
// compares and strict compares, tests that run getters each turn and one
// that throws on a later turn (its line), loops with continue and break, a
// let per turn that closures keep, values that are not numbers, and a loop
// that never runs its body.
(function () { "use strict";
var log = [];
var width = 4;
function sum(n) { var s = 0, i; for (i = 0; i < n; i++) s += i; return s; }
function sumle(n) { var s = 0; for (var i = 1; i <= width; i++) s += i * n; return s; }
function down(n) { var s = ""; while (n > 0) { s += n; n--; } return s; }
function downge(n) { var s = ""; for (; n >= 0; n -= 2) s += n; return s; }
function neq(a) { var i = 0, s = 0; while (i !== a.length) s += a[i++]; return s; }
function eqk(n) { var i = n; while (i === 5) i++; return i; }
function len(a) { var s = 0, i; for (i = 0; i < a.length; i++) s += a[i]; return s; }
function fld(o) { var s = 0, i; for (i = 0; i < o.n; i++) s += i; return s + "|" + log.join(); }
function thrower(o) { var i; for (i = 0; i < o.n; i++) {} return i; }
function cont(n) { var s = "", i; for (i = 0; i < n; i++) { if (i % 2) continue; if (i > 6) break; s += i; } return s; }
function lets(n) { var f = []; for (let i = 0; i < n; i++) f.push(function () { return i; }); return f.map(function (g) { return g(); }).join(); }
function strs(a, b) { var s = "", x = a; while (x < b) { s += x; x += "z"; } return s; }
function never(n) { var s = "none"; for (var i = 0; i < n; i++) s = "ran"; return s; }
function nested(n) { var s = 0, i, j; for (i = 0; i < n; i++) for (j = 0; j < i; j++) s += j; return s; }
var getn = { i: 0, get n() { log.push("n" + this.i); this.i++; return 3; } };
var out = [], round, g2;
for (round = 0; round < 3; round++) {
    out.push(sum(5), sum(0), sum(-1), sumle(2), down(3), downge(5), neq([1, 2, 3]), eqk(5), eqk(4));
    out.push(len([1, 2, 3, 4]), len([]), cont(12), lets(3), strs("a", "azz"), never(0), never(1), nested(4));
    log = [];
    getn.i = 0;
    out.push(fld(getn));
    g2 = { k: 0, get n() { if (++this.k === 3) throw new Error("third test"); return 9; } };
    try { thrower(g2); } catch (e) { out.push(e.message + " " + String(e.stack).split("\n").slice(1, 3).join(" ")); }
    try { thrower(null); } catch (e) { out.push(e.name + " " + String(e.stack).split("\n")[1]); }
    out.push(sum("3"), down("2"), len("abc"));
}
rvExpect(sum, sumle, down, downge, neq, eqk, len, fld, thrower, cont, lets, strs, never, nested);
return out.join(";");
})()
