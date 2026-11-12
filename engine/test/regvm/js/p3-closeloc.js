// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// A new binding of a let at each turn of a loop in the register loop
// (close_loc): each closure keeps the value of its own turn, the closures of
// one turn share it, a write after the closure is made is seen by the
// closures of that turn only, and a closure of the last turn sees the local
// the frame still has when it returns.
(function () {
  "use strict";
  function perTurn(n) {
    let prev = null;
    for (let i = 0; i < n; i++) {
      const p = prev;
      prev = function prevx() { return i + (p ? p() : 0); };
    }
    return prev();
  }
  function shareTurn(n) {
    let last = null;
    let j = 0;
    while (j < n) {
      let v = j + 100;
      function get() { return v; }
      function set(x) { v = x; }
      const p = last;
      last = function lastx() { return get() + (p ? p() : 0); };
      set(v + 1);
      j = 1 + j;
    }
    return last();
  }
  function lastTurn(n) {
    let f = null;
    let k = 0;
    while (k < n) {
      let w = k;
      f = function fx() { return w; };
      w = 100 + w;
      k = 1 + k;
    }
    return f();
  }
  return function () {
    let out = "";
    let turn = 0;
    while (turn < 3) {
      out = out + perTurn(4) + ";" + shareTurn(3) + ";" + lastTurn(2) + "|";
      turn = 1 + turn;
    }
    return out;
  };
})()
