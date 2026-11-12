// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// Object.prototype.toString of a string reads Symbol.toStringTag along the
// chain of its String object without making it (js_string_to_string_tag in
// engine/quickjs.c), and makes it when the Get could run code or show it: a
// getter must see a wrapper as this, a proxy in the chain must see one as
// the receiver. Each case is run on every kind of value, numbers and
// booleans too, and the chain is put back after each.
(function () {
  "use strict";
  var ts = Object.prototype.toString, out = [];
  var long1 = "";
  for (var i = 0; i < 40; i++) long1 += "abcdefgh";
  var rope = long1 + "x" + long1;
  var values = ["abc", "", rope, 1, 1.5, -0, NaN, 2147483648, true, false,
                null, undefined, Symbol("s"), 10n, new String("x"),
                new Number(1), new Boolean(true), [], function () {}, {},
                Object.create(null), /re/, new Error("e"), new Date(0)];
  var names = ["abc", "empty", "rope", "1", "1.5", "-0", "NaN", "2^31", "true",
               "false", "null", "undefined", "symbol", "bigint", "String",
               "Number", "Boolean", "array", "function", "object", "bare",
               "regexp", "error", "date"];
  function row(label) {
    var r = [];
    for (var k = 0; k < values.length; k++) {
      var s;
      try {
        s = ts.call(values[k]);
      } catch (e) {
        s = "throws " + e.name + " " + e.message;
      }
      r.push(names[k] + "=" + s);
    }
    out.push(label + ": " + r.join(" "));
  }
  function rows(label, setup, undo) {
    setup();
    try {
      row(label);
      row(label + " again");
    } finally {
      undo();
    }
  }
  function tag(obj, v) {
    Object.defineProperty(obj, Symbol.toStringTag, { value: v, configurable: true, writable: true });
  }
  function untag(obj) {
    delete obj[Symbol.toStringTag];
  }
  row("plain");

  // data properties: a string is used, anything else gives the builtin tag
  [[String.prototype, "String.prototype"], [Number.prototype, "Number.prototype"],
   [Boolean.prototype, "Boolean.prototype"], [Object.prototype, "Object.prototype"]]
  .forEach(function (e) {
    rows("data string on " + e[1], function () { tag(e[0], "T"); }, function () { untag(e[0]); });
    rows("data number on " + e[1], function () { tag(e[0], 42); }, function () { untag(e[0]); });
    rows("data undefined on " + e[1], function () { tag(e[0], undefined); }, function () { untag(e[0]); });
  });

  // getters: this is a new wrapper object each time
  var seen;
  function getter(obj) {
    Object.defineProperty(obj, Symbol.toStringTag, {
      configurable: true,
      get: function () {
        var w = this;
        seen.push(typeof w + "/" + (w instanceof Object) + "/" +
                  (w === null || w === undefined ? "-" : typeof w.valueOf()) + "/" + (seen.last === w));
        seen.last = w;
        return typeof w === "object" ? "G" + typeof w.valueOf() : 7;
      }
    });
  }
  [[String.prototype, "String.prototype"], [Number.prototype, "Number.prototype"],
   [Boolean.prototype, "Boolean.prototype"], [Object.prototype, "Object.prototype"]]
  .forEach(function (e) {
    seen = [];
    rows("getter on " + e[1], function () { getter(e[0]); }, function () { untag(e[0]); });
    out.push("getter on " + e[1] + " saw " + seen.join(","));
  });
  rows("throwing getter on String.prototype", function () {
    Object.defineProperty(String.prototype, Symbol.toStringTag, {
      configurable: true, get: function () { throw new TypeError("tag " + typeof this); }
    });
  }, function () { untag(String.prototype); });
  rows("getter with no get on String.prototype", function () {
    Object.defineProperty(String.prototype, Symbol.toStringTag, { configurable: true, set: function () {} });
  }, function () { untag(String.prototype); });

  // a proxy as the prototype of String.prototype: its get trap sees the wrapper
  var trapped = [];
  rows("proxy under String.prototype", function () {
    Object.setPrototypeOf(String.prototype, new Proxy(Object.prototype, {
      get: function (t, k, r) {
        if (k === Symbol.toStringTag)
          trapped.push(typeof r + "/" + (r instanceof String));
        return Reflect.get(t, k, r);
      }
    }));
  }, function () { Object.setPrototypeOf(String.prototype, Object.prototype); });
  out.push("proxy saw " + trapped.join(","));

  // other objects in the chain: a String object (exotic for indexes only),
  // an array (exotic), an object with its own tag
  var sobj = new String("zz");
  rows("String object under Number.prototype", function () {
    Object.setPrototypeOf(Number.prototype, sobj);
  }, function () { Object.setPrototypeOf(Number.prototype, Object.prototype); });
  rows("tagged String object under Number.prototype", function () {
    tag(sobj, "S");
    Object.setPrototypeOf(Number.prototype, sobj);
  }, function () { Object.setPrototypeOf(Number.prototype, Object.prototype); untag(sobj); });
  rows("array under Boolean.prototype", function () {
    Object.setPrototypeOf(Boolean.prototype, []);
  }, function () { Object.setPrototypeOf(Boolean.prototype, Object.prototype); });
  rows("tagged array under Boolean.prototype", function () {
    var a = [];
    tag(a, "A");
    Object.setPrototypeOf(Boolean.prototype, a);
  }, function () { Object.setPrototypeOf(Boolean.prototype, Object.prototype); });
  rows("null prototype of String.prototype", function () {
    Object.setPrototypeOf(String.prototype, null);
  }, function () { Object.setPrototypeOf(String.prototype, Object.prototype); });

  // the toString of another realm reads the prototypes of its own realm
  if (typeof newRealm === "function") {
    var other = newRealm("Object.prototype.toString");
    tag(String.prototype, "Here");
    try {
      out.push("other realm: " + other.call("abc") + " " + other.call(1) + " " +
               other.call(true) + " " + other.call(null) + " here: " + ts.call("abc"));
    } finally {
      untag(String.prototype);
    }
  }
  return out.join("\n");
})();
