// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// Elements read by a key that is a string, as for in gives them, where
// JS_GetPropertyValue takes the element of a fast array or a typed array
// without making an atom of the string (engine/quickjs.c): keys that are
// indexes and keys that only look like them ("00", "-0", "1.0", "+1", " 1",
// "4294967295", "4294967296"), holes and elements of the prototype, getters,
// arrays that are not fast, typed arrays, a detached and a shrunk buffer,
// arguments, strings, proxies, and for in over arrays changed in the loop.
(function () { "use strict";
var log = [];
var keys = ["0", "1", "2", "3", "7", "9", "00", "01", "-0", "-1", "1.0", "1.5", "+1", " 1", "1 ",
  "1e0", "4294967294", "4294967295", "4294967296", "99999999999", "length", "", "١"];
function id(x) { return x; }
function get(a, k) { return a[k]; }
function gett(a, k) { return id(a)[k]; }
function show(v) {
  if (typeof v === "string") return JSON.stringify(v);
  if (typeof v === "object" && v !== null) return "obj";
  return Object.is(v, -0) ? "-0" : String(v);
}
function guard(f, a, k) {
  try { return show(f(a, k)); } catch (e) { return e.name; }
}
function row(name, a) {
  var out = [], i;
  for (i = 0; i < keys.length; i++) out.push(keys[i] + "=" + guard(get, a, keys[i]) + "/" + guard(gett, a, keys[i]));
  log.push(name + " " + out.join(" "));
}
function sumIn(a) {
  var s = [], k;
  for (k in a) s.push(k + ":" + show(a[k]));
  return s.join(",");
}
var arr = [10, 11, 12, 13, 14, 15, 16, 17];
row("fast", arr);
var holes = [10, , 12, , 14];
row("holes", holes);
Array.prototype[3] = "proto3";
row("proto", holes);
log.push("in proto " + sumIn(holes));
delete Array.prototype[3];
var slow = [10, 11, 12];
slow.x = 1;
Object.defineProperty(slow, "1", { get: function () { return "getter"; } });
row("slow", slow);
var big = [];
big[4294967294] = "last";
row("sparse", big);
var classes = [Int8Array, Uint8Array, Uint8ClampedArray, Int16Array, Uint16Array, Int32Array,
  Uint32Array, Float16Array, Float32Array, Float64Array, BigInt64Array, BigUint64Array];
var c, t;
for (c = 0; c < classes.length; c++) {
  var C = classes[c], isBig = C === BigInt64Array || C === BigUint64Array;
  t = new C(8);
  t[1] = isBig ? -5n : -5;
  t[2] = isBig ? 7n : 2.5;
  row(C.name, t);
  log.push(C.name + " in " + sumIn(t));
}
var buf = new ArrayBuffer(8), d = new Uint8Array(buf);
d[1] = 9;
buf.transfer();
row("detached", d);
var rb = new ArrayBuffer(8, { maxByteLength: 16 }), r = new Uint8Array(rb);
r[6] = 6;
rb.resize(4);
row("shrunk", r);
rb.resize(12);
row("grown", r);
row("args", (function () { return arguments; })(20, 21, 22));
row("string", "abcdefgh");
row("proxy", new Proxy([30, 31, 32], { get: function (o, k) { return "p" + String(k) + ":" + o[k]; } }));
var obj = { 0: "a", 1: "b", "00": "c", "-0": "d", "4294967295": "e" };
row("object", obj);
var grow = [1, 2, 3];
log.push("grow " + (function () {
  var s = [], k;
  for (k in grow) { s.push(k + ":" + grow[k]); if (grow.length < 6) grow.push(grow.length + 1); }
  return s.join(",");
})());
var shrink = [1, 2, 3, 4, 5];
log.push("shrink " + (function () {
  var s = [], k;
  for (k in shrink) { s.push(k + ":" + show(shrink[k])); shrink.length = shrink.length - 1; }
  return s.join(",");
})());
var del = [1, 2, 3, 4];
log.push("delete " + (function () {
  var s = [], k;
  for (k in del) { s.push(k + ":" + show(del[k])); delete del[3]; }
  return s.join(",");
})());
log.push("mixed " + sumIn(Object.assign([5, 6], { a: 7 })));
rvExpect(get, gett, sumIn, id);
return log.join("\n");
})()
