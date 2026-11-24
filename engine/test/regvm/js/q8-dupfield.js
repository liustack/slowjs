// Copyright (c) 2026 The liustack/beastjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.
// rvdiff: a dup of a temp then get_field or get_length, as a destructuring
// of a call's result makes it: the fields in order, getters and proxies
// that see each read, an exception in the middle with the fields read before
// it kept, the length of strings and arrays, and null or undefined.
(function () { "use strict";
var log = [];
function src() { return { x: 1, y: "s", a: { x: [1, 2, 3] } }; }
function two() { var { x: p, y: q } = src(); return p + q; }
function nested() { var { a: { x: { length: n } }, y: w } = src(); return n + w; }
function defaults() { var { x: p = 5, z: r = 6 } = src(); return p + r; }
function strlen(s) { var { length: n } = s + "!"; return n; }
function arrlen(n) { var { length: m, 0: first } = new Array(n).fill(n); return m + ":" + first; }
function getters(o) { var a, b, c; ({ p: a, q: b, r: c } = o()); return [a, b, c].join(); }
function partial(o) { var p, q; try { ({ p: p, q: q } = o()); } catch (e) { return e.message + "/" + p + "/" + q; } return "none"; }
function rest() { var { x: p, ...others } = src(); return p + JSON.stringify(others); }
function inloop(n) { var s = 0, i; for (i = 0; i < n; i++) { var { x: p, a: { x: arr } } = src(); s += p + arr.length; } return s; }
function counted() { var o = { v: { k: "kept" } }, r = [], i; for (i = 0; i < 3; i++) { var { v: w } = (function () { return o; })(); r.push(w.k); } return r.join(); }
function mk() {
    return {
        get p() { log.push("p"); return 1; },
        get q() { log.push("q"); return 2; },
        get r() { log.push("r"); return 3; },
    };
}
function prox() {
    return new Proxy({ p: "P", q: "Q", r: "R" }, {
        get: function (t, k, rcv) { log.push("get " + String(k)); return t[k]; },
    });
}
function thrower() {
    return {
        get p() { return "first"; },
        get q() { throw new Error("q threw"); },
    };
}
var out = [], round;
for (round = 0; round < 3; round++) {
    log = [];
    out.push(two(), nested(), defaults(), strlen("abc"), strlen(""), arrlen(2), arrlen(0));
    out.push(getters(mk), log.join(), getters(prox), log.join());
    out.push(partial(thrower), rest(), inloop(4), counted());
    try { (function () { var { x: p } = null; return p; })(); } catch (e) { out.push(e.name + ": " + e.message); }
    try { (function () { var { x: p } = (function () {})(); return p; })(); } catch (e) { out.push(e.name + ": " + e.message); }
    try { (function () { var { x: { y: q } } = { x: undefined }; return q; })(); } catch (e) { out.push(e.name + ": " + e.message + " " + String(e.stack).split("\n")[1]); }
}
rvExpect(two, nested, defaults, strlen, arrlen, getters, partial, rest, inloop, counted);
return out.join(";");
})()
