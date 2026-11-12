// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// Regexps of many alternatives that start with a literal, as react-spring's
// color names (lre_split in engine/libregexp.c): alternatives that share a
// prefix with the shorter one first or last, the same alternative twice, an
// empty alternative, an alternative that starts with a class, a dot or an
// escape in the middle, nested groups and named groups, a back reference
// after the alternation, a lookbehind before it and an alternation inside a
// lookbehind, a quantified alternation, ignore case, unicode and v, first
// characters from 256 up and above the BMP, 15, 16 and 17 alternatives and
// many more, with sticky and global and lastIndex, and every builtin that
// runs a regexp.
(function () {
  "use strict";
  var log = [];
  var letters = "abcdefghijklmnopqrstuvwxyz";
  // n words of 2 to 4 letters, several of them with the same first letter
  function words(n, salt) {
    var out = [];
    for (var i = 0; i < n; i++) {
      var w = letters[(i * 7 + salt) % 9] + letters[(i * 5 + salt) % 26];
      if (i % 3 === 1) w += letters[(i * 11) % 26];
      if (i % 4 === 2) w += letters[(i * 3) % 26];
      out.push(w);
    }
    return out;
  }
  var colors = ["aliceblue", "antiquewhite", "aqua", "aquamarine", "azure", "beige", "bisque",
    "black", "blanchedalmond", "blue", "blueviolet", "brown", "burlywood", "cadetblue",
    "chartreuse", "chocolate", "coral", "cornflowerblue", "cornsilk", "crimson", "cyan",
    "darkblue", "darkcyan", "darkgoldenrod", "darkgray", "darkgreen", "darkgrey", "red",
    "tan", "teal", "thistle", "tomato", "turquoise", "violet", "wheat", "white", "yellow"];
  var pats = [
    new RegExp("(" + colors.join("|") + ")(?!\\w)", "g"),
    new RegExp("(" + words(15, 1).join("|") + ")"),
    new RegExp("(" + words(16, 1).join("|") + ")"),
    new RegExp("(" + words(17, 1).join("|") + ")"),
    new RegExp("(?:" + words(40, 3).join("|") + ")+"),
    // shorter first, then longer first
    new RegExp("(" + ["a", "ab", "abc", "abcd"].concat(words(16, 2)).join("|") + ")"),
    new RegExp("(" + ["abcd", "abc", "ab", "a"].concat(words(16, 2)).join("|") + ")"),
    // the same alternative twice, with other captures
    new RegExp(["(ab)", "(ab)", "(ac)"].concat(words(16, 4)).join("|")),
    // an empty alternative at the start, in the middle and at the end
    new RegExp("(|" + words(18, 5).join("|") + ")x"),
    new RegExp("(" + words(9, 5).join("|") + "||" + words(9, 6).join("|") + ")x"),
    new RegExp("(" + words(18, 5).join("|") + "|)x"),
    // alternatives that do not start with a literal, in the middle
    new RegExp("(" + words(9, 7).join("|") + "|[ab]z|.q|\\dd|\\bk|" + words(9, 8).join("|") + ")"),
    // nested groups, named groups, a back reference after
    new RegExp("((a)(b)|(a)c|" + words(16, 9).join("|") + "|((d)e))f?"),
    new RegExp("(?<p>ab)|(?<q>ac)|" + words(16, 10).map(function (w, i) { return "(?<n" + i + ">" + w + ")"; }).join("|")),
    new RegExp("(" + words(20, 11).join("|") + ")-\\1"),
    // a lookbehind before, an alternation inside a lookbehind
    new RegExp("(?<=[x=])(" + words(18, 12).join("|") + ")"),
    new RegExp("(?<=(" + words(18, 12).join("|") + "))=\\d"),
    // ignore case, unicode, v, first characters from 256 up and above the BMP
    new RegExp("(" + colors.join("|") + ")", "i"),
    new RegExp("(" + words(18, 13).join("|") + "|\u0100x|\u0101y|\u4e2d\u6587|\u212aelvin)", "i"),
    new RegExp("(" + words(18, 13).join("|") + "|\u{1f600}a|\u{1f601}b|\u4e2d)", "u"),
    new RegExp("(" + words(18, 13).join("|") + "|\u{1f600}a|\u{1f601}b|\u4e2d)", "v"),
    new RegExp("(" + words(18, 13).join("|") + "|\u212aelvin|\u017ft)", "iu"),
    new RegExp("(" + words(18, 14).join("|") + "|\u4e2d\u6587|\u6587\u4e2d|\u0100)"),
  ];
  var subj = [
    "", "a", "ab", "abc", "abcd", "abcdx", "x", "=ab", "xab=1", "ab-ab", "ac-ac ab-ab",
    "aliceblue red tan teal", "darkgreenish dark tan", "TEAL Red BLUE", "abf acf def de",
    "\u4e2d\u6587 \u6587\u4e2d \u0100", "\u0100X \u0101Y \u212aELVIN kelvin \u017fT st",
    "\u{1f600}a \u{1f601}b \u{1f600}", "zz yy xx ww", words(40, 3).join(""),
  ].concat([1, 2, 4, 5, 7, 9, 10, 11, 12, 13, 14].map(function (s) { return words(20, s).join(s & 1 ? " " : "="); }));
  function show(m) {
    if (m === null) return "null";
    var g = m.groups ? Object.keys(m.groups).filter(function (k) { return m.groups[k] !== undefined; }).join("+") : "";
    return m.index + ":" + JSON.stringify(Array.prototype.slice.call(m)) + (g ? "{" + g + "}" : "");
  }
  function one(re, s) {
    var flags = re.flags.replace("g", "");
    var r = new RegExp(re.source, flags), g = new RegExp(re.source, flags + "g"), out = [];
    out.push(r.test(s) ? 1 : 0, s.search(r), show(r.exec(s)), JSON.stringify(s.match(g)),
      s.replace(g, "[$&]"), s.split(r).length);
    var pos = [];
    for (var m of s.matchAll(g)) pos.push(m.index + "/" + m[0].length);
    out.push(pos.join(","));
    return out.join(" ");
  }
  // sticky from each lastIndex, and global from a lastIndex in the middle
  function sticky(re, s) {
    var y = new RegExp(re.source, re.flags.replace("g", "") + "y"), out = [];
    for (var i = 0; i <= s.length; i++) {
      y.lastIndex = i;
      var m = y.exec(s);
      if (m !== null) out.push(i + ">" + y.lastIndex + ":" + m[0]);
    }
    var g = new RegExp(re.source, re.flags.replace("g", "") + "g");
    g.lastIndex = s.length >> 1;
    var m2 = g.exec(s);
    out.push("g" + (m2 === null ? "null" : m2.index) + ">" + g.lastIndex);
    return out.join(",");
  }
  for (var p = 0; p < pats.length; p++) {
    var row = [String(pats[p]).slice(0, 30)];
    for (var k = 0; k < subj.length; k++) row.push(one(pats[p], subj[k]));
    log.push(row.join(" | "));
    log.push(" y " + sticky(pats[p], "ab abc ac-ac teal =de x=ab \u4e2d\u6587"));
  }
  // a long subject where the first letters match and no alternative does
  var names = new RegExp("(" + colors.join("|") + ")(?!\\w)", "g");
  var long = "ab".repeat(40000) + "red";
  log.push(long.replace(names, "<$1>").slice(-12), names.lastIndex);
  rvExpect(words, show, one, sticky);
  return log.join("\n");
})()
