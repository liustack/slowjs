// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// Regexps of many capture groups, past the 255 of upstream QuickJS: the count
// at each side of 256 and of the limit, which is that of V8 (32767 groups
// and group 0), and every op that names a group with an index from 256 up
// (save_start and save_end in both directions, save_reset of a group
// repeated, back references by number and by name, forward and backward, and
// a name that several groups share across more than 255 alternatives), the
// d flag, the groups object, matchAll, replace with $1 to $99, $<name> and a
// function, split, and groups nested 600 deep. What Node 24 prints, run
// with a stack of 64 MB (ulimit -s 65520, node --stack-size=60000): V8
// compiles a regexp of 32767 groups by recursion, and its default stack
// is too small for it.
(function () {
  "use strict";
  var out = [];
  function make(src, flags) {
    try {
      return new RegExp(src, flags);
    } catch (e) {
      return e.name;
    }
  }
  function pick(m, idx) {
    if (m === null) return "null";
    return idx.map(function (i) {
      return i + "=" + (m[i] === undefined ? "u" : JSON.stringify(m[i]));
    }).join(" ");
  }
  function n_groups(n) {
    var a = [];
    for (var i = 1; i <= n; i++) a.push("(" + String.fromCharCode(97 + (i % 26)) + ")");
    return a.join("");
  }
  function n_text(n) {
    var s = "";
    for (var i = 1; i <= n; i++) s += String.fromCharCode(97 + (i % 26));
    return s;
  }

  // the count of groups, at each side of 255 and of the limit
  [254, 255, 256, 300, 912, 32767, 32768, 65535, 65536].forEach(function (n) {
    var re = make(n_groups(n));
    if (typeof re === "string") {
      out.push(n + " groups: " + re);
      return;
    }
    var m = re.exec("." + n_text(n) + ".");
    out.push(n + " groups: length " + m.length + " index " + m.index + " " +
             pick(m, [1, 255, 256, n - 1, n]));
  });
  out.push("empty groups 32767: " + make("()".repeat(32767)).exec("").length);
  out.push("empty groups 32768: " + make("()".repeat(32768)));
  // a group opened past the limit inside a lookahead and a class are counted
  // as they are in the pattern
  out.push("lookahead 32768: " + make("(?=" + "()".repeat(32767) + ")()"));
  out.push("class 32767: " + make("()".repeat(32766) + "[(]()").exec("(").length);

  // 912 groups, every third named, with back references past 255 by number
  // and by name, forward and backward
  var src = "", text = "";
  for (var i = 1; i <= 912; i++) {
    var c = String.fromCharCode(97 + (i % 26));
    src += i % 3 === 0 ? "(?<g" + i + ">" + c + ")" : "(" + c + ")";
    text += c;
  }
  var big = make(src + "-\\300\\k<g600>\\912(?<=\\k<g912>)(?<=\\600.)", "d");
  var tail = text[299] + text[599] + text[911];
  var m = big.exec("x" + text + "-" + tail + "y");
  out.push("912: length " + m.length + " index " + m.index + " " + pick(m, [1, 3, 256, 300, 600, 911, 912]));
  out.push("912 groups: " + Object.keys(m.groups).length + " g300=" + m.groups.g300 +
           " g912=" + m.groups.g912 + " g3=" + m.groups.g3);
  out.push("912 indices: " + [1, 256, 300, 912].map(function (k) { return m.indices[k].join(","); }).join(" ") +
           " g600=" + m.indices.groups.g600.join(","));
  out.push("912 no match: " + big.exec("x" + text + "-" + tail.slice(1) + "y"));
  // a back reference inside a lookbehind reads backward
  var bb = make(n_groups(400) + "(?<=\\399\\400)");
  out.push("backward: " + pick(bb.exec(n_text(400) + "!"), [399, 400]));
  out.push("backward no match: " + make(n_groups(400) + "(?<=\\400\\399)").exec(n_text(400)));
  out.push("lookbehind groups: " + pick(make(n_groups(400) + "(?<=(j)(k))").exec(n_text(400)), [400, 401, 402]));
  out.push("ignore case: " + pick(make(n_groups(300) + "\\300\\k<x>(?<x>Q)", "i").exec(n_text(300) + "Oq"), [300, 301]) +
           " / " + pick(make(n_groups(300) + "(?<=\\300)O", "i").exec(n_text(300) + "o"), [300]));

  // groups from 256 up inside a repeated group are reset at each turn
  var rep = make(n_groups(300) + "(?:(x)|(y))+");
  out.push("reset: " + pick(rep.exec(n_text(300) + "xy"), [300, 301, 302]));
  out.push("reset: " + pick(rep.exec(n_text(300) + "yx"), [300, 301, 302]));
  var opt = make(n_groups(300) + "(?:(z)\\301)?k");
  out.push("optional: " + pick(opt.exec(n_text(300) + "k"), [301]) + " / " +
           pick(opt.exec(n_text(300) + "zzk"), [301]));

  // a name that several groups share, in alternatives more than 255 apart
  var alts = [];
  for (i = 0; i < 300; i++) alts.push("b" + i);
  var dup = make("(?:(?<d>a)|" + alts.join("|") + "|(?<d>c))\\k<d>");
  out.push("shared name: " + pick(dup.exec("cc"), [1, 2]) + " d=" + dup.exec("cc").groups.d +
           " / " + pick(dup.exec("aa"), [1, 2]) + " / " + dup.exec("ca"));
  [254, 255, 256, 65535].forEach(function (n) {
    var many = [];
    for (var k = 0; k < n; k++) many.push("b" + k);
    var r = make("(?<d>a)|" + many.join("|") + "|(?<d>c)");
    out.push("shared name, " + (n + 1) + " bars apart: " + (typeof r === "string" ? r : r.exec("c").groups.d));
  });

  // matchAll, replace and split
  var g = make(n_groups(300), "g");
  var s3 = n_text(300) + "-" + n_text(300);
  out.push("matchAll: " + Array.from(s3.matchAll(g), function (r) {
    return r.index + ":" + r.length + ":" + r[300];
  }).join(" "));
  out.push("replace: " + s3.replace(g, "[$1$99$256$300]"));
  out.push("replace fn: " + s3.replace(g, function () {
    return arguments.length + ":" + arguments[300] + ":" + arguments[301];
  }));
  var named = make(src, "g");
  out.push("replace named: " + (text + text).replace(named, "<$<g3>$<g912>$<g600>$912>"));
  out.push("replace named fn: " + text.replace(named, function () {
    var a = arguments;
    return a.length + ":" + a[a.length - 1].g912 + ":" + a[a.length - 3];
  }));
  var sp = ("0" + n_text(300) + "1").split(make(n_groups(300)));
  out.push("split: " + sp.length + " " + sp[0] + sp[1] + sp[300] + sp[301]);

  // groups in groups, under the depth of nesting the parser takes
  [100, 600].forEach(function (n) {
    var r = make("(".repeat(n) + "a" + ")".repeat(n));
    out.push("nested " + n + ": " + (typeof r === "string" ? r : r.exec("a").length));
  });
  return out.join("\n");
})();
