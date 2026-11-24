// Copyright (c) 2026 The liustack/beastjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.
// rvdiff: o[k] = v and o.x = v (PUTEL and PUTF, sloppy and strict) across two
// realms: the code that stores, the receiver and the prototype it inherits
// from each come from this realm or from another one (newRealm), in every
// combination. The prototypes have a writable data key, a read-only key, a
// setter and a getter alone, or are an Array, an Int8Array or Object.prototype
// of their realm. The receivers are plain objects made in their realm and
// objects that are not extensible. Each store is made twice, on two receivers
// made alike: once with the key as it is, an atom, and once with the same key
// made again as a string that is not an atom, which cannot take the path of a
// key that is its own atom. The two must give the same result, an exception
// of the same name from the same realm, run the same setters with the same
// receiver, and leave the same own keys with the same attributes. A line
// says MISMATCH when they do not. Also: the global object of each realm as
// the receiver, stored to from the code of each realm.
var SRC = '(function () {' +
  ' function wf(o, k, v) { switch (k) {' +
  '  case "zz": o.zz = v; break; case "a": o.a = v; break; case "dd": o.dd = v; break;' +
  '  case "ro": o.ro = v; break; case "st": o.st = v; break; case "gt": o.gt = v; break;' +
  '  case "__proto__": o.__proto__ = v; break; default: o[k] = v; } }' +
  ' function wfs(o, k, v) { "use strict"; switch (k) {' +
  '  case "zz": o.zz = v; break; case "a": o.a = v; break; case "dd": o.dd = v; break;' +
  '  case "ro": o.ro = v; break; case "st": o.st = v; break; case "gt": o.gt = v; break;' +
  '  case "__proto__": o.__proto__ = v; break; default: o[k] = v; } }' +
  ' function proto(env, kind) {' +
  '  if (kind === "array") return [];' +
  '  if (kind === "ta") return new Int8Array(2);' +
  '  if (kind === "op") return Object.prototype;' +
  '  var p = {};' +
  '  Object.defineProperty(p, "dd", { value: 1, writable: true, enumerable: true, configurable: true });' +
  '  Object.defineProperty(p, "ro", { value: 2, writable: false, enumerable: true, configurable: true });' +
  '  Object.defineProperty(p, "st", { set: function (v) { env.log.push("set " + (this === env.cur)); }, configurable: true });' +
  '  Object.defineProperty(p, "gt", { get: function () { return 3; }, configurable: true });' +
  '  return p; }' +
  ' function make(p, kind) {' +
  '  var o = Object.create(p); o.a = 1;' +
  '  if (kind === "fixed") Object.preventExtensions(o);' +
  '  return o; }' +
  ' return { wr: function (o, k, v) { o[k] = v; },' +
  '  wrs: function (o, k, v) { "use strict"; o[k] = v; },' +
  '  wf: wf, wfs: wfs, proto: proto, make: make, global: globalThis,' +
  '  TypeError: TypeError, Object: Object };' +
  '})()';
(function () {
var out = [];
var env = { log: [], cur: null };
var R = { main: (0, eval)(SRC), other: newRealm(SRC) };
var names = ["main", "other"];
var theValue = { theValue: 1 };
function fresh(k) { return (" " + k).slice(1); }
function realmOf(e) {
  if (e instanceof R.main.TypeError) return "@main";
  if (e instanceof R.other.TypeError) return "@other";
  return "@?";
}
function guard(f, o, k, v) {
  try { f(o, k, v); return "ok"; } catch (e) { return e.name + realmOf(e); }
}
function show(v) {
  if (v === theValue) return "V";
  if (v !== null && typeof v === "object") return "obj";
  return typeof v + ":" + String(v);
}
function state(o, p) {
  var keys = Reflect.ownKeys(o), s = [];
  for (var i = 0; i < keys.length; i++) {
    var d = Reflect.getOwnPropertyDescriptor(o, keys[i]);
    s.push(String(keys[i]) + "=" + ("value" in d ? show(d.value) : "acc") +
      (d.writable ? "w" : "") + (d.enumerable ? "e" : "") + (d.configurable ? "c" : ""));
  }
  return s.join(" ") + " proto:" + (Object.getPrototypeOf(o) === p ? "same" : "changed") +
    (Object.isExtensible(o) ? "" : " fixed");
}
var keys = ["zz", "a", "dd", "ro", "st", "gt", "__proto__", "-0", "1.5"];
var writers = ["wr", "wrs", "wf", "wfs"];
var rows = 0, bad = 0;
function one(code, w, recv, pr, pkind, rkind, k) {
  var p = R[pr].proto(env, pkind), res = [];
  for (var t = 0; t < 2; t++) {
    var o = R[recv].make(p, rkind);
    env.log = []; env.cur = o;
    var r = guard(R[code][w], o, t ? fresh(k) : k, theValue);
    res.push(r + " " + state(o, p) + (env.log.length ? " {" + env.log.join(";") + "}" : ""));
  }
  rows++;
  var line = code + " " + w + " recv:" + recv + " proto:" + pr + "/" + pkind + " " + rkind + " [" + k + "] " + res[0];
  if (res[0] !== res[1]) { bad++; line += " MISMATCH " + res[1]; }
  out.push(line);
}
for (var a = 0; a < 2; a++)
  for (var b = 0; b < 2; b++)
    for (var c = 0; c < 2; c++)
      ["chain", "array", "ta", "op"].forEach(function (pkind) {
        ["plain", "fixed"].forEach(function (rkind) {
          for (var w = 0; w < writers.length; w++)
            for (var i = 0; i < keys.length; i++)
              one(names[a], writers[w], names[b], names[c], pkind, rkind, keys[i]);
        });
      });
// the global object of each realm as the receiver, from the code of each
for (var a = 0; a < 2; a++)
  for (var b = 0; b < 2; b++)
    for (var w = 0; w < writers.length; w++) {
      var g = R[names[b]].global, res = [];
      for (var t = 0; t < 2; t++) {
        var k = "zzRealmGlobal";
        var r = guard(R[names[a]][writers[w]], g, t ? fresh(k) : k, theValue);
        var d = Reflect.getOwnPropertyDescriptor(g, k);
        res.push(r + " " + (d ? show(d.value) + (d.writable ? "w" : "") + (d.enumerable ? "e" : "") + (d.configurable ? "c" : "") : "none"));
        delete g[k];
      }
      rows++;
      var line = "global " + names[a] + " " + writers[w] + " of:" + names[b] + " " + res[0];
      if (res[0] !== res[1]) { bad++; line += " MISMATCH " + res[1]; }
      out.push(line);
    }
out.push(rows + " rows, " + bad + " differ");
rvExpect(R.main.wr, R.main.wrs, R.main.wf, R.main.wfs);
return out.join("\n");
})()
