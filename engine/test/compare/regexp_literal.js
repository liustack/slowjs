// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// The literal every match of a regexp holds (lre_plan_literal and lre_literal
// in engine/libregexp.c): when the string does not hold it from where the
// search starts there is no match, and a match starts no more than the units
// before the literal ahead of where it is. Every rule of what makes a literal
// required, each with strings that hold it, hold it in part, hold it only in
// a place no match can use, and do not hold it: alternatives, optional groups
// and quantifiers of 0, atoms repeated at least once, lookaheads and
// lookbehinds of both signs, back references, captures, the flags i (ASCII
// letters, and k and s, which U+212A and U+017F fold to under u and v), m, s,
// y, g, d, u and v, lone surrogates and pairs, strings of 8 and 16 bits,
// literals with units from 256 up, the empty string, strings shorter than
// the literal, lastIndex before, in and after the literal, literals across a
// chunk of the scan, many places of the unit the scan looks for, and every
// builtin that runs a regexp. What Node 24 prints.
(function () {
  "use strict";
  var out = [];
  function show(m) {
    if (m === null) return "null";
    var s = m.index + ":" + JSON.stringify(Array.prototype.slice.call(m));
    if (m.groups) s += " g" + JSON.stringify(m.groups);
    if (m.indices) s += " d" + JSON.stringify(Array.prototype.slice.call(m.indices));
    return s;
  }
  function short(s) {
    return s.length > 24 ? JSON.stringify(s.slice(0, 10)) + "..." + s.length + "..." + JSON.stringify(s.slice(-10)) : JSON.stringify(s);
  }
  // every builtin that runs the regexp, and exec from several lastIndex
  function run(re, s) {
    var flags = re.flags, base = flags.replace("g", "").replace("y", "");
    var r = new RegExp(re.source, flags.replace("g", ""));
    var g = new RegExp(re.source, base + "g" + (flags.indexOf("y") >= 0 ? "y" : ""));
    var line = [];
    r.lastIndex = 0;
    line.push(r.test(s) ? 1 : 0, r.lastIndex);
    line.push(s.search(r));
    r.lastIndex = 0;
    line.push(show(r.exec(s)), r.lastIndex);
    var all = [], m, n = 0;
    g.lastIndex = 0;
    while ((m = g.exec(s)) !== null && n++ < 40) {
      all.push(show(m));
      if (m[0] === "") g.lastIndex++;
    }
    line.push(all.length + "[" + all.slice(0, 6).join(" ") + "]", g.lastIndex);
    var mm = s.match(g);
    line.push(mm === null ? "null" : mm.length + ":" + short(mm.join("|")));
    line.push(short(s.replace(g, "<$&>")), short(s.replace(r, function () { return "{" + Array.prototype.slice.call(arguments, 0, -2).join(",") + "}"; })));
    line.push(short(s.split(r).join("|")), s.split(r, 2).length);
    var pos = [];
    try {
      for (var x of s.matchAll(g)) pos.push(x.index);
      line.push(pos.length + ":" + pos.slice(0, 8).join(","));
    } catch (e) {
      line.push(e.name);
    }
    // exec of the regexp with g (or y) from places around the literal
    var at = [];
    for (var li of [1, 2, s.length >> 1, s.length - 3, s.length - 1, s.length, s.length + 1]) {
      if (li < 0) continue;
      g.lastIndex = li;
      var e = g.exec(s);
      at.push(li + ">" + (e === null ? "null" : e.index + "," + e[0].length) + "," + g.lastIndex);
    }
    line.push(at.join(" "));
    return line.join(" ; ");
  }
  function check(res, strs) {
    for (var i = 0; i < res.length; i++) {
      for (var j = 0; j < strs.length; j++) {
        var re = res[i], s = strs[j];
        var line;
        try {
          line = run(re, s);
        } catch (e) {
          line = "threw " + e.name;
        }
        out.push(String(re) + " on " + short(s) + ": " + line);
      }
    }
  }

  // The regexp of the V8 RegExp benchmark that has a literal, and its subject.
  var date = /(^|[^\\])"\\\/Qngr\((-?[0-9]+)\)\\\/"/g;
  var json = '{"n":"x","b":[1,2,3],"q":"\\"quoted\\" and \\\\ back","u":"https:\\/\\/ex.com\\/a"}';
  var json2 = json + '"\\/Qngr(1234)\\/"' + json + ',"\\/Qngr(-5)\\/"' + json + '\\"\\/Qngr(7)\\/"';
  check([date, /"\\\/Qngr\(/g, /\\\/Qngr/], [json, json2, json.repeat(40) + json2, "", '"\\/Qngr(', '"\\/Qngr(1)\\/"']);
  out.push(json2.replace(date, "$1new Date($2)"));

  // Where the literal sits: at the start, in the middle, at the end, after
  // and before optional parts, inside groups, alternatives and quantifiers.
  var res = [
    /abc/, /xabc/, /a.c/, /ab(c)/, /(ab)c/, /a(?:b)c/, /x?abc/, /x*abc/, /x+abc/, /abcx?/, /abc.*z/,
    /(?:ab|cd)e/, /e(?:ab|cd)/, /(?:abc|abd)/, /(?:abc)?d/, /(?:abc)*d/, /(?:abc)+d/, /(?:abc){0,2}d/,
    /(?:abc){2}d/, /(?:abc){1,3}d/, /a{2}bc/, /ab{0}c/, /ab?c/, /a(b|)c/, /a(?:b|c)d/, /(a)bc\1/,
    /(?<n>ab)c\k<n>/, /(a)?bc/, /a(?=bc)bcd/, /(?=abc)/, /(?!abc)abd/, /a(?!bc)b/, /(?<=ab)c/,
    /(?<!ab)cd/, /(?<=a(b)c)d/, /.{0,3}abc/, /[^x]?abc/, /(^|[^\\])"ab/, /\babc\b/, /^abc/, /abc$/,
    /(?:^|,)ab/, /a\nb/, /a\\b/, /a\/b/, /(?:)abc/, /a()bc/, /ab(?:)c/,
  ];
  var strs = [
    "", "a", "ab", "abc", "xabc", "xxabcx", "abcabc", "abdabc", "aabcbc", "abcd", "eab", "cde",
    "abcabcd", "zzzzabc", "abc,ab", "a\nb", "a\\b a/b", "x\"ab", "\\\"ab", "abcz", "abc abc",
    "ab ab", "cdab", "abcababc", "aab",
  ];
  check(res, strs);

  // Flags: i (ASCII letters of either case, non letters, and the letters
  // other characters fold to under u and v), m, s, d, u, v.
  check([
    /abc/i, /ABC/i, /a-b/i, /k1/i, /s1/i, /k1/iu, /s1/iu, /k1/iv, /x(?i:ab)y/, /x(?-i:ab)y/i,
    /a.c/s, /^abc/m, /b$\nc/m, /(?<x>b)c/d, /é(b)/i, /Éb/iu,
  ], [
    "", "ABC", "aBc", "xAbCx", "a-b", "A-B", "k1", "K1", "K1", "s1", "S1", "ſ1", "xaby", "xABy",
    "XaBY", "a\nc", "x\nabc", "ab\nc", "zbc", "éb", "Éb", "ÉB", "K1 k1",
  ]);

  // Sticky: the literal is found relative to lastIndex, and a sticky match
  // succeeds or fails where it did.
  var sticky = [/abc/y, /x?abc/y, /(?:x|y)abc/y, /abc/gy];
  for (var re of sticky) {
    for (var s of ["abc", "xabc", "zabc", "abcabc", "zzzabc", ""]) {
      var r = [];
      for (var li = 0; li <= s.length + 1; li++) {
        re.lastIndex = li;
        var m = re.exec(s);
        r.push(li + ">" + (m === null ? "null" : m.index + ":" + m[0]) + "," + re.lastIndex);
      }
      out.push(String(re) + " on " + JSON.stringify(s) + ": " + r.join(" ") + " ; split " +
        JSON.stringify(s.split(re)) + " ; replace " + JSON.stringify(s.replace(re, "<$&>")));
    }
  }

  // Unicode: lone surrogates and pairs, a literal of a lone surrogate, of a
  // pair, of characters from 256 up, on strings of 8 and 16 bits, and
  // lastIndex in the middle of a pair.
  check([
    /\ud83dx/, /\ud83dx/u, /x\ude00/u, /x\ude00/, /\u{1f600}x/u, /a中b/, /a中b/u, /中文/,
    /(?:\ud83d|y)x/u, /éĀ/, /[^a]Āb/, /x😀/, /x😀/u, /x😀/v,
  ], [
    "", "x", "\ud83dx", "😀x", "x😀", "x\ude00", "😀\ude00", "a中b",
    "a中", "中文", "abc", "éĀ", "éā", "baĀb", "y\ud83dx😀x",
  ]);

  // Long strings, of 8 and 16 bits: the literal across the edge of a chunk
  // of the scan (65,536 units), at the end, missing, and many places of the
  // unit the scan looks for that are not the literal.
  var big = 1 << 16;
  var long = [
    "x".repeat(big - 2) + "Qngr(" + "x".repeat(10),
    "x".repeat(big - 2) + "Qngr" + "x".repeat(10),
    "x".repeat(3 * big + 7) + "Qngr(",
    "Q".repeat(2 * big) + "Qngr(",
    "Qngr".repeat(big) + "(",
    "中".repeat(big - 1) + "Qngr(" + "中",
    "中".repeat(2 * big + 3),
    "x".repeat(2 * big) + "QNGR(",
  ];
  check([/Qngr\(/, /(^|[^\\])Qngr\(/g, /qngr\(/i, /.Qngr\(/, /Q(?:ngr)\(/, /中Qngr/, /Qngr\(中/], long);

  // A literal of 16 units and more, and one that overlaps itself.
  check([/abcdefghijklmnopqrstuvwxyz/, /0123456789abcdefg/, /aab/, /aaab/, /abab/, /x(abcdefghijklmnopq)?y/],
    ["abcdefghijklmnopqrstuvwxyz", "abcdefghijklmnopqrstuvwxy", "0123456789abcdefg", "0123456789abcdef",
      "aaab", "aab", "aaaaaab", "ababab", "abaab", "xy", "x abcdefghijklmnopq y"]);

  // A regexp of another realm of String.prototype methods through a
  // subclass, and an exec that a script changed: the builtins call it.
  class R extends RegExp {
    exec(s) {
      out.push("exec " + this.lastIndex);
      return super.exec(s);
    }
  }
  var sub = new R("ab(c)", "g");
  out.push(JSON.stringify("xabcxabc".replace(sub, "[$1]")), JSON.stringify("xabcxabd".split(sub)),
    JSON.stringify("zzz".match(sub)), "zzz".search(sub));

  // A fixed set of random regexps of literals, classes, groups and
  // quantifiers, on random strings of a few letters.
  var seed = 12345;
  function rnd(n) {
    seed = (seed * 1103515245 + 12345) & 0x7fffffff;
    return (seed >> 8) % n;
  }
  var atoms = ["a", "b", "c", "ab", "ba", "\\.", ".", "[ab]", "[^a]", "\\d", "x"];
  function pat(depth) {
    var n = 1 + rnd(4), s = "";
    for (var k = 0; k < n; k++) {
      var t = rnd(12), a;
      if (t < 6 || depth > 2) a = atoms[rnd(atoms.length)];
      else if (t < 8) a = "(" + pat(depth + 1) + "|" + pat(depth + 1) + ")";
      else if (t < 9) a = "(?:" + pat(depth + 1) + ")";
      else if (t < 10) a = "(?=" + pat(depth + 1) + ")";
      else if (t < 11) a = "(?!" + pat(depth + 1) + ")";
      else a = "(?<=" + pat(depth + 1) + ")";
      // no quantifier on a lookaround (a SyntaxError under u, and on a
      // lookbehind always)
      var q = t >= 9 && depth <= 2 ? 9 : rnd(10);
      if (q === 0) a += "?";
      else if (q === 1) a += "*";
      else if (q === 2) a += "+";
      else if (q === 3) a += "{2}";
      else if (q === 4) a += "{0,2}";
      s += a;
    }
    return s;
  }
  var letters = "abcx.1";
  for (var k = 0; k < 150; k++) {
    var p = pat(0), flags = ["", "g", "i", "gi", "u", "m"][rnd(6)], ss = [];
    for (var t = 0; t < 4; t++) {
      var len = rnd(12), s = "";
      for (var u = 0; u < len; u++) s += letters[rnd(letters.length)];
      ss.push(s);
    }
    var re;
    try {
      re = new RegExp(p, flags);
    } catch (e) {
      out.push("/" + p + "/" + flags + ": " + e.name);
      continue;
    }
    for (var s of ss) out.push(String(re) + " on " + JSON.stringify(s) + ": " + run(re, s));
  }
  return out.join("\n");
})();
