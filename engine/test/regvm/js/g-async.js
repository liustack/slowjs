// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// Async functions in the register loop (engine/regvm-ops-gen.h): await of
// values, of promises that resolve and of ones that reject, inside loops,
// try, catch and finally, with values under the await on the stack, locals
// captured by closures that change them while the function waits, an async
// function that never awaits, and async functions that await one another.
// jsrun: -result
(function () { "use strict";
var log = [];
function note(x) { log.push(x); return x; }
function later(v) { return new Promise(function (res) { Promise.resolve().then(function () { res(v); }); }); }
function fail(e) { return Promise.reject(e); }
async function plain(a) { return a + 1; }
async function one(a) { var b = await a; return b * 2; }
async function stack(a) { return [a, await later(a + 1), a + (await a + 2), [await "x", a]].join(","); }
async function loop(n) {
  var s = 0;
  for (var i = 0; i < n; i++) s = s + (await later(i)) * 10 + await i;
  return s;
}
async function caught(v) {
  try { await fail(v); note("not here"); }
  catch (e) { note("caught" + e); return await later("c" + e); }
  finally { note("fin" + v); await later(0); note("fin2" + v); }
}
async function rejects(v) { await later(1); throw "r" + v; }
async function cells() {
  var a = 1;
  var bump = function () { a = a * 10; };
  later(0).then(bump);
  await later(0);
  await later(0);
  return a;
}
async function chain(n) {
  if (n === 0) return "base";
  var r = await chain(n - 1);
  return r + n;
}
async function all() {
  var out = [];
  out.push(await plain(1), await one(3), await one(later(4)), await stack(1), await loop(3));
  out.push(await caught("e"));
  try { await rejects(2); } catch (e) { out.push(e); }
  out.push(await cells(), await chain(4));
  return out.join(" ");
}
var runs = [], k;
for (k = 0; k < 3; k++) runs.push(all());
Promise.all(runs).then(function (r) {
  rvExpect(plain, one, stack, loop, caught, rejects, cells, chain, all);
  globalThis.result = r.join(" | ") + " | " + log.join(" ");
});
})();
