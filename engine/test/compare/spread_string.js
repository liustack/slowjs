// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// The spread of a value that is not an object into an object literal:
// CopyDataProperties makes an object of it (ToObject), so a string gives
// its characters (code units) as the keys "0", "1" ..., and a number, a
// boolean, a symbol or a bigint gives nothing. null and undefined give
// nothing. The output is what Node 24 prints.

var out = [];
function rec(label, f) {
  var r;
  try {
    r = f();
  } catch (e) {
    r = "throws " + e.name;
  }
  out.push(label + ": " + r);
}
// the keys and the code units of the values, so that a lone surrogate
// prints as a number
function show(o) {
  return Reflect.ownKeys(o).map(function (k) {
    var v = o[k], d = Object.getOwnPropertyDescriptor(o, k);
    var s = typeof v === "string"
      ? Array.from({ length: v.length }, function (_, i) { return v.charCodeAt(i); }).join(".")
      : String(v);
    return String(k) + "=" + s + (d.enumerable && d.writable && d.configurable ? "" : "/flags");
  }).join(",");
}

rec("string", function () { return show({ ..."ab" }); });
rec("empty string", function () { return show({ ..."" }); });
rec("surrogate pair", function () { return show({ ..."a\u{1F600}b" }); });
rec("lone surrogate", function () { return show({ ..."\ud800x" }); });
rec("keys around", function () { return show({ x: 1, ..."ab", y: 2 }); });
rec("overwrite", function () { return show({ 0: "z", 2: "w", ..."ab" }); });
rec("two strings", function () { return show({ ..."abc", ..."de" }); });
rec("String object", function () { return show({ ...new String("ab") }); });
rec("String object with a key", function () {
  var s = new String("ab");
  s.extra = 1;
  return show({ ...s });
});
rec("number", function () { return show({ ...5 }); });
rec("boolean", function () { return show({ ...true }); });
rec("symbol", function () { return show({ ...Symbol("s") }); });
rec("bigint", function () { return show({ ...10n }); });
rec("null", function () { return show({ a: 1, ...null }); });
rec("undefined", function () { return show({ a: 1, ...undefined }); });
rec("Object.assign", function () { return show(Object.assign({}, "ab", 5, null, "c")); });
rec("rest of a string", function () {
  const { 0: first, ...r } = "abc";
  return first + " " + show(r);
});
rec("rest of a number", function () {
  const { ...r } = 5;
  return show(r);
});
rec("rest of null", function () {
  const { ...r } = null;
  return show(r);
});
rec("rest of undefined", function () {
  const { ...r } = undefined;
  return show(r);
});
rec("a site of strings and objects", function () {
  var t = [];
  var srcs = ["ab", { a: 1 }, "", "xyz", null, { 0: "o" }, 7];
  for (var i = 0; i < 14; i++)
    t.push("{" + show({ k: i, ...srcs[i % srcs.length] }) + "}");
  return t.join(" ");
});
rec("String.prototype getter", function () {
  // an index of the string is its own key: a getter on the prototype
  // does not run
  var hit = 0;
  Object.defineProperty(String.prototype, "0", {
    get: function () { hit++; return "g"; }, configurable: true,
  });
  try {
    return show({ ..."ab" }) + " " + hit;
  } finally {
    delete String.prototype[0];
  }
});
rec("long", function () {
  var r = { ..."x".repeat(1000) };
  return Object.keys(r).length + " " + r[999];
});

out.join("\n");
