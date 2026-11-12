// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// rvdiff: with statements in the register loop. A name inside with is
// looked up in the object first (with_get_var, with_put_var,
// with_delete_var, with_get_ref for a call with the object as this), then
// in the scopes around it: a local, a closure variable, a global, a name
// that is nowhere. Symbol.unscopables hides a name, a getter and a proxy
// see each lookup in the order of the old loop, and a getter that throws
// throws from inside the with. A strict function inside a with reads and
// writes the object of the with through its closure: a name that goes away
// between the two lookups of a proxy is a ReferenceError in strict code
// and undefined in sloppy code, and a store to a read-only property throws
// in strict code only. A store, a compound assignment, an update and a
// destructuring target inside with make a reference (make_loc_ref,
// make_arg_ref, make_var_ref_ref, make_var_ref for a global) and read and
// write it with get_ref_value and put_ref_value. This file is sloppy.
// The functions wN run several times, so that their later calls run in
// the register loop.
(function () {
var out = [];
function p(x) { out.push(String(x)); }
function ck(f, a, b, c) {
  try { return "ok " + f(a, b, c); } catch (e) { return e.name + ": " + e.message; }
}
var outer = "outer";
var log = [];
function traced(o) {
  return new Proxy(o, {
    has(t, k) { log.push("has " + String(k)); return k in t; },
    get(t, k, r) { log.push("get " + String(k)); return Reflect.get(t, k, r); },
    set(t, k, v, r) { log.push("set " + String(k)); return Reflect.set(t, k, v, r); },
    deleteProperty(t, k) { log.push("delete " + String(k)); return Reflect.deleteProperty(t, k); }
  });
}
// a proxy whose has says yes once, then no
function flaky(o) {
  var n = 0;
  return new Proxy(o, { has(t, k) { return k === "x" ? n++ === 0 : k in t; } });
}

function wGet(o) { with (o) { return x; } }
function wOuter(o) { with (o) { return outer; } }
function wLocal(o) { var y = "local"; with (o) { return y; } }
function wNone(o) { with (o) { return nowhere; } }
function wTypeof(o) { with (o) { return typeof nowhere + typeof x; } }
function wPut(o, v) { var x = "local x"; with (o) { x = v; } return x + "/" + o.x; }
function wDel(o) { with (o) { return delete x; } }
function wCall(o) { with (o) { return f(1); } }
function wNested(o, q) { with (o) { with (q) { return x + y; } } }
function wLoop(arr) { var s = 0; for (var i = 0; i < arr.length; i++) { with (arr[i]) { s = s + x; } } return s; }
function wForIn(o) { var r = []; with (o) { for (x in { a: 1, b: 2 }) r.push(x); } return r.join() + "/" + o.x; }
function wThrow(o) { with (o) { return x; } }
function wBelow(o, k) { with (o) { return [k + "a", k + "b", x]; } }
function wStrictGet(o) {
  with (o) { return (function () { "use strict"; return x; })(); }
}
function wStrictPut(o, v) {
  with (o) { (function () { "use strict"; x = v; })(); }
  return o.x;
}
function wSloppyPut(o, v) {
  with (o) { (function () { x = v; })(); }
  return o.x;
}
function wAdd(o, v) { var x = 10; with (o) { x += v; x++; } return x + "/" + o.x; }
function wPattern(o, v) { var x = 0; with (o) { [x, y] = [v, v + 1]; } return x + "/" + o.x + "/" + o.y; }
function wGlobal(o, v) { with (o) { wGlobalName = v; } var r = wGlobalName; delete globalThis.wGlobalName; return r; }
function wStrictNone(o, v) {
  with (o) { return (function () { "use strict"; nowhereStrict = v; })(); }
}
function wArg(o, x) { with (o) { x = x + 1; } return x + "/" + o.x; }
function wCapture(o, v) { var x = "x"; function get() { return x; } with (o) { x = v; } return get() + "/" + o.x; }

var hasX = { x: 1, f: function (a) { return (this === hasX) + ":" + a; } };
var noX = { z: 1 };
for (var i = 0; i < 4; i++) {
  p(wGet(hasX)); p(ck(wGet, noX));
  p(wOuter(noX)); p(wOuter({ outer: "shadow" }));
  p(wLocal(noX)); p(wLocal({ y: "obj y" }));
  p(ck(wNone, noX)); p(wNone({ nowhere: 2 }));
  p(wTypeof(noX)); p(wTypeof(hasX));
  p(wPut({ x: 0 }, i)); p(wPut({}, i));
  p(wDel({ x: 1 })); p(wDel(noX)); p(wDel(Object.freeze({ x: 1 })));
  p(wCall(hasX)); p(ck(wCall, noX));
  p(wNested({ x: "a" }, { y: "b" })); p(wNested({ x: "a", y: "c" }, { x: "d" }));
  p(wLoop([{ x: 1 }, { x: 2 }, { x: 3 }]));
  p(wForIn({ x: 0 }));
  p(ck(wGet, { x: 1, [Symbol.unscopables]: { x: true } }));
  p(ck(wThrow, { get x() { throw new RangeError("getter " + i); } }));
  p(ck(wBelow, { get x() { throw new RangeError("below " + i); } }, "k" + i)); p(wBelow({ x: i }, "k"));
  log = []; p(wGet(traced({ x: 5 }))); p(log.join());
  log = []; p(wPut(traced({ x: 0 }), 7)); p(log.join());
  log = []; p(wDel(traced({ x: 0 }))); p(log.join());
  log = []; p(wCall(traced(hasX))); p(log.join());
  p(ck(wStrictGet, flaky({ x: 1 })));
  p(ck(wStrictPut, flaky({ x: 1 }), 2));
  p(ck(wStrictPut, Object.freeze({ x: 1 }), 3));
  p(ck(wSloppyPut, Object.freeze({ x: 1 }), 4));
  p(ck(wStrictGet, { x: "plain" }));
  p(wAdd({ x: 1 }, i)); p(wAdd({}, i));
  log = []; p(wAdd(traced({ x: 1 }), i)); p(log.join());
  p(wPattern({ x: 0, y: 0 }, i)); p(wPattern({}, i));
  p(wGlobal(noX, i)); p(wGlobal({ wGlobalName: "own" }, i));
  p(ck(wStrictNone, noX, i)); p(ck(wStrictNone, flaky({ nowhereStrict: 1 }), i));
  p(wArg({ x: 5 }, i)); p(wArg({}, i));
  p(wCapture({ x: 0 }, i)); p(wCapture({}, i));
  p(ck(wAdd, Object.freeze({ x: 1 }), i));
}
rvExpect(wGet, wOuter, wLocal, wNone, wTypeof, wDel, wCall, wNested, wForIn, wThrow, wBelow,
         wStrictGet, wStrictPut, wSloppyPut, wPut, wLoop,
         wAdd, wPattern, wGlobal, wArg, wCapture);
return out.join(";");
})()
