// Copyright (c) 2026 The liustack/tifajs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

// The step of a for in loop and the read o[k] of the key it just gave: keys
// deleted, added, and deleted and added again during the loop, the shape of
// the object changed inside the body (a key made a getter or not enumerable,
// the object frozen), keys shadowed on the prototype chain, keys that are not
// enumerable, getters, arrays, typed arrays, strings, arguments, the key
// written to a local, an argument, a captured binding, a global, a property
// and a destructuring target, the key changed before the read, the read on
// another object of the same shape, o[k] += and o[k]++, a body that throws,
// break, continue and return, and generators stopped inside a loop across a
// collection. The file is what Node 24 prints.
"use strict";
var out = [];
function show(name, f) {
  var r;
  try { r = f(); } catch (e) { r = "throws " + e.name + " " + e.message; }
  out.push(name + ": " + r);
}
function pairs(o) {
  var r = [];
  for (var k in o) r.push(k + "=" + String(o[k]));
  return r.join(",");
}
function make() { return { a: 1, b: 2, c: 3, d: 4 }; }

show("plain", function () { return pairs(make()); });
show("empty", function () { return pairs({}) || "-"; });
show("one key", function () { return pairs({ only: "x" }); });
show("many keys", function () {
  var o = {};
  for (var i = 0; i < 40; i++) o["k" + i] = i;
  return pairs(o).length;
});
show("numeric looking keys", function () {
  return pairs({ b: 1, "1": 2, "01": 3, "-0": 4, "4294967295": 5, "4294967294": 6, a: 7 });
});
show("symbols left out", function () {
  var o = { a: 1 };
  o[Symbol("s")] = 2;
  o.b = 3;
  return pairs(o);
});
show("same shape, read on the other object", function () {
  var o = make(), q = make(), r = [];
  q.c = 30;
  for (var k in o) r.push(k + "=" + q[k]);
  return r.join(",");
});
show("other shape, read on the other object", function () {
  var o = make(), q = { d: 40, c: 30, b: 20, a: 10 }, r = [];
  for (var k in o) r.push(k + "=" + q[k]);
  return r.join(",");
});
show("key changed before the read", function () {
  var o = make(), r = [];
  for (var k in o) { var j = k; k = k === "a" ? "d" : k === "d" ? "zz" : "a"; r.push(j + ">" + k + "=" + o[k]); }
  return r.join(",");
});
show("key made again before the read", function () {
  var o = make(), r = [];
  for (var k in o) { k = (k + "!").slice(0, -1); r.push(k + "=" + o[k]); }
  return r.join(",");
});
show("keys of an array read on other objects", function () {
  var a = [5, 6, 7], q = { 0: "q0", 2: "q2", x: 1 }, s = "st", r = [];
  for (var k in a) r.push(k + "=" + a[k] + "," + q[k] + "," + s[k]);
  return r.join(" ");
});
show("key made a number before the read", function () {
  var o = { a: 1, 0: "zero", b: 2 }, r = [];
  for (var k in o) { r.push(k + "=" + o[k]); k = 0; r.push(o[k]); }
  return r.join(",");
});
show("read of a key of the loop outside", function () {
  var o = make(), last;
  for (var k in o) last = k;
  return last + "=" + o[last] + " " + o[k];
});
show("compound and update", function () {
  var o = make();
  for (var k in o) { o[k] += 10; o[k]++; o[k] *= 2; }
  return pairs(o);
});
show("written during the loop", function () {
  var o = make(), r = [];
  for (var k in o) { o[k] = k + k; r.push(o[k]); }
  return r.join(",") + " " + pairs(o);
});

// keys and shapes changed during the loop
show("next key deleted", function () {
  var o = make(), r = [];
  for (var k in o) { r.push(k + "=" + o[k]); if (k === "a") delete o.b; }
  return r.join(",");
});
show("key just given deleted", function () {
  var o = make(), r = [];
  for (var k in o) { delete o[k]; r.push(k + "=" + o[k]); }
  return r.join(",") + " " + (pairs(o) || "-");
});
show("all keys deleted at the first", function () {
  var o = make(), r = [];
  for (var k in o) { r.push(k); for (var j in o) delete o[j]; }
  return r.join(",");
});
show("deleted and added again", function () {
  var o = make(), r = [];
  for (var k in o) { r.push(k + "=" + o[k]); if (k === "a") { delete o.c; o.c = 33; } }
  return r.join(",") + " " + pairs(o);
});
show("added during", function () {
  var o = make(), r = [];
  for (var k in o) { r.push(k + "=" + o[k]); o["n" + k] = 0; }
  return r.join(",") + " " + Object.keys(o).length;
});
show("value changed, shape kept", function () {
  var o = make(), r = [];
  for (var k in o) { r.push(k + "=" + o[k]); o.d = "D" + k; }
  return r.join(",");
});
show("a later key made a getter", function () {
  var o = make(), r = [], calls = 0;
  for (var k in o) {
    if (k === "a") Object.defineProperty(o, "c", { get: function () { calls++; return "got"; }, enumerable: true, configurable: true });
    r.push(k + "=" + o[k]);
  }
  return r.join(",") + " " + calls;
});
show("a later key made not enumerable", function () {
  var o = make(), r = [];
  for (var k in o) { if (k === "a") Object.defineProperty(o, "c", { enumerable: false }); r.push(k + "=" + o[k]); }
  return r.join(",");
});
show("the key just given made a getter", function () {
  var o = make(), r = [];
  for (var k in o) {
    Object.defineProperty(o, k, { get: function () { return "g"; }, enumerable: true, configurable: true });
    r.push(k + "=" + o[k]);
  }
  return r.join(",");
});
show("frozen during", function () {
  var o = make(), r = [];
  for (var k in o) { if (k === "a") Object.freeze(o); r.push(k + "=" + o[k]); try { o[k] = 0; } catch (e) { r.push(e.name); } }
  return r.join(",");
});
show("sealed and not extensible", function () {
  var o = Object.seal(make()), q = Object.preventExtensions({ x: 1, y: 2 });
  return pairs(o) + " " + pairs(q);
});
show("shape changed back and forth", function () {
  var o = make(), r = [];
  for (var k in o) { o.tmp = 1; delete o.tmp; r.push(k + "=" + o[k]); }
  return r.join(",");
});

// the prototype chain, flags and accessors
show("shadowed", function () {
  var base = { a: "ba", z: "bz", y: "by" };
  var top = Object.create(base);
  top.a = "ta"; top.t = "tt";
  Object.defineProperty(top, "z", { value: "tz", enumerable: false });
  return pairs(top);
});
show("getter on the prototype, data on the object", function () {
  var proto = {};
  Object.defineProperty(proto, "a", { get: function () { return "proto getter"; }, enumerable: true });
  var o = Object.create(proto);
  o.b = 2;
  return pairs(o);
});
show("own getters and setters", function () {
  var n = 0;
  var o = { a: 1, get b() { n++; return "b" + n; }, set c(v) {}, d: 4 };
  return pairs(o) + " " + n;
});
show("getter that deletes the next key", function () {
  var o = { a: 1, get b() { delete this.c; return "B"; }, c: 3, d: 4 };
  return pairs(o);
});
show("getter that adds keys", function () {
  var o = { get a() { this.e = 5; return "A"; }, b: 2 };
  return pairs(o) + " " + Object.keys(o).join("");
});
show("not enumerable", function () {
  var o = { a: 1 };
  Object.defineProperty(o, "h", { value: 2, enumerable: false });
  o.b = 3;
  return pairs(o);
});
show("class instance and methods", function () {
  class A { constructor() { this.x = 1; this.y = 2; } m() {} }
  return pairs(new A());
});
show("error object", function () {
  var e = new Error("m");
  e.code = 7;
  return pairs(e);
});

// exotic objects
show("array", function () { return pairs([5, 6, , 8]); });
show("array with keys", function () { var a = [1, 2]; a.x = "ax"; return pairs(a); });
show("typed array", function () { return pairs(new Uint8Array([3, 4])) + " " + pairs(new Float64Array([0.5])); });
show("string", function () { return pairs("xyz"); });
show("String object", function () { var s = new String("ab"); s.q = 1; return pairs(s); });
show("arguments", function () { return (function () { return pairs(arguments); })("p", "q"); });
show("function", function () { var f = function () {}; f.prop = 1; return pairs(f); });
show("null prototype", function () { var o = Object.create(null); o.b = 1; o.a = 2; return pairs(o); });
show("number and null", function () { return "[" + pairs(5) + "][" + pairs(null) + "][" + pairs(undefined) + "]"; });
show("proxy", function () {
  var p = new Proxy({ a: 1, b: 2 }, { get: function (t, k) { return "p" + String(t[k]); } });
  return pairs(p);
});

// where the key goes
show("let and const", function () {
  var o = make(), r = [];
  for (let k in o) r.push(k + "=" + o[k]);
  for (const k in o) r.push(k);
  return r.join(",");
});
show("captured", function () {
  var o = make(), fs = [];
  for (let k in o) fs.push(function () { return k + "=" + o[k]; });
  return fs.map(function (f) { return f(); }).join(",");
});
show("captured var", function () {
  var o = make(), f;
  for (var k in o) { f = function () { return k; }; }
  return f() + "=" + o[f()];
});
show("argument", function () {
  return (function (k, o) { var r = []; for (k in o) r.push(k + "=" + o[k]); return r.join(","); })("x", make());
});
show("property", function () {
  var t = {}, o = make(), r = [];
  for (t.p in o) r.push(t.p + "=" + o[t.p]);
  return r.join(",");
});
show("element", function () {
  var t = [], o = make(), r = [];
  for (t[0] in o) r.push(t[0] + "=" + o[t[0]]);
  return r.join(",");
});
show("destructured", function () {
  var o = { ab: 1, cd: 2 }, r = [], a, b;
  for ([a, b] in o) r.push(a + b);
  return r.join(",");
});

// leaving a loop
show("break", function () {
  var o = make(), r = [];
  for (var k in o) { if (k === "c") break; r.push(o[k]); }
  return r.join(",");
});
show("continue", function () {
  var o = make(), r = [];
  for (var k in o) { if (k === "b") continue; r.push(o[k]); }
  return r.join(",");
});
show("return", function () {
  function find(o, v) { for (var k in o) if (o[k] === v) return k; return "none"; }
  return find(make(), 3) + "," + find(make(), 9);
});
show("throw in the body", function () {
  var o = make(), r = [];
  try { for (var k in o) { r.push(o[k]); if (k === "b") throw new Error("at " + k); } } catch (e) { r.push(e.message); }
  for (var k2 in o) r.push(k2);
  return r.join(",");
});
show("throw from a getter", function () {
  var o = { a: 1, get b() { throw new TypeError("getter"); }, c: 3 };
  return pairs(o);
});
show("labelled", function () {
  var r = [];
  outer: for (var a in { x: 1, y: 2, z: 3 }) {
    for (var b in { p: 1, q: 2 }) { if (a === "y") continue outer; if (a === "z" && b === "q") break outer; r.push(a + b); }
  }
  return r.join(",");
});
show("nested over the same object", function () {
  var o = { a: 1, b: 2 }, r = [];
  for (var i in o) for (var j in o) r.push(i + j + "=" + (o[i] + o[j]));
  return r.join(",");
});
show("recursion", function () {
  function walk(o) { var n = 0; for (var k in o) n += typeof o[k] === "object" && o[k] ? walk(o[k]) : o[k]; return n; }
  return walk({ a: 1, b: { c: 2, d: { e: 3 } }, f: [4, 5] });
});

// generators stopped inside a loop
show("generators", function () {
  function* gen(o) { for (var k in o) yield k + "=" + o[k]; }
  var o = make(), g1 = gen(o), g2 = gen(make()), r = [];
  r.push(g1.next().value, g2.next().value);
  delete o.b;
  o.e = 5;
  if (typeof gc === "function") gc();
  r.push(g1.next().value, g1.next().value, g2.next().value);
  r.push(g1.return("stop").value, String(g1.next().done));
  try { g2.throw(new Error("in")); } catch (e) { r.push(e.message); }
  return r.join(",");
});
show("many loops", function () {
  var n = 0, o = make();
  for (var i = 0; i < 1000; i++) for (var k in o) n += o[k];
  return n;
});

// a global as the key, in the code of the script
var go = { g1: "x", g2: "y" }, gr = [];
for (var gk in go) gr.push(gk + "=" + go[gk]);
out.push("global: " + gr.join(",") + " " + gk);
out.join("\n");
