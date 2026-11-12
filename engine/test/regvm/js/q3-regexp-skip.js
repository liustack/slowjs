// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// Where a regexp starts to look for a match (lre_skip in engine/libregexp.c):
// a first character, a first class, a set of first units over alternatives
// and assertions, ^ outside multiline mode, and the regexps that run the loop
// over positions (unicode, sticky, a first op of no known units). Each runs
// on strings of 8 and 16 bits, with matches at the start, the end, units 255
// and 256, both sides of a chunk of 65,536 units, through test, exec, search,
// match, matchAll, replace and split, with lastIndex, named groups, regexps
// shared by the compile cache and a regexp of another realm.
(function () {
  "use strict";
  var log = [];
  var pats = [
    /ab/, /a/, /\xe9/, /\u4e2d/, /\u0100x/, /A/i, /a/i, /\xe9/i, /k/i, /K/iu, /_/,
    /["&<>]/, /[\x00-\x07\x0b\x0e-\x1f<\u2028\u2029\ud800-\udfff]/, /[\u0100-\u0200]/,
    /[^a-z]/, /[\xe9-\xff]/, /[a-z]/i, /[\0-\xff]/, /[\xff]/, /[\t\n\r]/, /[\xff\u0100]/,
    /(?:a|c)b/, /(^|[^\u0000-\u0020])[\u0000-\u0020]+$/, /[+\-]?\d+/, /(?=b)./, /x?y/,
    /(\xe9|\u4e2d)/, /(?:a|\u4e2d)z/, /\bfoo/, /(red|green|blue)/, /%(24|2C|2F|3A)/,
    /(transparent|aliceblue|antiquewhite|aqua|aquamarine|azure|beige)(?!\w)/,
    /label:\s*([^\s;{]+)\s*(;|$)/, /./, /\s/, /a*b/, /(?:)/, /$/, /(a)|b/, /ab/u, /ab/y,
    /\w+/, /^ab/, /^a/m, /^/, /^[\u0000-\u0020]+/, /(?<x>a)(?<y>[bc])/, /(?<n>\xe9)+/,
    /[\ud800-\udbff][\udc00-\udfff]/, /\u{1f600}/u, /[^]/, /(?!a)b/, /(?<=a)b/,
  ];
  var subj = [
    "", "ab", "xxab", "cb zz", "a\"b<c>&", "\xe9t\xe9", "\xff\u0100", "AbAB", "\u212a k",
    "a red, blue", "%24%2c%3A", "label: x ; label:y", "aliceblue azurex", "\u4e2d\u6587z az",
    "x\x01y\u2028", " 12 -3 +4 ", "\ud83d\ude00!", "foo food", "  \t\n", "x y z ",
  ];
  function show(m) {
    if (m === null) return "null";
    var r = m.index + ":" + JSON.stringify(Array.prototype.slice.call(m));
    if (m.groups) r += JSON.stringify(m.groups);
    return r;
  }
  function flags(re, add) { return re.flags.replace(add, "") + add; }
  function one(re, s) {
    var g = new RegExp(re.source, flags(re, "g")), out = [];
    out.push(re.test(s) ? 1 : 0, s.search(re), show(re.exec(s)));
    out.push(JSON.stringify(s.match(g)), s.replace(g, "[$&]"), JSON.stringify(s.split(re)));
    var pos = [];
    for (var m of s.matchAll(g)) pos.push(m.index);
    out.push(pos.join(","));
    re.lastIndex = 0;
    return out.join(" ");
  }
  function sticky(re, s) {
    var y = new RegExp(re.source, flags(re, "y")), out = [];
    for (var i = 0; i <= s.length && i < 8; i++) {
      y.lastIndex = i;
      var m = y.exec(s);
      out.push(m ? m.index + "/" + y.lastIndex : "-" + y.lastIndex);
    }
    return out.join(",");
  }
  function global(re, s) {
    var g = new RegExp(re.source, flags(re, "g")), out = [];
    for (var i = 0; i < 6; i++) {
      var m = g.exec(s);
      if (!m) { out.push("null@" + g.lastIndex); break; }
      out.push(m.index + "@" + g.lastIndex);
      if (m[0] === "") g.lastIndex++;
    }
    return out.join(",");
  }
  // a match only after a chunk of 65,536 units, or just before or after it
  function far(re, unit, at, tail) {
    var s = unit.repeat(at) + tail + unit.repeat(3);
    var g = new RegExp(re.source, flags(re, "g"));
    g.lastIndex = at > 2 ? at - 2 : 0;
    var m = g.exec(s);
    return [re.test(s) ? 1 : 0, s.search(re), m ? m.index + "/" + g.lastIndex : "null/" + g.lastIndex].join(":");
  }
  function cached(src, s) {
    var out = [];
    for (var i = 0; i < 3; i++) {
      var re = new RegExp(src, i === 2 ? "g" : "");
      out.push(re.test(s) ? 1 : 0, s.replace(re, "<$&>"));
    }
    return out.join(" ");
  }
  function realm(s) {
    var other = newRealm("[/[\"&<>]/, /(?:a|c)b/g, /(?<k>[bc])/]");
    return [other[0].test(s), s.search(other[1]), s.replace(other[1], "+"), show(other[2].exec(s)),
      Object.getPrototypeOf(other[0]) === RegExp.prototype].join(" ");
  }
  function named(s) {
    var re = /(?<x>a)(?<y>[bc])/g;
    return [s.replace(re, "$<y>$<x>"), s.replace(re, function (m, x, y, i, str, gr) { return gr.y + i; }),
      JSON.stringify(Array.from(s.matchAll(re), function (m) { return m.groups; }))].join(" ");
  }
  for (var p = 0; p < pats.length; p++) {
    var row = [String(pats[p])];
    for (var k = 0; k < subj.length; k++) row.push(one(pats[p], subj[k]));
    log.push(row.join(" | "));
    log.push(" y " + sticky(pats[p], "xab\"\xe9\u4e2db") + " g " + global(pats[p], "ab a\"b \u4e2dab"));
  }
  var farPats = [/b/, /\u4e2d/, /["&<>]/, /[\u0100-\u0200]/, /(?:b|c)d/, /^b/, /bd/i, /\xe9/];
  var tails = ["b", "\"", "\u0150", "cd", "\xe9", "Bd"];
  var ats = [65533, 65535, 65536, 65537, 131072];
  for (var f = 0; f < farPats.length; f++) {
    var r2 = [];
    for (var u = 0; u < 2; u++)
      for (var t = 0; t < tails.length; t++)
        for (var a = 0; a < ats.length; a += 2) r2.push(far(farPats[f], u ? "\u4e00" : "a", ats[a], tails[t]));
    log.push(String(farPats[f]) + " " + r2.join(" "));
  }
  log.push(cached("[\"&<>]", "a<b"), cached("(?:a|c)b", "xcb"), cached("\\xe9", "\u4e2d\xe9"));
  log.push(realm("acb\"x"), realm("\u4e2dcb"), named("abacx"), named("\u4e2dac"));
  rvExpect(show, flags, one, sticky, global, far, cached, realm, named);
  return log.join("\n");
})()
