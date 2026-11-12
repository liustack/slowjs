// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// Rest and the strict arguments object in the register loop
// (engine/regvm-ops-args.h): calls with zero, fewer, exactly as many and more
// arguments than parameters, from the register loop, from C (Array.prototype
// methods, Function.prototype.apply and call, Reflect.apply, past 255
// arguments) and through spread calls; arguments read, indexed, written,
// passed on, stored and outliving the call, its callee and Symbol.iterator;
// exceptions thrown by such functions; deep recursion through them.
(function () { "use strict";
var log = [];
function note(x) { log.push(x); return x; }
function show(v) {
  if (Array.isArray(v)) return "[" + v.map(show).join(",") + "]";
  if (v && typeof v === "object") {
    var ks = Object.keys(v), out = [];
    for (var i = 0; i < ks.length; i++) out.push(ks[i] + ":" + show(v[ks[i]]));
    return "{" + out.join(",") + "}";
  }
  return String(v);
}

// rest with fixed parameters before it
function r0(...r) { return r; }
function r2(a, b, ...r) { return [a, b, r.length, r]; }
function rcount(...r) { var s = 0; for (var i = 0; i < r.length; i++) s = s + r[i]; return s; }
// arguments object
function alen() { return arguments.length; }
function a2(a, b) { return [a, b, arguments.length, arguments[0], arguments[1], arguments[2], arguments[5]]; }
function awrite(a, b) {
  // strict: arguments does not alias the parameters
  arguments[0] = 100; a = 7;
  return [a, b, arguments[0], arguments[1], arguments.length];
}
function aiter(x) { var s = []; for (var v of arguments) s.push(v); return s; }
function acallee() { try { return arguments.callee; } catch (e) { return e.constructor.name; } }
function atag() { return Object.prototype.toString.call(arguments); }
function akeep() { return arguments; }
function apass(a) { return alen.apply(null, arguments) + 10 * r0.apply(null, arguments).length; }
function aspread(a) { return r2(...arguments); }
function both(a, ...r) { return [a, r, arguments.length, arguments[0]]; }
// exceptions
function throwA() { throw arguments.length; }
function throwR(a, ...r) { throw r.length; }
function catchA() { try { return throwA(1, 2, 3, {}); } catch (e) { return "a" + e; } }
function catchR() { try { return throwR(1, {}, {}, "s"); } catch (e) { return "r" + e; } }
// deep recursion with extra arguments, and through arguments
function deepA(n) { return n === 0 ? arguments.length : deepA(n - 1, arguments[1], arguments[2]); }
function deepR(n, ...r) { return n === 0 ? r.length : deepR(n - 1, ...r, n); }
// a method call, and this
var obj = { k: 3, m: function (a) { return this.k + a + arguments.length; } };
function callM() { return obj.m(1, 2, 3); }
// closures: an arrow with rest, an arrow reading the arguments of its
// function, a function with captured locals and extra arguments
var arr = (a, ...r) => r.length * 10 + a;
function outerA() { return (() => arguments.length + arguments[0])(); }
function capt(a) { var f = function () { return a + arguments.length; }; return f(1, 2) + arguments.length; }
function captRest(a, ...r) { var g = () => r.length + a; return g(); }

function run() {
  var out = [];
  out.push(show(r0()), show(r0(1)), show(r0(1, "s", {a: 1})));
  out.push(show(r2()), show(r2(1)), show(r2(1, 2)), show(r2(1, 2, 3)), show(r2(1, 2, 3, 4, 5)));
  out.push(rcount(), rcount(1, 2, 3, 4, 5, 6, 7, 8, 9, 10));
  out.push(alen(), alen(1), alen(1, 2, 3));
  out.push(show(a2()), show(a2(1)), show(a2(1, 2)), show(a2(1, 2, 3, 4, 5, 6)));
  out.push(show(awrite(1)), show(awrite(1, 2, 3)));
  out.push(show(aiter()), show(aiter(1, 2, 3)));
  out.push(acallee(), acallee(1, 2), atag(), atag(1));
  var kept = akeep(1, "two", {three: 3});
  out.push(kept.length, show(kept[2]), show(Array.prototype.slice.call(kept)));
  out.push(apass(), apass(1, 2, 3, 4));
  out.push(show(aspread(1, 2, 3, 4)), show(aspread()));
  out.push(show(both()), show(both(1)), show(both(1, 2, 3)));
  out.push(catchA(), catchA(), catchR(), catchR());
  out.push(deepA(3000, "a", "b"), deepR(100, "s"));
  out.push(callM(), callM());
  out.push(arr(1), arr(1, 2, 3), outerA(5, 6), outerA(7), capt(1), capt(1, {}, []), captRest(1), captRest(1, 2, 3));
  // from C
  out.push(show([1, 2, 3].map(r0)), show([4, 5].map(alen)), show([7].map(both)));
  out.push(show(r2.apply(null, [1, 2, 3, 4])), show(a2.call(null, 9, 8, 7)), show(Reflect.apply(both, null, [1, 2, 3])));
  var big = []; for (var i = 0; i < 300; i++) big.push(i);
  out.push(rcount.apply(null, big), alen.apply(null, big), r2(...big).length);
  // spread calls
  out.push(show(r0(...[1, 2], 3, ...[4])), show(a2(...[1, 2, 3])), show(both(...[1, 2, 3])));
  return out.join("\n");
}

var res = run();
res = res + "\n--\n" + run();
log.push(res);
rvExpect(r0, r2, rcount, alen, a2, awrite, aiter, acallee, atag, akeep, apass, aspread, both,
         throwA, throwR, catchA, catchR, deepA, deepR, callM, arr, outerA, capt, captRest, run);
return log.join("\n");
})()
