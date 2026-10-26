// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// Where lre_exec skips to the first position a match can start at, and the
// alternatives the split ops do not push: regexps that start with a char, a
// range, a set of first units, ^, or none of them, on 8-bit and 16-bit
// strings longer than a chunk of the skip, with captures that a failed try
// must leave cleared, and alternations past the budgets of both.
var out = [];
function show(m) {
  if (m === null) return 'null';
  return JSON.stringify([Array.from(m), m.index]);
}
function all(re, s) {
  var r = [], m, n = 0;
  re.lastIndex = 0;
  while ((m = re.exec(s)) !== null && n++ < 50) {
    r.push(m.index + ':' + JSON.stringify(Array.from(m)));
    if (m[0] === '') re.lastIndex++;
  }
  return r.length + ' ' + r.slice(0, 8).join(' ') + ' last ' + r.slice(-2).join(' ');
}
var long8 = 'x'.repeat(70000) + 'needle' + 'y'.repeat(70000) + 'Needle';
var long16 = '中'.repeat(70000) + 'needleé' + '文'.repeat(70000) + 'NEEDLE';
var strs = [
  '', 'a', 'abc', 'xxabcxxABC', 'the cat sat on the mat', 'aaa\nbbb\nccc', long8, long16,
  'été café CAFÉ', '😀 \ud83d x \ude00', 'kK K',
];
var res = [
  /needle/g, /needle/gi, /Needle|NEEDLE/g, /[nN]eedle/g, /[中-文]+needle/g, /[^x]e/g,
  /(a)?b|c/g, /(?:(a)|(b))c/g, /(?=n)ne/g, /(?!x)./g, /\bthe\b/g, /^a/g, /^a/gm, /^/g, /$/g,
  /a*b/g, /(?:x|y|z)+needle/g, /[éÉ]/g, /é/gi, /\ud83d/g, /\ude00/g, /k/gi,
  /[a-c]{2}/g, /c(?=a)|t/g, /(?<=c)a/g, /(?<!c)a/g, /a|b|c|d|e|f|g|h|i|j/g, /.at/g, /\s\w/g,
  /[\x00-￿]/g, /\u{1f600}/gu, /needle/y, /needle/gy, /(x)(y)?/g,
];
for (var i = 0; i < res.length; i++) {
  for (var j = 0; j < strs.length; j++) {
    var re = res[i], s = strs[j];
    out.push(String(re) + ' on ' + j + ': ' + all(re, s));
  }
  // split, replace, match and search take other ways into lre_exec.
  var short = strs[4];
  out.push(String(re) + ' split ' + JSON.stringify(short.split(re)) +
    ' replace ' + JSON.stringify(short.replace(re, '<$&>')) + ' search ' + short.search(re));
}
// An alternation of many words, as react-spring's color names are.
var names = ['transparent', 'aliceblue', 'antiquewhite', 'aqua', 'aquamarine', 'azure', 'beige', 'bisque', 'black',
  'blanchedalmond', 'blue', 'blueviolet', 'brown', 'burlywood', 'burntsienna', 'cadetblue', 'chartreuse',
  'chocolate', 'coral', 'cornflowerblue', 'cornsilk', 'crimson', 'cyan', 'darkblue', 'darkcyan'];
var many = [];
for (var r = 0; r < 6; r++) for (var k = 0; k < names.length; k++) many.push(names[k] + (r ? r : ''));
var colors = new RegExp('(' + many.join('|') + ')', 'g');
var text = 'color: aliceblue; border: 1px solid darkcyan3; x: crimson5 and cyan, notacolor, black4';
out.push('colors ' + all(colors, text));
out.push('colors i ' + all(new RegExp('\\b(' + many.join('|') + ')\\b', 'gi'), text.toUpperCase()));
// More alternatives than the split budget, and a first set past its budget.
var huge = [];
for (var h = 0; h < 5000; h++) huge.push('w' + h + 'z');
var hugeRe = new RegExp(huge.join('|'), 'g');
out.push('huge ' + all(hugeRe, 'aw12zbw4999zcw5000zdw0z'));
var deep = '';
for (var d = 0; d < 600; d++) deep += '(?:q' + d + '|';
deep += 'end' + ')'.repeat(600);
out.push('deep ' + all(new RegExp(deep, 'g'), 'xxq5 q599 end q12'));
// A failed try at one position leaves no capture for the next.
out.push('captures ' + all(/(a)?(?:(b)c|bd)|(e)/g, 'ab bd abd ce e'));
out.push(out.length + ' lines');
out.join('\n');
