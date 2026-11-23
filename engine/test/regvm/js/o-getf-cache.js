// Copyright (c) 2026 The liustack/tifajs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.
// rvdiff: o.name (GETF, GETFL, GETFT and the read of a method before CALLM)
// at a site that has read the key before, where the site's cache (the slot
// of the data property it last found, own or of the prototype) has gone
// stale: the key deleted, deleted and added again at another slot, made an
// accessor whose getter has side effects, made read-only, the object frozen,
// sealed or made non-extensible, the slots of its shape moved in place (many
// keys deleted), another key at the same slot, a slot past the keys of a
// smaller object, many shapes at the same site in turn (each one replacing
// the last), a prototype that gains or loses the key, setPrototypeOf, a
// proxy and exotic objects (arrays, typed arrays, String objects, arguments,
// functions, errors and their stack, the global object), primitives, objects
// of another realm read by code of this one and the other way round, and
// objects made after a full collection freed the shapes the site saw. Each
// read is checked against Reflect.get or against what the script set, a
// getter writes to a log, and each test runs three times so that the later
// runs read through a warm site.
// inject-hit: GETF 1
(function () {
var out = [], log = [], bad = 0;
function show(v) {
  if (typeof v === "function") return "fn " + v.name;
  if (v !== null && typeof v === "object") return "obj";
  if (typeof v === "symbol") return "sym";
  return typeof v + " " + String(v);
}
function guard(f, o) {
  try { return show(f(o)); } catch (e) { return "throws " + e.name + " " + e.message; }
}
// one site each: GETF into a temp, GETFL into a local, GETFT on a temp
function rdA(o) { return o.a; }
function rdAL(o) { var v; v = o.a; return v; }
function rdAT(o) { return [o][0].a; }
function rdC(o) { return o.c; }
function rdLen(o) { return o.length; }
function rdStack(o) { return o.stack; }
function rdM(o) { return o.m(); }
function rdX(o) { return o.x; }
function rdP(o) { return o.a; }
function check(name, o, rd, want) {
  var a = guard(rd, o);
  var ok = want === undefined ? true : a === want;
  if (!ok) bad++;
  out.push(name + " " + a + (ok ? "" : " MISMATCH want " + want));
}
function all(name, o) {
  var r = Reflect.get(Object(o), "a", o);
  var w = show(r);
  check(name + " .a", o, rdA, w);
  check(name + " .a local", o, rdAL, w);
  check(name + " .a temp", o, rdAT, w);
}

function run(round) {
  out.push("-- round " + round);
  // one shape, then the key deleted, added again at another slot, deleted
  var o = { b: 1, a: 2, c: 3 };
  all("plain", o);
  delete o.b;
  all("b deleted", o);
  delete o.a;
  all("a deleted", o);
  o.a = "again";
  all("a added again", o);
  // the slots move in place: many keys deleted
  var d = { a: "da" };
  for (var i = 0; i < 40; i++) d["k" + i] = i;
  all("dict before", d);
  for (var i = 0; i < 40; i++) delete d["k" + i];
  all("dict after", d);
  for (var i = 0; i < 40; i++) d["n" + i] = i;
  delete d.a;
  d.a = "da2";
  all("dict moved", d);
  // an accessor with side effects in place of the data property
  var g = { z: 0, a: 1 };
  all("data", g);
  Object.defineProperty(g, "a", { get: function () { log.push("get a"); return "ga"; }, configurable: true });
  var n0 = log.length;
  check("getter .a", g, rdA, "string ga");
  check("getter .a local", g, rdAL, "string ga");
  check("getter .a temp", g, rdAT, "string ga");
  check("getter calls", log.length - n0, function (n) { return n; }, "number 3");
  Object.defineProperty(g, "a", { value: "back", writable: true, configurable: true });
  all("data again", g);
  // a getter that turns into an own data property on the receiver
  var lazy = { get a() { Object.defineProperty(this, "a", { value: "made", writable: true }); return "first"; } };
  var under = Object.create(lazy);
  check("lazy first", under, rdA, "string first");
  check("lazy then", under, rdA, "string made");
  // read-only, frozen, sealed, not extensible
  var r = { a: "ro" };
  all("writable", r);
  Object.defineProperty(r, "a", { writable: false });
  all("read-only", r);
  Object.defineProperty(r, "a", { value: "ro2" });
  all("read-only changed", r);
  var f = { x: 1, a: "fa" };
  all("not frozen", f);
  Object.freeze(f);
  all("frozen", f);
  var s = Object.seal({ a: "sa" });
  all("sealed", s);
  var ne = Object.preventExtensions({ y: 0, a: "na" });
  all("not extensible", ne);
  // another key at the slot the site saw, a slot past the keys
  var big = { p: 1, q: 2, r: 3, s: 4, a: "big" };
  all("big", big);
  all("other key at slot", { p: 1, q: 2, r: 3, s: 4, t: "not a" });
  all("small", { a: "small" });
  all("empty", {});
  // many shapes at one site, in turn
  var shapes = [{ a: 1 }, { b: 2, a: 2 }, { c: 3, b: 3, a: 3 }, { a: 4, b: 4 }, Object.create({ a: "pa" }),
    Object.create(null), { __proto__: null, a: "np" }, [1, 2], function a() {}, { get a() { return "acc"; } }];
  shapes[7].a = "arr";
  for (var k = 0; k < 3; k++)
    for (var i = 0; i < shapes.length; i++)
      all("shape " + i, shapes[i]);
  // the prototype gains and loses the key, setPrototypeOf
  var proto = {};
  var child = Object.create(proto);
  child.z = 1;
  all("proto none", child);
  proto.a = "pa";
  all("proto has", child);
  child.a = "own";
  all("own over proto", child);
  delete child.a;
  all("own gone", child);
  Object.setPrototypeOf(child, { a: "other proto" });
  all("set proto", child);
  // a proxy, and an object whose prototype is one
  var px = new Proxy({ a: "ta" }, { get: function (t, k) { log.push("trap " + String(k)); return t[k]; } });
  n0 = log.length;
  all("proxy", px);
  all("under proxy", Object.create(px));
  check("trap calls", log.length - n0, function (n) { return n; }, "number 8");
  // exotic objects and primitives
  var arr = [1, 2, 3];
  check("array length", arr, rdLen, "number 3");
  arr.length = 1;
  check("array shorter", arr, rdLen, "number 1");
  check("typed length", new Int8Array(5), rdLen, "number 5");
  check("string obj length", new String("abcd"), rdLen, "number 4");
  check("string length", "abcde", rdLen, "number 5");
  check("args length", (function () { return arguments; })(1, 2), rdLen, "number 2");
  check("fn length", function (p, q, r) {}, rdLen, "number 3");
  check("plain length", { length: "own" }, rdLen, "string own");
  check("num", 1.5, rdA, "undefined undefined");
  Number.prototype.a = "na";
  check("num proto", 1.5, rdA, "string na");
  delete Number.prototype.a;
  check("num gone", 1.5, rdA, "undefined undefined");
  check("null", null, rdA);
  check("undefined", undefined, rdA);
  // the stack of an error, which a site must not keep
  var e1 = new Error("e1"), e2 = new Error("e2");
  check("stack own", { stack: "s" }, rdStack, "string s");
  check("stack error", e1, rdStack, show(e1.stack));
  check("stack error 2", e2, rdStack, show(e2.stack));
  check("stack error again", e1, rdStack, show(e1.stack));
  // the global object
  globalThis.gfc = "global";
  check("global", globalThis, function (o) { return o.gfc; }, "string global");
  delete globalThis.gfc;
  check("global gone", globalThis, function (o) { return o.gfc; }, "undefined undefined");
  // methods read before a call
  var m1 = { m: function () { return "m1"; } };
  var C = function () {}; C.prototype.m = function () { return "proto m"; };
  var m2 = new C();
  check("method own", m1, rdM, "string m1");
  check("method proto", m2, rdM, "string proto m");
  m2.m = function () { return "own m"; };
  check("method own over proto", m2, rdM, "string own m");
  delete m2.m;
  check("method proto again", m2, rdM, "string proto m");
  C.prototype.m = function () { return "proto m2"; };
  check("method proto replaced", m2, rdM, "string proto m2");
  var D = function () {}; D.prototype = Object.create(C.prototype);
  var m3 = new D();
  check("method two up", m3, rdM, "string proto m2");
  check("method two up again", m3, rdM, "string proto m2");
  D.prototype.m = function () { return "D m"; };
  check("method nearer", m3, rdM, "string D m");
  delete C.prototype.m;
  check("method proto deleted", m2, rdM);
  check("method nearer still", m3, rdM, "string D m");
  delete m1.m;
  check("method gone", m1, rdM);
  // a key whose object is freed, its shape with it, and new objects after a collection
  for (var j = 0; j < 4; j++) {
    var t = {};
    t["q" + j] = j;
    t.x = "x" + j;
    check("freed " + j, t, rdX, "string x" + j);
    t = null;
    if (typeof gc === "function") gc();
    var u = {};
    u.x = "u" + j;
    check("after gc " + j, u, rdX, "string u" + j);
  }
  // three shapes in turn at one site, then a delete and a change of the prototype
  var sA = { a: "s0" }, sB = { q: 0, a: "s1" }, sC = Object.create({ a: "s2" });
  var seen = "";
  for (var i = 0; i < 40; i++) seen += rdP([sA, sB, sC][i % 3]).slice(1);
  check("turns", seen, function (v) { return v; }, "string " + "012".repeat(13) + "0");
  check("turns then one", sB, rdP, "string s1");
  delete sB.a;
  check("turns deleted", sB, rdP, "undefined undefined");
  Object.getPrototypeOf(sC).a = "s3";
  check("turns proto changed", sC, rdP, "string s3");
  // another realm
  if (typeof newRealm === "function") {
    var other = newRealm("({ rd: function (o) { return o.a; }, rdm: function (o) { return o.m(); }," +
      " make: function (v) { return { z: 0, a: v }; }, makeC: function () { function K() {} K.prototype.m = function () { return 'other m'; }; return new K(); } })");
    var mine = { z: 1, a: "mine" }, theirs = other.make("theirs");
    check("realm code mine", mine, other.rd, "string mine");
    check("realm code theirs", theirs, other.rd, "string theirs");
    check("this code theirs", theirs, rdA, "string theirs");
    check("this code mine", mine, rdA, "string mine");
    check("realm method", other.makeC(), rdM, "string other m");
    var E = function () {}; E.prototype.m = function () { return "proto m"; };
    check("realm code method", new E(), other.rdm, "string proto m");
    delete theirs.a;
    check("realm deleted", theirs, other.rd, "undefined undefined");
    check("realm deleted here", theirs, rdA, "undefined undefined");
  }
}
for (var round = 0; round < 3; round++)
  run(round);
out.push(bad ? bad + " MISMATCH" : "all match");
return out.join("\n");
})()
