// Copyright (c) 2026 The liustack/tifajs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.
// The shapes that objects of new C() go through, from the empty shape of
// C.prototype to the last one, stay alive while an object of the last one
// lives (engine/quickjs.c, js_shape_adopt): they must keep nothing alive that
// the objects do not. A WeakRef to the prototype, to the constructor, to a
// symbol key, and to an object of a cycle with the constructor finds each
// gone at the same point as before: when the last object goes (freed at
// once, by the cycle collector, or by the young collection), when the
// prototype is replaced, or when an object leaves the shape of the others.
// The blocks of the heap in use are the same before the objects and after
// they go. And constructors and prototypes of another realm (newRealm).
(function () {
var log = [];
// a WeakRef holds its target to the end of the job: clearKeptObjects ends it
function state(wr, collect) { clearKeptObjects(); if (collect) collect(); return wr.deref() === undefined ? "gone" : "alive"; }

function make(n, keep, sym) {
  function F(v) { this.a = v; this.b = v; if (sym) this[sym] = v; this.c = v; }
  var objs = [];
  for (var i = 0; i < n; i++) { var o = new F(i); if (i % keep === 0) objs.push(o); }
  return { F: F, objs: objs };
}

// the prototype and the constructor go with the last object
(function () {
  var m = make(50, 1), wp = new WeakRef(m.F.prototype), wf = new WeakRef(m.F);
  var proto = m.F.prototype;
  m.F = null;
  proto.constructor = null;
  proto = null;
  log.push("objects alive: " + state(wp) + " " + state(wf));
  m.objs.length = 1;
  log.push("one object: " + state(wp));
  m.objs.length = 0;
  log.push("no object: " + state(wp) + " " + state(wf));
})();

// the prototype replaced while the constructor lives on
(function () {
  var m = make(50, 5), wp = new WeakRef(m.F.prototype);
  m.F.prototype.constructor = null;
  m.F.prototype = { other: 1 };
  log.push("replaced, objects alive: " + state(wp));
  m.objs.length = 0;
  log.push("replaced, no object: " + state(wp) + " next " + Object.keys(new m.F(1)));
  var w2 = new WeakRef(m.F.prototype);
  Object.defineProperty(m.F, "prototype", { value: { third: 1 } });
  log.push("defined, no object: " + state(w2) + " next " + Object.keys(new m.F(2)));
})();

// a symbol that only objects of a constructor have as a key
(function () {
  var s = Symbol("only"), ws = new WeakRef(s);
  var m = make(30, 3, s);
  s = null;
  m.F.prototype.constructor = null;
  m.F = null;
  log.push("symbol, objects alive: " + state(ws) + " " + m.objs[0][Object.getOwnPropertySymbols(m.objs[0])[0]]);
  m.objs.length = 0;
  log.push("symbol, no object: " + state(ws));
})();

// an object that leaves the shape of the others: another prototype, a delete
(function () {
  var m = make(20, 1), wp = new WeakRef(m.F.prototype), other = {};
  m.F.prototype.constructor = null;
  for (var i = 1; i < m.objs.length; i++) delete m.objs[i].c;
  Object.setPrototypeOf(m.objs[0], other);
  m.F = null;
  log.push("left the shape: " + state(wp));
  m.objs.length = 0;
  log.push("left, no object: " + state(wp));
})();

// a cycle: the objects, their constructor and its prototype hold each other
function cycle(collect) {
  function C(v) { this.a = v; this.self = this; this.ctor = C; }
  var objs = [];
  for (var i = 0; i < 40; i++) objs.push(new C(i));
  C.all = objs;
  var wp = new WeakRef(C.prototype), wc = new WeakRef(C);
  objs = null;
  C = null;
  log.push(collect + ", before: " + state(wp) + " " + state(wc));
  var run = collect === "gc" ? gc : youngGC;
  log.push(collect + ", after: " + state(wp, run) + " " + state(wc));
}
cycle("gc");
cycle("youngGC");

// the heap holds as many blocks after the objects as before them
(function () {
  function G(v) { this.p = v; this.q = v; this.r = v; this.s = v; }
  function fill(n) {
    var objs = [];
    for (var i = 0; i < n; i++) objs.push(new G(i));
    for (var i = 0; i < n; i++) new G(i);
    objs.length = 0;
  }
  // what a first call makes and keeps (the register code of fill and G, and
  // what G keeps for the objects it makes) is made before the count, so
  // that the count sees the objects alone, with the system malloc too
  fill(1);
  gc();
  var before = memoryBlocks();
  fill(200);
  gc();
  log.push("blocks after the objects: " + (memoryBlocks() - before));
})();

// constructors and prototypes of another realm
(function () {
  var r = newRealm("({ K: function K(v) { this.x = v; this.y = v; }, P: Object.create(null), O: {} })");
  var objs = [];
  for (var i = 0; i < 6; i++) objs.push(new r.K(i));
  function L(v) { this.x = v; this.y = v; }
  L.prototype = r.P;
  for (var i = 0; i < 6; i++) objs.push(new L(i));
  r.K.prototype = r.O;
  for (var i = 0; i < 3; i++) objs.push(new r.K(i));
  log.push("realm: " + objs.map(function (o) {
    var p = Object.getPrototypeOf(o);
    return Object.keys(o).join("") + (p === r.P ? "P" : p === r.O ? "O" : "K");
  }).join(" "));
})();

return log.join("\n");
})()
