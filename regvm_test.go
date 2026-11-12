// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package quickjs // import "github.com/liustack/quickjs"

import (
	"crypto/sha256"
	"flag"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/liustack/quickjs/internal/cengine"
)

// regvmStat is JS_RegvmStat: 0 converted, 1 refused, 2 entries, 3 and 4 the
// opcode and offset of the last refusal.
func regvmStat(m *VM, what int) (n int) {
	m.outer(func() { n = int(int32(cengine.JS_RegvmStat(m.sp, m.ctx, uintptr(what)))) })
	return n
}

// The micro scripts of stage 0: each global is a function of one count,
// made of what the register pass converts.
const regvmMicro = `"use strict";
var fibBench = (function () {
  function fib(n) { if (n < 2) return n; return fib(n - 1) + fib(n - 2); }
  return function (n) { return fib(n); };
})();
var callBench = (function () {
  var o = { k: 1, m: function (x) { return x + this.k; } };
  return function (n) { var p = o, s = 0; for (var i = 0; i < n; i++) s = s + p.m(i); return s; };
})();
var fieldBench = (function () {
  var o = { a: 1, b: 2 };
  return function (n) { var p = o, s = 0; for (var i = 0; i < n; i++) s = s + p.a + p.b; return s; };
})();
// the crossings of the loops: a loop of the old loop (the * is refused)
// calling a function of the register loop (xon), the other way round (xno),
// neither (xnn), a C function calling a function of the register loop
// (xcn), and a function of the register loop calling a C function (xnc)
var xonBench = (function () {
  function leaf(x) { return x; }
  return function (n) { var s = 0; for (var i = 0; i < n; i++) s = s + leaf(i) * 1; return s; };
})();
var xnoBench = (function () {
  function leaf(x) { return x * 1; }
  return function (n) { var s = 0; for (var i = 0; i < n; i++) s = s + leaf(i); return s; };
})();
var xnnBench = (function () {
  function leaf(x) { return x; }
  return function (n) { var s = 0; for (var i = 0; i < n; i++) s = s + leaf(i); return s; };
})();
var xcnBench = (function () {
  var a = [];
  for (var k = 0; k < 1000; k++) a.push(k);
  function leaf(x) { return x; }
  return function (n) { for (var j = 0; j < n; j = j + 1000) a.forEach(leaf); return n; };
})();
var xncBench = (function () {
  var abs = Math.abs;
  return function (n) { var s = 0; for (var i = 0; i < n; i++) s = s + abs(i); return s; };
})();
// a switch on strings (a chain of string cases), compares and tests that
// branch
var caseBench = (function () {
  var keys = ["div", "span", "a", "p", "li", "ul", "img", "x"];
  function kind(t) {
    switch (t) {
    case "div": return 1; case "span": return 2; case "a": return 3;
    case "p": return 4; case "li": return 5; case "ul": return 6;
    case "img": return 7; default: return 0;
    }
  }
  return function (n) { var s = 0; for (var i = 0; i < n; i++) s = s + kind(keys[i & 7]); return s; };
})();
// sloppy functions (Function makes them, this script is strict): their
// this is coerced, and a store throws by the mode of the function that
// makes it, whoever called it
var sloppyBench = (function () {
  var sThis = Function("return this === undefined ? \"u\" : typeof this + typeof this.valueOf();");
  var sRO = Function("o", "v", "o.x = v; return o.x;");
  var fz = Object.freeze({ x: 1 });
  function tRO(o, v) { o.x = v; return o.x; }
  return function (which, x) {
    if (which === 0) return sThis.call(x) + sThis();
    if (which === 1) return sRO(fz, x);
    if (which === 2) return tRO(fz, x);
    return -1;
  };
})();
// with and direct eval, in sloppy code made with Function: a name found in
// the object of a with or around it, a local an eval writes, a var an eval
// declares (the var object), a spread call of eval
var evalBench = (function () {
  var w = Function("o", "var y = 10; with (o) { return x + y; }");
  var mk = Function("return [(k) => { var l = 1; eval(\"l = l + k\"); return l; }, " +
    "(k) => { eval(\"var v = k * 2\"); return v; }, (k) => { var a = [\"k + 1\"]; return eval(...a); }];");
  var fs = mk();
  return function (which, x) {
    if (which === 0) return w({ x: x });
    if (which === 1) return w({ x: x, y: 1 });
    if (which < 5) return fs[which - 2](x);
    return -1;
  };
})();
// return f() inside a switch: the switch value is still under the call
var tailBench = (function () {
  var keys = ["div", "span", "a", "p", "li", "ul", "img", "x"];
  var o = { n: 3, f: function (x) { return this.n + x; } };
  function one(x) { return x + 1; }
  function kind(t, i) {
    switch (t) {
    case "div": return one(i); case "span": return o.f(i);
    case "a": switch (i & 1) { case 0: return one(2); default: return Math.max(i, 4); }
    default: return 0;
    }
  }
  return function (n) { var s = 0; for (var i = 0; i < n; i++) s = s + kind(keys[i & 7], i); return s; };
})();
var testBench = (function () {
  var vals = [null, undefined, 0, "s", 1, function () {}, {}, true];
  function f(v, i) {
    var r = 0;
    if (v === null) r = r + 1;
    if (v != null) r = r + 2;
    if (typeof v === "function") r = r + 3;
    if (!v) r = r + 4;
    if (i !== 3 && 2 <= i) r = r + 5;
    return r;
  }
  return function (n) { var s = 0; for (var i = 0; i < n; i++) s = s + f(vals[i & 7], i & 7); return s; };
})();
var mixBench = (function () {
  var o = { a: 1.5, s: "x", t: "a", x2: "x", n: null };
  function g(a, b) { return a - b; }
  function h(a) { return g(a); }
  function str(a, b) { return a + b; }
  function put(q, v) { q.v = v; q.w = 3; return q.v; }
  function eq(a, b) { if (a === b) return 1; return 0; }
  function lt(a, b) { var r = a < b; return r; }
  function thrower(q) { return q.x.y; }
  function deep(n) { if (n < 1) return 0; return deep(n - 1) + 1; }
  function getter() { var q = { get v() { return deep(10); } }; return q; }
  function readv(q) { return q.v + 1; }
  function loopy(n) { var s = 0, i = 0; while (i < n) { s = s + i; i = i + 1; } return s; }
  var o2 = { k: 2, m: function (x, y) { return x + this.k; }, mk: function () { return o2; } };
  function many(a, b) { var c = a + 1, d = c + 1, e = d + 1, f = e + 1, g2 = f + 1, h2 = g2 + 1, i = h2 + 1; return a + b + c + d + e + f + g2 + h2 + i; }
  function fl(n) { var t = 0.5, i = 0; while (i < n) { t = t - 0.25; t = t + i; t = 1.5 + t; i = i + 1; } return t - 1; }
  function miss(a, b) { return a; }
  return function (which, x) {
    if (which === 0) return g(5, 2);
    if (which === 1) return h(5);
    if (which === 2) return str(o.s, o.t);
    if (which === 3) return str(o.s, 1);
    if (which === 4) return str(o.a, 2);
    if (which === 5) return put(o, 7);
    if (which === 6) return eq(1, 1) + eq(o, o) + eq(1, 2) + eq(o.s, o.x2);
    if (which === 7) return lt(1, 2);
    if (which === 8) return lt(o.s, o.t);
    if (which === 9) return o.n;
    if (which === 10) return o.zz;
    if (which === 11) return thrower(x);
    if (which === 12) return deep(x);
    if (which === 13) return readv(getter());
    if (which === 14) return loopy(x);
    if (which === 15) return o2.mk().m(3) + o2.m(4, 5);
    if (which === 16) return many(1, 2);
    if (which === 17) return fl(x);
    if (which === 18) return miss(5);
    return -1;
  };
})();
// exceptions in the register loop (engine/regvm-ops-except.h): a catch with
// temps owned above its catch offset, finally on return and on throw,
// rethrow, nested try, a throw caught three frames up, and the stack of an
// error made and thrown in the loop, both caught and passed to Go
var exceptBench = (function () {
  var E = Error;
  function boom(m) { throw new E("boom " + m); }
  function mk(v) { return { e: v, n: 1 }; }
  function thr(x) { throw x; }
  function ident(x) { return x; }
  function catchTemps(o, f) { var r = 0; try { r = o.n + f(o.e) + o.n; } catch (e) { r = e; } return r; }
  function finRet(f, x) { var n = 0; try { n++; return f(x); } finally { n = 10 + n; } }
  function finCatch(f, x) { var r = 0; try { r = f(x); } catch (e) { r = 100; } finally { r++; } return r; }
  function rethrow(f, x) { try { f(x); } catch (e) { throw e; } return 0; }
  function nested(f, x) { var r = 0; try { try { r = f(x); } catch (e) { r = 1; thr(x + 1); } } catch (e2) { r = e2 + r; } return r; }
  function up0(x) { return thr(x) + 1; }
  function up1(x) { return up0(x) + 1; }
  function up2(x) { return up1(x) + 1; }
  function up3(x) { try { return up2(x); } catch (e) { return e + 1; } }
  function s0(x) { return boom(x) + 1; }
  function s1(x) { return s0(x) + 1; }
  function stack(x) { try { s1(x); } catch (e) { return e.stack; } return 0; }
  function field(o) { return o.x.y; }
  return function (which, x) {
    var i, s = 0;
    for (i = 0; i < 3; i++) {
      if (which === 0) s = catchTemps(mk(x), thr) + catchTemps(mk(x), ident);
      if (which === 1) s = finRet(ident, x);
      if (which === 2) s = finCatch(thr, x) + finCatch(ident, x);
      if (which === 3) s = nested(thr, x) + nested(ident, x);
      if (which === 4) s = up3(x);
      if (which === 5) s = stack(x);
      if (which === 6) { try { rethrow(thr, x); } catch (e) { s = e; } }
      if (which === 7 && i === 2) return rethrow(boom, x);
      if (which === 8 && i === 2) return field(x);
    }
    return s;
  };
})();
// the window ops (engine/regvm-ops-window.h): for-of with a break and with
// a body that throws (both close the iterator), for-in, object and array
// literals, spreads and delete
var windowMicro = (function () {
  var log = [];
  function iter(n) {
    var i = 0;
    return { next: function () { i++; return { done: i > n, value: i }; },
             return: function () { log.push("return" + i); return {}; },
             [Symbol.iterator]: function () { return this; } };
  }
  function vals(a) { var r = []; for (var x of a) r.push(x); return r; }
  function first(a, n) { var r = [], i = 0; for (var x of a) { if (i === n) break; r.push(x); i = i + 1; } return r; }
  function each(a, f) { var r = []; for (var x of a) r.push(f(x)); return r; }
  function keys(o) { var r = []; for (var k in o) r.push(k); return r; }
  function lit(a, b) { return { a: a, b: b, c: 1, d: a + b }; }
  function arr(a, b) { return [a, b, 1, ...a, ...b]; }
  function del(o, k) { return delete o[k]; }
  function bad(x) { if (x === 2) throw new TypeError("at 2"); return x; }
  return function (which) {
    if (which === 0) return vals([1, 2.5, "s", null]).join();
    if (which === 1) return first(iter(5), 2).join() + log.join();
    if (which === 2) return each(iter(5), bad).join();
    if (which === 3) return log.join();
    if (which === 4) return keys({ a: 1, b: 2, 3: 4 }).join();
    if (which === 5) return JSON.stringify(lit(1, 2)) + JSON.stringify(lit("x", "y"));
    if (which === 6) return JSON.stringify(arr("ab", [3]));
    if (which === 7) { var o = { p: 1, q: 2 }; return del(o, "p") + Object.keys(o).join(); }
    if (which === 8) return del(Object.freeze({ p: 1 }), "p");
    return -1;
  };
})();
// closures (package P3): a closure that outlives its frame and sees a later
// write, closure variables written from a temp, a local and a constant, a
// let read before its initialization, a new binding at each turn of a loop,
// and an exception that leaves a frame with captured locals
var closureBench = (function () {
  function counter(start) { let n = start; const get = function () { return n; }; n = n + 1; return get; }
  function cell() {
    let v = 0;
    return { t: function (o) { v = o.x; }, l: function (x) { v = x; }, k: function () { v = null; },
             s: function (x) { return (v = x + 1); }, get: function () { return v; } };
  }
  function tdz(c) { if (c) return x; let x = c + 1; return x; }
  function turns(n) { let prev = null; for (let i = 0; i < n; i++) { const p = prev; prev = function () { return i + (p ? p() : 0); }; } return prev(); }
  function leaves(o, h) { let v = o.a; h.g = function () { return v; }; v = v + 1; return o.a + o.b.c; }
  function deep(n) { let v = n; const f = function () { return v; }; if (n < 1) return f(); return deep(n - 1) + f() - n; }
  return function (which, x) {
    if (which === 0) return counter(x)() + counter(x + 1)();
    if (which === 1) { const c = cell(); c.t({ x: "t" }); const a = c.get(); c.l(x); const b = c.get(); c.k(); return a + b + c.get() + c.s(x) + c.get(); }
    if (which === 2) return tdz(x);
    if (which === 3) return turns(x);
    if (which === 4) { const h = {}; let r; try { r = leaves({ a: 7 }, h); } catch (e) { r = e.message; } return r + h.g(); }
    if (which === 5) return deep(x);
    return -1;
  };
})();
// new X(...) in the register loop, a call out of it (engine/regvm-ops-ctor.h)
var newBench = (function () {
  var o = { k: 2 }, M = Map, E = Error;
  function P(a, b) { this.a = a; this.b = b; }
  function R(k) { this.x = 1; if (k === 1) return o; return 5; }
  function Th(x) { if (x) throw new E(); this.ok = 1; }
  class C { constructor(x) { this.x = x; } }
  class Dd extends C { constructor(x) { super(x + 1); } }
  var Cv = C, Ddv = Dd, BP = P.bind(null, 3), arrow = () => 1;
  var count = function f(n) { if (n < 1) return 0; return f(n - 1) + 1; };
  function nt() { return new.target; }
  return function (which, x) {
    if (which === 0) return new P(x, o.k).b + new P(1).a;
    if (which === 1) return new R(0).x + new R(1).k;
    if (which === 2) return new Cv(x).x + new Ddv(x).x;
    if (which === 3) return new BP(x).a + new BP(x).b;
    if (which === 4) { var m = new M(); m.set(1, x); return m.get(1); }
    if (which === 5) return o.k + new Th(x).ok;
    if (which === 6) return new arrow();
    if (which === 7) return new x();
    if (which === 8) return count(x);
    if (which === 9) return nt();
    return -1;
  };
})();
// the functions too large for a byte operand or an off16
// (engine/regvm-ops-wide.h), made from source: 300 constants (LOADKW), 150
// lets and 200 locals before F's reach (MOVW, SETLW, CHKLW) and a loop
// around more than 32 KB of code (JMPW), and 40 calls nested as arguments,
// a stack of 80 entries (the temps past T[32] in pcmap_hi). Their caller is
// sloppy, so that each call of them enters the register loop.
var limitsMicro = (function () {
  function list(f, n) { var r = [], i; for (i = 0; i < n; i++) r.push(f(i)); return r; }
  var consts = Function("x", "'use strict'; var s = x;\n" +
    list(function (i) { return "s = s + 'k" + i + "';"; }, 300).join("\n") + "\nreturn s.length;");
  var locals = Function("x", "'use strict';\n" +
    "if (x === 'tdz') return q0;\n" +
    "let " + list(function (i) { return "q" + i + " = " + i; }, 150).join(", ") + ";\n" +
    "var " + list(function (i) { return "v" + i + " = " + i; }, 200).join(", ") + ";\n" +
    "q0 = x; v0 = q0; v1++; v2 += x;\n" +
    "return " + list(function (i) { return "q" + i; }, 150).concat(list(function (i) { return "v" + i; }, 200)).join(" + ") + ";");
  var long = Function("n", "'use strict'; var s = 0, i;\nfor (i = 0; i < n; i++) {\n" +
    list(function () { return "s = s + i; s = s - 1;"; }, 5000).join("\n") + "\n}\nreturn s;");
  var deep = Function("x", "'use strict'; var g = function (a, b) { if (b === 'throw') throw new Error(a); return a + b; };\nreturn " +
    list(function (i) { return "g(" + i + ", "; }, 40).join("") + "x" + list(function () { return ")"; }, 40).join("") + ";");
  // 300 captured locals: their var_refs on the heap (CLW), a closure that
  // keeps them past a return and a throw
  var keep = {};
  var clos = Function("keep", "x", "'use strict';\nvar " +
    list(function (i) { return "c" + i + " = " + i; }, 300).join(", ") + ";\n" +
    "keep.f = function () { return " + list(function (i) { return "c" + i; }, 300).join(" + ") + "; };\n" +
    "if (x === 'throw') throw new Error(keep.f());\nc0 = x;\nreturn keep.f();");
  return Function("consts", "locals", "long", "deep", "clos", "keep", "return function (which, x) {\n" +
    "if (which === 0) return consts(x);\n" +
    "if (which === 1) return locals(x);\n" +
    "if (which === 2) return long(x);\n" +
    "if (which === 3) return deep(x);\n" +
    "if (which === 4) return clos(keep, x);\n" +
    "if (which === 5) return keep.f();\n" +
    "return -1; };")(consts, locals, long, deep, clos, keep);
})();
// constructors run in the register loop: new.target in a slot, the object
// made from it, what a constructor returns, super(...) and the checks of a
// class constructor (engine/regvm-ops-ctor.h)
var ctorBench = (function () {
  var o = { k: 2 }, E = Error;
  function P(a, b) { this.a = a; this.b = b; }
  function R(k) { this.x = 1; if (k === 1) return o; if (k === 2) return 5; }
  function NT() { this.t = new.target === NT; }
  function Deep(n) { this.v = n > 0 ? new Deep(n - 1).v + 1 : 0; }
  class C { constructor(x) { this.x = x; } }
  class D extends C { constructor(x) { super(x + 1); this.d = 1; } }
  class Dn extends C {}
  class Dr extends C { constructor(k) { super(k); if (k === 1) return 7; if (k === 2) return o; } }
  class Du extends C { constructor(k) { if (k) super(k); } }
  var Cv = C;
  return function (which, x) {
    if (which === 0) return new P(x, o.k).a + new P(1).b;
    if (which === 1) return new R(0).x + new R(1).k + new R(2).x;
    if (which === 2) return new NT().t;
    if (which === 3) return new Deep(x).v;
    if (which === 4) { var q = new D(x); return q.x + q.d + new C(x).x; }
    if (which === 5) return new Dn().x;
    if (which === 6) return new Dr(x).k;
    if (which === 7) return new Du(x).x;
    if (which === 8) return Cv(x);
    if (which === 9) return Reflect.construct(D, [x], NT).x;
    return -1;
  };
})();
// P2: elements, objects, globals, the other arithmetic
// classes and private fields through OLD (package K): a class made with and
// without heritage, a computed method key, super property reads, private
// fields, methods and brands, each also on a wrong object
var classMicro = (function () {
  class Base { get who() { return "base"; } }
  function make(base, k) {
    class C extends base {
      #x = 1;
      #m() { return this.#x; }
      [k]() { return "k" + super.who; }
      get x() { return this.#x; }
      callM() { return this.#m(); }
      static has(o) { return #x in o; }
      static read(o) { return o.#x; }
    }
    return C;
  }
  var C = make(Base, "kk"), c = new C();
  var x = Object.getOwnPropertyDescriptor(C.prototype, "x").get, kk = C.prototype.kk;
  var callM = C.prototype.callM, has = C.has, read = C.read;
  function lit(p) { return { __proto__: p, m() { return super.who; } }.m(); }
  return function (which) {
    if (which === 0) return make(Base, "a").name + typeof make(Object, "b");
    if (which === 1) return make(5, "c");
    if (which === 2) return x.call(c) + kk.call(c) + callM.call(c);
    if (which === 3) return has(c) + "," + has({});
    if (which === 4) return read({});
    if (which === 5) return callM.call({});
    if (which === 6) return lit({ who: "lit" }) + lit(null);
    return -1;
  };
})();
var p2Bench = (function () {
  var arr = [1, 2, 3];
  function id(x) { return x; }
  function el(a, i) { return a[i] + a[0]; }
  function elt(a, i) { return id(a)[i]; }
  function put(a, i, v) { a[i] = v; return a[i]; }
  function push(a, n) { var i = 0; while (i < n) { a[a.length] = i; i = i + 1; } return a.length; }
  function upd(a, i) { a[i] += 1; return a[i]; }
  function bits(a, b) { return ((a & b) | (a ^ b)) + (a << 3) + (a >> 1) + (~a); }
  function muls(a, b) { return a * b + a % b + a / b; }
  function un(a) { return -a + +a; }
  function incs(a) { var b = a++; var c = ++a; return b + c + a; }
  function addl(a, b) { var s = a; s += b; s += b; return s; }
  function obj(a) { var o = {}; o.x = a; return o.x; }
  function comp(k, v) { var o = { [k]: v }; return o[k]; }
  function has(k, o) { return k in o; }
  function inst(o) { return o instanceof Array; }
  function ty(a) { return typeof a; }
  function glob(a) { return Math.abs(a); }
  function miss() { return missingP2; }
  // the arrays come from functions of their own: an array literal is P5's
  function a0() { return []; }
  function a2() { return [1, 2]; }
  return function (which, x) {
    if (which === 0) return el(arr, 1);
    if (which === 1) return el(arr, 7);
    if (which === 2) return elt(arr, 2) + elt(x, 1);
    if (which === 3) return put(a2(), 1, x) + put(a2(), 5, x);
    if (which === 4) return push(a0(), x);
    if (which === 5) return upd(a2(), 0) + upd(a2(), 9);
    if (which === 6) return bits(x, 6);
    if (which === 7) return muls(x, 4);
    if (which === 8) return un(x);
    if (which === 9) return incs(x);
    if (which === 10) return addl(x, x);
    if (which === 11) return obj(x);
    if (which === 12) return comp(x, 5);
    if (which === 13) return has(x, arr);
    if (which === 14) return inst(arr) + inst(x);
    if (which === 15) return ty(x);
    if (which === 16) return glob(x);
    if (which === 17) return miss();
    return -1;
  };
})();
// package A: calls with more arguments than parameters, rest and the
// arguments object, and a throw with extra arguments pending
var argsBench = (function () {
  function r2(a, ...r) { return r.length + a; }
  function alen() { return arguments.length + (arguments.length > 1 ? arguments[1] : 0); }
  function plain(a) { return a; }
  function thr(a) { throw a; }
  return function (which, n) {
    var s = 0;
    for (var i = 0; i < n; i++) {
      if (which === 0) s = s + r2(i, 1, 2, 3);
      else if (which === 1) s = s + alen(i, 2);
      else if (which === 2) s = s + plain(i, i, {});
      else if (which === 3) s = s + r2(i) + alen();
      else { try { thr(i, {}, 1); } catch (e) { s = s + e; } }
    }
    return s;
  };
})();
// the speed work of the old loop carried over: a string appended to the
// variable that holds it (a local, an argument, a closure variable as
// React's renderToString writes its page), hasOwnProperty.call, and two
// strings added
var appendBench = (function () {
  var a = "";
  function push(u) { return u !== null && (a += u), true; }
  function arg(s, n) { for (var i = 0; i < n; i++) s = s + "<i>" + i + "</i>"; return s; }
  return function (which, n) {
    var s = "";
    if (which === 0) { a = ""; for (var i = 0; i < n; i++) push("<div>" + i + "</div>"); return a.length; }
    if (which === 1) { for (var i = 0; i < n; i++) s = s + "<p>" + i; return s.length; }
    if (which === 2) return arg("", n).length;
    for (var i = 0; i < n; i++) s = "" + i + s.slice(0, 8);
    return s;
  };
})();
var hasOwnBench = (function () {
  var has = Object.prototype.hasOwnProperty;
  return function (which, n) {
    var o = { a: 1, b: 2, 3: 4 }, c = 0;
    for (var i = 0; i < n; i++) {
      if (which === 0) c += has.call(o, "a") + has.call(o, "z") + has.call(o, 3);
      else if (which === 1) c += Object.prototype.hasOwnProperty.call(o, "b");
      else c += has.call([1, 2], 0) + has.call(o, "toString");
    }
    return c;
  };
})();
`

// regvmMicroGolden is what each call of TestRegvmMicro gave, a line each:
// the old loop and the register loop gave the same until the old loop was
// removed (the test compared the two then), and the register loop has
// given these since. -update-regvm-micro writes it again: read the diff
// before it goes into a commit.
const regvmMicroGolden = "testdata/regvm_micro.golden"

var updateRegvmMicro = flag.Bool("update-regvm-micro", false, "rewrite "+regvmMicroGolden)

// The micro functions give the answers of regvmMicroGolden and run in the
// register loop.
func TestRegvmMicro(t *testing.T) {
	type call struct {
		fn   string
		args []any
	}
	calls := []call{{"fibBench", []any{20}}, {"callBench", []any{1000}}, {"fieldBench", []any{1000}},
		{"caseBench", []any{1000}}, {"tailBench", []any{1000}}, {"testBench", []any{1000}}}
	for i := 0; i <= 18; i++ {
		var x any
		switch i {
		case 11:
			x = nil
		case 12:
			x = 3000
		case 14:
			x = 100000
		case 17:
			x = 1000
		}
		calls = append(calls, call{"mixBench", []any{i, x}})
	}
	calls = append(calls, call{"mixBench", []any{11, map[string]any{"x": map[string]any{"y": 5}}}},
		call{"mixBench", []any{12, 1000000}})
	for i := 0; i <= 8; i++ {
		var x any = 3
		if i == 8 {
			x = map[string]any{}
		}
		calls = append(calls, call{"exceptBench", []any{i, x}})
	}
	// each twice: the first call converts the function, the second runs it in
	// the register loop. The function the calls enter is refused, so the
	// entries are those of the second calls.
	entries := len(calls)
	for i := 0; i <= 8; i++ {
		calls = append(calls, call{"windowMicro", []any{i}}, call{"windowMicro", []any{i}})
	}
	entries += 9
	for _, c := range [][]any{{0, 1}, {1, 2.5}, {2, 0}, {2, 1}, {3, 4}, {4, nil}, {5, 3000}} {
		calls = append(calls, call{"closureBench", c}, call{"closureBench", c})
	}
	for i := 0; i <= 9; i++ {
		calls = append(calls, call{"newBench", []any{i, 4}})
	}
	for i := 0; i <= 6; i++ {
		calls = append(calls, call{"classMicro", []any{i}}, call{"classMicro", []any{i}})
	}
	calls = append(calls, call{"newBench", []any{5, 0}}, call{"newBench", []any{7, nil}})
	for _, c := range [][]any{{0, 3}, {1, nil}, {2, nil}, {3, 20}, {4, 3}, {5, nil}, {6, 0}, {6, 1},
		{6, 2}, {7, 4}, {7, 0}, {8, 1}, {9, 5}} {
		calls = append(calls, call{"ctorBench", c}, call{"ctorBench", c})
	}
	for _, x := range []any{nil, 5, "s", true, map[string]any{"a": 1}} {
		calls = append(calls, call{"sloppyBench", []any{0, x}})
	}
	calls = append(calls, call{"sloppyBench", []any{1, 2}}, call{"sloppyBench", []any{2, 2}})
	for i := 0; i <= 4; i++ {
		calls = append(calls, call{"evalBench", []any{i, 3}}, call{"evalBench", []any{i, "s"}})
	}
	for _, c := range [][]any{{0, nil}, {1, nil}, {2, "ab"}, {2, nil}, {3, 7}, {3, "s"}, {4, 50},
		{5, nil}, {6, 5}, {6, 1.5}, {6, "7"}, {6, -2147483648}, {7, 7}, {7, 2147483647}, {7, -8},
		{7, "x"}, {8, 0}, {8, -2147483648}, {8, 2.5}, {8, nil}, {9, 2147483646}, {9, 1.5}, {9, "3"},
		{10, 1073741824}, {10, "ab"}, {10, 0.5}, {11, "v"}, {12, "k"}, {12, 1}, {13, 1}, {13, "x"},
		{13, "length"}, {14, 1}, {15, 1}, {15, "s"}, {15, nil}, {16, -3}, {16, "x"}, {17, nil}} {
		calls = append(calls, call{"p2Bench", c})
	}
	for _, c := range [][]any{{0, 1}, {0, "s"}, {1, 3}, {1, "tdz"}, {1, "x"}, {2, 3}, {2, 0}, {3, 1}, {3, "s"}, {3, "throw"},
		{4, 1}, {4, "s"}, {5, nil}, {4, "throw"}, {5, nil}} {
		calls = append(calls, call{"limitsMicro", c}, call{"limitsMicro", c})
	}
	entries += 12
	for i := 0; i <= 4; i++ {
		calls = append(calls, call{"argsBench", []any{i, 1000}})
	}
	for i := 0; i <= 3; i++ {
		calls = append(calls, call{"appendBench", []any{i, 1000}})
	}
	for i := 0; i <= 2; i++ {
		calls = append(calls, call{"hasOwnBench", []any{i, 1000}})
	}
	m := newTestVM(t)
	if _, err := m.Eval(regvmMicro, EvalGlobal); err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	for _, c := range calls {
		r, err := m.Call(c.fn, c.args...)
		fmt.Fprintf(&out, "%s%v: %v %v\n", c.fn, c.args, r, err)
	}
	t.Logf("converted %d, refused %d, entries %d, last refusal op %d at %d",
		regvmStat(m, 0), regvmStat(m, 1), regvmStat(m, 2), regvmStat(m, 3), regvmStat(m, 4))
	if regvmStat(m, 0) < 10 || regvmStat(m, 2) < entries {
		t.Errorf("the micro functions did not run in the register loop")
	}
	if *updateRegvmMicro {
		if err := os.WriteFile(regvmMicroGolden, []byte(out.String()), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(regvmMicroGolden)
	if err != nil {
		t.Fatal(err)
	}
	got, wl := strings.Split(out.String(), "\n"), strings.Split(string(want), "\n")
	if len(got) != len(wl) {
		t.Fatalf("%d calls, %s has %d lines", len(got)-1, regvmMicroGolden, len(wl)-1)
	}
	for i := range got {
		if got[i] != wl[i] {
			t.Errorf("call %d: got %q, want %q", i+1, got[i], wl[i])
		}
	}
}

// TestRegvmRun runs one micro function for the measurements of stage 0,
// when REGVM_BENCH names it (fib, call, field or an x bench, or a page: react,
// table and spiral, see regvmPage, shop and mui, see regvmApp): REGVM_N runs of REGVM_ARG (default 25 for fib, 1000000 for
// the loops), one warm run first. It prints the answer and the time of each
// run.
func TestRegvmRun(t *testing.T) {
	name := os.Getenv("REGVM_BENCH")
	if name == "" {
		t.Skip("REGVM_BENCH not set")
	}
	n, _ := strconv.Atoi(os.Getenv("REGVM_N"))
	arg, _ := strconv.Atoi(os.Getenv("REGVM_ARG"))
	if arg == 0 {
		arg = map[string]int{"fib": 25}[name]
		if arg == 0 {
			arg = 1000000
		}
	}
	if name == "react" {
		regvmReact(t, n)
		return
	}
	if name == "table" || name == "spiral" {
		regvmPage(t, n, name)
		return
	}
	if name == "shop" || name == "mui" {
		regvmApp(t, n, name)
		return
	}
	m := newTestVM(t)
	if _, err := m.Eval(regvmMicro, EvalGlobal); err != nil {
		t.Fatal(err)
	}
	fn := name + "Bench"
	if _, err := m.Call(fn, arg); err != nil {
		t.Fatal(err)
	}
	var r any
	var err error
	times := make([]time.Duration, n)
	for i := range n {
		start := time.Now()
		r, err = m.Call(fn, arg)
		times[i] = time.Since(start)
		if err != nil {
			t.Fatal(err)
		}
	}
	fmt.Printf("regvm %s arg=%d n=%d result=%v entries=%d ns=", name, arg, n, r, regvmStat(m, 2))
	for _, d := range times {
		fmt.Printf("%d ", d.Nanoseconds())
	}
	fmt.Println()
}

// The scripts of TestRegvmDepth: each recursion writes the depth it reached
// into depthO.d until the call depth or the stack ends it with a RangeError.
// big has 60 locals, and calls itself through Function.prototype.call, a C
// function, and back: JS_Call. depthNew and depthNewBig recurse through new
// X(...), a constructor call, the second with a frame of 2000 locals.
const regvmDepth = `"use strict";
var depthO = { d: 0 };
var depthJS = (function () {
  var o = depthO;
  function rec(n) { o.d = n; return rec(n + 1); }
  return function () { return rec(0); };
})();
var depthBig = (function () {
  var o = depthO;
  function rec(n) { var v1 = n + 1, v2 = n + 1, v3 = n + 1, v4 = n + 1, v5 = n + 1, v6 = n + 1, v7 = n + 1, v8 = n + 1, v9 = n + 1, v10 = n + 1, v11 = n + 1, v12 = n + 1, v13 = n + 1, v14 = n + 1, v15 = n + 1, v16 = n + 1, v17 = n + 1, v18 = n + 1, v19 = n + 1, v20 = n + 1, v21 = n + 1, v22 = n + 1, v23 = n + 1, v24 = n + 1, v25 = n + 1, v26 = n + 1, v27 = n + 1, v28 = n + 1, v29 = n + 1, v30 = n + 1, v31 = n + 1, v32 = n + 1, v33 = n + 1, v34 = n + 1, v35 = n + 1, v36 = n + 1, v37 = n + 1, v38 = n + 1, v39 = n + 1, v40 = n + 1, v41 = n + 1, v42 = n + 1, v43 = n + 1, v44 = n + 1, v45 = n + 1, v46 = n + 1, v47 = n + 1, v48 = n + 1, v49 = n + 1, v50 = n + 1, v51 = n + 1, v52 = n + 1, v53 = n + 1, v54 = n + 1, v55 = n + 1, v56 = n + 1, v57 = n + 1, v58 = n + 1, v59 = n + 1, v60 = n + 1; o.d = n; return rec(n + 1) + v1 + v60; }
  return function () { return rec(0); };
})();
var depthC = (function () {
  var o = depthO;
  function rec(n) { o.d = n; return rec.call(null, n + 1); }
  return function () { return rec(0); };
})();
var depthBigC = (function () {
  var o = depthO;
  function rec(n) { var v1 = n + 1, v2 = n + 1, v3 = n + 1, v4 = n + 1, v5 = n + 1, v6 = n + 1, v7 = n + 1, v8 = n + 1, v9 = n + 1, v10 = n + 1, v11 = n + 1, v12 = n + 1, v13 = n + 1, v14 = n + 1, v15 = n + 1, v16 = n + 1, v17 = n + 1, v18 = n + 1, v19 = n + 1, v20 = n + 1, v21 = n + 1, v22 = n + 1, v23 = n + 1, v24 = n + 1, v25 = n + 1, v26 = n + 1, v27 = n + 1, v28 = n + 1, v29 = n + 1, v30 = n + 1, v31 = n + 1, v32 = n + 1, v33 = n + 1, v34 = n + 1, v35 = n + 1, v36 = n + 1, v37 = n + 1, v38 = n + 1, v39 = n + 1, v40 = n + 1, v41 = n + 1, v42 = n + 1, v43 = n + 1, v44 = n + 1, v45 = n + 1, v46 = n + 1, v47 = n + 1, v48 = n + 1, v49 = n + 1, v50 = n + 1, v51 = n + 1, v52 = n + 1, v53 = n + 1, v54 = n + 1, v55 = n + 1, v56 = n + 1, v57 = n + 1, v58 = n + 1, v59 = n + 1, v60 = n + 1; o.d = n; return rec.call(null, n + 1) + v1 + v60; }
  return function () { return rec(0); };
})();
var depthMap = (function () {
  var o = depthO, a = [0];
  function rec(x) { o.d = o.d + 1; return a.map(rec); }
  return function () { return rec(0); };
})();
var depthNew = (function () {
  var o = depthO;
  function g(n) { o.d = n; return new X(n + 1); }
  function X(n) { this.v = g(n + 1); }
  return function () { return g(0); };
})();
var depthNewBig = (function () {
  var o = depthO;
  function g(n) { o.d = n; return new X(n + 1); }
  function X(n) { BIGVARS; this.v = g(n + 1) + v1 + v2000; }
  return function () { return g(0); };
})();
`

// regvmDepthFloor is, for each recursion of TestRegvmDepth and each
// architecture, the depth the register loop reaches on the generated engine.
// A script recurses at least as deep as before, and may recurse deeper
// (AGENTS.md), so the table only goes up: when a change makes a case deeper,
// raise its floor to the new depth, and never lower a floor without the
// owner's word. Every case ends on the count of levels, so its floor is that
// count: 4997 under the default of 5000 levels and 9996 under
// maxCallDepthLimit, and about half of 4997 for depthMap, where each level of
// the script is also a level of Array.prototype.map. A call through
// Function.prototype.call is made as the call it stands for, so depthC and
// depthBigC take one level a call and reach 4997 too.
// depthNewBig reached 9996 with the register loop, where the old loop's large
// frames ran out of stack bytes at 1982 levels on arm64 and 1928 on amd64.
//
// Where the numbers come from: the generated engine at 167dd40c, natively on
// darwin/arm64 and linux/amd64 and on darwin/amd64 under Rosetta, all the
// same.
var regvmDepthFloor = map[string]map[string]int{
	"arm64": {"depthJS": 4997, "depthBig": 4997, "depthC": 4997, "depthBigC": 4997, "depthMap": 2499, "depthNew": 9996, "depthNewBig": 9996},
	"amd64": {"depthJS": 4997, "depthBig": 4997, "depthC": 4997, "depthBigC": 4997, "depthMap": 2499, "depthNew": 9996, "depthNewBig": 9996},
}

// No path recurses less deep than it did before: pure JavaScript, with small
// and large frames, JavaScript to C to JavaScript, callbacks, and
// constructor calls.
func TestRegvmDepth(t *testing.T) {
	floor, ok := regvmDepthFloor[runtime.GOARCH]
	if !ok {
		t.Fatalf("no depth floors for %s", runtime.GOARCH)
	}
	for _, fn := range []string{"depthJS", "depthBig", "depthC", "depthBigC", "depthMap", "depthNew", "depthNewBig"} {
		m := newTestVM(t)
		if strings.HasPrefix(fn, "depthNew") {
			// the most levels, so that a frame of depthNewBig that
			// takes more stack than now ends it before the count
			m.SetMaxStackSize(maxCallDepthLimit)
		}
		if _, err := m.Eval(regvmDepthScript(), EvalGlobal); err != nil {
			t.Fatal(err)
		}
		_, err := m.Call(fn)
		if err == nil || !strings.Contains(err.Error(), "RangeError") {
			t.Fatalf("%s: %v, want a RangeError", fn, err)
		}
		v, err := m.Eval("depthO.d", EvalGlobal)
		if err != nil {
			t.Fatal(err)
		}
		d, conv := v.(int), regvmStat(m, 0)
		t.Logf("%s: depth %d, floor %d (%d converted)", fn, d, floor[fn], conv)
		if conv < 2 {
			t.Errorf("%s: the recursion did not run in the register loop", fn)
		}
		if d < floor[fn] {
			t.Errorf("%s: %d levels, less than its floor of %d", fn, d, floor[fn])
		}
	}
}

// regexpNestFloor is the deepest nesting of groups, lookarounds and classes a
// regexp still compiles and runs with. The parser counts a level of the
// internal depth for each group and class it enters (re_parse_disjunction,
// re_parse_nested_class), and the whole pattern is one more, so the count of
// JS_MAX_INTERNAL_DEPTH (2000) ends it at 1999, one level past it with a
// SyntaxError. The match itself does not recurse: lre_exec_backtrack keeps
// what it backtracks to on a stack in the heap, so a lookahead in a repeated
// group runs on a subject as long as memory allows. Like regvmDepthFloor, it
// only goes up. Measured at 167dd40c, the same on every architecture.
const regexpNestFloor = 1999

// No regexp nests less deep than it did before, or stops on a long subject:
// nested lookaheads, negative lookaheads, lookbehinds, groups and classes, and
// lookarounds in a repeated group on 4 million characters.
func TestRegexpDepth(t *testing.T) {
	m := newTestVM(t)
	for _, c := range []struct{ name, js string }{
		{"lookahead", `(n) => new RegExp("(?=".repeat(n) + "a" + ")".repeat(n)).test("a")`},
		{"negative_lookahead", `(n) => new RegExp("(?!" + "(?=".repeat(n - 1) + "b" + ")".repeat(n)).test("a")`},
		{"lookbehind", `(n) => new RegExp("a" + "(?<=".repeat(n) + "a" + ")".repeat(n)).test("a")`},
		{"group", `(n) => new RegExp("(?:".repeat(n) + "a" + ")".repeat(n)).test("a")`},
		{"class", `(n) => new RegExp("[".repeat(n) + "a" + "]".repeat(n), "v").test("a")`},
	} {
		if _, err := m.Eval("var nest = "+c.js, EvalGlobal); err != nil {
			t.Fatal(err)
		}
		runs := func(n int) (bool, error) {
			v, err := m.Eval(fmt.Sprintf("nest(%d)", n), EvalGlobal)
			if err == nil && v != true {
				t.Fatalf("%s: nesting %d gave %v, want true", c.name, n, v)
			}
			return err == nil, err
		}
		lo, hi := 1, 2
		for ok, _ := runs(hi); ok; ok, _ = runs(hi) {
			if hi >= 1<<20 {
				t.Fatalf("%s: nesting %d still runs", c.name, hi)
			}
			lo, hi = hi, hi*2
		}
		for hi-lo > 1 {
			mid := (lo + hi) / 2
			if ok, _ := runs(mid); ok {
				lo = mid
			} else {
				hi = mid
			}
		}
		_, err := runs(lo + 1)
		t.Logf("%s: deepest %d, floor %d, at %d: %v", c.name, lo, regexpNestFloor, lo+1, firstLine(err))
		if lo < regexpNestFloor {
			t.Errorf("%s: nests %d deep, less than its floor of %d", c.name, lo, regexpNestFloor)
		}
		if !strings.Contains(err.Error(), "SyntaxError") {
			t.Errorf("%s: at %d: %v, want a SyntaxError", c.name, lo+1, err)
		}
	}
	for _, re := range []string{`/^(?:(?=a)a)*$/`, `/^(?:(?=(a))a)*$/`, `/^(?:(?!b)a)*$/`, `/^(?:(?<=a|^)a)*$/`} {
		if v, err := m.Eval(re+".test('a'.repeat(1 << 22))", EvalGlobal); err != nil || v != true {
			t.Errorf("%s on 4194304 characters: %v %v, want true", re, v, err)
		}
	}
}

// JS_RegvmStat 5 and 6 measure the two frames a recursion pays the stack
// for: a frame of JS_RegvmRun, and one of JS_CallInternal, the entry of a
// call from C, which a call of a C function the loop makes directly counts
// as taken. Both are measured. How deep the recursions go through them is
// TestRegvmDepth's floors.
func TestRegvmFrame(t *testing.T) {
	m := newTestVM(t)
	rv, entry := regvmStat(m, 5), regvmStat(m, 6)
	t.Logf("a frame of JS_RegvmRun takes %d bytes of stack, the entry JS_CallInternal %d", rv, entry)
	if rv <= 0 || entry <= 0 {
		t.Errorf("JS_RegvmRun %d bytes, JS_CallInternal %d", rv, entry)
	}
	// 12, what a call of a C function from call, apply or a bound function
	// counts as taken of the frame of JS_CallInternal it skips
	// (js_call_c_hop): -1 would send those calls through JS_CallInternal
	if hop := regvmStat(m, 12); hop < 0 || hop >= entry {
		t.Errorf("js_call_c_hop counts %d bytes of the %d of JS_CallInternal", hop, entry)
	}
	directCCalls(t, m, false)
	if hop := regvmStat(m, 12); hop != -1 {
		t.Errorf("direct calls off: js_call_c_hop counts %d bytes, want -1", hop)
	}
}

// regvmReact is TestRegvmRun's react: the React page of compare/engines
// (react-ssr.js, render()), checked against its HTML, one warm render, then
// n renders.
func regvmReact(t *testing.T, n int) {
	src, err := os.ReadFile("compare/engines/react/react-ssr.js")
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("compare/engines/react/react-ssr.expected.html")
	if err != nil {
		t.Fatal(err)
	}
	m := newTestVM(t)
	if _, err := m.Eval(string(src), EvalGlobal); err != nil {
		t.Fatal(err)
	}
	html, err := m.Call("render")
	if err != nil {
		t.Fatal(err)
	}
	if html != string(want) {
		t.Fatalf("the HTML of the React page differs (%d bytes, want %d)", len(html.(string)), len(want))
	}
	times := make([]time.Duration, n)
	for i := range n {
		start := time.Now()
		_, err = m.Call("render")
		times[i] = time.Since(start)
		if err != nil {
			t.Fatal(err)
		}
	}
	fmt.Printf("regvm react n=%d result=%d converted=%d refused=%d entries=%d ns=", n, len(html.(string)), regvmStat(m, 0), regvmStat(m, 1), regvmStat(m, 2))
	for _, d := range times {
		fmt.Printf("%d ", d.Nanoseconds())
	}
	fmt.Println()
}

// regvmDepthScript is regvmDepth with the 2000 locals of depthNewBig.
func regvmDepthScript() string { return regvmBigVars.Replace(regvmDepth) }

var regvmBigVars = func() *strings.Replacer {
	var b strings.Builder
	b.WriteString("var ")
	for i := 1; i <= 2000; i++ {
		if i > 1 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "v%d = n + %d", i, i)
	}
	return strings.NewReplacer("BIGVARS", b.String())
}()

// regvmPage is TestRegvmRun's table and spiral: a page of the bench bundle
// with tifa's four functions, from REGVM_PAGES (the folder that holds
// bundles/bench.js, prelude/iterator-shim.js and in/bench/PAGE.json), a
// render as tifa makes it: startRender(json), the jobs, renderResult. One
// warm render, then n. It prints the length and a sum of the HTML, which
// the old and the register loop must give the same.
func regvmPage(t *testing.T, n int, page string) {
	dir := os.Getenv("REGVM_PAGES")
	if dir == "" {
		t.Skip("REGVM_PAGES not set")
	}
	read := func(p string) string {
		b, err := os.ReadFile(dir + "/" + p)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	m := newTestVM(t)
	for _, err := range []error{m.RegisterEscapeText("__tifa_escape"), m.RegisterStartTag("__tifa_start_tag"),
		m.RegisterEndTag("__tifa_end_tag"), m.RegisterCopyProps("__tifa_copy_props")} {
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, p := range []string{"prelude/iterator-shim.js", "bundles/bench.js"} {
		if _, err := m.Eval(read(p), EvalGlobal); err != nil {
			t.Fatalf("%s: %v", p, err)
		}
	}
	json := read("in/bench/" + page + ".json")
	g := m.GlobalObject()
	rr, err := m.NewAtom("renderResult")
	if err != nil {
		t.Fatal(err)
	}
	st, err := m.NewAtom("startRender")
	if err != nil {
		t.Fatal(err)
	}
	start, err := g.GetPropertyValue(st)
	if err != nil {
		t.Fatal(err)
	}
	render := func() string {
		if _, err := start.Call(g, json); err != nil {
			t.Fatal(err)
		}
		if _, err := m.ExecutePendingJobs(); err != nil {
			t.Fatal(err)
		}
		r, err := g.Property(rr)
		if err != nil {
			t.Fatal(err)
		}
		s, ok := r.(string)
		if !ok {
			t.Fatalf("no renderResult after the jobs ran: %T", r)
		}
		if err := g.SetProperty(rr, Undefined{}); err != nil {
			t.Fatal(err)
		}
		return s
	}
	html := render()
	times := make([]time.Duration, n)
	for i := range n {
		t0 := time.Now()
		if h := render(); len(h) != len(html) {
			t.Fatal("the HTML changed between renders")
		}
		times[i] = time.Since(t0)
	}
	fmt.Printf("regvm %s n=%d result=%d sum=%x converted=%d refused=%d entries=%d ns=", page, n, len(html), sha256.Sum256([]byte(html)), regvmStat(m, 0), regvmStat(m, 1), regvmStat(m, 2))
	for _, d := range times {
		fmt.Printf("%d ", d.Nanoseconds())
	}
	fmt.Println()
}
