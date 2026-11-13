// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// rv-expect-op: DSTART DNEXT
// jsrun: -result
// Array destructuring with no iterator object (DSTART and DNEXT,
// regvm-ops-dst.h): every form of pattern, then what a script can change
// before, during (from a default value or a setter) and after one:
// Array.prototype[Symbol.iterator], the next and the return of the
// iterator prototypes (as values and as getters that log their reads), the
// array itself (push, pop, length, holes read through the prototype, an
// element far out that makes it slow), an own Symbol.iterator, a subclass,
// a Proxy, an array of another realm, a default that throws before and
// after the iterator is done, and a generator and an async function that
// stop inside a default.
(function () { "use strict";
var log = [];
function say(s) { log.push(s); }
function show(v) {
  if (Array.isArray(v)) return "[" + v.map(show).join(",") + "]";
  if (typeof v === "function") return "fn";
  if (v !== null && typeof v === "object") return "obj";
  return String(v);
}
function guard(f) {
  try { return show(f()); } catch (e) { return e.name + ":" + e.message; }
}

// the forms
function two(a) { const [x, y] = a; return [x, y]; }
function one(a) { const [x] = a; return [x]; }
function none(a) { const [] = a; return "ok"; }
function three(a) { let [x, y, z] = a; return [x, y, z]; }
function elide(a) { const [, y, , w] = a; return [y, w]; }
function rest(a) { const [x, ...r] = a; return [x, r]; }
function onlyRest(a) { const [...r] = a; return r; }
function defs(a) { const [x = "dx", y = "dy", z = "dz"] = a; return [x, y, z]; }
function nested(a) { const [[x, y], [z] = ["nz"], { p }] = a; return [x, y, z, p]; }
function assign(a) { var x, y, o = {}, k = "kk"; [x, o.m, o[k], y] = a; return [x, o.m, o.kk, y]; }
function params([x, y], [z] = [9]) { return [x, y, z]; }
function arrow(a) { return (([x, y]) => [y, x])(a); }
function forHead(a) { var s = []; for (const [k, v] of a) s.push(k + "=" + v); return s; }
function forIn(a) { var s = [], x, y; for ([x, y] of a) s.push(y); return s; }
function caught(a) { try { throw a; } catch ([x, y]) { return [x, y]; } }
function swap(a) { var x = a[0], y = a[1]; [x, y] = [y, x]; return [x, y]; }
function chain(a) { var x, y, z; [x, y] = [z] = a; return [x, y, z]; }
function exprValue(a) { var x; var r = ([x] = a); return [x, r === a]; }
function sloppyish(a) { var g = {}; [g.a, g["b"]] = a; return [g.a, g.b]; }

function forms(tag, a, b) {
  say(tag + " " + [guard(() => two(a)), guard(() => one(a)), guard(() => none(a)), guard(() => three(a)),
    guard(() => elide(a)), guard(() => rest(a)), guard(() => onlyRest(a)), guard(() => defs(a)),
    guard(() => nested(b)), guard(() => assign(a)), guard(() => params(a, a)), guard(() => params(a)),
    guard(() => arrow(a)), guard(() => forHead([a, a])), guard(() => forIn([a, a])), guard(() => caught(a)),
    guard(() => swap(a)), guard(() => chain(a)), guard(() => exprValue(a)), guard(() => sloppyish(a))].join(" | "));
}
function plain(tag) {
  forms(tag + " len0", [], [[1, 2], [3], { p: 4 }]);
  forms(tag + " len1", ["a"], [[1], , { p: 4 }]);
  forms(tag + " len2", ["a", "b"], [[1, 2, 3], [], { p: 4 }]);
  forms(tag + " len5", ["a", "b", "c", "d", "e"], [[1, 2], [3], { p: 4 }]);
  forms(tag + " undef", [undefined, null, undefined], [[undefined], [undefined], {}]);
  var holes = ["h0", , "h2"];
  forms(tag + " holes", holes, [[, 2], [3], { p: 4 }]);
  var long = ["a", "b"];
  long.length = 5;
  forms(tag + " longer", long, [long, long, { p: long.length }]);
  forms(tag + " string", "xyz", ["ab", "c", { p: "s" }]);
  forms(tag + " typed", new Int8Array([7, 8, 9]), [new Int8Array([1, 2]), [3], { p: 4 }]);
  forms(tag + " args", (function () { return arguments; })(5, 6, 7), [[1, 2], [3], { p: 4 }]);
  forms(tag + " set", new Set([1, 2, 2, 3]), [new Set([4, 5]), new Set([6]), { p: 4 }]);
  forms(tag + " map", new Map([[1, 2], [3, 4]]), [new Map([[5, 6]]), [3], { p: 4 }]);
  forms(tag + " re", /(a)(b)?/.exec("ab"), [/(c)/.exec("c"), [3], { p: 4 }]);
  forms(tag + " frozen", Object.freeze(["f1", "f2"]), [Object.freeze([1, 2]), [3], { p: 4 }]);
  forms(tag + " extra", Object.assign(["e1", "e2"], { extra: 1 }), [[1, 2], [3], { p: 4 }]);
  forms(tag + " null", null, null);
  forms(tag + " number", 5, 5);
}
plain("plain");

// the length past the count of a fast array, with a getter on the
// prototype at index 3: the iterator reads 2 to 4 through the prototype
function five(a) { const [p, q, r, s, t] = a; return [p, q, r, s, t]; }
function fiveLog() {
  var a = [1, 2];
  a.length = 5;
  Object.defineProperty(Array.prototype, 3, { get: function () { say("get 3 of " + this.length); return "p3"; }, configurable: true });
  try { say("five " + guard(() => five(a))); } finally { delete Array.prototype[3]; }
  var b = [1, 2];
  b.length = 3;
  Object.defineProperty(Array.prototype, 1, { get: function () { say("get 1"); return "p1"; }, configurable: true });
  Object.defineProperty(Array.prototype, 2, { get: function () { say("get 2"); return "p2"; }, configurable: true });
  try { say("five holes " + guard(() => five(b)) + " " + guard(() => five([0, , 2]))); }
  finally { delete Array.prototype[1]; delete Array.prototype[2]; }
}
fiveLog();

// the prototypes of the iterator: return, next and Symbol.iterator, as
// values and as getters, put there before a destructuring and from a
// default value inside one
var AIP = Object.getPrototypeOf([][Symbol.iterator]());
var IP = Object.getPrototypeOf(AIP);
var values0 = Array.prototype[Symbol.iterator];
var next0 = AIP.next;
function withReturn(where, how, f) {
  var obj = where === "AIP" ? AIP : where === "IP" ? IP : Object.prototype;
  if (how === "value")
    Object.defineProperty(obj, "return", { value: function () { say("return of " + where + " " + typeof this); return {}; }, configurable: true, writable: true });
  else
    Object.defineProperty(obj, "return", { get: function () { say("get return of " + where); return function () { say("return called"); return {}; }; }, configurable: true });
  try { return f(); } finally { delete obj.return; }
}
function retLater(a, where) {
  const [x = withReturnNow(where), y] = a;
  return [x, y];
}
var pending = [];
function withReturnNow(where) {
  var obj = where === "AIP" ? AIP : where === "IP" ? IP : Object.prototype;
  Object.defineProperty(obj, "return", { get: function () { say("late get return of " + where); return undefined; }, configurable: true });
  pending.push(obj);
  return "d";
}
function clearPending() { while (pending.length) delete pending.pop().return; }
function protos() {
  ["AIP", "IP", "OP"].forEach(function (w) {
    ["value", "getter"].forEach(function (how) {
      say("return " + w + " " + how + " " + withReturn(w, how, function () {
        return [guard(() => two([1, 2, 3])), guard(() => two([1])), guard(() => rest([1, 2])),
          guard(() => three([1, 2, 3])), guard(() => defs([1, 2, 3, 4])), guard(() => none([1]))].join(" ");
      }));
    });
    say("late " + w + " " + guard(() => retLater([undefined, 2, 3], w)) + " " + guard(() => retLater([undefined], w)));
    clearPending();
  });
  // next replaced, then put back
  AIP.next = function () { say("next called"); return next0.call(this); };
  say("next replaced " + [guard(() => two([1, 2])), guard(() => rest([1, 2, 3]))].join(" "));
  AIP.next = next0;
  Object.defineProperty(AIP, "next", { get: function () { say("get next"); return next0; }, configurable: true });
  say("next getter " + guard(() => two([1, 2])));
  Object.defineProperty(AIP, "next", { value: next0, configurable: true, writable: true, enumerable: false });
  // Symbol.iterator replaced, then put back
  Array.prototype[Symbol.iterator] = function* () { say("gen iterator"); yield "g0"; yield "g1"; };
  say("iterator replaced " + [guard(() => two([1, 2])), guard(() => rest([1, 2, 3]))].join(" "));
  Array.prototype[Symbol.iterator] = values0;
  Object.defineProperty(Array.prototype, Symbol.iterator, { get: function () { say("get iterator"); return values0; }, configurable: true });
  say("iterator getter " + guard(() => two([1, 2])));
  Object.defineProperty(Array.prototype, Symbol.iterator, { value: values0, configurable: true, writable: true, enumerable: false });
  delete Array.prototype[Symbol.iterator];
  say("iterator deleted " + guard(() => two([1, 2])));
  Array.prototype[Symbol.iterator] = values0;
  // from a default, during the destructuring: the record has its next already
  function midNext(a) {
    const [x = (AIP.next = function () { say("mid next"); return { done: true }; }, "d"), y, z] = a;
    return [x, y, z];
  }
  say("mid next " + guard(() => midNext([undefined, 2, 3])) + " after " + guard(() => two([1, 2])));
  AIP.next = next0;
  function midIter(a) {
    const [x = (Array.prototype[Symbol.iterator] = function* () { yield "no"; }, "d"), y] = a;
    return [x, y];
  }
  say("mid iterator " + guard(() => midIter([undefined, 2])) + " after " + guard(() => two([1, 2])));
  Array.prototype[Symbol.iterator] = values0;
  say("restored " + [guard(() => two([1, 2])), guard(() => rest([1, 2, 3]))].join(" "));
}
protos();

// the array changed from a default value or a setter between two reads
function mutate() {
  function push(a) { const [x = a.push("P"), y, z] = a; return [x, y, z, a.length]; }
  function pop(a) { const [x = a.pop(), y, z] = a; return [x, y, z, a.length]; }
  function zero(a) { const [x = (a.length = 0), y, z] = a; return [x, y, z]; }
  function grow(a) { const [x = (a.length = 4), y, z, w] = a; return [x, y, z, w]; }
  function far(a) { const [x = (a[100] = "far"), y, z] = a; return [x, y, z, a.length]; }
  function del(a) { const [x = delete a[1], y, z] = a; return [x, y, z]; }
  function shift(a) { const [x = a.shift(), y, z] = a; return [x, y, z]; }
  function setter(a) {
    var o = { set m(v) { a.length = 1; say("setter " + v); } };
    var x, y;
    [o.m, x, y] = a;
    return [x, y];
  }
  function protoHole(a) {
    const [x = Object.defineProperty(Array.prototype, 2, { get: function () { say("get proto 2"); return "pp"; }, configurable: true }) && "d", y, z] = a;
    delete Array.prototype[2];
    return [x, y, z];
  }
  function inner(a) { const [[x = a.pop()], y] = a; return [x, y]; }
  say("mutate " + [guard(() => push([undefined, 1])), guard(() => pop([undefined, 1, 2])),
    guard(() => zero([undefined, 1, 2])), guard(() => grow([undefined, 1])),
    guard(() => far([undefined, 1, 2])), guard(() => del([undefined, 1, 2])),
    guard(() => shift([undefined, 1, 2, 3])), guard(() => setter([1, 2, 3])),
    guard(() => protoHole([undefined, 1])), guard(() => inner([[undefined], 5]))].join(" | "));
}
mutate();

// an own Symbol.iterator, a subclass, a Proxy (as the source and in the
// chain of the iterator prototype), an array of another realm
function others() {
  var own = [1, 2];
  own[Symbol.iterator] = function* () { say("own iterator"); yield "o1"; };
  class Sub extends Array {}
  var sub = Sub.from([1, 2, 3]);
  Object.defineProperty(Sub.prototype, Symbol.iterator, { value: function* () { say("sub iterator"); yield "s"; }, configurable: true });
  var subPlain = new (class extends Array {})(3).fill("sp");
  var px = new Proxy([1, 2, 3], { get: function (t, k) { say("proxy get " + String(k)); return t[k]; } });
  var realm = newRealm("[10, 20, 30]");
  say("others " + [guard(() => two(own)), guard(() => rest(sub)), guard(() => two(subPlain)),
    guard(() => two(px)), guard(() => two(realm)), guard(() => rest(realm))].join(" | "));
  var saved = Object.getPrototypeOf(IP);
  Object.setPrototypeOf(IP, new Proxy({}, { get: function (t, k, r) { say("chain get " + String(k)); return undefined; } }));
  try { say("proxy chain " + [guard(() => two([1, 2, 3])), guard(() => two([1])), guard(() => rest([1, 2]))].join(" ")); }
  finally { Object.setPrototypeOf(IP, saved); }
}
others();

// a default that throws, before and after the iterator is done, with a
// return to call or not; the record after the end read again
function throwing() {
  function thrower(t) { throw new Error("thrown " + t); }
  function before(a) { const [x = thrower("before"), y] = a; return [x, y]; }
  function after(a) { const [x, y, z = thrower("after"), w = "w"] = a; return [x, y, z, w]; }
  function afterAll(a) { var x, y, z; [x, y, z] = a; return [x, y, z]; }
  function nestedThrow(a) { const [[x = thrower("inner")], y] = a; return [x, y]; }
  ["none", "AIP", "OP"].forEach(function (w) {
    var f = function () {
      return [guard(() => before([undefined, 1, 2])), guard(() => after([1])), guard(() => after([1, 2])),
        guard(() => afterAll([1])), guard(() => nestedThrow([[undefined, 9], 1])), guard(() => nestedThrow([[], 1]))].join(" ");
    };
    say("throw " + w + " " + (w === "none" ? f() : withReturn(w, "getter", f)));
  });
}
throwing();

// a generator and an async function that stop inside a default, then go on
// or end there
function gens() {
  function* g(a) { const [x = yield "y1", y = yield "y2", z] = a; return [x, y, z]; }
  function run(a, steps) {
    var it = g(a), out = [], r;
    try {
      for (var i = 0; i < steps.length; i++) {
        var s = steps[i];
        r = s[0] === "next" ? it.next(s[1]) : s[0] === "return" ? it.return(s[1]) : it.throw(new Error(s[1]));
        out.push(show(r.value) + (r.done ? "!" : ""));
      }
    } catch (e) { out.push(e.name + ":" + e.message); }
    return out.join(",");
  }
  ["none", "AIP"].forEach(function (w) {
    var f = function () {
      return [run([undefined, undefined, 3], [["next"], ["next", "a"], ["next", "b"]]),
        run([undefined, 2, 3], [["next"], ["return", "r"]]),
        run([undefined, 2, 3], [["next"], ["throw", "t"]]),
        run([], [["next"], ["next", "a"], ["return", "r2"]]),
        run([], [["next"], ["next", "a"], ["throw", "t2"]]),
        run([1, undefined], [["next"], ["return", "r3"]])].join(" ");
    };
    say("gen " + w + " " + (w === "none" ? f() : withReturn(w, "getter", f)));
  });
  async function af(a, p) { const [x = await p, y] = a; return [x, y]; }
  af([undefined, 2], Promise.resolve("aw")).then(function (v) { say("async " + show(v)); });
  af([undefined, 2], Promise.reject(new Error("rej"))).catch(function (e) { say("async " + e.message); });
  af([1, 2], null).then(function (v) { say("async plain " + show(v)); });
  rvExpect(g, af);
}
gens();

rvExpect(two, one, none, three, elide, rest, onlyRest, defs, nested, assign, params, arrow, forHead,
  forIn, caught, swap, chain, exprValue, sloppyish, five, retLater, forms);
Promise.resolve().then(function () {}).then(function () {}).then(function () { globalThis.result = log.join("\n"); });
})();
