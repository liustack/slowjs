// Copyright (c) 2026 The liustack/tifajs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.
// rvdiff: ++k and --k of a local whose value is still read on the stack
// below them (x[k] = x[++k], k + ++k): the reads below get the old value,
// the element and the sum see the new one, with numbers, strings, BigInts,
// objects with valueOf, a valueOf that throws (k and the array unchanged),
// and getters and setters that see each step.
(function () { "use strict";
var log = [];
function shift(x, k) { x[k] = x[++k]; x[k] = x[++k]; return x.join() + "|" + k; }
function back(x, k) { x[k] = x[--k]; return x.join() + "|" + k; }
function sum(k) { var s = k + ++k; return s + "|" + k; }
function sum2(k) { var s = k - --k + k; return s + "|" + k; }
function loop(x) { var k = 0, n = x.length - 1; while (k < n) x[k] = x[++k] * 2; return x.join() + "|" + k; }
function thrower(x, k) { try { x[k] = x[++k]; } catch (e) { return e.message + "|" + x.join() + "|" + String(k); } return "none"; }
function obs(k) {
    var o = {
        set 0(v) { log.push("set0=" + v); }, get 0() { log.push("get0"); return "a"; },
        set 1(v) { log.push("set1=" + v); }, get 1() { log.push("get1"); return "b"; },
    };
    o[k] = o[++k];
    return log.join() + "|" + k;
}
function prox(k) {
    var p = new Proxy({ 0: "p0", 1: "p1" }, {
        get: function (t, key) { log.push("get " + String(key)); return t[key]; },
        set: function (t, key, v) { log.push("set " + String(key) + "=" + v); t[key] = v; return true; },
    });
    p[k] = p[++k];
    return log.join() + "|" + k;
}
var out = [], round, vo;
for (round = 0; round < 3; round++) {
    out.push(shift([1, 2, 3, 4], 0), shift([1, 2, 3, 4], "1"), back([1, 2, 3], 2), back(["a", "b"], "1"));
    out.push(sum(1), sum("1"), sum(1n), sum2(5), sum2("x"), loop([1, 2, 3, 4, 5]));
    vo = { valueOf: function () { log.push("valueOf"); return 0; }, toString: function () { return "vo"; } };
    log = [];
    out.push(shift([7, 8, 9], vo), log.join());
    out.push(thrower([1, 2], { valueOf: function () { throw new Error("no number"); }, toString: function () { return "k"; } }));
    out.push(thrower([1, 2], Symbol("s")));
    log = [];
    out.push(obs(0));
    log = [];
    out.push(prox(0));
}
rvExpect(shift, back, sum, sum2, loop, thrower, obs, prox);
return out.join(";");
})()
