// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// Object literals made in the register loop (W_OBJECT_LIT,
// engine/regvm-ops-window.h): the empty literal, a literal run the first
// time (its shape is made then) and again, values of every kind of operand,
// a literal whose shape grew in place after it was made, one with more
// properties than fit in the block of the object, nested literals, and
// enough of them in a loop that the collector runs while they are made.
(function () {
  "use strict";
  var out = [];
  function empty() { return {}; }
  function pair(a, b) { return { a: a, b: b }; }
  function kinds(a, s) {
    var t = a * 2;
    return { i: a, f: a + 0.5, s: s, t: t, n: null, u: undefined, b: true,
             o: { x: a }, arr: [a, t], k: 7, str: "lit" };
  }
  function show(o) { return JSON.stringify(o) + " " + Object.keys(o).join(","); }
  out.push("empty " + show(empty()) + " " + (empty() !== empty()));
  for (var i = 0; i < 3; i++) out.push("pair " + show(pair(i, "v" + i)));
  out.push("kinds " + show(kinds(3, "q")) + " " + show(kinds(-1.5, "")));
  // the shape of a literal grown in place, then more literals of it
  function grow(v) { return { g1: v, g2: v + 1 }; }
  var g = grow(1);
  g.g3 = 3;
  g.g4 = 4;
  var h = grow(10);
  h.g5 = 5;
  out.push("grow " + show(g) + " " + show(h) + " " + show(grow(20)));
  // more properties than the block of the object holds
  function wide(a) {
    return { p00: a, p01: a, p02: a, p03: a, p04: a, p05: a, p06: a, p07: a,
             p08: a, p09: a, p10: a, p11: a, p12: a, p13: a, p14: a, p15: a,
             p16: a, p17: a, p18: a, p19: a, p20: a, p21: a, p22: a, p23: a,
             p24: a, p25: a, p26: a, p27: a, p28: a, p29: a, p30: a, p31: a,
             p32: a, p33: a, p34: a, p35: a, p36: a, p37: a, p38: a, p39: a };
  }
  var w = wide("w");
  out.push("wide " + Object.keys(w).length + " " + w.p00 + w.p39 + " " + Object.keys(wide(1)).length);
  // a chain of literals, so that the collector runs while they are made
  var head = null, sum = 0;
  for (var j = 0; j < 60000; j++) {
    head = { v: j, next: head, tag: { t: j & 7 } };
    if ((j & 1023) === 0) head.next = null;
  }
  for (var p = head; p; p = p.next) sum += p.v + p.tag.t;
  out.push("chain " + sum);
  // literals as arguments and in expressions
  function area(r) { return r.w * r.h; }
  var total = 0;
  for (var k = 0; k < 100; k++) total += area({ w: k, h: 2 });
  out.push("area " + total);
  // a literal whose values are its own temps, reused
  function swap(o) { return { a: o.b, b: o.a }; }
  out.push("swap " + show(swap(swap({ a: 1, b: 2 }))));
  return out.join("\n");
})();
