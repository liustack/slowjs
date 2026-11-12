// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// Plain generators in the register loop (engine/regvm-ops-gen.h): yields
// with values under them on the stack, the values sent back, return() and
// throw() into try, catch and finally, locals that closures capture and
// change while the generator is suspended, arguments past the parameters,
// this, and generators that resume one another.
(function () { "use strict";
var log = [];
function note(x) { log.push(x); return x; }
function* count(n) { for (var i = 0; i < n; i++) yield i; return "end" + n; }
function* sums(a) {
  var t = a;
  while (true) {
    var x = yield t;
    if (x === undefined) return t;
    t = t + x * 10 + (yield x + t);
  }
}
function* deep(a, b) {
  // values of the expression wait on the stack across each yield
  var r = [a, (yield a + 1), b + (yield b + 2), [a, b, (yield "c")]];
  return r.join(",");
}
function* fin(n) {
  try {
    try { yield n; yield n + 1; } finally { note("in" + n); yield "f" + n; }
  } catch (e) { note("caught" + e); yield "c" + e; return "after" + e; }
  finally { note("out" + n); }
  return "done" + n;
}
function* cells() {
  var a = 1, b = "x";
  var get = function () { return a + b; };
  var set = function (v) { a = v; b = b + v; };
  yield get;
  yield set;
  yield a + b;
  a = a * 100;
  return get();
}
function* args(p, q) { yield p; yield q; yield this === undefined ? "u" : this.tag; }
function* inner(n) { var s = 0; for (var i = 0; i < n; i++) s = s + (yield i); return s; }
function* outer(n) {
  var g = inner(n), r = g.next(), out = [];
  while (!r.done) { out.push(r.value); r = g.next(yield r.value * 2); }
  return out.join("") + ":" + r.value;
}
function drain(g, sends) {
  var out = [], r, i = 0;
  for (;;) {
    r = g.next(sends ? sends[i] : undefined);
    i++;
    out.push(r.value);
    if (r.done) break;
  }
  return out.join(" ");
}
function forOf(g) { var s = ""; for (var v of g) s = s + v + "."; return s; }
function withThrow() {
  var g = fin(1), out = [g.next().value];
  out.push(g.throw("e1").value, g.next().value, g.next().done);
  g = fin(2);
  out.push(g.next().value, g.next().value);
  out.push(g.throw("e2").value);   // into the finally block's yield
  try { g.next(); } catch (e) { out.push("rethrown" + e); }
  return out.join(" ");
}
function withReturn() {
  var g = fin(3), out = [g.next().value, g.return("r").value, g.next().value, g.next().done];
  g = count(4);
  out.push(g.next().value, g.return("early").value, g.next().done);
  g = count(2);
  out.push(g.return("start").value, g.next().done);
  return out.join(" ");
}
function withCells() {
  var g = cells(), get = g.next().value, set = g.next().value;
  var out = [get()];
  set(7);
  out.push(get(), g.next().value);
  set(9);
  out.push(g.next().value, get());
  return out.join(" ");
}
function* keep() { var v = 1; var me = yield function () { v = v + 1; return v; }; yield v + (me ? 100 : 0); }
function dropped() {
  // the closure keeps the suspended frame alive through its var_ref, and
  // a cycle through the generator is collected by gc()
  var f = keep().next().value, g = keep(), h;
  g.next();
  h = g.next(g).value;
  gc();
  return [f(), f(), h].join(",");
}
function withArgs() {
  var o = { tag: "T" }, g = args.call(o, 1, 2, 3, 4);
  return drain(g) + " / " + drain(args(5));
}
return function () {
  var r = [], k;
  for (k = 0; k < 3; k++) {
    r.push(drain(count(3)), drain(sums(1), [undefined, 2, 3, 4, undefined]),
           drain(deep(1, 2), [undefined, "A", "B", "C"]), withThrow(),
           withReturn(), withCells(), withArgs(), dropped(), forOf(count(4)),
           drain(outer(3), [undefined, 1, 2, 3]));
  }
  rvExpect(count, sums, deep, fin, cells, args, inner, outer, drain, forOf,
           withThrow, withReturn, withCells, withArgs, keep, dropped);
  return r.join(" | ") + " | " + log.join(" ");
};
})()
