// Copyright (c) 2026 The liustack/beastjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.
// rvdiff: a[i] and a[i] = v with an int key (GETEL, GETELT, GETELK,
// GETELKT, GETEL3 and PUTEL, sloppy and strict) on Arrays that are no
// longer fast arrays, and past the count of fast ones, against Reflect.get
// and Reflect.set on a twin made alike, which do not run in the register
// loop. Arrays: filled from the top down, with holes, an element deleted,
// own getters and setters, a read-only element, frozen, sealed and
// non-extensible, a length that cannot be written, a subclass. Prototypes:
// Array.prototype and Object.prototype with an index (a value, a getter and
// setter, read-only), a Proxy, an Int8Array, a String object, arguments, a
// fast array with elements, an Array of another realm, null. Keys from 0 to
// past the length, 2^31 - 1, and below 0. A getter, setter or trap writes
// to a log with its receiver, so one run twice, not at all or on another
// object shows. Also: the old value of an element is an object (freed when
// replaced), the array itself, and the result written to the slot of the
// key or of the array.
(function () {
var out = [];
// strings, not arrays: a setter on Array.prototype or Object.prototype would
// take their elements
var log = "";
function id(x) { return x; }
function show(v) {
  if (v === undefined) return "u";
  if (Array.isArray(v)) return "arr" + v.length;
  if (v !== null && typeof v === "object") return "obj";
  return typeof v === "string" ? v : String(v);
}
function guard(f) {
  try { return show(f()); } catch (e) { return e.name; }
}
function rd(a, i) { return a[i]; }
function rdt(a, i) { return id(a)[i]; }
function rdk(a) { return a[3]; }
function rdkt(a) { return id(a)[3]; }
function rdmax(a) { return a[2147483647]; }
function wr(a, i, v) { a[i] = v; }
function wrs(a, i, v) { "use strict"; a[i] = v; }
function add(a, i) { a[i] += 1; return a[i]; }
function selfKey(a, i) { i = a[i]; i = a[i]; return i; }
function selfObj(a, i) { a = a[i]; a = a[i]; return a; }

function down(n) { var a = new Array(); for (var i = n - 1; i >= 0; i--) a[i] = i; return a; }
var makers = {
  down: function () { return fin(down(5)); },
  holes: function () { var a = []; a[4] = 4; a[2] = 2; return fin(a); },
  deleted: function () { var a = [0, 1, 2, 3, 4]; delete a[1]; return fin(a); },
  fastShort: function () { var a = [0, 1]; a.length = 5; return fin(a); },
  getter: function () {
    var a = down(5);
    Object.defineProperty(a, 1, { get: function () { log += "get1 " + (this === a) + ";"; return "g1"; },
      set: function (v) { log += "set1 " + v + " " + (this === a) + ";"; }, configurable: true });
    return fin(a);
  },
  readOnly: function () { var a = down(5); Object.defineProperty(a, 2, { value: "ro", writable: false, enumerable: true }); return fin(a); },
  frozen: function () { return Object.freeze(fin(down(5))); },
  sealed: function () { return Object.seal(fin(down(5))); },
  prevented: function () { return Object.preventExtensions(fin(down(5))); },
  fixedLength: function () { return Object.defineProperty(fin(down(5)), "length", { writable: false }); },
  sub: function () { class Sub extends Array {} var a = new Sub(); a[4] = 4; a[0] = 0; return fin(a); },
};
var protos = {
  none: undefined,
  proxy: function () { return new Proxy([], { get: function (o, k, r) { log += "trap get " + String(k) + " " + Array.isArray(r) + ";"; return "P" + String(k); },
    set: function (o, k, v, r) { log += "trap set " + String(k) + " " + v + ";"; return true; } }); },
  int8: function () { return new Int8Array([7, 8, 9, 10]); },
  string: function () { return new String("wxyz"); },
  args: function () { return (function () { return arguments; })("a0", "a1", "a2", "a3"); },
  fast: function () { return [100, 101, 102, 103]; },
  realm: function () { return newRealm("var p = []; p[3] = 'r3'; Array.prototype[1] = 'rp1'; p"); },
  nul: function () { return null; },
};
var keys = [0, 1, 2, 3, 4, 5, 9, 2147483647, -1];
function snapshot(a) {
  var s = "";
  var names = Object.getOwnPropertyNames(a);
  for (var i = 0; i < names.length; i++) {
    var d = Object.getOwnPropertyDescriptor(a, names[i]);
    s += " " + names[i] + (d.get || d.set ? "=acc" : "=" + show(d.value) + (d.writable ? "w" : ""));
  }
  return s;
}
// the prototype of the next array a maker makes, set before it is frozen, sealed or made non-extensible
var nextProto;
function fin(a) { if (nextProto !== undefined) Object.setPrototypeOf(a, nextProto); return a; }
function make(mk, pk) {
  nextProto = protos[pk] ? protos[pk]() : undefined;
  try { return mk(); } finally { nextProto = undefined; }
}
function check(mn, pk, k) {
  var a = make(makers[mn], pk), b = make(makers[mn], pk), n0, n1, n2;
  n0 = log.length;
  var r1 = guard(function () { return rd(a, k); });
  n1 = log.length;
  var r2 = guard(function () { return Reflect.get(b, k); });
  n2 = log.length;
  var r3 = guard(function () { return rdt(a, k); });
  var ok = r1 === r2 && r1 === r3 && n1 - n0 === n2 - n1;
  n0 = log.length;
  var w1 = guard(function () { wr(a, k, "v"); return a[k]; });
  n1 = log.length;
  var w2 = guard(function () { Reflect.set(b, k, "v"); return b[k]; });
  n2 = log.length;
  ok = ok && w1 === w2 && n1 - n0 === n2 - n1 && snapshot(a) === snapshot(b);
  var s1 = guard(function () { wrs(a, k, "s"); return a.length; });
  var s2 = guard(function () { if (!Reflect.set(b, k, "s")) throw new TypeError(); return b.length; });
  ok = ok && s1 === s2 && snapshot(a) === snapshot(b);
  out.push(mn + " " + pk + " [" + k + "] " + r1 + " " + w1 + " " + s1 + (ok ? "" : " MISMATCH " + [r2, r3, w2, s2, snapshot(a), snapshot(b)].join(" | ")));
}
for (var mn in makers)
  for (var pk in protos)
    for (var j = 0; j < keys.length; j++)
      check(mn, pk, keys[j]);
// the constant keys and GETEL3
for (var mn in makers) {
  var a = makers[mn]();
  out.push(mn + " k " + [guard(function () { return rdk(a); }), guard(function () { return rdkt(a); }), guard(function () { return rdmax(a); }),
    guard(function () { return add(a, 0); }), guard(function () { return add(a, 1); }), guard(function () { return add(a, 3); }), guard(function () { return add(a, 7); })].join(","));
}
// indexed properties on the prototypes of the realm
function onProtos(what, f) {
  try {
    if (what === "value") Array.prototype[3] = "ap3";
    if (what === "object") Object.prototype[3] = "op3";
    if (what === "accessor") Object.defineProperty(Array.prototype, 3, { get: function () { log += "pget " + this.length + ";"; return "pg3"; },
      set: function (v) { log += "pset " + v + " " + this.length + ";"; }, configurable: true });
    if (what === "readOnly") Object.defineProperty(Object.prototype, 3, { value: "ro3", writable: false, configurable: true });
    return f();
  } finally {
    delete Array.prototype[3];
    delete Object.prototype[3];
  }
}
["value", "object", "accessor", "readOnly"].forEach(function (what) {
  onProtos(what, function () {
    check("holes", "none", 3);
    check("fastShort", "none", 3);
    check("down", "none", 3);
    check("deleted", "none", 3);
    check("sub", "none", 3);
  });
});
// values with references, the array in itself, the result in the slot of the key or the array
var a = down(6), o = { n: 1 };
wr(a, 1, o); wr(a, 2, a); wr(a, 1, "flat"); wr(a, 3, o); wr(a, 3, o);
var chain = down(3); chain[2] = down(4); chain[2][3] = 1; chain[1] = 2;
out.push("refs " + [rd(a, 1), rd(a, 2) === a, rd(a, 3) === o, selfKey(down(3), 2), show(selfObj(chain, 2))].join(","));
// a long loop, as crypto.js runs one
var b = new Array(), c = down(64), s = 0;
for (var i = 63; i >= 0; i--) b[i] = 0;
for (var r = 0; r < 4; r++) for (var i = 0; i < 64; i++) { b[i] = (b[i] + c[i] * 3 + r) & 0xffff; s += b[i]; }
out.push("loop " + s + " " + b[63]);
// a key below 0 is a name, never the element 2^31 - 1 (the bits of -1 as an
// index), on an array that has that element
var top = []; top[2147483647] = "top"; top[1] = 1;
wr(top, -1, "neg"); wrs(top, -2, "neg2");
out.push("neg " + [rd(top, -1), rd(top, -2), rd(top, 2147483647), rdmax(top), top.length, snapshot(top)].join(","));
// GETEL3 on values that are not objects
out.push("prim " + [guard(function () { return add("abc", 1); }), guard(function () { return add(5, 0); }),
  guard(function () { return add(true, 0); }), guard(function () { return add(null, 0); }),
  guard(function () { return add(undefined, 0); }), guard(function () { return add(Symbol("s"), 0); })].join(","));
out.push(log);
rvExpect(rd, rdt, rdk, rdkt, rdmax, wr, wrs, add, selfKey, selfObj);
return out.join("\n");
})()
