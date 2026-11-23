// Copyright (c) 2026 The liustack/tifajs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.
// rv-expect-op: W_APPEND
// jsrun: -result
// The spread of an array with no Array Iterator (js_append_array,
// engine/quickjs.c): every form the compiler makes OP_append for (array
// literals, calls, methods, new, super, the default constructor of a
// derived class, a direct eval, built-ins), with every kind of source (empty,
// short, long, holes, a length past the count, frozen, sealed, not
// extensible, an extra property, an own Symbol.iterator, another prototype,
// a subclass, a Proxy, an array of another realm, arguments, a typed array,
// strings, Set, Map, generators), what a script can change before and
// between two spreads (Array.prototype[Symbol.iterator], the next and the
// return of the iterator prototypes, as values and as getters that log
// their reads, getters and setters at an index of Array.prototype and
// Object.prototype, the array itself), and the limit on the number of
// arguments. The golden was recorded before the change. With the messages
// of errors left out, the lines are what Node 24 prints, but for two: the
// limit of the arguments is the engine's (65534), and V8 runs eval(...a)
// as an indirect eval, where the specification makes it direct.
(function () { "use strict";
var log = [];
function say(s) { log.push(s); }
function show(v) {
  if (Array.isArray(v)) return "[" + v.map(show).join(",") + "]";
  if (typeof v === "function") return "fn";
  if (v !== null && typeof v === "object") return "obj";
  if (typeof v === "symbol") return "sym";
  if (Object.is(v, -0)) return "-0";
  return String(v);
}
function guard(f) {
  try { return show(f()); } catch (e) { return e.name + ":" + e.message; }
}

// the forms
function args() { return Array.prototype.slice.call(arguments); }
function lit(a) { return [...a]; }
function litPre(a) { return [0, ...a]; }
function litPost(a) { return [...a, 9]; }
function litTwice(a) { return [...a, ...a]; }
function litHoleBefore(a) { var r = [, ...a]; return [r.length, 0 in r, r[1], r[r.length - 1]]; }
function litHoleAfter(a) { var r = [...a, , 8]; return [r.length, (r.length - 2) in r, r[r.length - 1]]; }
function litLong(a) {
  var r = [0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25,
    26, 27, 28, 29, 30, 31, 32, 33, 34, 35, 36, 37, 38, 39, ...a, "end"];
  return [r.length, r[39], r[40], r[r.length - 1]];
}
function nested(a) { return [...[...a]]; }
function call(a) { return args(...a); }
function callMid(a) { return args(1, ...a, 2); }
function callTwo(a) { return args(...a, ...a); }
function callLong(a) {
  return args(0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25,
    26, 27, 28, 29, 30, 31, 32, 33, 34, 35, ...a).slice(34);
}
function method(a) { var o = { m: args }; return o.m(...a); }
function keyed(a) { var o = { m: args }, k = "m"; return o[k](...a); }
function arrow(a) { return ((...r) => r)(...a); }
class Base { constructor(...r) { this.r = r; } }
class Derived extends Base { constructor(a) { super("s", ...a); } }
class Default extends Base {}
function ctor(a) { return new Base(...a).r; }
function sup(a) { return new Derived(a).r; }
function dflt(a) { return new Default(...a).r; }
function max(a) { return Math.max(...a); }
function pushed(a) { var t = ["t"]; t.push(...a); return t; }
function apply(a) { return args.apply(null, [...a]); }
var forms = [lit, litPre, litPost, litTwice, litHoleBefore, litHoleAfter, litLong, nested, call, callMid,
  callTwo, callLong, method, keyed, arrow, ctor, sup, dflt, max, pushed, apply];

function each(tag, mk) {
  say(tag + " " + forms.map(function (f) { return guard(function () { return f(mk()); }); }).join(" | "));
}
function sources(tag) {
  each(tag + " empty", function () { return []; });
  each(tag + " one", function () { return [1]; });
  each(tag + " three", function () { return [1, 2, 3]; });
  each(tag + " values", function () { return ["a", undefined, null, {}, NaN, -0, 1.5, true]; });
  each(tag + " holes", function () { return [1, , 3]; });
  each(tag + " longer", function () { var a = [1, 2]; a.length = 4; return a; });
  each(tag + " shorter", function () { var a = [1, 2, 3, 4]; a.length = 3; return a; });
  each(tag + " slow", function () { var a = [1, 2, 3]; a[10] = 1; a.length = 3; return a; });
  each(tag + " frozen", function () { return Object.freeze([1, 2]); });
  each(tag + " sealed", function () { return Object.seal([1, 2]); });
  each(tag + " fixed", function () { return Object.preventExtensions([1, 2]); });
  each(tag + " extra", function () { return Object.assign([1, 2], { extra: 1 }); });
  each(tag + " own iterator", function () {
    var a = [1, 2];
    a[Symbol.iterator] = function* () { say("own iterator"); yield "o"; };
    return a;
  });
  each(tag + " own getter", function () {
    var a = [1, 2];
    Object.defineProperty(a, 1, { get: function () { say("own get 1"); return "g"; }, enumerable: true });
    return a;
  });
  each(tag + " other proto", function () { return Object.setPrototypeOf([1, 2], Object.create(Array.prototype)); });
  each(tag + " string", function () { return "ab\u{1d4b3}"; });
  each(tag + " typed", function () { return new Int8Array([7, 8]); });
  each(tag + " set", function () { return new Set([1, 2, 2]); });
  each(tag + " map", function () { return new Map([[1, 2]]); });
  each(tag + " args", function () { return (function () { return arguments; })(5, 6); });
  each(tag + " gen", function () { return (function* () { yield 1; yield 2; })(); });
  each(tag + " null", function () { return null; });
  each(tag + " number", function () { return 5; });
  each(tag + " object", function () { return {}; });
}
sources("plain");

// a long array, and the identity and the independence of the copy
(function () {
  var big = [];
  for (var i = 0; i < 1000; i++) big.push(i);
  var r = [...big], s = args(...big), o = {}, src = [o, big];
  var c = [...src];
  c.push("x");
  c[0] = 1;
  say("long " + [r.length, r[999], s.length, s[500], big.length, src.length, src[0] === o,
    [...src][0] === o, [...src][1] === big, Object.getOwnPropertyNames([...[1, 2]]).join()].join(" "));
  var three = [1, 2, 3], copy = [...three];
  copy.length = 1;
  three.push(4);
  say("independent " + show(copy) + " " + show(three) + " " + show([...copy, ...three]));
})();

// subclasses, a Proxy, other realms
(function () {
  class Sub extends Array {}
  var sub = Sub.from([1, 2, 3]);
  var px = new Proxy([1, 2, 3], { get: function (t, k) { say("proxy get " + String(k)); return t[k]; } });
  var realm = newRealm("[10, 20, 30]");
  var otherSpread = newRealm("(function (a) { return [...a]; })");
  var otherCall = newRealm("(function (f, a) { return f(...a); })");
  var mine = [1, 2];
  say("others " + [guard(() => lit(sub)), guard(() => call(sub)), guard(() => lit(px)), guard(() => lit(realm)),
    guard(() => call(realm)), String(Object.getPrototypeOf(lit(realm)) === Array.prototype),
    guard(() => otherSpread(mine)), String(Object.getPrototypeOf(otherSpread(mine)) === Array.prototype),
    guard(() => otherCall(args, mine)), guard(() => otherSpread(realm))].join(" | "));
  Object.defineProperty(Sub.prototype, Symbol.iterator, { value: function* () { say("sub iterator"); yield "s"; }, configurable: true });
  say("sub iterator " + guard(() => lit(sub)) + " " + guard(() => lit(new Sub(2))));
})();

// the prototypes of the iterator, as values and as getters, put there
// before a spread
var AIP = Object.getPrototypeOf([][Symbol.iterator]());
var IP = Object.getPrototypeOf(AIP);
var values0 = Array.prototype[Symbol.iterator];
var next0 = AIP.next;
function some() {
  return [guard(() => lit([1, 2])), guard(() => call([1, 2])), guard(() => litPre([])), guard(() => sup([3])),
    guard(() => dflt([4, 5]))].join(" ");
}
(function () {
  ["AIP", "IP", "OP"].forEach(function (w) {
    var obj = w === "AIP" ? AIP : w === "IP" ? IP : Object.prototype;
    Object.defineProperty(obj, "return", { value: function () { say("return of " + w); return {}; }, configurable: true, writable: true });
    say("return value " + w + " " + some());
    Object.defineProperty(obj, "return", { get: function () { say("get return of " + w); return undefined; }, configurable: true });
    say("return getter " + w + " " + some());
    delete obj.return;
  });
  AIP.next = function () { say("next called"); return next0.call(this); };
  say("next replaced " + some());
  Object.defineProperty(AIP, "next", { get: function () { say("get next"); return next0; }, configurable: true });
  say("next getter " + some());
  delete AIP.next;
  say("next deleted " + some());
  IP.next = function () { say("IP next"); return next0.call(this); };
  say("next on IP " + some());
  delete IP.next;
  var saved = Object.getPrototypeOf(IP);
  Object.setPrototypeOf(IP, new Proxy({}, { get: function (t, k) { say("chain get " + String(k)); return undefined; } }));
  say("next missing, proxy chain " + some());
  Object.defineProperty(AIP, "next", { value: next0, configurable: true, writable: true, enumerable: false });
  say("proxy chain " + some());
  Object.setPrototypeOf(IP, saved);
  Array.prototype[Symbol.iterator] = function* () { say("gen iterator"); yield "g0"; yield "g1"; };
  say("iterator replaced " + some());
  Object.defineProperty(Array.prototype, Symbol.iterator, { get: function () { say("get iterator"); return values0; }, configurable: true });
  say("iterator getter " + some());
  delete Array.prototype[Symbol.iterator];
  say("iterator deleted " + some());
  Object.prototype[Symbol.iterator] = function* () { say("object iterator"); yield "op"; };
  say("iterator on Object.prototype " + some());
  delete Object.prototype[Symbol.iterator];
  Object.defineProperty(Array.prototype, Symbol.iterator, { value: values0, configurable: true, writable: true, enumerable: false });
  Array.prototype.extra = "x";
  Object.defineProperty(Array, Symbol.species, { get: function () { say("species"); return Array; }, configurable: true });
  say("restored " + some());
  delete Array.prototype.extra;
})();

// the array changed between two spreads, by the argument after it, by a
// default and by an iterator of its own spread before it
(function () {
  function pushAfter(a) { return [...a, ...(a.push(9), a)]; }
  function popArg(a) { return args(...a, a.pop(), ...a); }
  function lenArg(a) { return args(...a, (a.length = 0), ...a); }
  function grow(a) { return [...a, (a.length = 4), ...a]; }
  function iterFirst(a) {
    var it = { [Symbol.iterator]: function* () { a.push("from it"); yield "it"; } };
    return [...it, ...a];
  }
  function iterAfter(a) {
    var it = { [Symbol.iterator]: function* () { a.length = 0; yield "it"; } };
    return [...a, ...it, ...a];
  }
  function deflt(a, b = a.push("d")) { return args(...a, b); }
  say("between " + [guard(() => pushAfter([1, 2])), guard(() => popArg([1, 2, 3])), guard(() => lenArg([1, 2])),
    guard(() => grow([1, 2])), guard(() => iterFirst([1])), guard(() => iterAfter([1, 2])),
    guard(() => deflt([1]))].join(" | "));
})();

// the limit on the number of arguments, the engine's (65534): the most that
// works, alone and after an argument, then the least that fails
(function () {
  function n(k) { var a = []; for (var i = 0; i < k; i++) a.push(i); return a; }
  function count() { return arguments.length; }
  var max = n(65534), max1 = n(65533), max2 = n(65532);
  say("limit max " + [guard(() => count(...max)), guard(() => count(0, ...max1)),
    guard(() => new Base(...max).r.length), guard(() => new Derived(max1).r.length),
    guard(() => new Default(...max).r.length), guard(() => count(0, ...max2, 1)),
    guard(() => Math.max(...max))].join(" | "));
  var at = n(65535), past = n(65536);
  say("limit " + [guard(() => count(...at)), guard(() => count(...past)), guard(() => count(0, ...max)),
    guard(() => new Base(...past).r.length), guard(() => new Derived(max).r.length),
    guard(() => new Default(...past).r.length), guard(() => [...past, ...past].length),
    guard(() => Math.max(...past))].join(" | "));
})();

// a direct eval with a spread, and new of what is not a constructor
(function () {
  var x = "local";
  function ev(a) { var x = "inner"; return eval(...a); }
  say("eval " + [guard(() => ev(["x"])), guard(() => ev(["x + 1", "ignored"])), guard(() => ev([])),
    guard(() => ev([5]))].join(" | "));
  var notCtor = () => 1;
  say("not a constructor " + [guard(() => new notCtor(...[1])), guard(() => new Math.max(...[1]))].join(" | "));
})();

// getters and setters at an index of the prototypes (last: they turn some
// fast paths off for good)
(function () {
  Object.defineProperty(Object.prototype, 2, { get: function () { say("OP get 2"); return "op2"; }, configurable: true });
  say("OP getter " + [guard(() => lit([1, 2])), guard(() => lit([1, , 3])),
    guard(() => { var a = [1]; a.length = 3; return lit(a); })].join(" | "));
  delete Object.prototype[2];
  Object.defineProperty(Array.prototype, 0, { set: function (v) { say("AP set 0 " + v); }, get: function () { say("AP get 0"); return "ap0"; }, configurable: true });
  Object.defineProperty(Array.prototype, 3, { get: function () { say("AP get 3"); return "ap3"; }, configurable: true });
  say("AP accessors " + [guard(() => lit([1, 2])), guard(() => call([1, 2])), guard(() => litHoleBefore([1])),
    guard(() => { var a = [1, 2]; a.length = 5; return lit(a); }), guard(() => lit([])), guard(() => sup([7])),
    guard(() => pushed([1]))].join(" | "));
  delete Array.prototype[0];
  delete Array.prototype[3];
  say("after " + some());
})();

rvExpect(lit, litPre, litPost, litTwice, litHoleBefore, litHoleAfter, litLong, nested, call, callMid, callTwo,
  callLong, method, keyed, arrow, ctor, sup, dflt, max, pushed, apply, some);
Promise.resolve().then(function () {}).then(function () {}).then(function () { globalThis.result = log.join("\n"); });
})();
