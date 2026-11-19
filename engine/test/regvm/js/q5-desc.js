// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

// The descriptor of Object.defineProperty read from the shape of a plain
// object (js_obj_to_desc_plain), beside the generic path the same call takes
// for a getter, an inherited field or a proxy: the results, the messages of
// the errors (a getter or setter that is not callable, both kinds of fields,
// not an object), objects of another realm and a change of its
// Object.prototype, and calls in loops, where the throw injection makes each
// call throw.
var out = [];
function show(o, k) {
  var d = Object.getOwnPropertyDescriptor(o, k);
  if (!d) return "none";
  if ("value" in d) return "v=" + String(d.value) + " " + d.writable + d.enumerable + d.configurable;
  return "a " + typeof d.get + typeof d.set + " " + d.enumerable + d.configurable;
}
function err(e) { return e.constructor.name + ": " + e.message; }
function def(d) {
  try { return show(Object.defineProperty({}, "x", d), "x"); } catch (e) { return err(e); }
}
var fn = function () { return 1; };

// the messages
out.push(def({ get: 1 }), def({ set: "s" }), def({ get: fn, value: 1 }), def({ set: fn, writable: true }),
  def({ get: 1, set: 1 }), def(5), def(null), def(Object.create({ get: 2 })),
  def({ get get() { return 3; } }), def(Object.create({ value: 1, get: fn })));
try { Object.defineProperties({}, { a: { get: {} } }); } catch (e) { out.push(err(e)); }
try { Object.create(null, { a: { set: 0 } }); } catch (e) { out.push(err(e)); }
try { Reflect.defineProperty({}, "a", { value: 1, get: undefined }); } catch (e) { out.push(err(e)); }

// another realm: its plain objects and its Object.prototype
var realm = newRealm("(function () { return { plain: function (i) { return { value: i, enumerable: true }; }, proto: Object.prototype }; })")();
out.push(def(realm.plain(4)));
realm.proto.configurable = true;
out.push(def(realm.plain(5)));
delete realm.proto.configurable;
out.push(def(realm.plain(6)));

// loops of both kinds of each call
function loops() {
  var o = {}, r = [], i, d;
  var getterDesc = { get value() { return "g"; }, enumerable: true };
  var proxyDesc = new Proxy({ value: "p", writable: true }, {});
  for (i = 0; i < 6; i++) {
    d = i % 3 === 0 ? { value: i, writable: true, enumerable: true, configurable: true } :
      i % 3 === 1 ? getterDesc : proxyDesc;
    Object.defineProperty(o, "k" + i, d);
    Object.defineProperties(o, { ["m" + i]: { value: i, enumerable: i % 2 === 0 } });
    Reflect.defineProperty(o, "r" + i, { value: i, configurable: true });
  }
  r.push(Object.keys(o).join(), show(o, "k1"), show(o, "k2"), show(o, "r3"), show(Object.create(null, { c: { value: 1 } }), "c"));
  return r.join(" ");
}
out.push(loops(), loops());
out.join("\n");
