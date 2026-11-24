// Copyright (c) 2026 The liustack/beastjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

// bind of a target whose length and name are plain data properties
// (js_bind_read_plain), beside the generic path for a getter, a deleted
// name and a proxy: the names and lengths, a target of another realm, and
// calls in loops, where the throw injection makes each call throw.
var out = [];
var realm = newRealm("(function () { return function realmFn(a, b) { }; })")();
var rb = realm.bind(null, 1);
out.push(rb.name + "/" + rb.length + "/" + Reflect.ownKeys(rb).join("+") + " " + (Object.getPrototypeOf(rb) === Object.getPrototypeOf(realm)));

function loops() {
  var r = [], i, b;
  var named = function named(a, b, c) { return a + b + c; };
  var withGetter = function (a) { };
  Object.defineProperty(withGetter, "name", { get: function () { return "getter"; } });
  var noName = function (a, b) { };
  delete noName.name;
  var proxied = new Proxy(named, {});
  var kinds = [named, withGetter, noName, proxied, Math.max, function* gen(a) { }];
  for (i = 0; i < 12; i++) {
    b = kinds[i % kinds.length].bind(null, i);
    r.push(b.name + "/" + b.length);
  }
  b = named.bind(null, 1).bind(null, 2);
  r.push(b(3), new ((function F(a) { this.a = a; }).bind(null, 7))().a);
  return r.join(" ");
}
out.push(loops(), loops());
out.join("\n");
