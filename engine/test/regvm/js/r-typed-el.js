// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// Elements of typed arrays read and written in the register loop, where
// GETEL, GETEL3 and PUTEL take a typed array of numbers without the slow
// side (engine/regvm-ops-object.h): every class, ints, floats, -0, NaN and
// values out of the range of the element, the clamp of Uint8ClampedArray,
// indexes out of range and negative, the BigInt arrays and values that are
// not numbers (which keep the slow side), a buffer detached by transfer and
// a resizable one shrunk under the array, the T forms (an array in a temp)
// and a result into a local and into a temp.
(function () { "use strict";
var log = [];
var calls = { n: 0 };
var classes = [Int8Array, Uint8Array, Uint8ClampedArray, Int16Array, Uint16Array, Int32Array,
  Uint32Array, Float16Array, Float32Array, Float64Array, BigInt64Array, BigUint64Array];
var values = [0, 1, -1, 127, 128, 255, 256, -129, 65535, 65536, 2147483647, -2147483648,
  4294967295, 1.5, 2.5, -0.5, -0, NaN, Infinity, -Infinity, 1e300, 0.1, "7", true, null, undefined,
  { valueOf: function () { calls.n = calls.n + 1; return 9; } }, 5n, -1n, 18446744073709551615n];
var idx = [0, 3, 7, 8, -1, 1.5, "2", 4294967295];
function id(x) { return x; }
function get(a, i) { return a[i]; }
function getk(a) { return a[3]; }
function gett(a, i) { return id(a)[i]; }
function getl(a, i) { var v = a[i]; return v; }
function getsum(a) { var s = 0, i; for (i = 0; i < 8; i++) s = s + a[i]; return s; }
function inc(a, i) { a[i] += 1; return a[i]; }
function put(a, i, v) { a[i] = v; return a[i]; }
function putt(a, i, v) { id(a)[i] = v; return a[i]; }
function putk(a, v) { a[2] = v; return a[2]; }
function fill(a, v) { var i; for (i = 0; i < 8; i++) a[i] = v; return a.join(","); }
function guard(f, a, b, c) {
  try { var r = f(a, b, c); return typeof r === "bigint" ? r + "n" : Object.is(r, -0) ? "-0" : String(r); }
  catch (e) { return e.name + ":" + e.message; }
}
for (var c = 0; c < classes.length; c++) {
  var C = classes[c], big = C === BigInt64Array || C === BigUint64Array;
  var a = new C(8), i, j, row;
  for (i = 0; i < 8; i++) a[i] = big ? BigInt(i * 3) : i * 3;
  row = [];
  for (i = 0; i < idx.length; i++)
    row.push(guard(get, a, idx[i]) + "|" + guard(gett, a, idx[i]) + "|" + guard(getl, a, idx[i]));
  log.push(C.name + " get " + row.join(" ") + " k " + guard(getk, a) + " sum " + guard(getsum, a));
  row = [];
  for (j = 0; j < values.length; j++) {
    var b = new C(8);
    row.push(guard(put, b, 1, values[j]) + "|" + guard(putt, b, 2, values[j]) + "|" +
             guard(putk, b, values[j]) + "|" + guard(put, b, 9, values[j]) + "|" +
             guard(put, b, -1, values[j]) + "|" + guard(inc, b, 1));
  }
  log.push(C.name + " put " + row.join(" "));
  log.push(C.name + " fill " + guard(fill, new C(8), big ? 2n : 2.75));
  var buf = new ArrayBuffer(8 * C.BYTES_PER_ELEMENT), t = new C(buf);
  t[1] = big ? 4n : 4;
  buf.transfer();
  log.push(C.name + " detached " + guard(get, t, 1) + "|" + guard(put, t, 1, big ? 5n : 5) + "|" + guard(getsum, t));
  var rb = new ArrayBuffer(8 * C.BYTES_PER_ELEMENT, { maxByteLength: 16 * C.BYTES_PER_ELEMENT }), r = new C(rb);
  r[6] = big ? 6n : 6;
  rb.resize(4 * C.BYTES_PER_ELEMENT);
  log.push(C.name + " shrunk " + guard(get, r, 6) + "|" + guard(put, r, 6, big ? 7n : 7) + "|" + guard(get, r, 3) + "|" + r.length);
  rb.resize(12 * C.BYTES_PER_ELEMENT);
  log.push(C.name + " grown " + guard(get, r, 6) + "|" + guard(put, r, 10, big ? 8n : 8) + "|" + guard(get, r, 10) + "|" + r.length);
}
log.push("calls " + calls.n);
rvExpect(get, getk, gett, getl, getsum, inc, put, putt, putk, fill, id);
return log.join("\n");
})()
