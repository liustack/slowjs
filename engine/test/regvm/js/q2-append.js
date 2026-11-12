// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// A string appended to the variable that holds it grows in place, as the old
// loop's js_add_string_store lets it (js_add_string_into in engine/quickjs.c):
// x = x + y into a local or an argument (ADDL, ADDKL, rv_binop_slow in
// engine/regvm-ops-basic.h), a += u into a closure variable (ADDVR in
// engine/regvm-ops-closure.h), attached to its frame and closed. The string
// must not grow when anything else holds it (a copy kept, an object, an
// array, another variable), and the right side may be a rope, the same
// string, a number, an object whose valueOf runs JS that writes the
// variable, or throw. Strings are long (the append starts at 64 characters).
(function () {
  "use strict";
  var log = [];
  var inner = {};
  var long = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ_-";
  function sum(s) {
    var h = 0;
    for (var i = 0; i < s.length; i++) h = (h * 31 + s.charCodeAt(i)) | 0;
    return s.length + ":" + h;
  }

  // a local and an argument, with copies kept along the way
  function loc(n, part) {
    var s = long, kept = [], o = {};
    for (var i = 0; i < n; i++) {
      s = s + part;
      if (i % 7 === 3) kept.push(s);
      if (i % 11 === 5) o.k = s;
      s = s + "-" ;
    }
    return [sum(s), kept.map(sum).join(","), o.k === undefined ? "-" : sum(o.k)].join(" ");
  }
  function arg(s, n, part) {
    var other;
    for (var i = 0; i < n; i++) {
      s = s + part;
      if (i === 2) other = s;
    }
    return sum(s) + " " + (other === undefined ? "-" : sum(other));
  }
  // the same string on both sides, on the right only, a constant
  function sides(n) {
    var s = long, t = long + long;
    for (var i = 0; i < n; i++) {
      s = s + s;
      t = "<" + t;
      t = t + "</div>";
      if (s.length > 5000) s = long;
    }
    return sum(s) + " " + sum(t);
  }
  // not strings: numbers, a BigInt with a string, an object with valueOf,
  // a Symbol that throws, the variable kept by the throw
  function mixed(v) {
    var s = long, r = [];
    s = s + v;
    r.push(sum(String(s)));
    var x = 5;
    x = x + v;
    r.push(typeof x === "symbol" ? "sym" : String(x).length);
    return r.join(" ");
  }
  function throws(v) {
    var s = long + long;
    try { s = s + v; } catch (e) { return sum(s) + " " + e.constructor.name; }
    return sum(s);
  }
  // a captured local, appended in its own frame and read by its closure
  function captured(n) {
    var s = long;
    function peek() { return s.length; }
    var seen = [];
    for (var i = 0; i < n; i++) {
      s = s + "x" + i;
      if (i % 5 === 0) seen.push(peek());
    }
    return sum(s) + " " + seen.join(",");
  }

  // a closure variable: attached while the outer frame runs, then closed
  function builder() {
    var a = "";
    var n = 0;
    var keep = null;
    function push(u) { a += u; }
    function pushv(u) { return u !== null && (a += u), true; }
    function pushr(u) { return (a += u).length; }
    function count(k) { n += k; }
    function countv(k) { var r = (n += k); return r; }
    function hold() { keep = a; }
    function take() { return a; }
    function reset() { a = ""; }
    return { push: push, pushv: pushv, pushr: pushr, count: count, countv: countv, hold: hold, take: take, reset: reset,
             n: function () { return n; }, keep: function () { return keep; } };
  }
  function attached(m) {
    var a = "";
    function push(u) { a += u; }
    inner.attached = push;
    for (var i = 0; i < m; i++) push(long.slice(i % 10) + i);
    var mid = a;
    for (var i = 0; i < m; i++) push("<b>" + i + "</b>");
    return sum(a) + " " + sum(mid);
  }
  function closed(m) {
    var b = builder(), r = [];
    for (var i = 0; i < m; i++) {
      b.push(long);
      b.push("<span>" + i + "</span>");
      if (i === 3) b.hold();
      b.count(i);
      b.pushv(i % 3 ? "<i>" + i + "</i>" : null);
      r.push(b.pushr(long), b.countv(i));
    }
    r.push(sum(b.take()), sum(b.keep()), b.n());
    b.reset();
    b.push(long); b.push(long); b.push(1); b.push(null); b.push({ toString() { return "obj"; } });
    r.push(sum(b.take()));
    b.count(2147483647); b.count(1); b.count(0.5);
    r.push(b.n());
    return r.join(" ");
  }
  // valueOf of the right side writes the closure variable while the add runs
  function reentry() {
    var a = long + long, r = [];
    var o = { valueOf() { a = "changed by valueOf"; return 7; } };
    function push(u) { a += u; }
    inner.reentry = push;
    push(o);
    r.push(a);
    push(long);
    r.push(sum(a));
    try { push(Symbol("s")); } catch (e) { r.push(e.constructor.name, sum(a)); }
    try { push({ valueOf() { throw new Error("vo"); } }); } catch (e) { r.push(e.message, sum(a)); }
    return r.join(" ");
  }

  for (var rep = 0; rep < 3; rep++) {
    log.push(loc(40 + rep, "ab"), loc(20, long));
    log.push(arg(long, 30, "xyz"), arg("short", 30, "q"), arg(long, 5, long + long + long));
    log.push(sides(8));
    log.push(mixed(1), mixed(2.5), mixed("s"), mixed({ valueOf() { return 3; } }));
    log.push(throws(Symbol("t")), throws(1), throws(10n), throws({ toString() { throw new TypeError("ts"); } }));
    log.push(captured(30), attached(25), closed(12), reentry());
  }
  var bb = builder();
  bb.push(long); bb.push(long); bb.count(1); bb.count(1); bb.pushv("x"); bb.pushv("y"); bb.pushr("z"); bb.pushr("w"); bb.countv(1); bb.countv(2);
  rvExpect(loc, arg, sides, mixed, throws, captured, attached, closed, reentry, sum, builder,
           inner.attached, inner.reentry, bb.push, bb.count, bb.pushv, bb.pushr, bb.countv);
  return log.join("\n");
})()
