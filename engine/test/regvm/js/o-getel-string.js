// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// rvdiff: s[i] with a string as the object (GETEL, GETELT, GETELK, GETELKT
// and a key in a temp), against Reflect.get on the String object of the
// string with the string as the receiver, which never takes the path of the
// loop. Strings: empty, 8-bit, 16-bit, lone surrogates, a rope, a long one,
// atoms, strings of another realm. Keys: every int from -2 to past the
// length, the int limits, numbers that are not indexes, strings of numbers
// and symbols. With indexes, getters and a proxy on String.prototype and
// Object.prototype, in this realm and in another one (a function of the other
// realm reads with its own String.prototype). The results also go to the slot
// of the key and of the string.
(function () {
"use strict";
var out = [];
var log = [], nlog = 0;
function L(s) { nlog++; if (log.length < 20) log.push(s); }
function rd(s, i) { return s[i]; }
function rdk(s) { return s[2]; }
function rdk0(s) { return s[0]; }
function rdt(a, b, i) { return (a + b)[i]; }
function rdkt(a, b) { return (a + b)[1]; }
function rdx(s, i) { return s[i + 1]; }
function selfKey(s, i) { i = s[i]; return i; }
function selfStr(s, i) { s = s[i]; return s; }
function ref(s, k) { return Reflect.get(Object(s), k, s); }
function show(v) {
  if (typeof v === "string") {
    var a = [];
    for (var i = 0; i < v.length; i++) a.push(v.charCodeAt(i).toString(16));
    return "'" + a.join(" ") + "'";
  }
  return typeof v === "symbol" ? "sym" : String(v);
}
function same(a, b) { return a === b || (a !== a && b !== b); }
var other = newRealm(
  "({ rd: function (s, i) { return s[i]; }," +
  "   str: 'r\\u00e9al\\u20acm'," +
  "   S: String })");
var ropeA = "", i;
for (i = 0; i < 30; i++) ropeA += "abcdefghij";
var strs = ["", "a", "abcéÿ", "a€😀z", "x\ud800y\udfffz\ud800",
  ropeA + "€" + ropeA, "q".repeat(5000) + "!", "length", "constructor", "0", "12",
  other.str, String.fromCharCode(0, 255, 256, 0xffff)];
var nkeys = [2 ** 31 - 1, -(2 ** 31), 2 ** 31, 2 ** 32 - 1, 2 ** 32, 0.5, -0, NaN, Infinity, "1", "01", "length", Symbol.iterator];
var formNames = ["rd", "rdx", "other"];
var forms = { rd: rd, rdx: function (s, i) { return typeof i === "number" ? rdx(s, i - 1) : rd(s, i); }, other: other.rd };
var bad = [];
function check(tag) {
  var n = 0;
  for (var a = 0; a < strs.length; a++) {
    var s = strs[a];
    var keys = [];
    for (var k = -2; k <= Math.min(s.length, 40) + 2; k++) keys.push(k);
    if (s.length > 40) keys.push(s.length - 1, s.length, s.length - 2);
    keys = keys.concat(nkeys);
    for (var fi = 0; fi < formNames.length; fi++) {
      var f = formNames[fi];
      for (var j = 0; j < keys.length; j++) {
        var want = ref(s, keys[j]);
        if (f === "other" && !Object.prototype.hasOwnProperty.call(s, keys[j]))
          want = Reflect.get(other.S.prototype, keys[j], s);
        var got = forms[f](s, keys[j]);
        n++;
        if (!same(got, want) && bad.length < 20)
          bad.push(tag + " " + f + " " + a + " " + String(keys[j]) + " " + show(got) + " " + show(want));
      }
    }
    // the constant and temp forms
    if (!same(rdk(s), ref(s, 2)) || !same(rdk0(s), ref(s, 0)))
      bad.push(tag + " k " + a);
    var h = s.length >> 1;
    if (!same(rdt(s.slice(0, h), s.slice(h), 2), ref(s, 2)) ||
        !same(rdkt(s.slice(0, h), s.slice(h)), ref(s, 1)) ||
        !same(rdt(s.slice(0, h), s.slice(h), s.length), ref(s, s.length)))
      bad.push(tag + " t " + a);
    if (!same(selfKey(s, 1), ref(s, 1)) || !same(selfStr(s, 0), ref(s, 0)))
      bad.push(tag + " self " + a);
    n += 7;
  }
  out.push(tag + " " + n + " reads, " + bad.length + " differ");
}
check("plain");
// indexes, getters and a proxy on the prototypes of this realm
Object.defineProperty(String.prototype, "1", { get: function () { L("S1 " + typeof this); return "p1"; }, configurable: true });
String.prototype[3] = "p3";
Object.defineProperty(Object.prototype, "900", { get: function () { L("O900 " + typeof this + " " + this.length); return "o9"; }, configurable: true });
Object.prototype["-1"] = "m1";
Object.prototype["0.5"] = "half";
check("protos");
out.push("seen " + nlog + " " + log.slice(0, 6).join(","));
out.push("reads " + [rd("ab", 1), rd("ab", 3), rd("abcdef", 3), rd("ab", 900), rd("", 900), rd("ab", -1), rd("ab", 0.5), rdk("ab"), rdk("abc")].map(show).join(","));
delete String.prototype[1];
delete String.prototype[3];
delete Object.prototype[900];
delete Object.prototype["-1"];
delete Object.prototype["0.5"];
log = []; nlog = 0;
var saved = Object.getPrototypeOf(String.prototype);
Object.setPrototypeOf(String.prototype, new Proxy({}, {
  get: function (o, k, r) { L("trap " + String(k) + " " + typeof r); return "t" + String(k); }
}));
check("proxy");
out.push("traps " + nlog + " " + log.slice(0, 4).join(","));
Object.setPrototypeOf(String.prototype, saved);
// the other realm's prototypes answer the reads of its own functions
other.S.prototype[7] = "other7";
Object.defineProperty(other.S.prototype, "2", { get: function () { return "other2"; }, configurable: true });
check("other protos");
out.push("other " + [other.rd("abc", 7), rd("abc", 7), other.rd("abc", 2), rd("abcdefgh", 7), other.rd("ab", 2)].map(show).join(","));
delete other.S.prototype[7];
delete other.S.prototype[2];
out.push(bad.join("\n"));
rvExpect(rd, rdk, rdk0, rdt, rdkt, rdx, selfKey, selfStr, check);
return out.join("\n");
})()
