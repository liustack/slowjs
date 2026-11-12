// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// Async generators and for await in the register loop
// (engine/regvm-ops-gen.h): yield and await in one body, requests queued
// before the first one settles, return() and throw() through try and
// finally, yield* of an async generator and of a sync iterable, and for
// await over async generators, sync iterables and promises, with break,
// a throw in the body and a rejection.
// jsrun: -result
(function () { "use strict";
var log = [];
function note(x) { log.push(x); return x; }
function later(v) { return new Promise(function (res) { Promise.resolve().then(function () { res(v); }); }); }
async function* gen(n) {
  try {
    for (var i = 0; i < n; i++) {
      var got = yield await later(i * 10);
      note("got" + got);
    }
  } finally { note("genfin" + n); await later(0); }
  return "end" + n;
}
async function* deleg(n) { var r = yield* gen(n); note("r" + r); yield* [7, 8]; return r; }
async function* thrower() {
  try { yield 1; yield 2; } catch (e) { note("in" + e); yield "c" + e; }
}
async function drain(g, sends) {
  var out = [], r, i = 0;
  for (;;) {
    r = await g.next(sends ? sends[i] : undefined);
    i++;
    out.push(r.value);
    if (r.done) break;
  }
  return out.join(" ");
}
async function queued() {
  var g = gen(3), ps = [g.next("a"), g.next("b"), g.return("q"), g.next("c")];
  var rs = await Promise.all(ps);
  return rs.map(function (r) { return r.value + ":" + r.done; }).join(" ");
}
async function throws() {
  var g = thrower(), out = [(await g.next()).value];
  out.push((await g.throw("x")).value, (await g.next()).done);
  g = thrower();
  try { await g.throw("early"); } catch (e) { out.push("early" + e); }
  return out.join(" ");
}
async function forAwait() {
  var s = "", x;
  for await (x of gen(3)) s = s + x + ".";
  for await (x of [later(1), 2, later(3)]) s = s + x + ",";
  for await (x of gen(5)) { if (x === 20) break; s = s + x + ";"; }
  try { for await (x of gen(4)) { if (x === 10) throw "body"; } } catch (e) { s = s + e; }
  try { for await (x of [1, Promise.reject("rej")]) s = s + x; } catch (e) { s = s + e; }
  return s;
}
async function all() {
  return [await drain(gen(2), [undefined, "x", "y"]), await drain(deleg(2)), await queued(),
          await throws(), await forAwait()].join(" | ");
}
var runs = [], k;
for (k = 0; k < 3; k++) runs.push(all());
Promise.all(runs).then(function (r) {
  rvExpect(gen, deleg, thrower, drain, queued, throws, forAwait, all);
  globalThis.result = r.join(" || ") + " || " + log.join(" ");
});
})();
