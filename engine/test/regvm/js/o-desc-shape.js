// Copyright (c) 2026 The liustack/tifajs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.
// rvdiff: the objects Object.getOwnPropertyDescriptor, Reflect's and
// Object.getOwnPropertyDescriptors make, as FromPropertyDescriptor says: an
// ordinary extensible object of Object.prototype with value, writable,
// enumerable and configurable, or get, set, enumerable and configurable, in
// that order, each an enumerable, writable and configurable data property.
// The keys are defined, so a setter or a read-only key of the same name on
// Object.prototype changes nothing. Descriptors are changed after they are
// made (a key added, deleted, written, the object frozen) and later ones must
// not see it; each case runs three times, so that later runs make their
// descriptors after earlier ones were changed.
(function () {
var out = [], bad = 0;
function show(v) {
  if (typeof v === "function") return "fn " + (v.name || "anon");
  if (v !== null && typeof v === "object") return "obj";
  return typeof v + " " + String(v);
}
function shape(d) {
  if (d === undefined) return "undefined";
  var keys = Reflect.ownKeys(d), s = [];
  for (var i = 0; i < keys.length; i++) {
    var k = keys[i], p = Object.getOwnPropertyDescriptor(d, k);
    if (!("value" in p) || !p.writable || !p.enumerable || !p.configurable)
      s.push(String(k) + " not plain");
    else
      s.push(String(k) + "=" + show(p.value));
  }
  return "proto " + (Object.getPrototypeOf(d) === Object.prototype) +
    ", ext " + Object.isExtensible(d) + ": " + s.join(" ");
}
function check(name, got, want) {
  if (got !== want) {
    bad++;
    out.push(name + ": " + got + " MISMATCH want " + want);
  } else {
    out.push(name + ": " + got);
  }
}
function getter() { return 1; }
function setter(v) {}
var o = { a: 1 };
Object.defineProperty(o, "ro", { value: "r", writable: false, enumerable: false, configurable: false });
Object.defineProperty(o, "acc", { get: getter, set: setter, enumerable: true, configurable: true });
Object.defineProperty(o, "g", { get: getter, enumerable: false, configurable: false });
Object.defineProperty(o, "s", { set: setter });
var sym = Symbol("k");
o[sym] = "sv";
var data = "proto true, ext true: value=number 1 writable=boolean true enumerable=boolean true configurable=boolean true";
for (var round = 0; round < 3; round++) {
  var r = " " + round;
  check("data" + r, shape(Object.getOwnPropertyDescriptor(o, "a")), data);
  check("read-only" + r, shape(Object.getOwnPropertyDescriptor(o, "ro")),
    "proto true, ext true: value=string r writable=boolean false enumerable=boolean false configurable=boolean false");
  check("accessor" + r, shape(Object.getOwnPropertyDescriptor(o, "acc")),
    "proto true, ext true: get=fn getter set=fn setter enumerable=boolean true configurable=boolean true");
  check("getter only" + r, shape(Object.getOwnPropertyDescriptor(o, "g")),
    "proto true, ext true: get=fn getter set=undefined undefined enumerable=boolean false configurable=boolean false");
  check("setter only" + r, shape(Object.getOwnPropertyDescriptor(o, "s")),
    "proto true, ext true: get=undefined undefined set=fn setter enumerable=boolean false configurable=boolean false");
  check("symbol" + r, shape(Object.getOwnPropertyDescriptor(o, sym)),
    "proto true, ext true: value=string sv writable=boolean true enumerable=boolean true configurable=boolean true");
  check("missing" + r, shape(Object.getOwnPropertyDescriptor(o, "none")), "undefined");
  check("reflect" + r, shape(Reflect.getOwnPropertyDescriptor(o, "a")), data);
  check("string index" + r, shape(Object.getOwnPropertyDescriptor("xy", 1)),
    "proto true, ext true: value=string y writable=boolean false enumerable=boolean true configurable=boolean false");
  check("array length" + r, shape(Object.getOwnPropertyDescriptor([1, 2], "length")),
    "proto true, ext true: value=number 2 writable=boolean true enumerable=boolean false configurable=boolean false");
  var all = Object.getOwnPropertyDescriptors(o);
  check("all keys" + r, Reflect.ownKeys(all).map(String).join(","), "a,ro,acc,g,s,Symbol(k)");
  check("all a" + r, shape(all.a), data);
  check("all acc" + r, shape(all.acc),
    "proto true, ext true: get=fn getter set=fn setter enumerable=boolean true configurable=boolean true");
  // a descriptor changed after it was made: the next one is as before
  var d1 = Object.getOwnPropertyDescriptor(o, "a");
  d1.extra = 1;
  delete d1.writable;
  d1.value = 7;
  var d2 = Object.getOwnPropertyDescriptor(o, "a");
  check("after a change" + r, shape(d2), data);
  check("the changed one" + r, shape(d1),
    "proto true, ext true: value=number 7 enumerable=boolean true configurable=boolean true extra=number 1");
  var d3 = Object.getOwnPropertyDescriptor(o, "acc");
  Object.freeze(d3);
  var d4 = Object.getOwnPropertyDescriptor(o, "acc");
  check("after a freeze" + r, shape(d4),
    "proto true, ext true: get=fn getter set=fn setter enumerable=boolean true configurable=boolean true");
  d4.get = null;
  check("written" + r, show(d4.get) + " " + show(Object.getOwnPropertyDescriptor(o, "acc").get),
    "object null fn getter");
  // a descriptor round-trips through defineProperty
  var t = {};
  Object.defineProperty(t, "a", Object.getOwnPropertyDescriptor(o, "ro"));
  check("round trip" + r, shape(Object.getOwnPropertyDescriptor(t, "a")),
    "proto true, ext true: value=string r writable=boolean false enumerable=boolean false configurable=boolean false");
}
// setters and read-only keys of the same names on Object.prototype: the keys
// are defined, not set
var seen = [];
var names = ["value", "writable", "enumerable", "configurable", "get", "set"];
for (var i = 0; i < names.length; i++) {
  (function (n) {
    // a descriptor of no prototype: the accessors put on Object.prototype
    // before would be read as its fields
    var dsc = Object.create(null);
    dsc.set = function () { seen.push(n); };
    dsc.configurable = true;
    Object.defineProperty(Object.prototype, n, dsc);
  })(names[i]);
}
for (var round = 0; round < 3; round++) {
  var r = " " + round;
  check("proto setters data" + r, shape(Object.getOwnPropertyDescriptor(o, "a")), data);
  check("proto setters accessor" + r, shape(Object.getOwnPropertyDescriptor(o, "acc")),
    "proto true, ext true: get=fn getter set=fn setter enumerable=boolean true configurable=boolean true");
}
check("setters called", seen.join(","), "");
for (var i = 0; i < names.length; i++)
  delete Object.prototype[names[i]];
var ro = Object.create(null);
ro.value = 0;
ro.configurable = true;
Object.defineProperty(Object.prototype, "value", ro);
check("proto read-only", shape(Object.getOwnPropertyDescriptor(o, "a")), data);
delete Object.prototype.value;
// many descriptors kept alive at once, then a collection
var keep = [];
for (var i = 0; i < 2000; i++)
  keep.push(Object.getOwnPropertyDescriptor(i & 1 ? o : { a: i }, i & 1 ? "acc" : "a"));
var ok = 0;
for (var i = 0; i < keep.length; i++)
  if (i & 1 ? keep[i].get === getter : keep[i].value === i) ok++;
check("kept", ok, 2000);
// another realm, when the host has one: its Object.prototype
if (typeof newRealm === "function") {
  var other = newRealm("({ d: function (o, k) { return Object.getOwnPropertyDescriptor(o, k); }, proto: Object.prototype })");
  var d = other.d({ a: 1 }, "a");
  check("realm", Object.getPrototypeOf(d) === other.proto, true);
  check("realm here", Object.getPrototypeOf(Object.getOwnPropertyDescriptor({ a: 1 }, "a")) === Object.prototype, true);
}
out.push(bad ? bad + " MISMATCH" : "all match");
return out.join("\n");
})()
