// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// Calls with more arguments than parameters in the register loop
// (engine/regvm-run.h, rv_enter_args): the frame starts past the extra
// arguments, which stay alive until the call returns (a WeakRef to an object
// only they hold finds it) and are freed by the return and by an
// exception that leaves the frame, caught one and three frames up; with
// locals, with no parameters, a method call, deep recursion, calls from C
// and spread calls.
(function () { "use strict";
var log = [];
function plain(a) { return a; }
function plain0() { return 5; }
function plainLoc(a, b) { var x = a, y = b, z = 3; return x + y + z; }
var wr;
function mk() { var o = { v: 1 }; wr = new WeakRef(o); return o; }
// A WeakRef keeps its target to the end of the job: clearKeptObjects of the
// test host ends that hold, as the end of a job does, so that only the extra
// argument holds the object here, and after the call nothing does
function seesExtra(a) { clearKeptObjects(); var o = wr.deref(); return o === undefined ? "gone" : "alive" + o.v; }
function goneAfter() { clearKeptObjects(); return wr.deref() === undefined ? "gone" : "kept"; }
function thrower(a) { throw new Error("t" + a); }
function mid1(a) { return thrower(a, {}, "x", [1]); }
function mid2(a) { return mid1(a, 1, 2); }
function catch1(a) { try { return thrower(a, {}, [], "s"); } catch (e) { return "c1:" + e.message; } }
function catch3(a) { try { return mid2(a, "e", {}); } catch (e) { return "c3:" + e.message; } }
function deep(n) { return n === 0 ? 0 : 1 + deep(n - 1, n, {}, "x"); }
function deepMix(n) { return n === 0 ? 0 : (n % 2 ? deepMix(n - 1) : deepMix(n - 1, {}, n)) + 1; }
var obj = { k: 3, m: function (a) { return this.k + a; } };
function callM() { return obj.m(1, 2, 3); }

function run() {
  var out = [];
  out.push(plain(1, 2, 3), plain(), plain(1), plain0(1, 2), plain0(), plainLoc(1, 2, 3, 4, 5));
  out.push(seesExtra(1, mk()), goneAfter(), seesExtra(1, mk(), mk()), goneAfter());
  out.push(catch1(1), catch1("y"), catch3(1), catch3("z"));
  out.push(deep(3000), deepMix(3000), callM(), callM());
  out.push(String([6, 7].map(plain)), String([1].map(plainLoc)), plain.apply(null, [1, 2, 3]));
  out.push(Reflect.apply(plainLoc, null, [1, 2, 3, 4]), plain.call(null, 4, 5));
  out.push(plain(...[1, 2, 3]), plainLoc(...[1, 2], 3, ...[4]));
  return out.join("\n");
}

var res = run();
log.push(res + "\n--\n" + run());
rvExpect(plain, plain0, plainLoc, seesExtra, goneAfter, thrower, mid1, mid2, catch1, catch3, deep, deepMix,
         callM, run);
return log.join("\n");
})()
