// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// Calls whose result is dropped (CALLD, CALLMD, CALLMLD in
// engine/regvm-ops-basic.h): a call statement, and a call right before the
// end of a function (its return_undef). The callee runs in the loop (its
// return frees the result, RV_DST_DROP), with more arguments than it has or
// reading its arguments (a frame of rv_enter_args), out of the loop (a C
// function, a bound function, a function not converted), and throws; the
// results have reference counts (objects, strings), so a result freed twice
// or never shows in the counts of the runtime.
(function () {
  "use strict";
  var log = [];
  var made = 0;
  function mk(x) { made++; return { x: x, s: "v" + x }; }
  function two(a, b) { made++; return [a, b]; }
  function many() { made++; return arguments.length + "/" + arguments[0]; }
  function thrower(x) { if (x > 1) throw new RangeError("t" + x); return { t: x }; }
  var bound = mk.bind(null, 7);
  var obj = {
    n: 0,
    m: function (x) { this.n++; return { m: x }; },
    c: function (x) { this.n++; return "s" + x; }
  };
  function id(x) { return x; }

  // statements: CALLD to a function of the loop, a C function, extras
  function stmts(k) {
    mk(k);
    two(k, k + 1);
    two(k, k + 1, k + 2, { extra: k });
    many(k, "a", "b");
    Math.max(k, 3);
    String(k);
    bound();
    [k].concat([k]);
    id(mk)(k);
    return made;
  }
  // CALLMD: the this in a temp; CALLMLD: the this a local
  function methods(o, k) {
    obj.m(k);
    obj.c(k);
    id(obj).m(k);
    o.m(k);
    o.c(k);
    o.m(k, k, k);
    [1, 2, 3].map(id);
    o.toString();
    return o.n;
  }
  // the call right before the end: its result dropped by the call
  function tailish(o, k) { o.m(k); }
  function tailish2(k) { mk(k); }
  function tailish3(k) { Math.min(k, 1); }
  // a temp below the call when it throws, the result of the calls before
  // dropped
  function throws(k) {
    var r = [];
    try { mk(k); thrower(k); r.push("no"); } catch (e) { r.push(e.message); }
    try { id(k) + thrower(k); } catch (e) { r.push(e.message); }
    try { obj.m(k); k.nothere(); } catch (e) { r.push(e.constructor.name); }
    try { (void 0)(); } catch (e) { r.push(e.constructor.name); }
    return r.join(",");
  }
  // deep: dropped results through recursion
  function rec(n) {
    if (n <= 0) return { end: n };
    rec(n - 1);
    mk(n);
    return { n: n };
  }

  for (var rep = 0; rep < 3; rep++) {
    log.push(stmts(rep), methods(obj, rep), throws(rep));
    log.push(String(tailish(obj, rep)), String(tailish2(rep)), String(tailish3(rep)));
    log.push(JSON.stringify(rec(5 + rep)));
  }
  log.push(made, obj.n);
  rvExpect(stmts, methods, tailish, tailish2, tailish3, throws, rec, mk, two, many, thrower);
  return log.join("\n");
})()
