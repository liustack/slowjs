// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// An exception that unwinds many frames with captured locals at once, in the
// register loop and, past the register stack, through calls of the old
// loop: each frame closes its var_refs before its slots are freed, so the
// closure kept from the deepest frame still reads its value. Then a chain of
// closures made on the way back up a recursion.
function tryCall(f, a, b) { try { return f(a, b); } catch (e) { return e.constructor.name + ": " + e.message; } }
(function () {
  "use strict";
  function down(n, h, o) {
    var v = n;
    function g() { return v; }
    h.k = g;
    if (n < 1) return o.x.y;
    h.k = g;
    var r = down(n - 1, h, o) + 1;
    v = 100;
    return r;
  }
  function collect(n, h) {
    var v = n;
    function g() { return v; }
    if (n < 1) { h.prev = g; return 0; }
    collect(n - 1, h);
    var p = h.prev;
    function g2() { return v + p(); }
    h.prev = g2;
    return 0;
  }
  return function () {
    var h = {}, out = "";
    out = out + tryCall(down, 50, h) + "," + h.k() + ";";
    out = out + tryCall(down, 3000, h, {}) + "," + h.k() + ";";
    var h2 = {};
    collect(300, h2);
    out = out + h2.prev();
    return out;
  };
})()
