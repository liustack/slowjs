// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// P2 of the register loop: get_array_el (and 2 and 3), put_array_el,
// define_array_el, object, define_field, to_propkey, get_var, put_var, in
// and instanceof, on fast arrays in and out of their range, holes, typed
// arrays, strings, getters and setters on prototypes, proxies, frozen
// arrays, keys that are strings of numbers, -0, objects whose toString runs
// JavaScript or throws, null and undefined, globals that do not exist and
// lexical globals read before they are set. Some throw with temps owned
// below the throwing instruction.
var gv = 1;
const gc0 = 2;
globalThis.gp = 3;
Object.defineProperty(globalThis, "gacc", { get: function () { return globalThis.gp * 2; }, configurable: true });
(function () { "use strict";
var log = [];
var calls = { n: 0 };
var proto = {};
Object.defineProperty(proto, "2", { get: function () { calls.n = calls.n + 1; return "got2"; },
  set: function (v) { calls.n = calls.n + 10; } });
Object.defineProperty(proto, "3", { get: function () { throw new RangeError("getter threw"); },
  set: function (v) { throw new RangeError("setter threw"); } });
function mk() { var o = Object.create(proto); o[0] = "a"; return o; }
function arr() { return [10, 20, 30]; }
var px = new Proxy([1, 2], {
  get: function (t, k) { calls.n = calls.n + 1; return typeof k === "string" ? "p" + k : "psym"; },
  set: function (t, k, v) { calls.n = calls.n + 1; return k !== "9"; },
  has: function (t, k) { calls.n = calls.n + 1; if (k === "x") throw new Error("has threw"); return k === "0"; } });
var keys = [0, 1, 2, 3, 5, -1, -0, 1.5, 4294967295, "0", "1", "2", "length", "x", null, undefined,
  { toString: function () { calls.n = calls.n + 1; return "1"; } },
  { toString: function () { throw new TypeError("toString threw"); } }, Symbol.iterator, 10n];
function objs() {
  var holes = [1, , 3];
  return [arr(), holes, new Uint8Array([7, 8, 9]), new Float64Array(2), "abc", mk(), px,
    Object.freeze([1, 2]), { 0: "z", 1: "y" }, null, undefined, 5, true];
}
function id(x) { return x; }
function get(o, k) { return o[k]; }
function getk(o) { return o[1]; }
function gett(o, k) { return id(o)[k]; }
function gettk(o) { return id(o)[2]; }
function getkt(o, k) { return o[id(k)]; }
function getown(o, k) { return id(k) + o[k]; }
function call2(o, k) { return o[k](k); }
function put(o, k, v) { o[k] = v; return o[k]; }
function putt(o, k, v) { id(o)[id(k)] = id(v); return o[k]; }
function putown(o, k, v) { var r = id(v); o[k] = v; return r + o[k]; }
function upd(o, k) { o[k] += 1; return o[k]; }
function upd2(o, k) { o[k] = o[k] * 2; return o[k]; }
function push(o, n) { var i = 0; while (i < n) { o[o.length] = i; i = i + 1; } return o.length; }
function obj0() { return {}; }
function obj1(a, b) { var o = {}; o.x = a; o.y = b; return o; }
function comp(k, v) { return { [k]: v }; }
function has(k, o) { return k in o; }
function hast(k, o) { return id(k) + (id(k) in id(o)); }
function inst(o, c) { return o instanceof c; }
function instt(o, c) { return id(o) + (id(o) instanceof id(c)); }
function gget() { return gv + gc0 + gp + gacc; }
function gmiss() { return missingGlobal; }
function gtypeof() { return typeof missingGlobal; }
function gtdz() { return lateLet; }
function gset(v) { gv = v; gp = v; return gv + gp; }
function gsetc(v) { gc0 = v; }
function gsetmiss(v) { missingGlobal2 = v; }
function gsettdz(v) { lateLet = v; }
function show(v) {
  if (typeof v === "symbol") return "sym";
  if (typeof v === "bigint") return v + "n";
  if (Object.is(v, -0)) return "-0";
  if (v === px) return "proxy";
  if (typeof v === "object" && v !== null) {
    try { return "{" + Object.keys(v).map(function (k) { return k + ":" + show(v[k]); }).join(",") + "}"; }
    catch (e) { return "{?}"; }
  }
  try { return typeof v + ":" + String(v); } catch (e) { return typeof v + ":?"; }
}
function guard(f, a, b, c) {
  try { return show(f(a, b, c)); } catch (e) { return e.name + ":" + e.message; }
}
var fs1 = [get, gett, getkt, getown, call2, put, putt, putown, upd, upd2, has, hast, comp];
for (var i = 0; i < 13; i++) {
  var row = [], os = objs();
  for (var j = 0; j < keys.length; j++) {
    var cell = [];
    for (var f = 0; f < fs1.length; f++) {
      os = objs();
      cell.push(guard(fs1[f], fs1[f] === has || fs1[f] === hast ? keys[j] : os[i],
        fs1[f] === has || fs1[f] === hast ? os[i] : keys[j], keys[(j + 3) % keys.length]));
    }
    row.push(cell.join("|"));
  }
  os = objs();
  row.push(guard(getk, os[i]), guard(gettk, os[i]));
  log.push(i + " => " + row.join(" "));
}
log.push(push([], 20) + " " + push([1, 2], 3) + " " + guard(push, Object.freeze([]), 1));
for (var twice = 0; twice < 2; twice++)
  log.push([guard(obj0), guard(obj1, 1, "s"), guard(obj1, undefined, null)].join(" "));
function C() {}
class F { x = id(1); y = get([1, 2], 1); [id("z")] = 3; }
function fields() { var a = new F(); return a.x + a.y + a.z; }
log.push(guard(fields) + guard(fields) + guard(fields));
var bound = C.bind(null);
var hi = { [Symbol.hasInstance]: function (v) { calls.n = calls.n + 1; return v === 1; } };
var hit = { [Symbol.hasInstance]: function (v) { throw new Error("hasInstance threw"); } };
var cs = [C, Object, Array, bound, hi, hit, {}, null, 5];
var vs = [new C(), {}, [], 1, null, Object.create(null)];
for (var i = 0; i < cs.length; i++) {
  var row = [];
  for (var j = 0; j < vs.length; j++)
    row.push(guard(inst, vs[j], cs[i]) + "|" + guard(instt, vs[j], cs[i]));
  log.push("inst " + i + " " + row.join(" "));
}
for (twice = 0; twice < 2; twice++)
  log.push([guard(gget), guard(gmiss), guard(gtypeof), guard(gtdz), guard(gset, 5), guard(gget),
  guard(gsetc, 1), guard(gsetmiss, 1), guard(gsettdz, 1), guard(gset, "s")].join(" "));
log.push("calls " + calls.n);
rvExpect(call2, comp, get, getk, getkt, getown, gett, gettk, gget, gmiss, gset, gsetc, gsetmiss, gsettdz, gtdz, gtypeof, has, hast, id, inst, instt, mk, obj0, obj1, push, put, putown, putt, upd, upd2);
rvExpect(fields, arr, objs);
return log.join("\n");
})()
let lateLet = 4;
