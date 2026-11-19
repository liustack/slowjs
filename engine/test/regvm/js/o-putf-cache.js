// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.
// rvdiff: o.name = v (PUTF) at a site that has stored the key before, where
// what the site keeps has gone stale. For an own key it keeps the slot: the
// key deleted and added again at another slot, another key at that slot, the
// key made an accessor or read-only, the object frozen, the length of an
// array. For a key it adds it keeps the step from the shape of the object to
// the shape with the key: a setter or a read-only key put on the prototype,
// the prototype frozen, setPrototypeOf of the prototype to one with a setter,
// a key deleted from the prototype that showed one with a setter further up,
// a setter on Object.prototype, the object made non-extensible, the shape the
// site kept grown in place by its one object, the shapes the site kept freed
// by a full collection and other shapes made after, objects whose properties
// sit in their own block or need more room, functions, arrays, the global
// object and objects of another realm. Each store is checked against the
// own descriptor it leaves, a setter writes to a log, and each test runs
// three times so that the later runs store through a warm site.
// inject-hit: PUTF 1
(function () {
var out = [], log = [], bad = 0, held = [];
// a store that fails throws in strict code (the script runs both ways)
var strict = (function () { return this === undefined; })();
function fails(rest) { return (strict ? "throws TypeError" : "ok") + ", " + rest; }
function show(v) {
  if (typeof v === "function") return "fn " + v.name;
  if (v !== null && typeof v === "object") return "obj";
  return typeof v + " " + String(v);
}
// one site each: the value in a local, a constant, the object in a temp
function stX(o, v) { o.x = v; }
function stXK(o) { o.x = "k"; }
function stXT(o, v) { [o][0].x = v; }
function stY(o, v) { o.y = v; }
function stLen(o, v) { o.length = v; }
function guard(f, o, v) {
  try { f(o, v); return "ok"; } catch (e) { return "throws " + e.name; }
}
function own(o, k) {
  var d = Object.getOwnPropertyDescriptor(o, k);
  if (!d) return "none";
  if ("value" in d) return "value " + show(d.value) + (d.writable ? "" : " ro");
  return "accessor";
}
function check(name, got, want) {
  var ok = got === want;
  if (!ok) bad++;
  out.push(name + " " + got + (ok ? "" : " MISMATCH want " + want));
}
function store(name, f, o, v, want) {
  var r = guard(f, o, v);
  check(name, r + ", " + own(o, f === stY ? "y" : f === stLen ? "length" : "x") +
        (log.length ? ", log " + log.join(" ") : ""), want);
  log.length = 0;
}
// the objects stay alive (held), so that the shape before the key has more
// than one object and an add copies it: the step a site keeps
function warm(f, mk, n) {
  for (var i = 0; i < n; i++) {
    var o = mk();
    held.push(o);
    f(o, i);
  }
}

function run(round) {
  out.push("-- round " + round);
  held = [];
  // the slot of an own key
  var o = { b: 1, x: 2, c: 3 };
  for (var i = 0; i < 4; i++) stX(o, i);
  store("own", stX, o, 10, "ok, value number 10");
  delete o.x;
  o.x = 0;
  store("own moved", stX, o, 11, "ok, value number 11");
  var q = { b: 1, y: 2, c: 3 };
  store("other key at the slot", stX, q, 12, "ok, value number 12");
  check("other key kept", own(q, "y"), "value number 2");
  Object.defineProperty(o, "x", { get: function () { return 0; }, set: function (v) { log.push("own set " + v); }, configurable: true });
  store("own accessor", stX, o, 13, "ok, accessor, log own set 13");
  var r = { x: 1 };
  stX(r, 1); stX(r, 2);
  Object.defineProperty(r, "x", { writable: false });
  store("own read-only", stX, r, 14, fails("value number 2 ro"));
  var fz = { a: 0, x: 1 };
  stX(fz, 3); stX(fz, 4);
  Object.freeze(fz);
  store("own frozen", stX, fz, 15, fails("value number 4 ro"));
  var arr = [1, 2, 3];
  store("array length", stLen, { length: 9 }, 1, "ok, value number 1");
  store("array length real", stLen, arr, 1, "ok, value number 1");
  check("array cut", arr.join(","), "1");
  store("array length again", stLen, { length: 9 }, 2, "ok, value number 2");
  stXK({ x: 0 });
  store("constant", stXK, { b: 1, x: 2, c: 3 }, 0, "ok, value string k");

  // the step of a key added by a constructor's objects
  function C(a) { this.a = a; }
  warm(stX, function () { return new C(1); }, 30);
  store("step", stX, new C(1), 20, "ok, value number 20");
  store("step temp", stXT, new C(1), 21, "ok, value number 21");
  Object.defineProperty(C.prototype, "x", { set: function (v) { log.push("C set " + v); }, configurable: true });
  store("proto setter", stX, new C(1), 22, "ok, none, log C set 22");
  store("proto setter temp", stXT, new C(1), 23, "ok, none, log C set 23");
  delete C.prototype.x;
  store("proto setter gone", stX, new C(1), 24, "ok, value number 24");
  Object.defineProperty(C.prototype, "x", { value: "ro", writable: false, configurable: true });
  store("proto read-only", stX, new C(1), 25, fails("none"));
  delete C.prototype.x;
  store("proto read-only gone", stX, new C(1), 26, "ok, value number 26");

  // a writable key on the prototype, then frozen
  function F() { this.a = 1; }
  F.prototype.x = "p";
  warm(stX, function () { return new F(); }, 30);
  store("proto writable", stX, new F(), 27, "ok, value number 27");
  Object.freeze(F.prototype);
  store("proto frozen", stX, new F(), 28, fails("none"));

  // setPrototypeOf of the prototype
  function D() { this.a = 1; }
  warm(stX, function () { return new D(); }, 30);
  var S = { set x(v) { log.push("S set " + v); } };
  Object.setPrototypeOf(D.prototype, S);
  store("proto of proto", stX, new D(), 29, "ok, none, log S set 29");

  // a key deleted from the prototype shows a setter above it
  var H = { set x(v) { log.push("H set " + v); } };
  function G() { this.a = 1; }
  G.prototype = Object.create(H);
  Object.defineProperty(G.prototype, "x", { value: "shadow", writable: true, configurable: true });
  warm(stX, function () { return new G(); }, 30);
  store("shadowed", stX, new G(), 30, "ok, value number 30");
  delete G.prototype.x;
  store("shadow deleted", stX, new G(), 31, "ok, none, log H set 31");

  // a setter on Object.prototype
  warm(stY, function () { return { b: 1 }; }, 30);
  store("plain", stY, { b: 1 }, 32, "ok, value number 32");
  Object.defineProperty(Object.prototype, "y", { set: function (v) { log.push("OP set " + v); }, configurable: true });
  store("Object.prototype setter", stY, { b: 1 }, 33, "ok, none, log OP set 33");
  delete Object.prototype.y;
  store("Object.prototype setter gone", stY, { b: 1 }, 34, "ok, value number 34");

  // a receiver made non-extensible
  function E() { this.a = 1; }
  warm(stX, function () { return new E(); }, 30);
  var ne = new E();
  Object.preventExtensions(ne);
  store("not extensible", stX, ne, 35, fails("none"));
  store("extensible", stX, new E(), 36, "ok, value number 36");

  // the shape kept grows in place: its one object gets a key
  function M() { this.a = 1; }
  var m1 = new M();
  stX(m1, 1);
  var m4 = new M(), m2 = new M();
  stX(m2, 2);
  m2 = null;
  m1.z = 3;
  store("kept shape grew", stX, m4, 37, "ok, value number 37");
  check("kept shape grew keys", Object.keys(m4).join(","), "a,x");
  check("grown keys", Object.keys(m1).join(","), "a,x,z");

  // the shape kept freed while objects of the shape before it live, other
  // shapes made after; then all freed by a full collection
  function L() { this.a = 1; }
  var keepL = new L();
  warm(stX, function () { return new L(); }, 30);
  held = [];
  var junk = [];
  for (var j = 0; j < 200; j++) { var t = {}; t["j" + j] = j; t.k = j; junk.push(t); }
  store("kept shape freed", stX, new L(), 38, "ok, value number 38");
  check("kept shape freed keys", Object.keys(junk[7]).join(","), "j7,k");
  keepL = null;
  junk = null;
  if (typeof gc === "function") gc();
  junk = [];
  for (var j = 0; j < 200; j++) { var t = {}; t["g" + j] = j; t.k = j; junk.push(t); }
  store("after gc", stX, new L(), 39, "ok, value number 39");
  // the shape kept grows past its room in place, which moves it: its one
  // object gets many keys
  function W() { this.a = 1; }
  var w1 = new W(), w4 = new W(), w2 = new W();
  stX(w1, 1);
  for (var j = 0; j < 40; j++) w1["k" + j] = j;
  store("kept shape moved", stX, w4, 40, "ok, value number 40");
  check("kept shape moved keys", Object.keys(w4).join(","), "a,x");

  // properties in the block of the object, and more room
  warm(stY, function () { return { p: 1, q: 2 }; }, 30);
  var lit = { p: 1, q: 2 };
  store("inline", stY, lit, 41, "ok, value number 41");
  check("inline keys", Object.keys(lit).join(","), "p,q,y");
  function R() { this.a = 1; this.b = 2; this.c = 3; this.d = 4; this.e = 5; this.f = 6; }
  warm(stX, function () { return new R(); }, 30);
  var big = new R();
  store("room", stX, big, 42, "ok, value number 42");
  check("room keys", Object.keys(big).join(","), "a,b,c,d,e,f,x");

  // functions, arrays, the global object
  function fa() {}
  function fb() {}
  stX(fa, 1);
  store("function", stX, fb, 43, "ok, value number 43");
  var a1 = [1], a2 = [2];
  stX(a1, 1);
  store("array", stX, a2, 44, "ok, value number 44");
  store("global", stY, globalThis, 45, "ok, value number 45");
  delete globalThis.y;

  // another realm
  if (typeof newRealm === "function") {
    var other = newRealm("({ st: function (o, v) { o.x = v; }, make: function () { function K() { this.a = 1; } return new K(); } })");
    for (var k = 0; k < 10; k++) { var ko = other.make(); held.push(ko); other.st(ko, k); }
    store("realm", other.st, other.make(), 46, "ok, value number 46");
    store("realm here", stX, other.make(), 47, "ok, value number 47");
    function N() { this.a = 1; }
    warm(stX, function () { return new N(); }, 10);
    store("realm code here", other.st, new N(), 48, "ok, value number 48");
  }
}
for (var round = 0; round < 3; round++)
  run(round);
out.push(bad ? bad + " MISMATCH" : "all match");
return out.join("\n");
})()
