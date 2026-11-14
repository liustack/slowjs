// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// rvdiff: o[k] with a string key (GETEL, GETELT, GETELK and a key in a temp)
// on every kind of object, against two reads that cannot take the path of a
// key that is its own atom: Reflect.get, and the same key made again as a
// string that is not an atom. Keys: names, the empty string, indexes and
// strings that look like numbers ("0", "10", "01", "4294967294",
// "4294967295", "-0", "-1", "1e3", "Infinity", "NaN"), "length", "stack",
// "__proto__" and a key with the name of a symbol. Objects: plain ones, with
// the key on a prototype or nowhere, null prototypes, getters (own and on a
// prototype), arrays, typed arrays (and objects whose prototype is one),
// arguments (mapped, unmapped, strict), String objects, functions, errors,
// proxies (and objects whose prototype is one), the global object, an
// object in dictionary mode, frozen objects, and primitives as the object
// (strings, numbers, booleans, null and undefined, with the key in the
// TypeError). A getter or a trap writes to a log with its receiver, so a read
// that runs it twice, not at all or on another object shows, and objects are
// shown by identity. Names on the prototype of a typed array show whether a
// canonical number stops at the array. Also: a key whose only reference is the
// temp of the read, the first read of the stack of an error, and results
// written to the slot of the key or of the object.
var gaTopVar = 5;
(function () {
var out = [];
var log = [];
function id(x) { return x; }
// a string of the same characters that is not an atom
function fresh(k) { return (" " + k).slice(1); }
function rd(o, k) { return o[k]; }
function rdt(o, k) { return id(o)[k]; }
function rdk(o, k) { return o[id(k)]; }
function rdc(o) { return o["c"]; }
function rdl(o) { return o["length"]; }
function rdz(o) { return o["zz"]; }
// the key a temp that holds the only reference to its atom
function rdm(o, i) { return o[Object.keys({ ["gaMade" + i]: 1 })[0]]; }
var own2 = { gaMade0: "m0", gaMade1: "m1", gaMade2: "m2" };
function mkErr() { return new Error("e"); }
function selfKey(o, k) { k = o[k]; k = o[k + k]; return k; }
function selfObj(o, k) { o = o[k]; o = o[k]; return o[k]; }
// objects, functions and symbols by identity: the n-th one seen
var seen = [];
function ident(v) { var i = seen.indexOf(v); if (i < 0) { seen.push(v); i = seen.length - 1; } return i; }
function show(v) {
  if (typeof v === "symbol") return "sym#" + ident(v);
  if (typeof v === "bigint") return v + "n";
  if (typeof v === "function") return "fn:" + v.name + "#" + ident(v);
  if (Object.is(v, -0)) return "-0";
  if (v !== null && typeof v === "object") return "obj#" + ident(v);
  return typeof v + ":" + String(v);
}
function guard(f, a, b) {
  try { return show(f(a, b)); } catch (e) { return e.name + ":" + e.message; }
}
function same(a, b) { return a === b || (a !== a && b !== b); }
// the read through f, Reflect.get and a key that is not an atom, as one line
function check(name, o, k) {
  var a, b, c, n0 = log.length, n1, n2;
  a = guard(rd, o, k);
  n1 = log.length;
  b = guard(rdt, o, k);
  var c1 = guard(rdk, o, k);
  n2 = log.length;
  try {
    var r = Reflect.get(Object(o), k, o);
    c = show(r);
  } catch (e) { c = e.name + ":" + e.message; }
  var d = typeof k === "string" ? guard(rd, o, fresh(k)) : a;
  var ok = a === b && a === c1 && (o === null || o === undefined || a === c) && a === d &&
    n1 - n0 === (n2 - n1) / 2;
  out.push(name + " [" + String(k) + "] " + a + (ok ? "" : " MISMATCH " + [b, c1, c, d, n1 - n0, n2 - n1].join(" | ")));
}
var keys = ["a", "c", "zz", "", "0", "10", "01", "4294967294", "4294967295", "-0", "-1", "1e3",
  "Infinity", "NaN", "length", "stack", "__proto__", "foo", "toString", "callee"];
var proto = { c: "pc", foo: "pfoo" };
var withProto = Object.create(proto);
withProto.a = "a1";
var deep = Object.create(Object.create(Object.create(proto)));
var bare = Object.create(null);
bare.a = 1; bare.c = undefined;
var getters = { get a() { log.push("get a " + (this === underGetters ? "under" : this === getters ? "own" : "other")); return "ga"; }, get c() { throw new RangeError("c"); },
  set zz(v) {} };
var underGetters = Object.create(getters);
var numKeys = { "0": "n0", "10": "n10", "01": "n01", "4294967294": "nmax", "4294967295": "nmax1",
  "-0": "nm0", "-1": "nm1", "1e3": "ne3", "Infinity": "ninf", "NaN": "nnan", "": "empty" };
var arr = [10, 20, 30];
arr.foo = "afoo"; arr["-1"] = "aminus"; arr["1e3"] = "a1e3";
var holey = [1, , 3];
var sparse = []; sparse[100] = 1;
var i8 = new Int8Array([1, 2, 3]);
// names on the prototype of the typed array: a canonical number stops at the array, a name does not
Object.setPrototypeOf(i8, Object.create(Object.getPrototypeOf(i8), { "-0": { value: "s-0" }, "-1": { value: "s-1" },
  "Infinity": { value: "sinf" }, "NaN": { value: "snan" }, "1e3": { value: "s1e3" }, "zz": { value: "szz" },
  "a": { get: function () { log.push("ta get " + (this === i8 ? "i8" : this === underTA ? "under" : "other")); return "sa"; } } }));
i8.foo = "tfoo";
var f64 = new Float64Array(2);
var underTA = Object.create(i8);
var str = new String("abc");
str.foo = "sfoo";
function fn(a, b) { return a; }
fn.foo = "ffoo";
class K { static c = "kc"; }
var err = new Error("boom");
var errStack = err.stack;
var proxy = new Proxy({ c: "tc" }, { get: function (t, k, r) { log.push("trap " + String(k) + (r === proxy ? " self" : r === underProxy ? " under" : " other")); return t[k]; } });
var underProxy = Object.create(proxy);
var dict = { a: 1, b: 2, c: 3, d: 4 };
for (var i = 0; i < 40; i++) dict["x" + i] = i;
for (var i = 0; i < 40; i++) delete dict["x" + i];
var frozen = Object.freeze({ a: "fa", c: "fc" });
var symKey = Symbol.for("foo");
var withSym = { foo: "sf" }; withSym[symKey] = "sym";
function margs(a) { return arguments; }
function sargs(a) { "use strict"; return arguments; }
var ma = margs(1, 2), sa = sargs(1, 2);
var objects = [
  ["plain", { a: 1, c: "C", length: 3, foo: null, __proto__: proto }],
  ["proto", withProto], ["deep", deep], ["bare", bare], ["getters", getters], ["under-getters", underGetters],
  ["numkeys", numKeys], ["array", arr], ["holey", holey], ["sparse", sparse], ["int8", i8], ["f64", f64],
  ["under-ta", underTA], ["string-obj", str], ["fn", fn], ["class", K], ["error", err], ["proxy", proxy],
  ["under-proxy", underProxy], ["global", globalThis], ["dict", dict], ["frozen", frozen],
  ["with-sym", withSym], ["margs", ma], ["sargs", sa],
  ["str", "abc"], ["num", 1.5], ["bool", true], ["null", null], ["undefined", undefined]
];
for (var round = 0; round < 3; round++) {
  for (var j = 0; j < objects.length; j++)
    for (var m = 0; m < keys.length; m++)
      check(objects[j][0], objects[j][1], keys[m]);
  check("global", globalThis, "gaTopVar");
  check("global", globalThis, "Math");
  check("with-sym", withSym, symKey);
  check("fn", fn, "prototype");
  check("fn", fn, "name");
  out.push("stack " + (err["stack"] === errStack) + " " + typeof err[id("stack")]);
  out.push("const " + guard(rdc, { c: 1 }) + " " + guard(rdc, withProto) + " " + guard(rdc, bare) +
    " " + guard(rdc, getters) + " " + guard(rdl, arr) + " " + guard(rdl, "abcd") + " " + guard(rdl, str) +
    " " + guard(rdl, i8) + " " + guard(rdz, {}) + " " + guard(rdz, null));
  // the property changes between reads: a new value, deleted, a getter
  var ch = { c: 1 };
  var r1 = rd(ch, "c");
  ch.c = 2;
  var r2 = rd(ch, "c");
  delete ch.c;
  var r3 = rd(ch, "c");
  Object.defineProperty(ch, "c", { get: function () { log.push("late"); return 4; }, configurable: true });
  var r4 = rd(ch, "c");
  proto.zz = "pzz";
  var r5 = rd(withProto, "zz");
  delete proto.zz;
  out.push("change " + [r1, r2, r3, r4, r5, rd(withProto, "zz")].join(","));
  // the last reference of the key is the temp the read takes
  var made = Object.keys({ gaOnlyKey: 1 })[0];
  out.push("last " + show(rdk({ gaOnlyKey: "k" }, made)));
  made = null;
  out.push("only " + [rdm(own2, round), rdm(Object.create(own2), round), rdm({}, round + 10)].map(show).join(","));
  // the first read of the stack of an error is through o[k]
  var e1 = mkErr(), e2 = mkErr();
  var s1 = rd(e1, "stack"), s2 = Reflect.get(e2, "stack");
  out.push("first stack " + typeof s1 + " " + (s1.split("\n")[0] === s2.split("\n")[0]) + " " + (rd(e1, "stack") === s1));
  // the result goes to the slot of the key or of the object
  out.push("alias " + [selfKey({ c: "c", cc: 1 }, "c"), selfObj({ n: { n: { n: "end" } } }, "n")].map(show).join(","));
}
out.push(log.join(","));
rvExpect(rd, rdt, rdk, rdc, rdl, rdz, rdm, selfKey, selfObj, check, guard);
return out.join("\n");
})()
