// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// s[i] on a string: an index inside the length gives the string of the code
// unit there (8-bit and 16-bit strings, lone surrogates, ropes), and every
// other read takes the generic path: an index out of the length or negative,
// which String.prototype and Object.prototype can answer (a value or a getter
// that sees the string as its receiver), numbers that are not array indexes
// (fractions, -0, 2**32 and more, NaN, Infinity), keys that are strings
// ("0", "01", "length", other names), symbols, String objects, a subclass of
// String, and the other primitives. The key is a constant, a local and a
// temp, and the string a local and a temp. The file is what Node 24 prints.
"use strict";
var out = [];
function show(name, f) {
  var r;
  try { r = f(); } catch (e) { r = "throws " + e.name; }
  out.push(name + ": " + r);
}
// a value as text, a string by its code units so lone surrogates show
function t(v) {
  if (typeof v !== "string") return typeof v === "symbol" ? "symbol" : String(v);
  var a = [];
  for (var i = 0; i < v.length; i++) a.push(v.charCodeAt(i).toString(16));
  return "'" + a.join(" ") + "'";
}
function at(s, i) { return s[i]; }
function all(s) {
  var a = [];
  for (var i = 0; i < s.length; i++) a.push(t(s[i]));
  return a.join(",");
}
function sum(a, b, i) { return (a + b)[i]; }

var s8 = "abcéÿ";
var s16 = "a€😀z";
var lone = "x\ud800y\udfffz\ud800";
show("8-bit", function () { return all(s8); });
show("16-bit", function () { return all(s16); });
show("lone surrogates", function () { return all(lone); });
show("empty", function () { return [t(""[0]), t(at("", 0))].join(); });
show("constant keys", function () { return [t(s8[0]), t(s8[4]), t(s16[2]), t(s8[5]), t(s8[-1])].join(); });
show("temp string", function () { return [t(sum("ab", "cd", 2)), t(sum("ab", "€", 2)), t(sum("ab", "cd", 9))].join(); });
show("temp key", function () { var i = 1; return [t(s8[i + 1]), t(s16[i * 3]), t(s8[i - 2])].join(); });
show("rope", function () {
  var r = "";
  for (var i = 0; i < 40; i++) r += "0123456789";
  var r2 = r + "€" + r;
  return [t(r[0]), t(r[399]), t(r[400]), t(r2[400]), t(r2[401]), t(r2.length)].join();
});
show("long", function () { var l = "q".repeat(100000) + "!"; return [t(l[100000]), t(l[99999]), t(l[100001])].join(); });
show("same string twice", function () { var a = s8[1], b = s8[1]; return [a === b, a === "b", typeof a].join(); });

// keys that are numbers but not indexes inside the length
var keys = [0, 1, 4, 5, 6, -1, -0, 0.5, 1.5, -1.5, 2 ** 31 - 1, 2 ** 31, 2 ** 32 - 2, 2 ** 32 - 1, 2 ** 32, 2 ** 53, NaN, Infinity, -Infinity];
show("numbers", function () { return keys.map(function (k) { return t(s8[k]); }).join(); });
// keys that are strings
var skeys = ["0", "1", "01", "1.0", "-0", "-1", "4", "5", " 1", "1 ", "+1", "length", "charAt", "nope", "__proto__", ""];
show("strings", function () {
  return skeys.map(function (k) { var v = s8[k]; return typeof v === "function" ? "fn" : typeof v === "object" ? "obj" : t(v); }).join();
});
show("symbols", function () { return [typeof s8[Symbol.iterator], t(s8[Symbol("x")]), t(s8[Symbol.toStringTag])].join(); });
show("length", function () { return [s8.length, s8["length"], s16.length, at(lone, "length")].join(); });

// String objects and a subclass: indexes are own properties of the object
var so = new String("str");
show("String object", function () { return [t(so[0]), t(so[2]), t(so[3]), t(so[-1]), t(so["1"])].join(); });
so[5] = "five";
show("String object with an index past its length", function () { return [t(so[5]), t(so[4]), Object.keys(so).join("|")].join(); });
class MyStr extends String { get [1]() { return "getter"; } }
var ms = new MyStr("abc");
show("subclass", function () { return [t(ms[0]), t(ms[1]), t(ms[2]), t(ms[3]), ms instanceof String, ms.length].join(); });

// indexes on the prototypes: never asked inside the length, asked outside it
var seen = [];
function withProtos(f) {
  Object.defineProperty(String.prototype, "1", { get: function () { seen.push("S1 " + typeof this); return "proto1"; }, configurable: true });
  String.prototype[6] = "proto6";
  Object.defineProperty(Object.prototype, "7", { get: function () { seen.push("O7 " + typeof this + " " + this.length); return "obj7"; }, configurable: true });
  Object.prototype[-1] = "minus";
  Object.prototype["0.5"] = "half";
  try { return f(); } finally {
    delete String.prototype[1];
    delete String.prototype[6];
    delete Object.prototype[7];
    delete Object.prototype[-1];
    delete Object.prototype["0.5"];
  }
}
show("indexes on the prototypes", function () {
  seen = [];
  var r = withProtos(function () {
    return [t(s8[1]), t(at(s8, 1)), t(s8[5]), t(s8[6]), t(at(s8, 6)), t(s8[7]), t(at("ab", 7)), t(s8[-1]), t(s8[0.5]), t(""[1]), t(""[6])].join();
  });
  return r + " [" + seen.join(";") + "]";
});
show("after the prototypes", function () { return [t(s8[1]), t(s8[6]), t(s8[7]), t(s8[-1])].join(); });
show("a prototype of String.prototype", function () {
  var old = Object.getPrototypeOf(String.prototype);
  var p = new Proxy({}, { get: function (o, k, r) { return "trap " + String(k) + " " + typeof r; } });
  Object.setPrototypeOf(String.prototype, p);
  try { return [t(s8[2]), t(s8[9]), t(at(s8, 9)), t(s8[-3])].join(); } finally { Object.setPrototypeOf(String.prototype, old); }
});

// the other primitives
show("primitives", function () {
  Number.prototype[0] = "n0";
  Boolean.prototype[0] = "b0";
  try { return [t((5)[0]), t((1.5)[0]), t(true[0]), t((5)[1]), t(10n[0]), t(Symbol.iterator[0])].join(); }
  finally { delete Number.prototype[0]; delete Boolean.prototype[0]; }
});
show("null", function () { return null[0]; });
show("undefined", function () { var u; return u[0]; });

// a loop that reads every unit, as code that walks a string does
show("walk", function () {
  var text = "Hello, wörld € 😀", n = 0, c = "";
  for (var i = 0; i < text.length; i++) { c += text[i]; n += text[i].length; }
  return [c === text, n].join();
});
out.join("\n");
