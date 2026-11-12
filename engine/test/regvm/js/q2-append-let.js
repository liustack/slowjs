// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// An add stored into a let that a closure writes (put_var_ref_check, ADDVRC
// in engine/regvm-ops-closure.h): strings that grow in place, ints, ints
// that overflow, objects whose valueOf runs JS, and the store before the let
// is initialized, which throws a ReferenceError after the add ran (its
// valueOf counted), with the stack of the store. The value also kept on the
// stack (dup then put_var_ref_check).
(function () {
  "use strict";
  var log = [];
  var calls = 0;
  var inner = {};
  function build(n) {
    let s = "";
    const add = x => { s += x; };
    inner.build = add;
    for (let i = 0; i < n; i++) add(i % 7 ? "ab" : "é");
    return s;
  }
  function count(n) {
    let c = 2147483640;
    const inc = x => { c = c + x; };
    inner.count = inc;
    for (let i = 0; i < n; i++) inc(1);
    return c;
  }
  function kept(n) {
    let s = "";
    const add = x => (s += x);
    inner.kept = add;
    let last;
    for (let i = 0; i < n; i++) last = add("xy");
    return last.length + ":" + s.length;
  }
  function objs() {
    let v = 1;
    const add = x => { v += x; };
    inner.objs = add;
    add({ valueOf() { calls++; return 5; } });
    add("s");
    add(2n === 2n ? 3 : 0);
    return v;
  }
  function early(o) {
    const add = () => { t = o + "z"; };
    const addK = () => (t += o);
    inner.early = add; inner.earlyK = addK;
    var r = [];
    try { add(); } catch (e) { r.push(e.constructor.name + ":" + e.message + ":" + (e.stack || "").split("\n")[1].trim()); }
    try { addK(); } catch (e) { r.push(e.constructor.name + ":" + e.message); }
    let t = "init";
    add(); r.push(t);
    return r.join("|");
  }
  function early2(o) {
    const add = () => { t = 1 +
      o; };
    inner.early2 = add;
    var r;
    try { add(); } catch (e) { r = e.constructor.name + ":" + (e.stack || "").split("\n")[1].trim(); }
    let t = 0;
    return r;
  }

  for (var rep = 0; rep < 3; rep++) {
    var s = build(3000);
    log.push(s.length, s.slice(0, 20), s.slice(-20), count(20), kept(500), objs(), calls);
    calls = 0;
    log.push(early("x"), calls, early({ valueOf() { calls++; return "v"; } }), calls);
    log.push(early2(3), early2({ valueOf() { calls++; return 2; } }), calls);
  }
  rvExpect(build, count, kept, objs, early, early2, inner.build, inner.count, inner.kept,
           inner.objs, inner.early, inner.earlyK, inner.early2);
  return log.join("\n");
})()
