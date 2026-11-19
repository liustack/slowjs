// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

// Object.isFrozen, Object.isSealed, Object.freeze and Object.seal answered
// and done in a pass over the shape (js_shape_test_integrity,
// js_shape_set_integrity), beside the generic path the same calls take for a
// proxy, a typed array, a String object, arguments and the global object:
// objects that share a shape, an object alone with its shape, arrays fast and
// not, private names, accessors, objects of another realm, and calls in
// loops, where the throw injection makes each call throw.
var out = [];
function lv(o) {
  return (Object.isExtensible(o) ? "1" : "0") + (Object.isSealed(o) ? 1 : 0) + (Object.isFrozen(o) ? 1 : 0);
}
function flags(o) {
  return Reflect.ownKeys(o).map(function (k) {
    var d = Object.getOwnPropertyDescriptor(o, k);
    return String(k) + ":" + ("value" in d ? (d.writable ? "w" : "-") : "a") + (d.enumerable ? "e" : "-") + (d.configurable ? "c" : "-");
  }).join(" ");
}
class P {
  #x = 1;
  y = 2;
  bump() { return ++this.#x; }
}

function loops() {
  var r = [], i, a, b, c, arr, p;
  for (i = 0; i < 6; i++) {
    a = { x: i, y: 2, get g() { return 1; } };
    b = { x: i, y: 2, get g() { return 1; } };
    c = {};
    c["k" + (i % 2)] = i;
    c.z = 1;
    arr = i % 2 ? [i, 2, 3] : [];
    if (i % 3 === 0) Object.freeze(a); else Object.seal(a);
    Object.preventExtensions(b);
    Object.freeze(c);
    Object.freeze(arr);
    p = Object.freeze(new P());
    r.push(lv(a) + lv(b) + lv(c) + lv(arr) + lv(p) + p.bump());
  }
  r.push(flags(a), flags(c), flags(arr), b.x = 9, a.x);
  return r.join(" ");
}
out.push(loops(), loops());

// the generic path
function generic() {
  var log = [], r = [];
  var px = new Proxy({ a: 1 }, {
    isExtensible: function (t) { log.push("isExtensible"); return Reflect.isExtensible(t); },
    ownKeys: function (t) { log.push("ownKeys"); return Reflect.ownKeys(t); },
  });
  r.push(Object.isFrozen(px), Object.isFrozen(Object.freeze(px)), log.join());
  r.push(lv(Object.freeze(new Uint8Array(0))), lv(Object.freeze(new String("ab"))));
  r.push(lv(Object.freeze((function () { return arguments; })(1, 2))));
  return r.join(" ");
}
out.push(generic());

// another realm
var realm = newRealm("(function () { return { make: function (i) { return { v: i, w: [i] }; }, f: Object.freeze, isF: Object.isFrozen }; })")();
var ro = realm.make(3);
out.push(lv(ro), realm.isF(realm.f(ro)), Object.isFrozen(ro), lv(Object.freeze(realm.make(4))), Object.isFrozen(ro.w));

// many keys
function many() {
  var o = {}, i;
  for (i = 0; i < 300; i++) o["p" + i] = i;
  delete o.p7;
  Object.seal(o);
  var s = lv(o);
  Object.freeze(o);
  return s + " " + lv(o) + " " + Object.keys(o).length + " " + flags(o).length;
}
out.push(many(), many());
var gv = 1;
Object.freeze(globalThis);
out.push(lv(globalThis), gv, Object.getOwnPropertyDescriptor(globalThis, "gv").writable);
out.join("\n");
