// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// The mapped arguments object of a sloppy function in the register loop
// (engine/regvm-ops-args.h): arguments[i] and the parameter i are one
// binding while the call runs, for the arguments passed (not for the
// missing ones), written either way, read after a write through the other,
// kept after the call (the binding is closed, each side its own value), with
// extra arguments, its callee, a delete and a defineProperty that end the
// binding, and a throw with it alive.
(function () {
var log = [];
function show(v) {
  if (v && typeof v === "object") {
    var out = [];
    for (var i = 0; i < v.length; i++) out.push(show(v[i]));
    return "[" + out.join(",") + "]";
  }
  return String(v);
}
function viaArgs(a, b) { arguments[0] = 10; return a + b; }
function viaParam(a, b) { a = 5; b = 6; return [arguments[0], arguments[1], arguments.length]; }
function missing(a, b) { b = 9; arguments[1] = 8; return [a, b, arguments[1], arguments.length]; }
function extra(a) { arguments[2] = "x"; a = "y"; return [a, arguments[0], arguments[1], arguments[2], arguments.length]; }
function keep(a) { return arguments; }
function keepThenWrite(a) { var g = arguments; a = 2; return g; }
function callee() { return arguments.callee === callee; }
function del(a) { delete arguments[0]; a = 3; return [a, arguments[0]]; }
function def(a) { Object.defineProperty(arguments, 0, { value: 4, writable: false }); a = 5; return [a, arguments[0]]; }
function readLoop(a, b) { var s = 0; for (var i = 0; i < arguments.length; i++) { s = s + arguments[i]; a = a + 1; } return s + a; }
function pass(a) { arguments[0] = 7; return viaParam.apply(null, arguments); }
function thr(a) { arguments[0] = "t"; throw a; }
function catches(a) { try { return thr(a, {}, []); } catch (e) { return "c" + e; } }
function closure(a) { var f = function () { return a; }; arguments[0] = "cl"; return f(); }

function run() {
  var out = [];
  out.push(viaArgs(1, 2), viaArgs(1), show(viaParam(1, 2)), show(viaParam()), show(viaParam(1, 2, 3)));
  out.push(show(missing(1)), show(missing()), show(missing(1, 2)));
  out.push(show(extra(1)), show(extra(1, 2, 3, 4)));
  var k = keep(1, 2); k[0] = 3; out.push(show(k), k.length);
  out.push(show(keepThenWrite(1, "e")), show(keepThenWrite()));
  out.push(callee(), callee(1, 2));
  out.push(show(del(1)), show(def(1)));
  out.push(readLoop(1, 2), readLoop(1, 2, 3, 4), readLoop());
  out.push(show(pass(1, 2)), catches(1), catches(), closure(1));
  return out.join("\n");
}
var r = run();
log.push(r + "\n--\n" + run());
rvExpect(viaArgs, viaParam, missing, extra, keep, keepThenWrite, callee, del, def, readLoop, pass, thr, catches, closure, run);
return log.join("\n");
})()
