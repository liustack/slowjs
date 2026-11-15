// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// rvdiff: INSTOF, x instanceof C, which answers without calling
// Function.prototype[Symbol.hasInstance] when that is the function the test
// would call (rv_instof). What only this engine can show, beside
// engine/test/compare/instanceof.js (which Node checks): two realms
// (newRealm), the code that tests, the right side and the left side each from
// this realm or the other one, and a function of one realm whose prototype
// chain goes through the Function.prototype of the other. Then the messages
// of the errors and the realm each error comes from, which the path must
// leave to the generic code. Each test runs twice, warm the second time, and
// a line says MISMATCH when the two differ.
// inject-hit: INSTOF 1
var SRC = '(function () {' +
  ' function io(a, b) { return a instanceof b; }' +
  ' function iostrict(a, b) { "use strict"; return a instanceof b; }' +
  ' function F() {}' +
  ' class A {} class B extends A {}' +
  ' return { io: io, iostrict: iostrict, F: F, A: A, B: B, f: new F(), b: new B(),' +
  '  obj: {}, arr: [], arrow: function () { return () => 1; },' +
  '  Function: Function, Object: Object, Array: Array, TypeError: TypeError, Error: Error,' +
  '  fproto: Function.prototype, oproto: Object.prototype, newObj: function () { return {}; } };' +
  '})()';
var R = { main: (0, eval)(SRC), other: newRealm(SRC) };
var names = ["main", "other"];
var out = [], bad = 0;

function realmOf(e) {
  if (e === null || typeof e !== "object") return "none";
  if (e instanceof R.main.Error) return "main";
  if (e instanceof R.other.Error) return "other";
  return "?";
}
function guard(code, a, b) {
  try {
    return String(code(a, b));
  } catch (e) {
    return "throws " + e.name + " (" + e.message + ") of " + realmOf(e);
  }
}
function one(label, code, a, b) {
  var r0 = guard(code, a, b), r1 = guard(code, a, b);
  var line = label + ": " + r0;
  if (r0 !== r1) { bad++; line += " MISMATCH " + r1; }
  out.push(line);
}

// code, right side and left side from each realm
for (var c = 0; c < 2; c++)
  for (var r = 0; r < 2; r++)
    for (var l = 0; l < 2; l++) {
      var C = R[names[c]], RR = R[names[r]], L = R[names[l]];
      var tag = names[c] + " code, " + names[r] + " right, " + names[l] + " left";
      one(tag + ", F", C.io, L.f, RR.F);
      one(tag + ", B", C.io, L.b, RR.B);
      one(tag + ", A of B", C.iostrict, L.b, RR.A);
      one(tag + ", Object", C.io, L.obj, RR.Object);
      one(tag + ", Array", C.io, L.arr, RR.Array);
      one(tag + ", Function", C.io, L.F, RR.Function);
      one(tag + ", primitive", C.io, 1, RR.F);
      one(tag + ", arrow", C.io, L.obj, RR.arrow());
      one(tag + ", arrow, primitive", C.io, 1, RR.arrow());
      one(tag + ", object", C.io, L.obj, RR.obj);
      one(tag + ", number", C.io, L.obj, 7);
    }

// a function whose chain goes through the Function.prototype of the other
// realm: that object has its own Symbol.hasInstance, the same built-in
// function of the other realm
for (var c = 0; c < 2; c++) {
  var C = R[names[c]], O = R[names[1 - c]];
  function H() { }
  var h = new H();
  Object.setPrototypeOf(H, O.fproto);
  one(names[c] + " code, chain through the other Function.prototype", C.io, h, H);
  one(names[c] + " code, chain through the other Function.prototype, primitive", C.io, 1, H);
  H.prototype = 3;
  one(names[c] + " code, chain through the other Function.prototype, prototype 3", C.io, h, H);
  var Hb = Object.create(O.fproto);
  one(names[c] + " code, not callable, the other Function.prototype", C.io, {}, Hb);
}

// a shadow in the other realm's function, seen from this realm's code
(function () {
  var O = R.other;
  var G = O.io.constructor("return function G() {}")();
  var g = new G();
  var r = [String(R.main.io(g, G))];
  Object.defineProperty(G, Symbol.hasInstance, { value: function () { return false; }, configurable: true });
  r.push(String(R.main.io(g, G)));
  delete G[Symbol.hasInstance];
  r.push(String(R.main.io(g, G)));
  out.push("shadow on a function of the other realm: " + r.join());
})();

// Object.prototype of the other realm gets a hasInstance: a function of the
// other realm still finds Function.prototype's first, a plain object of it
// finds the new one
(function () {
  var O = R.other;
  Object.defineProperty(O.oproto, Symbol.hasInstance, { value: function () { return true; }, configurable: true });
  var r = [guard(R.main.io, 1, O.F), guard(R.main.io, O.f, O.F), guard(R.main.io, 1, O.newObj()), guard(R.main.io, 1, {})];
  delete O.oproto[Symbol.hasInstance];
  out.push("Object.prototype of the other realm: " + r.join());
})();

// the messages of the errors, from this realm's code
(function () {
  var io = R.main.io;
  function P() { }
  P.prototype = null;
  function Q() { }
  Object.defineProperty(Q, Symbol.hasInstance, { value: 1 });
  function S() { }
  Object.defineProperty(S, Symbol.hasInstance, { get: function () { throw new RangeError("getter"); } });
  var cases = [
    ["right side a number", {}, 1], ["right side undefined", {}, undefined], ["right side a plain object", {}, {}],
    ["arrow", {}, function () { return () => 1; }()], ["bound arrow", {}, (() => 1).bind(null)],
    ["prototype null", {}, P], ["hasInstance not callable", {}, Q], ["hasInstance a throwing getter", {}, S],
    ["revoked proxy", {}, (function () { var r = Proxy.revocable(function () { }, {}); r.revoke(); return r.proxy; })()]
  ];
  for (var i = 0; i < cases.length; i++)
    one("message, " + cases[i][0], io, cases[i][1], cases[i][2]);
})();

// one function of each kind as the right side, many times: the answers of a
// loop that runs warm
(function () {
  function T() { }
  class U extends T { }
  var xs = [new T(), new U(), {}, [], 1, "s", null, undefined, Object.create(null), T, U];
  var rs = [T, U, Object, Array, Function, R.other.Object];
  var n = 0;
  for (var k = 0; k < 330; k++) {
    var x = xs[k % xs.length], y = rs[k % rs.length];
    if (x instanceof y) n += k % 7 + 1;
  }
  out.push("loop: " + n);
})();

out.push(bad + " differ");
rvExpect(R.main.io, R.main.iostrict);
out.join("\n");
