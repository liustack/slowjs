// Copyright (c) 2026 The liustack/beastjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.
// rvdiff: o[k] = v with a string key (PUTEL with the object, the key and the
// value in locals and temps, sloppy and strict, and o.x = v of PUTF) on every
// kind of receiver, each store made twice on two receivers made alike: once
// with the key as it is, an atom when it is a name, and once with the same key
// made again as a string that is not an atom, which cannot take the path of a
// key that is its own atom. The two must give the same result or exception,
// run the same setters and traps with the same receiver, and leave the
// receivers with the same own keys, in the same order, with the same values
// and attributes, the same prototype and extensibility. Keys: names new and
// present, names a prototype has as a writable or a read-only data property,
// a setter, a getter alone, "length", "__proto__", "prototype", "name",
// "stack", "constructor", "push", strings that look like numbers ("-0",
// "-1", "1e3", "1.5", "Infinity", "NaN", "0", "10") and the empty string.
// Receivers: plain objects (a literal, empty, a class instance, in dictionary
// mode, with many keys), null prototypes, prototypes with data, read-only
// data, setters and getters (one and three levels up), own setters and
// read-only keys, objects that are not extensible, sealed or frozen, arrays
// and objects whose prototype is one (also frozen), typed arrays and objects
// whose prototype is one, arguments (mapped and strict) and objects whose
// prototype is one, String objects and objects whose prototype is one,
// functions, classes, errors, proxies with and without a set trap and objects
// whose prototype is one, the global object, keys put on Object.prototype,
// Array.prototype and String.prototype, and primitives (strings, numbers,
// booleans, symbols, bigints, null and undefined). Also: adds copied in a
// loop as Object.assign makes them, a key whose only reference is the temp
// of the store, a value that is the object or the key, and a store that
// replaces the last reference to an object.
var paTop = 1;
(function () {
var out = [];
var log = [];
var cur = null;
function id(x) { return x; }
// a string of the same characters that is not an atom
function fresh(k) { return typeof k === "string" ? (" " + k).slice(1) : k; }
function wr(o, k, v) { o[k] = v; }
function wrs(o, k, v) { "use strict"; o[k] = v; }
function wrt(o, k, v) { id(o)[k] = v; }
function wrts(o, k, v) { "use strict"; id(o)[k] = v; }
function wrk(o, k, v) { o[id(k)] = v; }
function wrv(o, k, v) { o[k] = id(v); }
function wrc(o, k) { o[k] = "c"; }
// o.name = v (PUTF) for the names among the keys, o[k] = v for the others
function wf(o, k, v) {
  switch (k) {
  case "a": o.a = v; break; case "zz": o.zz = v; break; case "c": o.c = v; break;
  case "pd": o.pd = v; break; case "ro": o.ro = v; break; case "st": o.st = v; break;
  case "gt": o.gt = v; break; case "length": o.length = v; break; case "__proto__": o.__proto__ = v; break;
  case "prototype": o.prototype = v; break; case "name": o.name = v; break; case "stack": o.stack = v; break;
  case "constructor": o.constructor = v; break; case "push": o.push = v; break; case "callee": o.callee = v; break;
  case "toString": o.toString = v; break; case "zzOpSet": o.zzOpSet = v; break; case "zzOpRo": o.zzOpRo = v; break;
  case "zzOpData": o.zzOpData = v; break; case "zzApSet": o.zzApSet = v; break; case "zzSpSet": o.zzSpSet = v; break;
  case "zzSpRo": o.zzSpRo = v; break;
  default: o[k] = v;
  }
}
function wfs(o, k, v) {
  "use strict";
  switch (k) {
  case "a": o.a = v; break; case "zz": o.zz = v; break; case "c": o.c = v; break;
  case "pd": o.pd = v; break; case "ro": o.ro = v; break; case "st": o.st = v; break;
  case "gt": o.gt = v; break; case "length": o.length = v; break; case "__proto__": o.__proto__ = v; break;
  case "prototype": o.prototype = v; break; case "name": o.name = v; break; case "stack": o.stack = v; break;
  case "constructor": o.constructor = v; break; case "push": o.push = v; break; case "callee": o.callee = v; break;
  case "toString": o.toString = v; break; case "zzOpSet": o.zzOpSet = v; break; case "zzOpRo": o.zzOpRo = v; break;
  case "zzOpData": o.zzOpData = v; break; case "zzApSet": o.zzApSet = v; break; case "zzSpSet": o.zzSpSet = v; break;
  case "zzSpRo": o.zzSpRo = v; break;
  default: o[k] = v;
  }
}
var writers = [["local", wr], ["strict", wrs], ["temp-obj", wrt], ["temp-obj-strict", wrts],
  ["temp-key", wrk], ["temp-val", wrv], ["const-val", wrc], ["field", wf, wr], ["field-strict", wfs, wrs]];
// objects, functions and symbols by identity: the n-th one seen
var seen = [];
function ident(v) { var i = seen.indexOf(v); if (i < 0) { seen.push(v); i = seen.length - 1; } return i; }
var theValue = { theValue: 1 };
function show(v) {
  if (v === theValue) return "V";
  if (typeof v === "symbol") return "sym#" + ident(v);
  if (typeof v === "bigint") return v + "n";
  if (typeof v === "function") return "fn";
  if (Object.is(v, -0)) return "-0";
  if (v !== null && typeof v === "object") return "obj";
  return typeof v + ":" + String(v);
}
function who(x) {
  if (x === cur) return "self";
  if (x === null || (typeof x !== "object" && typeof x !== "function")) return "prim " + typeof x;
  return "other";
}
function guard(f, a, b, c) {
  try { f(a, b, c); return "ok"; } catch (e) { return e.name + ":" + e.message; }
}
function gname(f, a, b, c) { return guard(f, a, b, c).split(":")[0]; }
function protoName(o, own) {
  var p = Object.getPrototypeOf(o);
  if (p === null) return "null";
  if (p === Object.prototype) return "OP";
  if (p === own) return "own";
  if (p === theValue) return "V";
  return "other";
}
// the own keys of o that a receiver made alike does not have, and the key k,
// with their values and attributes, its prototype and extensibility
function state(o, k, init, own) {
  if (o === null || (typeof o !== "object" && typeof o !== "function")) return "-";
  var keys = Reflect.ownKeys(o), s = [];
  for (var i = 0; i < keys.length; i++) {
    var key = keys[i];
    if (init.indexOf(key) >= 0 && key !== k) continue;
    var d = Reflect.getOwnPropertyDescriptor(o, key);
    s.push(String(key) + "=" + ("value" in d ? show(d.value) : "acc") +
      (d.writable ? "w" : "") + (d.enumerable ? "e" : "") + (d.configurable ? "c" : ""));
  }
  return s.join(" ") + " proto:" + protoName(o, own) + (Object.isExtensible(o) ? "" : " fixed");
}
function setterLog(name) {
  return function (v) { log.push("set " + name + " " + who(this) + " " + show(v)); };
}
function getterLog(name) {
  return function () { log.push("get " + name + " " + who(this)); return "g" + name; };
}
// a receiver made by mk, or the same value each time for a primitive; the
// store through w with k, then with fresh(k), on another receiver
function check(name, mk, k, v) {
  for (var j = 0; j < writers.length; j++) {
    var wname = writers[j][0], w = writers[j][1], twin = writers[j][2] || w;
    var a = mk(), b = mk(), init = (a !== null && (typeof a === "object" || typeof a === "function")) ?
      Reflect.ownKeys(mk()) : [];
    var ownA = a !== null && typeof a === "object" ? Object.getPrototypeOf(a) : null;
    var ownB = b !== null && typeof b === "object" ? Object.getPrototypeOf(b) : null;
    var n0 = log.length;
    cur = a;
    var ra = guard(w, a, k, v);
    var la = log.slice(n0).join(";");
    var sa = state(a, k, init, ownA);
    var n1 = log.length;
    cur = b;
    var rb = guard(twin, b, fresh(k), v);
    var lb = log.slice(n1).join(";");
    var sb = state(b, k, init, ownB);
    cur = null;
    var line = name + " [" + String(k) + "] " + wname + " " + ra.split(":")[0] + (la ? " {" + la + "}" : "") + " " + sa;
    if (ra !== rb || la !== lb || sa !== sb)
      line += " MISMATCH " + rb + " {" + lb + "} " + sb;
    out.push(line);
  }
}
var keys = ["a", "zz", "c", "pd", "ro", "st", "gt", "length", "__proto__", "prototype", "name",
  "stack", "constructor", "push", "-0", "-1", "1e3", "1.5", "Infinity", "NaN", "0", "10", "",
  "callee", "toString"];
function mkProto() {
  var p = { pd: "ppd" };
  Object.defineProperty(p, "ro", { value: "pro", writable: false, enumerable: true, configurable: true });
  Object.defineProperty(p, "st", { set: setterLog("st"), enumerable: true, configurable: true });
  Object.defineProperty(p, "gt", { get: getterLog("gt"), enumerable: true, configurable: true });
  return p;
}
function margs(a) { return arguments; }
function sargs(a) { "use strict"; return arguments; }
function mkProxy(trap) {
  var h = {};
  if (trap)
    h.set = function (t, key, val, r) {
      log.push("trap " + String(key) + " " + who(r) + " " + show(val));
      return Reflect.set(t, key, val, r);
    };
  return new Proxy({}, h);
}
var receivers = [
  ["literal", function () { return { a: 1, c: "C" }; }],
  ["empty", function () { return {}; }],
  ["instance", function () { return new (class P { constructor() { this.a = 1; } })(); }],
  ["dict", function () {
    var d = { a: 1, b: 2 };
    for (var i = 0; i < 20; i++) d["x" + i] = i;
    for (var i = 0; i < 20; i++) delete d["x" + i];
    return d;
  }],
  ["many", function () { var m = {}; for (var i = 0; i < 40; i++) m["m" + i] = i; return m; }],
  ["bare", function () { return Object.create(null); }],
  ["proto", function () { return Object.create(mkProto()); }],
  ["deep", function () { return Object.create(Object.create(Object.create(mkProto()))); }],
  ["own-acc", function () {
    var o = {};
    Object.defineProperty(o, "st", { set: setterLog("own st"), enumerable: true, configurable: true });
    Object.defineProperty(o, "gt", { get: getterLog("own gt"), enumerable: true, configurable: true });
    Object.defineProperty(o, "ro", { value: "oro", writable: false, enumerable: true, configurable: true });
    return o;
  }],
  ["no-ext", function () { return Object.preventExtensions({ a: 1 }); }],
  ["sealed", function () { return Object.seal({ a: 1 }); }],
  ["frozen", function () { return Object.freeze({ a: 1 }); }],
  ["no-ext-proto", function () { return Object.preventExtensions(Object.create(mkProto())); }],
  ["array", function () { return [1, 2, 3]; }],
  ["under-array", function () { return Object.create([1, 2]); }],
  ["under-frozen-array", function () { return Object.create(Object.freeze([1])); }],
  ["int8", function () { return new Int8Array(2); }],
  ["under-int8", function () { return Object.create(new Int8Array(2)); }],
  ["under-f64-proto", function () { return Object.create(Object.create(new Float64Array(1))); }],
  ["margs", function () { return margs(1, 2); }],
  ["sargs", function () { return sargs(1, 2); }],
  ["under-margs", function () { return Object.create(margs(1, 2)); }],
  ["string-obj", function () { return new String("ab"); }],
  ["under-string-obj", function () { return Object.create(new String("ab")); }],
  ["fn", function () { return function f(x) { return x; }; }],
  ["class", function () { return class K { static s = 1; }; }],
  ["error", function () { return new Error("e"); }],
  ["proxy-trap", function () { return mkProxy(true); }],
  ["under-proxy-trap", function () { return Object.create(mkProxy(true)); }],
  ["under-proxy", function () { return Object.create(mkProxy(false)); }],
  ["str", function () { return "abc"; }],
  ["num", function () { return 1.5; }],
  ["bool", function () { return true; }],
  ["sym", function () { return Symbol.iterator; }],
  ["big", function () { return 10n; }],
  ["null", function () { return null; }],
  ["undefined", function () { return undefined; }]
];
// keys put on the prototypes of the built-ins, for a plain object, an
// object whose prototype is an array and a primitive
function patchBuiltins() {
  Object.defineProperty(Object.prototype, "zzOpSet", { set: setterLog("OP"), configurable: true });
  Object.defineProperty(Object.prototype, "zzOpRo", { value: 1, writable: false, configurable: true });
  Object.defineProperty(Object.prototype, "zzOpData", { value: 1, writable: true, configurable: true });
  Object.defineProperty(Array.prototype, "zzApSet", { set: setterLog("AP"), configurable: true });
  Object.defineProperty(String.prototype, "zzSpSet", { set: setterLog("SP"), configurable: true });
  Object.defineProperty(String.prototype, "zzSpRo", { value: 1, writable: false, configurable: true });
}
function unpatchBuiltins() {
  delete Object.prototype.zzOpSet; delete Object.prototype.zzOpRo; delete Object.prototype.zzOpData;
  delete Array.prototype.zzApSet; delete String.prototype.zzSpSet; delete String.prototype.zzSpRo;
}
var patchedKeys = ["zzOpSet", "zzOpRo", "zzOpData", "zzApSet", "zzSpSet", "zzSpRo"];
var patchedOn = [receivers[0], receivers[1], receivers[5], receivers[14], receivers[22], receivers[30]];

for (var round = 0; round < 2; round++) {
  for (var i = 0; i < receivers.length; i++)
    for (var m = 0; m < keys.length; m++)
      check(receivers[i][0], receivers[i][1], keys[m], m % 2 ? theValue : "v");
  // a symbol key never takes the path of an atom string
  check("literal", receivers[0][1], Symbol.iterator, "v");
  // the prototype set by __proto__ to the value
  check("literal", receivers[0][1], "__proto__", theValue);
  check("bare", receivers[5][1], "__proto__", theValue);
  patchBuiltins();
  for (var i = 0; i < patchedOn.length; i++)
    for (var m = 0; m < patchedKeys.length; m++)
      check(patchedOn[i][0] + "+patched", patchedOn[i][1], patchedKeys[m], "v");
  unpatchBuiltins();
  // after the keys are gone, the same stores add
  for (var m = 0; m < patchedKeys.length; m++)
    check("literal+unpatched", receivers[0][1], patchedKeys[m], "v");
  // the global object: a variable, a new key, a built-in
  var g1 = gname(wr, globalThis, "paTop", 2), g2 = gname(wrs, globalThis, "paNewA" + "", 3);
  var g3 = gname(wr, globalThis, Object.keys({ paNewB: 1 })[0], 4);
  var g4 = gname(wrs, globalThis, "undefined", 5), g5 = gname(wr, globalThis, "NaN", 6);
  out.push("global " + [g1, g2, g3, g4, g5, paTop, globalThis.paNewA, globalThis.paNewB,
    Object.keys(globalThis).filter(function (s) { return s.slice(0, 2) === "pa"; }).join("+")].join(" "));
  delete globalThis.paNewA; delete globalThis.paNewB;
  // a copy key by key, from for in and from Object.keys, against
  // Object.assign: the same keys in the same order
  var src = { b: 1, a: 2, "-0": 3, zz: 4, Infinity: 5, __proto__: { inh: 6 } };
  var c1 = {}, c2 = {}, c3 = Object.assign({}, src);
  for (var k in src) c1[k] = src[k];
  var ks = Object.keys(src);
  for (var j = 0; j < ks.length; j++) c2[ks[j]] = src[ks[j]];
  out.push("copy " + JSON.stringify(c1) + " " + JSON.stringify(c2) + " " + JSON.stringify(c3) + " " +
    Reflect.ownKeys(c1).join() + " " + Reflect.ownKeys(c2).join());
  // the key's only reference is the temp of the store
  var made = {};
  made[Object.keys({ ["paMade" + round]: 1 })[0]] = "m";
  made[Object.keys({ ["paMade" + round]: 1 })[0]] = "m2";
  out.push("made " + Object.keys(made).join() + " " + made["paMade" + round]);
  // the value is the object or the key, and a store drops the last
  // reference to an object
  var self = {}, kk = Object.keys({ paSelf: 1 })[0];
  self[kk] = self; self[kk + ""] = kk;
  var drop = { d: { big: [1, 2, 3] } };
  wr(drop, "d", null);
  out.push("alias " + (self.paSelf === "paSelf") + " " + Object.keys(self).join() + " " + JSON.stringify(drop));
  // o.x = v of PUTF: a new key, an inherited setter, a frozen object
  var pf = Object.create(mkProto()), fr = Object.freeze({});
  cur = pf;
  pf.nx = 1; pf.st = 2; pf.pd = 3;
  cur = null;
  var pfs = gname(function () { "use strict"; pf.ro = 4; }), frs = gname(function () { "use strict"; fr.nx = 1; });
  var frl = gname(function () { fr.nx = 1; });
  out.push("putf " + Object.keys(pf).join() + " " + pfs + " " + frs + " " + frl);
}
out.push(log.length + " " + log.slice(0, 6).join(";"));
rvExpect(wr, wrs, wrt, wrts, wrk, wrv, wrc, wf, wfs);
return out.join("\n");
})()
// What the script prints is what Node 24 prints (the script run with rvExpect
// a function that does nothing), but for two kinds of lines, the same on the
// engine before the change: the callee of strict arguments set in sloppy code
// throws a TypeError here, as the specification says (the setter is
// %ThrowTypeError%), where Node 24 ignores the store, and the stack of an
// error is a data property here (docs/internals/changes/errors-and-stacks.md) and an
// accessor in Node 24.
