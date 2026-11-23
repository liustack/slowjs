// Copyright (c) 2026 The liustack/tifajs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

// String.prototype.split with a regexp of the realm that nothing changed,
// against the steps of the specification written out: a sticky copy of the
// regexp tried at each position, the position stepped one character (one code
// point with u or v) after a miss and after an empty match where the last
// piece ended. Every regexp on every string, with and without a limit: empty
// matches, matches at the end, captures that match and do not, lookbehinds and
// lookaheads, ^ and $ with and without m, the u and v flags at surrogate
// pairs and lone surrogates, regexps that match the empty string everywhere,
// sticky and global regexps, strings of 8 and 16 bits, strings longer than a
// step of the scan, and regexps that hold a literal every match holds. A line
// gives the regexp, how many splits differ from the steps, and a hash of the
// pieces. The file is what Node 24 prints.
var out = [];

function advance(s, i, unicode) {
  if (!unicode || i + 1 >= s.length) return i + 1;
  var c = s.charCodeAt(i);
  if (c < 0xd800 || c > 0xdbff) return i + 1;
  c = s.charCodeAt(i + 1);
  return c >= 0xdc00 && c <= 0xdfff ? i + 2 : i + 1;
}

// RegExp.prototype[Symbol.split], step by step
function steps(rx, S, limit) {
  var flags = rx.flags;
  var unicode = flags.indexOf('u') >= 0 || flags.indexOf('v') >= 0;
  var splitter = new RegExp(rx, flags.indexOf('y') >= 0 ? flags : flags + 'y');
  var A = [];
  var lim = limit === undefined ? 4294967295 : limit >>> 0;
  if (lim === 0) return A;
  var size = S.length, z;
  if (size === 0) {
    z = splitter.exec(S);
    if (z === null) A.push(S);
    return A;
  }
  var p = 0, q = p;
  while (q < size) {
    splitter.lastIndex = q;
    z = splitter.exec(S);
    if (z === null) {
      q = advance(S, q, unicode);
      continue;
    }
    var e = Math.min(splitter.lastIndex, size);
    if (e === p) {
      q = advance(S, q, unicode);
      continue;
    }
    A.push(S.substring(p, q));
    if (A.length === lim) return A;
    p = e;
    for (var i = 1; i < z.length; i++) {
      A.push(z[i]);
      if (A.length === lim) return A;
    }
    q = p;
  }
  A.push(S.substring(p, size));
  return A;
}

function show(a) {
  return '[' + a.map(function (x) { return x === undefined ? '~' : JSON.stringify(x); }).join(',') + ']';
}
function hash(h, s) {
  for (var i = 0; i < s.length; i++) h = (h * 31 + s.charCodeAt(i)) | 0;
  return h;
}

var res = [
  /,/, /,/g, /,/y, /,/gy, /(,)/, /(,)|(;)/, /\s+/, /(\s+)/, /\s*/, /\b/, /\B/, /(?:)/, /(?:)/u, /(?:)/v, /(?:)/y,
  /x*/, /a*?/, /(a)?/, /(a)?b?/, /$/, /$/m, /^/, /^/m, /^a/, /^a/m, /a$/, /(?=a)/, /(?!a)/, /(?<=a)/, /(?<!a)/,
  /(?<=a)b/, /(?<![a-z])b/, /(?<=^|,)/, /(?<=,)/y, /(?<=(a))(b)/, /\b(?=\w)/, /./, /./u, /./s, /./y, /.?/,
  /[\ud800-\udbff]/, /[\udc00-\udfff]/, /[\ud800-\udfff]/u, /\ud83d/, /\ude00/, /\ude00/u, /\ud83d/u, /😀/u, /😀/,
  /(😀)/u, /(?<=😀)/u, /(?<=\ud83d)/, /(?<=\ud83d)/u, /(?=\ude00)/, /(?=\ude00)/u, /\p{L}/u, /[\p{L}--[a-z]]/v,
  /a|ab/, /(a)|(b)/, /(?<x>a)(?<y>b)?/, /A/i, /a/gi, /É/i, /\n/m, /(a)\1/, /(?:ab)+/, /(a|b)*/, /é/, /中/, /(é|中)/,
  /(\n|\r\n)/, /([^\S\r\n]+|[()[\]{}'"\r\n]|\b)/, /(\s+|[()[\]{}'"]|\b)/, /([{}:;,]|\s+)/, /(\S.+?[.!?])(?=\s+|$)/,
  /\r?\n/, /(?:\r\n|\n)+/g, /(a)(?:b|(c))/d, /,(?!$)/, /(?=,)|(?<=,)/, /(?:)(?=b)/, /a(?=)/,
];

var strs = [
  '', 'a', 'b', ',', ',,', 'a,b', 'a,b,', ',a,b', 'a,,b', 'ab', 'aab', 'abab', 'ba', 'ba,ab', 'aaa', 'a b  c',
  ' a ', 'a\nb\r\nc\n', '\n', '\r\n\r\n', 'a;b,c;', 'abcabc', 'xyz', 'word, word. Sent! Next? end', 'f(a)[b]{c} "d"',
  '{"a": 1, "b": [2, 3]}', 'é', 'éa,é', 'a中b中', '中,文', '😀', 'a😀b', '😀😀', '😀\ud83d', '\ude00😀', '😀\ude00',
  'a\ud800b', '\ud800', '\udc00', 'x\ude00y\ud83d', 'AaÉé', 'aaaa', 'abcbca', 'a\nb\nc', 'ab\nab',
];
// short strings of the characters the regexps care about, made the same way each run
var seed = 12345;
function next(n) {
  seed = (Math.imul(seed, 1103515245) + 12345) | 0;
  return ((seed >>> 16) & 0x7fff) % n;
}
var alphabet = ['a', 'b', 'c', ',', ' ', '\n', '\r', 'é', '中', '😀', '\ud83d', '\ude00', 'A'];
for (var k = 0; k < 160; k++) {
  var n = next(9), s = '';
  for (var j = 0; j < n; j++) s += alphabet[next(alphabet.length)];
  strs.push(s);
}

var limits = [undefined, 0, 1, 2, 3, 5];
for (var ri = 0; ri < res.length; ri++) {
  var re = res[ri], differ = 0, h = 0, first = '';
  for (var si = 0; si < strs.length; si++) {
    for (var li = 0; li < limits.length; li++) {
      // the limits on the first strings only
      if (li > 0 && si >= 48) break;
      var lim = limits[li];
      re.lastIndex = 3;
      var got = show(strs[si].split(re, lim)), want = show(steps(re, strs[si], lim));
      if (got !== want) {
        if (differ === 0) first = ' first ' + JSON.stringify(strs[si]) + ' ' + lim + ' ' + got + ' want ' + want;
        differ++;
      }
      if (re.lastIndex !== 3) differ++;
      h = hash(h, got);
    }
  }
  out.push(re + ' differ ' + differ + ' hash ' + h + first);
}

// strings longer than a step of the scan (65,536 characters), 8 and 16 bits
var long8 = 'x'.repeat(70000) + ',' + 'y'.repeat(70000) + ',z', long16 = '中'.repeat(70000) + ',' + 'é'.repeat(70000);
var longRes = [/,/, /(,)/, /[,z]/, /(?<=x),/, /\b/, /$/, /(?:)/u, /y+/, /(?:\r\n|\n)/];
for (var i = 0; i < longRes.length; i++) {
  var a = long8.split(longRes[i]), b = long16.split(longRes[i]);
  out.push('long ' + longRes[i] + ' ' + a.length + ' ' + a.slice(0, 3).map(function (x) { return x.length; }) +
    ' ' + (show(a) === show(steps(longRes[i], long8))) + ' ' + b.length + ' ' + (show(b) === show(steps(longRes[i], long16))));
}

// regexps that hold a literal every match holds, which the search looks for first: with u (no
// plan to skip by) and without, the literal at the start of the match, after it and after a
// lookbehind, on strings where it lies before, at and after where the search starts, in 8 and 16
// bits, and where a match ends at the end of the string
var litRes = [
  /a.c/, /a.c/u, /.?bc/, /.?bc/u, /(\d+)px/, /(\d+)px/u, /[a-c]x\d/, /[a-c]x\d/u, /\s*,\s*/, /\s*,\s*/u,
  /(?<=,)ab/, /(?<=,)ab/u, /(.)\1x/, /(.)\1x/u, /\bend\b/u, /é.中/u, /😀x/u, /x😀/u, /(?:)x/u, /x(?:)/u,
  /.x$/u, /(?=.x)/u, /(?<=x).?/u, /x?yz/u, /[^,]*,x/u,
];
var litStrs = [
  'abc', 'xabcabc', 'a1c,abc', 'abcabcabc', '12px 3px', '12px3px4', 'ax1,bx2,cx3', 'a , b,c ,d', ',ab,ab',
  'ab,ab,', 'aax,bbx', 'aaxbbxccx', 'the end', 'end,end', 'é中é.中', '😀x😀x', 'x😀x😀', 'xx', 'axbx',
  'bcbc', 'zbc', 'xyz,yz', 'a,x,b,x', 'yzyz', 'x',
];
var litAll = litStrs.concat(litStrs.map(function (s) { return '中' + s; }), strs.slice(0, 48));
for (var ri = 0; ri < litRes.length; ri++) {
  var re = litRes[ri], differ = 0, h = 0, first = '';
  for (var si = 0; si < litAll.length; si++) {
    for (var li = 0; li < 3; li++) {
      var lim = limits[li];
      var got = show(litAll[si].split(re, lim)), want = show(steps(re, litAll[si], lim));
      if (got !== want) {
        if (differ === 0) first = ' first ' + JSON.stringify(litAll[si]) + ' ' + lim + ' ' + got + ' want ' + want;
        differ++;
      }
      h = hash(h, got);
    }
  }
  out.push('literal ' + re + ' differ ' + differ + ' hash ' + h + first);
}

// some splits written out
out.push(show('a,b,,c'.split(/(,)/)));
out.push(show('ab'.split(/(?:)/)));
out.push(show('a😀b'.split(/(?:)/u)));
out.push(show('a😀b'.split(/(?:)/)));
out.push(show('x1y22z'.split(/(\d)(\d)?/)));
out.push(show('abc'.split(/$/)));
out.push(show('a\nb'.split(/$/m)));
out.push(show('aXbXc'.split(/x/i, 2)));
out.push(show('test'.split(/(?:)/, -1).slice(0, 2)));
out.push(show('a,b'.split(/,/, 2 ** 32 + 1)));
out.push(show('abc'.split(/(?<=b)/)));
out.push(show('one  two\tthree'.split(/(\s+)/)));
out.push(out.length + ' lines');
out.join('\n');
