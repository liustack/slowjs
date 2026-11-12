// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// Regexps that start with a literal of several characters (lre_prefix in
// engine/libregexp.c): emotion's label pattern on CSS text with many l's, a
// literal that overlaps itself, one cut by the end of the string, one longer
// than the string, literals of 255, 256 and 257 characters, characters above
// 255 against strings of 8 and 16 bits, a literal followed by a quantifier, a
// group or a capture, ignore case, unicode and sticky, with lastIndex and
// every builtin that runs a regexp.
(function () {
  "use strict";
  var log = [];
  var long = "q".repeat(254);
  var pats = [
    /label:\s*([^\s;{]+)\s*(;|$)/g, /abc/, /aab/, /aa/, /ab{2}c/, /ab*c/, /(?:ab)+c/, /a(b)c/,
    /ab|ac/, /\u4e2d\u6587/, /x\u0100y/, /\xe9\xe8/, /LABEL:/i, /ab/y, /ab/u, /a\u{1f600}/u,
    /%2C/g, /<\/script/, /--/, /a\nb/, /a.b/, /ab$/, /ab\b/, /ab(?=c)/, /ab(?!c)/,
    new RegExp(long + "z"), new RegExp(long + "zz"), new RegExp(long + "zzz"),
  ];
  var subj = [
    "", "a", "ab", "abc", "aab", "aaab", "abbc", "ababc", "xabcx", "acab",
    "display:flex;color:blue;label:Button;", "label:x", "label: y ;label:z", "LABEL:A label:b",
    "\u4e2d\u6587\u4e2d", "x\u0100y x\u0101y", "\xe9\xe8\xe9", "a%2C%2c%2Cb", "</scrip</script>",
    "a\nb a\rb", "ab abc", "a\u{1f600}", long + "zz", long + "zzz", long + "z",
  ];
  function show(m) {
    if (m === null) return "null";
    return m.index + ":" + JSON.stringify(Array.prototype.slice.call(m));
  }
  function one(re, s) {
    var flags = re.flags.replace("g", "");
    var r = new RegExp(re.source, flags), g = new RegExp(re.source, flags + "g"), out = [];
    out.push(r.test(s) ? 1 : 0, s.search(r), show(r.exec(s)), JSON.stringify(s.match(g)),
      s.replace(g, "[$&]").length, s.split(r).length);
    var pos = [];
    for (var m of s.matchAll(g)) pos.push(m.index);
    out.push(pos.join(","));
    return out.join(" ");
  }
  function lastIndex(re, s) {
    var flags = re.flags.replace("g", "").replace("y", "");
    var g = new RegExp(re.source, flags + "g"), y = new RegExp(re.source, flags + "y"), out = [];
    for (var i = 0; i <= s.length && i < 10; i++) {
      g.lastIndex = i;
      y.lastIndex = i;
      var a = g.exec(s), b = y.exec(s);
      out.push((a ? a.index : "-") + "/" + g.lastIndex + "/" + (b ? b.index : "-") + "/" + y.lastIndex);
    }
    return out.join(",");
  }
  // emotion's loop over the label pattern
  function labels(styles) {
    var re = /label:\s*([^\s;{]+)\s*(;|$)/g, name = "", m;
    re.lastIndex = 0;
    while ((m = re.exec(styles)) !== null) name += "-" + m[1];
    return name;
  }
  function wide(n) {
    var s = "\u4e2d".repeat(n) + "\u4e2d\u6587" + "a".repeat(3);
    return [s.search(/\u4e2d\u6587/), s.search(/\u4e2d\u6588/), /\u6587a/.exec(s).index].join(",");
  }
  for (var p = 0; p < pats.length; p++) {
    var row = [String(pats[p]).slice(0, 40)];
    for (var k = 0; k < subj.length; k++) row.push(one(pats[p], subj[k]));
    log.push(row.join(" | "));
    log.push(" i " + lastIndex(pats[p], "aabcab abc label:q"));
  }
  log.push(labels("color:red;label:Box;display:flex;label:Inner;"), labels("lllll label:;label:a"),
    labels("l".repeat(70000) + "label:end"), labels(""));
  log.push(wide(3), wide(65536), wide(70000));
  rvExpect(show, one, lastIndex, labels, wide);
  return log.join("\n");
})()
