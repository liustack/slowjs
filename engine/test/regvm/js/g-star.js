// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// yield* in the register loop (engine/regvm-ops-gen.h, the iterator ops of
// W_GEN): delegation to a generator and to iterators made by hand, the
// values sent through it, return() and throw() passed on to the inner
// iterator, an inner iterator with no return or throw method (the
// TypeError of throw_error), results that are not objects, and the value
// of a finished delegation.
(function () { "use strict";
var log = [];
function note(x) { log.push(x); return x; }
function* inner(n) {
  try { for (var i = 0; i < n; i++) note("got" + (yield i)); }
  finally { note("innerfin" + n); }
  return "in" + n;
}
function* outer(n) { var a = yield* inner(n); var b = yield* inner(1); return a + b; }
function hand(n, withReturn, withThrow) {
  var i = 0, it = {
    next: function (v) { note("next" + v); i++; return { done: i > n, value: i * 10 }; },
    [Symbol.iterator]: function () { return this; }
  };
  if (withReturn)
    it.return = function (v) { note("ret" + v); return { done: true, value: "R" + v }; };
  if (withThrow)
    it.throw = function (v) { note("thr" + v); return { done: v === "stop", value: "T" + v }; };
  return it;
}
function* overHand(it) { var r = yield* it; note("res" + r); return r; }
function bad() {
  return { next: function () { return 5; }, [Symbol.iterator]: function () { return this; } };
}
function* overBad() { yield* bad(); }
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
function cases() {
  var out = [], g;
  out.push(drain(outer(2), [undefined, "a", "b", "c"]));
  out.push(drain(overHand(hand(3, false, false)), [undefined, 1, 2, 3]));
  g = overHand(hand(5, true, false));
  out.push(g.next().value, g.return("x").value, g.next().done);
  g = overHand(hand(5, false, true));
  out.push(g.next().value, g.throw("go").value, g.throw("stop").done);
  g = overHand(hand(5, true, false));
  g.next();
  try { g.throw("nothrow"); } catch (e) { out.push(e instanceof TypeError ? "TypeError" : e); }
  g = overHand(hand(5, false, false));
  out.push(g.next().value, g.return("plain").value);
  g = outer(3);
  out.push(g.next().value, g.next("p").value, g.return("q").value, g.next().done);
  g = outer(3);
  g.next();
  try { g.throw("boom"); } catch (e) { out.push("caught" + e); }
  try { overBad().next(); } catch (e) { out.push(e instanceof TypeError ? "TypeError" : e); }
  return out.join(" ");
}
return function () {
  var r = [], k;
  for (k = 0; k < 3; k++)
    r.push(cases());
  rvExpect(inner, outer, overHand, overBad, drain, cases);
  return r.join(" | ") + " | " + log.join(" ");
};
})()
