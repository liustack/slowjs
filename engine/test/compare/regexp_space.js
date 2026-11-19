// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

// Regexps that start with \s or \S, which skip to a unit a match can start
// with (lre_first_set): where each finds its first match, and how long it
// is, for every unit from 0 to 0xffff put between letters and between
// spaces, in strings of 8 and of 16 bits, and split, replace and match by
// the regexps of this kind that pages and the V8 RegExp benchmark use. The
// file is what Node 24 prints.
var out = [];
var frames = [["ab", "x"], ["\u4e00b", "x"], ["  ", " "], ["\u3000 ", " "]];
function esc(v) {
  return JSON.stringify(v).replace(/[\u0080-\uffff]/g, function (c) {
    return "\\u" + c.charCodeAt(0).toString(16).padStart(4, "0");
  });
}
var res = [
  /\s+/g, /\s/, /\S/, /\S|a/, /\s?;\s?/, /\s*x/, /(?:^|\s+)x(?:\s+|$)/,
  /(?<=a)\s/, /\s/i, /\S/i, /\s+$/, /\s/u, /\S+/gu,
];
for (var r of res) {
  // for each frame, a hash of where the first match starts and its length
  var line = String(r);
  for (var f of frames) {
    var h = 0;
    for (var u = 0; u < 0x10000; u++) {
      r.lastIndex = 0;
      var m = r.exec(f[0] + String.fromCharCode(u) + f[1]);
      h = (h * 31 + (m ? m.index * 7 + m[0].length : -1)) | 0;
    }
    line += " " + h;
  }
  out.push(line);
}
var ws = "\t\n\v\f\r \u00a0\u1680\u2000\u2005\u200a\u2028\u2029\u202f\u205f\u3000\ufeff";
var s = "a ;b;  c\t;\u00a0d" + ws + "e; f\u3000;g";
out.push(esc(s.split(/\s?;\s?/)));
out.push(esc(s.split(/\s+/)));
out.push(esc(s.split(/(\s)/, 7)));
out.push(esc(s.replace(/\s+/g, "_")));
out.push(esc(s.match(/\S+/g)));
out.push(esc(("x" + s + "x").replace(/(?:^|\s+)x(?:\s+|$)/g, "#")));
out.push(esc(ws.split("").map(function (c) { return /\s/.test(c); })));
out.join("\n");
